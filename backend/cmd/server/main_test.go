package main

import (
	"testing"

	"github.com/panding999/agent-dance/backend/internal/config"
	"github.com/panding999/agent-dance/backend/internal/store"
)

func TestNewLiveRunnerFactoryCreatesRunner(t *testing.T) {
	factory := newLiveRunnerFactory(config.Config{
		DoubaoAPIKey:        "api-key",
		DoubaoASTResourceID: "volc.service_type.10053",
		DoubaoASTModelID:    "seed-liveinterpret-2",
	})

	runner, err := factory(store.Session{
		ID:             "session-1",
		SourceLanguage: "zh",
		TargetLanguage: "en",
	})
	if err != nil {
		t.Fatalf("factory() error = %v", err)
	}
	if runner == nil {
		t.Fatal("factory() returned nil runner")
	}
}
