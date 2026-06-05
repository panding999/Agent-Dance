package live

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panding999/agent-dance/backend/internal/audio"
	"github.com/panding999/agent-dance/backend/internal/store"
	"nhooyr.io/websocket"
)

func TestGatewayAcceptsValidAudioFrame(t *testing.T) {
	ctx := context.Background()
	st, session := newLiveTestStore(t)
	cache := audio.NewChunkCache(4)
	server := httptest.NewServer(NewGateway(st, cache))
	defer server.Close()

	conn := dialLive(t, server.URL, session.ID)
	defer conn.Close(websocket.StatusNormalClosure, "test done")

	ready := readEvent(t, conn)
	if ready.Type != EventSessionReady {
		t.Fatalf("ready event type = %q", ready.Type)
	}
	if ready.SessionID != session.ID {
		t.Fatalf("ready session_id = %q, want %q", ready.SessionID, session.ID)
	}

	if err := conn.Write(ctx, websocket.MessageBinary, makeBrowserFrame(1, 100, []byte{1, 0, 2, 0})); err != nil {
		t.Fatalf("write audio frame: %v", err)
	}

	accepted := readEvent(t, conn)
	if accepted.Type != EventAudioFrameAccepted {
		t.Fatalf("accepted event type = %q", accepted.Type)
	}
	if accepted.Sequence != 1 {
		t.Fatalf("accepted sequence = %d", accepted.Sequence)
	}

	recent := cache.Recent()
	if len(recent) != 1 {
		t.Fatalf("cached frames = %d, want 1", len(recent))
	}
	if recent[0].Sequence != 1 {
		t.Fatalf("cached sequence = %d, want 1", recent[0].Sequence)
	}
}

func TestGatewayRejectsMissingSessionID(t *testing.T) {
	st, _ := newLiveTestStore(t)
	gateway := NewGateway(st, audio.NewChunkCache(4))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/live/ws", nil)
	gateway.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGatewayRejectsMalformedFrame(t *testing.T) {
	ctx := context.Background()
	st, session := newLiveTestStore(t)
	server := httptest.NewServer(NewGateway(st, audio.NewChunkCache(4)))
	defer server.Close()

	conn := dialLive(t, server.URL, session.ID)
	defer conn.Close(websocket.StatusNormalClosure, "test done")
	_ = readEvent(t, conn)

	if err := conn.Write(ctx, websocket.MessageBinary, []byte{1, 2, 3}); err != nil {
		t.Fatalf("write malformed frame: %v", err)
	}

	got := readEvent(t, conn)
	if got.Type != EventSessionError {
		t.Fatalf("event type = %q, want %q", got.Type, EventSessionError)
	}
	if got.Code != ErrorInvalidAudioFrame {
		t.Fatalf("error code = %q, want %q", got.Code, ErrorInvalidAudioFrame)
	}
}

func TestGatewayRejectsOutOfOrderFrame(t *testing.T) {
	ctx := context.Background()
	st, session := newLiveTestStore(t)
	server := httptest.NewServer(NewGateway(st, audio.NewChunkCache(4)))
	defer server.Close()

	conn := dialLive(t, server.URL, session.ID)
	defer conn.Close(websocket.StatusNormalClosure, "test done")
	_ = readEvent(t, conn)

	if err := conn.Write(ctx, websocket.MessageBinary, makeBrowserFrame(2, 200, []byte{1, 0})); err != nil {
		t.Fatalf("write first frame: %v", err)
	}
	_ = readEvent(t, conn)

	if err := conn.Write(ctx, websocket.MessageBinary, makeBrowserFrame(1, 100, []byte{2, 0})); err != nil {
		t.Fatalf("write out-of-order frame: %v", err)
	}

	got := readEvent(t, conn)
	if got.Type != EventSessionError {
		t.Fatalf("event type = %q, want %q", got.Type, EventSessionError)
	}
	if got.Code != ErrorOutOfOrderFrame {
		t.Fatalf("error code = %q, want %q", got.Code, ErrorOutOfOrderFrame)
	}
}

func TestGatewayClosesSessionOnClientClose(t *testing.T) {
	st, session := newLiveTestStore(t)
	server := httptest.NewServer(NewGateway(st, audio.NewChunkCache(4)))
	defer server.Close()

	conn := dialLive(t, server.URL, session.ID)
	_ = readEvent(t, conn)
	if err := conn.Close(websocket.StatusNormalClosure, "done"); err != nil {
		t.Fatalf("close websocket: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		got, err := st.GetSession(context.Background(), session.ID)
		if err != nil {
			t.Fatalf("GetSession() error = %v", err)
		}
		if got.Status == "closed" && got.ClosedAt != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	got, err := st.GetSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	t.Fatalf("session status = %q closed_at = %v, want closed with timestamp", got.Status, got.ClosedAt)
}

func newLiveTestStore(t *testing.T) (*store.SQLiteStore, store.Session) {
	t.Helper()

	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "agent-dance.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})

	session, err := st.CreateSession(context.Background(), store.CreateSessionParams{
		Mode:           "live",
		SourceLanguage: "en",
		TargetLanguage: "zh",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	return st, session
}

func dialLive(t *testing.T, serverURL string, sessionID string) *websocket.Conn {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)

	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/api/live/ws?sessionId=" + sessionID
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial live websocket: %v", err)
	}
	return conn
}

func readEvent(t *testing.T, conn *websocket.Conn) Event {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	messageType, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read websocket event: %v", err)
	}
	if messageType != websocket.MessageText {
		t.Fatalf("message type = %v, want text", messageType)
	}

	var event Event
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	return event
}

func makeBrowserFrame(sequence uint32, timestampMS uint64, pcm []byte) []byte {
	raw := make([]byte, audio.BrowserFrameHeaderSize+len(pcm))
	binary.LittleEndian.PutUint32(raw[0:4], sequence)
	binary.LittleEndian.PutUint64(raw[4:12], timestampMS)
	copy(raw[12:], pcm)
	return raw
}
