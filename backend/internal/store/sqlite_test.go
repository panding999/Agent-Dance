package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestOpenInitializesExpectedTables(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "agent-dance.db")

	st, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer st.Close()

	tables := map[string]bool{}
	rows, err := st.DB().QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatalf("query tables: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		tables[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}

	for _, table := range []string{
		"sessions",
		"segments",
		"segment_revisions",
		"glossary_terms",
		"audio_chunks",
		"provider_events",
	} {
		if !tables[table] {
			t.Fatalf("expected table %q to be initialized; got %v", table, tables)
		}
	}
}

func TestOpenMigratesLegacySessionStatusColumns(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "agent-dance.db")

	legacyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy sqlite: %v", err)
	}
	_, err = legacyDB.ExecContext(ctx, `
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			mode TEXT NOT NULL,
			source_language TEXT NOT NULL,
			target_language TEXT NOT NULL,
			voice_enabled INTEGER NOT NULL DEFAULT 0 CHECK (voice_enabled IN (0, 1)),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("create legacy sessions table: %v", err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	st, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer st.Close()

	created, err := st.CreateSession(ctx, CreateSessionParams{
		Mode:           "live",
		SourceLanguage: "en",
		TargetLanguage: "zh",
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if created.Status != "created" {
		t.Fatalf("Status = %q, want created", created.Status)
	}

	if err := st.CloseSession(ctx, created.ID); err != nil {
		t.Fatalf("CloseSession() error = %v", err)
	}
	got, err := st.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if got.Status != "closed" || got.ClosedAt == nil {
		t.Fatalf("Status = %q ClosedAt = %v, want closed with timestamp", got.Status, got.ClosedAt)
	}
}

func TestCreateAndGetSession(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	created, err := st.CreateSession(ctx, CreateSessionParams{
		Mode:           "live",
		SourceLanguage: "en",
		TargetLanguage: "zh",
		VoiceEnabled:   true,
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected generated session ID")
	}

	got, err := st.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
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
	if got.CreatedAt.IsZero() {
		t.Fatal("CreatedAt is zero")
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt is zero")
	}
	if got.Status != "created" {
		t.Fatalf("Status = %q", got.Status)
	}
}

func TestGetSessionReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	_, err := st.GetSession(ctx, "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSession() error = %v, want ErrNotFound", err)
	}
}

func TestCloseSessionMarksSessionClosed(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	created, err := st.CreateSession(ctx, CreateSessionParams{
		Mode:           "live",
		SourceLanguage: "en",
		TargetLanguage: "zh",
	})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if err := st.CloseSession(ctx, created.ID); err != nil {
		t.Fatalf("CloseSession() error = %v", err)
	}

	got, err := st.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if got.Status != "closed" {
		t.Fatalf("Status = %q, want closed", got.Status)
	}
	if got.ClosedAt == nil {
		t.Fatal("ClosedAt is nil, want close timestamp")
	}
	if got.UpdatedAt.Before(created.UpdatedAt) {
		t.Fatalf("UpdatedAt = %s, want >= %s", got.UpdatedAt, created.UpdatedAt)
	}
}

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()

	st, err := Open(context.Background(), filepath.Join(t.TempDir(), "agent-dance.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil && !errors.Is(err, sql.ErrConnDone) {
			t.Fatalf("Close() error = %v", err)
		}
	})
	return st
}
