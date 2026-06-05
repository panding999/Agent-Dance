package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type SQLiteStore struct {
	db *sql.DB
}

type CreateSessionParams struct {
	Mode           string `json:"mode"`
	SourceLanguage string `json:"source_language"`
	TargetLanguage string `json:"target_language"`
	VoiceEnabled   bool   `json:"voice_enabled"`
}

type Session struct {
	ID             string     `json:"id"`
	Mode           string     `json:"mode"`
	SourceLanguage string     `json:"source_language"`
	TargetLanguage string     `json:"target_language"`
	VoiceEnabled   bool       `json:"voice_enabled"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ClosedAt       *time.Time `json:"closed_at,omitempty"`
}

func Open(ctx context.Context, databaseURL string) (*SQLiteStore, error) {
	if err := ensureDatabaseDir(databaseURL); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	st := &SQLiteStore{db: db}
	if err := st.init(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return st, nil
}

func (s *SQLiteStore) DB() *sql.DB {
	return s.db
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *SQLiteStore) CreateSession(ctx context.Context, params CreateSessionParams) (Session, error) {
	now := time.Now().UTC()
	session := Session{
		ID:             newID(),
		Mode:           strings.TrimSpace(params.Mode),
		SourceLanguage: strings.TrimSpace(params.SourceLanguage),
		TargetLanguage: strings.TrimSpace(params.TargetLanguage),
		VoiceEnabled:   params.VoiceEnabled,
		Status:         "created",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if session.Mode == "" {
		session.Mode = "live"
	}
	if session.SourceLanguage == "" {
		session.SourceLanguage = "auto"
	}
	if session.TargetLanguage == "" {
		session.TargetLanguage = "zh"
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (
			id, mode, source_language, target_language, voice_enabled, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		session.ID,
		session.Mode,
		session.SourceLanguage,
		session.TargetLanguage,
		boolToInt(session.VoiceEnabled),
		session.Status,
		formatTime(session.CreatedAt),
		formatTime(session.UpdatedAt),
	)
	if err != nil {
		return Session{}, fmt.Errorf("insert session: %w", err)
	}

	return session, nil
}

func (s *SQLiteStore) GetSession(ctx context.Context, id string) (Session, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, mode, source_language, target_language, voice_enabled, status, created_at, updated_at, closed_at
		FROM sessions
		WHERE id = ?
	`, id)

	session, err := scanSession(row)
	if err != nil {
		return Session{}, err
	}
	return session, nil
}

func (s *SQLiteStore) CloseSession(ctx context.Context, id string) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		UPDATE sessions
		SET status = ?, updated_at = ?, closed_at = ?
		WHERE id = ?
	`, "closed", formatTime(now), formatTime(now), id)
	if err != nil {
		return fmt.Errorf("close session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("close session rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) init(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		return fmt.Errorf("set busy timeout: %w", err)
	}

	for _, stmt := range schemaStatements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("initialize schema: %w", err)
		}
	}
	if err := s.migrate(ctx); err != nil {
		return err
	}

	return nil
}

func (s *SQLiteStore) migrate(ctx context.Context) error {
	if err := s.ensureColumn(ctx, "sessions", "status", "TEXT NOT NULL DEFAULT 'created'"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "sessions", "closed_at", "TEXT"); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) ensureColumn(ctx context.Context, table string, column string, definition string) error {
	exists, err := s.columnExists(ctx, table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition)
	if _, err := s.db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, column, err)
	}
	return nil
}

func (s *SQLiteStore) columnExists(ctx context.Context, table string, column string) (bool, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, fmt.Errorf("inspect table %s: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int

		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, fmt.Errorf("scan table %s column: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("inspect table %s rows: %w", table, err)
	}
	return false, nil
}

func scanSession(scanner interface {
	Scan(dest ...any) error
}) (Session, error) {
	var session Session
	var voiceEnabled int
	var status string
	var createdAt string
	var updatedAt string
	var closedAt sql.NullString

	err := scanner.Scan(
		&session.ID,
		&session.Mode,
		&session.SourceLanguage,
		&session.TargetLanguage,
		&voiceEnabled,
		&status,
		&createdAt,
		&updatedAt,
		&closedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("scan session: %w", err)
	}

	parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Session{}, fmt.Errorf("parse created_at: %w", err)
	}
	parsedUpdatedAt, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return Session{}, fmt.Errorf("parse updated_at: %w", err)
	}

	session.VoiceEnabled = voiceEnabled != 0
	session.Status = status
	session.CreatedAt = parsedCreatedAt
	session.UpdatedAt = parsedUpdatedAt
	if closedAt.Valid && closedAt.String != "" {
		parsedClosedAt, err := time.Parse(time.RFC3339Nano, closedAt.String)
		if err != nil {
			return Session{}, fmt.Errorf("parse closed_at: %w", err)
		}
		session.ClosedAt = &parsedClosedAt
	}
	return session, nil
}

func ensureDatabaseDir(databaseURL string) error {
	if databaseURL == "" || databaseURL == ":memory:" || strings.HasPrefix(databaseURL, "file:") {
		return nil
	}

	dir := filepath.Dir(databaseURL)
	if dir == "." || dir == "" {
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	return nil
}

func newID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(fmt.Sprintf("read random bytes: %v", err))
	}
	return hex.EncodeToString(buf[:])
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		mode TEXT NOT NULL,
		source_language TEXT NOT NULL,
		target_language TEXT NOT NULL,
		voice_enabled INTEGER NOT NULL DEFAULT 0 CHECK (voice_enabled IN (0, 1)),
		status TEXT NOT NULL DEFAULT 'created',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		closed_at TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS segments (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		provider_segment_id TEXT,
		start_ms INTEGER NOT NULL DEFAULT 0,
		end_ms INTEGER,
		source_text TEXT NOT NULL DEFAULT '',
		target_text TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	)`,
	`CREATE TABLE IF NOT EXISTS segment_revisions (
		id TEXT PRIMARY KEY,
		segment_id TEXT NOT NULL,
		reason TEXT NOT NULL DEFAULT '',
		source_text TEXT NOT NULL DEFAULT '',
		target_text TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		FOREIGN KEY (segment_id) REFERENCES segments(id) ON DELETE CASCADE
	)`,
	`CREATE TABLE IF NOT EXISTS glossary_terms (
		id TEXT PRIMARY KEY,
		source_term TEXT NOT NULL,
		target_term TEXT NOT NULL,
		note TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS audio_chunks (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		start_ms INTEGER NOT NULL,
		duration_ms INTEGER NOT NULL,
		format TEXT NOT NULL,
		storage_path TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	)`,
	`CREATE TABLE IF NOT EXISTS provider_events (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		provider TEXT NOT NULL,
		event_type TEXT NOT NULL,
		log_id TEXT NOT NULL DEFAULT '',
		payload_json TEXT NOT NULL,
		created_at TEXT NOT NULL,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	)`,
	`CREATE INDEX IF NOT EXISTS idx_segments_session_id ON segments(session_id)`,
	`CREATE INDEX IF NOT EXISTS idx_audio_chunks_session_id ON audio_chunks(session_id)`,
	`CREATE INDEX IF NOT EXISTS idx_provider_events_session_id ON provider_events(session_id)`,
}

func parseBoolColumn(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case string:
		parsed, _ := strconv.ParseBool(typed)
		return parsed
	default:
		return false
	}
}
