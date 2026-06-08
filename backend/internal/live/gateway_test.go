package live

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panding999/agent-dance/backend/internal/audio"
	"github.com/panding999/agent-dance/backend/internal/doubao/ast"
	"github.com/panding999/agent-dance/backend/internal/store"
	"github.com/panding999/agent-dance/backend/internal/subtitle"
	"nhooyr.io/websocket"
)

func TestGatewayAcceptsValidAudioFrame(t *testing.T) {
	ctx := context.Background()
	st, session := newLiveTestStore(t)
	cache := audio.NewSessionChunkCache(4)
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

	recent := cache.Recent(session.ID)
	if len(recent) != 1 {
		t.Fatalf("cached frames = %d, want 1", len(recent))
	}
	if recent[0].Sequence != 1 {
		t.Fatalf("cached sequence = %d, want 1", recent[0].Sequence)
	}
}

func TestGatewayRejectsMissingSessionID(t *testing.T) {
	st, _ := newLiveTestStore(t)
	gateway := NewGateway(st, audio.NewSessionChunkCache(4))

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
	server := httptest.NewServer(NewGateway(st, audio.NewSessionChunkCache(4)))
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
	server := httptest.NewServer(NewGateway(st, audio.NewSessionChunkCache(4)))
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
	server := httptest.NewServer(NewGateway(st, audio.NewSessionChunkCache(4)))
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

func TestGatewayRejectsClosedSession(t *testing.T) {
	st, session := newLiveTestStore(t)
	if err := st.CloseSession(context.Background(), session.ID); err != nil {
		t.Fatalf("CloseSession() error = %v", err)
	}
	server := httptest.NewServer(NewGateway(st, audio.NewSessionChunkCache(4)))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/live/ws?sessionId=" + session.ID
	_, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err == nil {
		t.Fatal("dial closed session succeeded, want failure")
	}
	if resp == nil || resp.StatusCode != http.StatusConflict {
		if resp == nil {
			t.Fatal("dial closed session response is nil, want 409")
		}
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
}

func TestGatewayRejectsDuplicateLiveConnection(t *testing.T) {
	st, session := newLiveTestStore(t)
	server := httptest.NewServer(NewGateway(st, audio.NewSessionChunkCache(4)))
	defer server.Close()

	first := dialLive(t, server.URL, session.ID)
	defer first.Close(websocket.StatusNormalClosure, "test done")
	_ = readEvent(t, first)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/live/ws?sessionId=" + session.ID
	_, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err == nil {
		t.Fatal("duplicate dial succeeded, want failure")
	}
	if resp == nil || resp.StatusCode != http.StatusConflict {
		if resp == nil {
			t.Fatal("duplicate dial response is nil, want 409")
		}
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
}

func TestGatewayAllowsConfiguredCrossOriginWebSocket(t *testing.T) {
	st, session := newLiveTestStore(t)
	server := httptest.NewServer(NewGatewayWithOptions(st, audio.NewSessionChunkCache(4), GatewayOptions{
		OriginPatterns: []string{"localhost:3000"},
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/live/ws?sessionId=" + session.ID
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"http://localhost:3000"}},
	})
	if err != nil {
		t.Fatalf("dial configured origin websocket: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "test done")

	ready := readEvent(t, conn)
	if ready.Type != EventSessionReady {
		t.Fatalf("ready event type = %q", ready.Type)
	}
}

func TestGatewayRejectsUnconfiguredCrossOriginWebSocket(t *testing.T) {
	st, session := newLiveTestStore(t)
	server := httptest.NewServer(NewGateway(st, audio.NewSessionChunkCache(4)))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/live/ws?sessionId=" + session.ID
	_, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"http://localhost:3000"}},
	})
	if err == nil {
		t.Fatal("dial unconfigured origin succeeded, want failure")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		if resp == nil {
			t.Fatal("dial unconfigured origin response is nil, want 403")
		}
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestGatewayWithRunnerForwardsAudioToASTAndSubtitleToBrowser(t *testing.T) {
	ctx := context.Background()
	st, session := newLiveTestStore(t)
	fakeAST := newFakeASTClient()
	server := httptest.NewServer(NewGatewayWithRunner(st, audio.NewSessionChunkCache(4), func(session store.Session) (*SessionRunner, error) {
		return NewSessionRunner(fakeAST, SessionRunnerOptions{})
	}))
	defer server.Close()

	conn := dialLive(t, server.URL, session.ID)
	defer conn.Close(websocket.StatusNormalClosure, "test done")
	_ = readEvent(t, conn)

	if err := conn.Write(ctx, websocket.MessageBinary, makeBrowserFrame(1, 1000, makeRunnerPCM(audio.DoubaoPacketBytes))); err != nil {
		t.Fatalf("write audio frame: %v", err)
	}
	accepted := readEvent(t, conn)
	if accepted.Type != EventAudioFrameAccepted {
		t.Fatalf("event type = %q, want %q", accepted.Type, EventAudioFrameAccepted)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(fakeAST.sentPackets()) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	packets := fakeAST.sentPackets()
	if len(packets) != 1 {
		t.Fatalf("fake AST packets = %d, want 1", len(packets))
	}

	fakeAST.emit(ast.ProviderEvent{
		Event:       ast.EventTranslationSubtitleResponse,
		SegmentID:   "seg-1",
		Text:        "你好",
		StartTimeMS: 1000,
		EndTimeMS:   1200,
	})
	got := readSubtitleEvent(t, conn)
	if got.Type != subtitle.EventSegmentPartial || got.Text != "你好" {
		t.Fatalf("subtitle event = %+v", got)
	}
}

func TestGatewayReportsRunnerFactoryError(t *testing.T) {
	st, session := newLiveTestStore(t)
	server := httptest.NewServer(NewGatewayWithRunner(st, audio.NewSessionChunkCache(4), func(store.Session) (*SessionRunner, error) {
		return nil, errors.New("Doubao credentials are not configured")
	}))
	defer server.Close()

	conn := dialLive(t, server.URL, session.ID)
	defer conn.Close(websocket.StatusNormalClosure, "test done")

	event := readEvent(t, conn)
	if event.Type != EventSessionError {
		t.Fatalf("event type = %q, want %q", event.Type, EventSessionError)
	}
	if event.Message != "Doubao credentials are not configured" {
		t.Fatalf("event message = %q", event.Message)
	}
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

func readSubtitleEvent(t *testing.T, conn *websocket.Conn) subtitle.InterpretationEvent {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	messageType, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read websocket subtitle event: %v", err)
	}
	if messageType != websocket.MessageText {
		t.Fatalf("message type = %v, want text", messageType)
	}

	var event subtitle.InterpretationEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatalf("decode subtitle event: %v", err)
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
