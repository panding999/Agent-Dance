package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/panding999/agent-dance/backend/internal/audio"
	"github.com/panding999/agent-dance/backend/internal/live"
	"github.com/panding999/agent-dance/backend/internal/store"
)

type Server struct {
	store          *store.SQLiteStore
	liveGateway    *live.Gateway
	mux            *http.ServeMux
	allowedOrigins []string
}

type ServerOptions struct {
	LiveRunnerFactory live.SessionRunnerFactory
	LiveChunkCache    *audio.SessionChunkCache
	AllowedOrigins    []string
}

func NewServer(st *store.SQLiteStore) *Server {
	return NewServerWithOptions(st, ServerOptions{})
}

func NewServerWithOptions(st *store.SQLiteStore, options ServerOptions) *Server {
	cache := options.LiveChunkCache
	if cache == nil {
		cache = audio.NewSessionChunkCache(256)
	}
	allowedOrigins := compactStrings(options.AllowedOrigins)
	s := &Server{
		store: st,
		liveGateway: live.NewGatewayWithOptions(st, cache, live.GatewayOptions{
			RunnerFactory:  options.LiveRunnerFactory,
			OriginPatterns: websocketOriginPatterns(allowedOrigins),
		}),
		mux:            http.NewServeMux(),
		allowedOrigins: allowedOrigins,
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	if len(s.allowedOrigins) > 0 {
		return withCORS(s.mux, s.allowedOrigins)
	}
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.HandleFunc("/readyz", s.handleReady)
	s.mux.HandleFunc("/api/sessions", s.handleSessions)
	s.mux.HandleFunc("/api/sessions/", s.handleSessionByID)
	s.mux.Handle("/api/live/ws", s.liveGateway)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	if err := s.store.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not_ready",
			"error":  err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	var params store.CreateSessionParams
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&params); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request body"})
		return
	}

	session, err := s.store.CreateSession(r.Context(), params)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create session failed"})
		return
	}

	w.Header().Set("Location", "/api/sessions/"+session.ID)
	writeJSON(w, http.StatusCreated, session)
}

func (s *Server) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}

	session, err := s.store.GetSession(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "get session failed"})
		return
	}

	writeJSON(w, http.StatusOK, session)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	w.Header().Set("Allow", "GET, POST")
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}

func withCORS(next http.Handler, allowedOrigins []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" {
			if originAllowed(origin, allowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.Header().Add("Vary", "Origin")
				w.Header().Add("Vary", "Access-Control-Request-Method")
				w.Header().Add("Vary", "Access-Control-Request-Headers")
			} else if isCORSPreflight(r) {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
		}

		if isCORSPreflight(r) {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isCORSPreflight(r *http.Request) bool {
	return r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""
}

func originAllowed(origin string, allowedOrigins []string) bool {
	for _, allowed := range allowedOrigins {
		if origin == allowed {
			return true
		}
	}
	return false
}

func websocketOriginPatterns(allowedOrigins []string) []string {
	patterns := make([]string, 0, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		parsed, err := url.Parse(origin)
		if err == nil && parsed.Host != "" {
			patterns = append(patterns, parsed.Host)
			continue
		}
		patterns = append(patterns, origin)
	}
	return patterns
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
