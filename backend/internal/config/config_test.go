package config

import (
	"strings"
	"testing"
)

func TestLoadFromEnvReportsMissingRequiredValues(t *testing.T) {
	t.Setenv("DOUBAO_API_KEY", "")
	t.Setenv("DOUBAO_APP_ID", "")
	t.Setenv("DOUBAO_APP_KEY", "")
	t.Setenv("DOUBAO_ACCESS_KEY", "")
	for _, key := range requiredEnvKeys {
		t.Setenv(key, "")
	}

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected missing required values error")
	}

	for _, key := range requiredEnvKeys {
		if !strings.Contains(err.Error(), key) {
			t.Fatalf("expected error to mention %s, got %q", key, err.Error())
		}
	}
	if !strings.Contains(err.Error(), "DOUBAO_API_KEY") {
		t.Fatalf("expected error to mention DOUBAO_API_KEY auth option, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "DOUBAO_APP_KEY") {
		t.Fatalf("expected error to mention DOUBAO_APP_KEY auth option, got %q", err.Error())
	}
}

func TestLoadFromEnvReadsBackendConfigWithAPIKey(t *testing.T) {
	t.Setenv("DOUBAO_API_KEY", "api-key")
	t.Setenv("DOUBAO_APP_ID", "")
	t.Setenv("DOUBAO_APP_KEY", "")
	t.Setenv("DOUBAO_ACCESS_KEY", "")
	t.Setenv("DOUBAO_AST_RESOURCE_ID", "ast-resource")
	t.Setenv("DOUBAO_AST_MODEL_ID", "model-id")
	t.Setenv("DOUBAO_AUC_RESOURCE_ID", "auc-resource")
	t.Setenv("DATABASE_URL", "runtime/agent-dance.db")
	t.Setenv("UPLOAD_DIR", "uploads")
	t.Setenv("HTTP_ADDR", ":18080")

	got, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if got.DoubaoAPIKey != "api-key" {
		t.Fatalf("DoubaoAPIKey = %q", got.DoubaoAPIKey)
	}
	if got.DoubaoASTResourceID != "ast-resource" {
		t.Fatalf("DoubaoASTResourceID = %q", got.DoubaoASTResourceID)
	}
	if got.DoubaoASTModelID != "model-id" {
		t.Fatalf("DoubaoASTModelID = %q", got.DoubaoASTModelID)
	}
	if got.DoubaoAUCResourceID != "auc-resource" {
		t.Fatalf("DoubaoAUCResourceID = %q", got.DoubaoAUCResourceID)
	}
	if got.DatabaseURL != "runtime/agent-dance.db" {
		t.Fatalf("DatabaseURL = %q", got.DatabaseURL)
	}
	if got.UploadDir != "uploads" {
		t.Fatalf("UploadDir = %q", got.UploadDir)
	}
	if got.HTTPAddr != ":18080" {
		t.Fatalf("HTTPAddr = %q", got.HTTPAddr)
	}
}

func TestLoadFromEnvReadsHTTPAllowedOrigins(t *testing.T) {
	t.Setenv("DOUBAO_API_KEY", "api-key")
	t.Setenv("DOUBAO_APP_ID", "")
	t.Setenv("DOUBAO_APP_KEY", "")
	t.Setenv("DOUBAO_ACCESS_KEY", "")
	t.Setenv("DOUBAO_AST_RESOURCE_ID", "ast-resource")
	t.Setenv("DOUBAO_AUC_RESOURCE_ID", "auc-resource")
	t.Setenv("DATABASE_URL", "runtime/agent-dance.db")
	t.Setenv("UPLOAD_DIR", "uploads")
	t.Setenv("HTTP_ALLOWED_ORIGINS", " http://localhost:3000, http://127.0.0.1:3000 ,,")

	got, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	want := []string{"http://localhost:3000", "http://127.0.0.1:3000"}
	if len(got.HTTPAllowedOrigins) != len(want) {
		t.Fatalf("HTTPAllowedOrigins = %#v, want %#v", got.HTTPAllowedOrigins, want)
	}
	for i := range want {
		if got.HTTPAllowedOrigins[i] != want[i] {
			t.Fatalf("HTTPAllowedOrigins = %#v, want %#v", got.HTTPAllowedOrigins, want)
		}
	}
}

func TestLoadFromEnvReadsLegacyAppCredentials(t *testing.T) {
	t.Setenv("DOUBAO_API_KEY", "")
	t.Setenv("DOUBAO_APP_ID", "app-id")
	t.Setenv("DOUBAO_APP_KEY", "app-key")
	t.Setenv("DOUBAO_ACCESS_KEY", "access-key")
	t.Setenv("DOUBAO_AST_RESOURCE_ID", "ast-resource")
	t.Setenv("DOUBAO_AUC_RESOURCE_ID", "auc-resource")
	t.Setenv("DATABASE_URL", "runtime/agent-dance.db")
	t.Setenv("UPLOAD_DIR", "uploads")

	got, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if got.DoubaoAppID != "app-id" {
		t.Fatalf("DoubaoAppID = %q", got.DoubaoAppID)
	}
	if got.DoubaoAppKey != "app-key" {
		t.Fatalf("DoubaoAppKey = %q", got.DoubaoAppKey)
	}
	if got.DoubaoAccessKey != "access-key" {
		t.Fatalf("DoubaoAccessKey = %q", got.DoubaoAccessKey)
	}
}

func TestLoadFromEnvDefaultsHTTPAddr(t *testing.T) {
	t.Setenv("DOUBAO_API_KEY", "api-key")
	t.Setenv("DOUBAO_APP_ID", "")
	t.Setenv("DOUBAO_APP_KEY", "")
	t.Setenv("DOUBAO_ACCESS_KEY", "")
	t.Setenv("DOUBAO_AST_RESOURCE_ID", "ast-resource")
	t.Setenv("DOUBAO_AUC_RESOURCE_ID", "auc-resource")
	t.Setenv("DATABASE_URL", "runtime/agent-dance.db")
	t.Setenv("UPLOAD_DIR", "uploads")
	t.Setenv("HTTP_ADDR", "")

	got, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}
	if got.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q", got.HTTPAddr)
	}
}
