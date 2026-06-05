package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/panding999/agent-dance/backend/internal/store"
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

func TestGetMissingSessionReturnsNotFound(t *testing.T) {
	handler := newTestHandler(t)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sessions/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET missing session status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func newTestHandler(t *testing.T) http.Handler {
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

	return NewServer(st).Handler()
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
