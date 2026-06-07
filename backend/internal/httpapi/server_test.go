package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panding999/agent-dance/backend/internal/live"
	"github.com/panding999/agent-dance/backend/internal/store"
	"nhooyr.io/websocket"
)

func TestHealthAndReadinessEndpoints(t *testing.T) {
	handler := newTestHandler(t)

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("GET /healthz status = %d", health.Code)
	}
	assertJSONStatus(t, health.Body.Bytes(), "ok")

	ready := httptest.NewRecorder()
	handler.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if ready.Code != http.StatusOK {
		t.Fatalf("GET /readyz status = %d", ready.Code)
	}
	assertJSONStatus(t, ready.Body.Bytes(), "ready")
}

func TestCreateAndGetSession(t *testing.T) {
	handler := newTestHandler(t)

	createBody := []byte(`{
		"mode": "live",
		"source_language": "en",
		"target_language": "zh",
		"voice_enabled": true
	}`)

	create := httptest.NewRecorder()
	handler.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader(createBody)))
	if create.Code != http.StatusCreated {
		t.Fatalf("POST /api/sessions status = %d body = %s", create.Code, create.Body.String())
	}

	var created store.Session
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected created session ID")
	}
	if create.Header().Get("Location") != "/api/sessions/"+created.ID {
		t.Fatalf("Location = %q", create.Header().Get("Location"))
	}

	get := httptest.NewRecorder()
	handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/sessions/"+created.ID, nil))
	if get.Code != http.StatusOK {
		t.Fatalf("GET /api/sessions/{id} status = %d body = %s", get.Code, get.Body.String())
	}

	var got store.Session
	if err := json.Unmarshal(get.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("ID = %q, want %q", got.ID, created.ID)
	}
	if got.Mode != "live" {
		t.Fatalf("Mode = %q", got.Mode)
	}
	if got.SourceLanguage != "en" {
		t.Fatalf("SourceLanguage = %q", got.SourceLanguage)
	}
	if got.TargetLanguage != "zh" {
		t.Fatalf("TargetLanguage = %q", got.TargetLanguage)
	}
	if !got.VoiceEnabled {
		t.Fatal("VoiceEnabled = false, want true")
	}
}

func TestConfiguredCORSOriginCanCreateSession(t *testing.T) {
	handler := newTestHandlerWithOptions(t, ServerOptions{
		AllowedOrigins: []string{"http://localhost:3000"},
	})

	preflight := httptest.NewRecorder()
	preflightRequest := httptest.NewRequest(http.MethodOptions, "/api/sessions", nil)
	preflightRequest.Header.Set("Origin", "http://localhost:3000")
	preflightRequest.Header.Set("Access-Control-Request-Method", http.MethodPost)
	preflightRequest.Header.Set("Access-Control-Request-Headers", "Content-Type")
	handler.ServeHTTP(preflight, preflightRequest)

	if preflight.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d body = %s", preflight.Code, preflight.Body.String())
	}
	if preflight.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatalf("Access-Control-Allow-Origin = %q", preflight.Header().Get("Access-Control-Allow-Origin"))
	}
	if preflight.Header().Get("Access-Control-Allow-Methods") != "GET, POST, OPTIONS" {
		t.Fatalf("Access-Control-Allow-Methods = %q", preflight.Header().Get("Access-Control-Allow-Methods"))
	}

	create := httptest.NewRecorder()
	createRequest := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewReader([]byte(`{"mode":"live"}`)))
	createRequest.Header.Set("Origin", "http://localhost:3000")
	createRequest.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(create, createRequest)

	if create.Code != http.StatusCreated {
		t.Fatalf("POST /api/sessions status = %d body = %s", create.Code, create.Body.String())
	}
	if create.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatalf("Access-Control-Allow-Origin = %q", create.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestUnconfiguredCORSOriginIsNotAllowed(t *testing.T) {
	handler := newTestHandlerWithOptions(t, ServerOptions{
		AllowedOrigins: []string{"http://localhost:3000"},
	})

	preflight := httptest.NewRecorder()
	preflightRequest := httptest.NewRequest(http.MethodOptions, "/api/sessions", nil)
	preflightRequest.Header.Set("Origin", "http://example.com")
	preflightRequest.Header.Set("Access-Control-Request-Method", http.MethodPost)
	handler.ServeHTTP(preflight, preflightRequest)

	if preflight.Code != http.StatusForbidden {
		t.Fatalf("preflight status = %d body = %s", preflight.Code, preflight.Body.String())
	}
	if preflight.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q", preflight.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestGetMissingSessionReturnsNotFound(t *testing.T) {
	handler := newTestHandler(t)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sessions/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET missing session status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestServerOptionsWireLiveRunnerFactory(t *testing.T) {
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

	called := make(chan store.Session, 1)
	server := httptest.NewServer(NewServerWithOptions(st, ServerOptions{
		LiveRunnerFactory: func(session store.Session) (*live.SessionRunner, error) {
			called <- session
			return nil, errors.New("runner factory failed")
		},
	}).Handler())
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/live/ws?sessionId=" + session.ID
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial live websocket: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "test done")

	got := readLiveError(t, conn)
	if got.Type != live.EventSessionError || got.Code != live.ErrorASTSession {
		t.Fatalf("live error = %+v", got)
	}

	select {
	case gotSession := <-called:
		if gotSession.ID != session.ID {
			t.Fatalf("runner session ID = %q, want %q", gotSession.ID, session.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("runner factory was not called")
	}
}

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	return newTestHandlerWithOptions(t, ServerOptions{})
}

func newTestHandlerWithOptions(t *testing.T, options ServerOptions) http.Handler {
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

	return NewServerWithOptions(st, options).Handler()
}

func readLiveError(t *testing.T, conn *websocket.Conn) live.Event {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	messageType, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read live event: %v", err)
	}
	if messageType != websocket.MessageText {
		t.Fatalf("message type = %v, want text", messageType)
	}

	var event live.Event
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatalf("decode live event: %v", err)
	}
	return event
}

func assertJSONStatus(t *testing.T, body []byte, want string) {
	t.Helper()

	var payload struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if payload.Status != want {
		t.Fatalf("status = %q, want %q", payload.Status, want)
	}
}
