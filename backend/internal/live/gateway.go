package live

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/panding999/agent-dance/backend/internal/audio"
	"github.com/panding999/agent-dance/backend/internal/store"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

const (
	EventSessionReady       = "session.ready"
	EventAudioFrameAccepted = "audio.frame.accepted"
	EventSessionError       = "session.error"

	ErrorInvalidAudioFrame = "invalid_audio_frame"
	ErrorOutOfOrderFrame   = "out_of_order_frame"
	ErrorPingTimeout       = "ping_timeout"
)

const (
	writeTimeout = 2 * time.Second
	closeTimeout = 2 * time.Second
	pingInterval = 30 * time.Second
	pingTimeout  = 5 * time.Second
)

type pinger interface {
	Ping(context.Context) error
	Close(websocket.StatusCode, string) error
}

type Event struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`
	Sequence  uint32 `json:"sequence,omitempty"`
	Code      string `json:"code,omitempty"`
	Message   string `json:"message,omitempty"`
}

type Gateway struct {
	store *store.SQLiteStore
	cache *audio.SessionChunkCache
}

func NewGateway(st *store.SQLiteStore, cache *audio.SessionChunkCache) *Gateway {
	if cache == nil {
		cache = audio.NewSessionChunkCache(256)
	}
	return &Gateway{store: st, cache: cache}
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		http.Error(w, "missing sessionId", http.StatusBadRequest)
		return
	}

	session, err := g.store.GetSession(r.Context(), sessionID)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "get session failed", http.StatusInternalServerError)
		return
	}
	if session.Status != store.SessionStatusCreated {
		http.Error(w, "session is not connectable", http.StatusConflict)
		return
	}
	if err := g.store.StartSession(r.Context(), session.ID); err != nil {
		if errors.Is(err, store.ErrSessionNotConnectable) {
			http.Error(w, "session is not connectable", http.StatusConflict)
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		http.Error(w, "start session failed", http.StatusInternalServerError)
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
		defer cancel()
		_ = g.store.CloseSession(ctx, session.ID)
		return
	}
	conn.SetReadLimit(audio.BrowserFrameHeaderSize + audio.MaxBrowserFramePCMBytes)
	defer conn.Close(websocket.StatusNormalClosure, "session closed")
	pingCtx, stopPing := context.WithCancel(r.Context())
	defer stopPing()
	go runPingLoop(pingCtx, conn, pingInterval, pingTimeout)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
		defer cancel()
		_ = g.store.CloseSession(ctx, session.ID)
	}()

	if err := writeEvent(r.Context(), conn, Event{
		Type:      EventSessionReady,
		SessionID: session.ID,
	}); err != nil {
		return
	}

	var lastSequence uint32
	var hasSequence bool

	for {
		messageType, payload, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		if messageType != websocket.MessageBinary {
			_ = g.writeErrorAndClose(r.Context(), conn, ErrorInvalidAudioFrame, "expected binary audio frame")
			return
		}

		frame, err := audio.ParseBrowserFrame(payload)
		if err != nil {
			_ = g.writeErrorAndClose(r.Context(), conn, ErrorInvalidAudioFrame, err.Error())
			return
		}
		if hasSequence && frame.Sequence <= lastSequence {
			_ = g.writeErrorAndClose(r.Context(), conn, ErrorOutOfOrderFrame, "audio frame sequence must increase")
			return
		}

		hasSequence = true
		lastSequence = frame.Sequence
		g.cache.Add(session.ID, frame)

		if err := writeEvent(r.Context(), conn, Event{
			Type:     EventAudioFrameAccepted,
			Sequence: frame.Sequence,
		}); err != nil {
			return
		}
	}
}

func (g *Gateway) writeErrorAndClose(ctx context.Context, conn *websocket.Conn, code string, message string) error {
	if err := writeEvent(ctx, conn, Event{
		Type:    EventSessionError,
		Code:    code,
		Message: message,
	}); err != nil {
		return err
	}
	return conn.Close(websocket.StatusPolicyViolation, code)
}

func writeEvent(ctx context.Context, conn *websocket.Conn, event Event) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return wsjson.Write(ctx, conn, event)
}

func runPingLoop(ctx context.Context, conn pinger, interval time.Duration, timeout time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, timeout)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				_ = conn.Close(websocket.StatusPolicyViolation, ErrorPingTimeout)
				return
			}
		}
	}
}
