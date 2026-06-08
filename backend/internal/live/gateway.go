package live

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/panding999/agent-dance/backend/internal/audio"
	"github.com/panding999/agent-dance/backend/internal/store"
	"github.com/panding999/agent-dance/backend/internal/subtitle"
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
	ErrorASTSession        = "ast_session_error"
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

type SessionRunnerFactory func(store.Session) (*SessionRunner, error)

type GatewayOptions struct {
	RunnerFactory  SessionRunnerFactory
	OriginPatterns []string
}

type Gateway struct {
	store          *store.SQLiteStore
	cache          *audio.SessionChunkCache
	runnerFactory  SessionRunnerFactory
	originPatterns []string
}

func NewGateway(st *store.SQLiteStore, cache *audio.SessionChunkCache) *Gateway {
	return NewGatewayWithOptions(st, cache, GatewayOptions{})
}

func NewGatewayWithRunner(st *store.SQLiteStore, cache *audio.SessionChunkCache, runnerFactory SessionRunnerFactory) *Gateway {
	return NewGatewayWithOptions(st, cache, GatewayOptions{
		RunnerFactory: runnerFactory,
	})
}

func NewGatewayWithOptions(st *store.SQLiteStore, cache *audio.SessionChunkCache, options GatewayOptions) *Gateway {
	if cache == nil {
		cache = audio.NewSessionChunkCache(256)
	}
	return &Gateway{
		store:          st,
		cache:          cache,
		runnerFactory:  options.RunnerFactory,
		originPatterns: compactStrings(options.OriginPatterns),
	}
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

	conn, err := websocket.Accept(w, r, g.acceptOptions())
	if err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
		defer cancel()
		_ = g.store.CloseSession(ctx, session.ID)
		return
	}
	conn.SetReadLimit(audio.BrowserFrameHeaderSize + audio.MaxBrowserFramePCMBytes)
	defer conn.Close(websocket.StatusNormalClosure, "session closed")
	writer := newWebSocketEventWriter(conn)
	pingCtx, stopPing := context.WithCancel(r.Context())
	defer stopPing()
	go runPingLoop(pingCtx, conn, pingInterval, pingTimeout)

	var runner *SessionRunner
	if g.runnerFactory != nil {
		runner, err = g.runnerFactory(session)
		if err != nil {
			_ = g.writeErrorAndClose(r.Context(), writer, conn, ErrorASTSession, err.Error())
			return
		}
		if err := runner.Start(r.Context(), session); err != nil {
			_ = g.writeErrorAndClose(r.Context(), writer, conn, ErrorASTSession, err.Error())
			return
		}
		go g.forwardRunnerEvents(r.Context(), writer, conn, runner)
	}

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
		defer cancel()
		if runner != nil {
			_ = runner.Finish(ctx)
		}
		_ = g.store.CloseSession(ctx, session.ID)
	}()

	if err := writer.WriteJSON(r.Context(), Event{
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
			_ = g.writeErrorAndClose(r.Context(), writer, conn, ErrorInvalidAudioFrame, "expected binary audio frame")
			return
		}

		frame, err := audio.ParseBrowserFrame(payload)
		if err != nil {
			_ = g.writeErrorAndClose(r.Context(), writer, conn, ErrorInvalidAudioFrame, err.Error())
			return
		}
		if hasSequence && frame.Sequence <= lastSequence {
			_ = g.writeErrorAndClose(r.Context(), writer, conn, ErrorOutOfOrderFrame, "audio frame sequence must increase")
			return
		}

		hasSequence = true
		lastSequence = frame.Sequence
		g.cache.Add(session.ID, frame)

		if runner != nil {
			if err := runner.PushAudioFrame(r.Context(), frame); err != nil {
				_ = g.writeErrorAndClose(r.Context(), writer, conn, ErrorASTSession, err.Error())
				return
			}
		}

		if err := writer.WriteJSON(r.Context(), Event{
			Type:     EventAudioFrameAccepted,
			Sequence: frame.Sequence,
		}); err != nil {
			return
		}
	}
}

func (g *Gateway) acceptOptions() *websocket.AcceptOptions {
	if len(g.originPatterns) == 0 {
		return nil
	}
	return &websocket.AcceptOptions{OriginPatterns: g.originPatterns}
}

func (g *Gateway) writeErrorAndClose(ctx context.Context, writer *webSocketEventWriter, conn *websocket.Conn, code string, message string) error {
	if err := writer.WriteJSON(ctx, Event{
		Type:    EventSessionError,
		Code:    code,
		Message: message,
	}); err != nil {
		return err
	}
	return conn.Close(websocket.StatusPolicyViolation, code)
}

func (g *Gateway) forwardRunnerEvents(ctx context.Context, writer *webSocketEventWriter, conn *websocket.Conn, runner *SessionRunner) {
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-runner.Events():
			if err := writer.WriteJSON(ctx, event); err != nil {
				return
			}
			if event.Type == subtitle.EventSessionError {
				_ = conn.Close(websocket.StatusInternalError, string(subtitle.EventSessionError))
				return
			}
		case err := <-runner.Errors():
			message := ""
			if err != nil {
				message = err.Error()
			}
			_ = writer.WriteJSON(ctx, Event{
				Type:    EventSessionError,
				Code:    ErrorASTSession,
				Message: message,
			})
			_ = conn.Close(websocket.StatusInternalError, ErrorASTSession)
			return
		}
	}
}

type webSocketEventWriter struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func newWebSocketEventWriter(conn *websocket.Conn) *webSocketEventWriter {
	return &webSocketEventWriter{conn: conn}
}

func (w *webSocketEventWriter) WriteJSON(ctx context.Context, event any) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	w.mu.Lock()
	defer w.mu.Unlock()
	return wsjson.Write(ctx, w.conn, event)
}

func writeEvent(ctx context.Context, conn *websocket.Conn, event Event) error {
	return newWebSocketEventWriter(conn).WriteJSON(ctx, event)
}

func compactStrings(values []string) []string {
	compacted := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			compacted = append(compacted, value)
		}
	}
	return compacted
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
