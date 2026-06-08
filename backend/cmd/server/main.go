package main

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/panding999/agent-dance/backend/internal/config"
	"github.com/panding999/agent-dance/backend/internal/doubao/ast"
	"github.com/panding999/agent-dance/backend/internal/httpapi"
	"github.com/panding999/agent-dance/backend/internal/live"
	"github.com/panding999/agent-dance/backend/internal/store"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}

	st, err := store.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() {
		if err := st.Close(); err != nil {
			log.Printf("close store: %v", err)
		}
	}()

	api := httpapi.NewServerWithOptions(st, httpapi.ServerOptions{
		LiveRunnerFactory: newLiveRunnerFactory(cfg),
		AllowedOrigins:    cfg.HTTPAllowedOrigins,
	})
	server := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: api.Handler(),
	}

	log.Printf("agent-dance backend listening on %s", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func newLiveRunnerFactory(cfg config.Config) live.SessionRunnerFactory {
	return func(store.Session) (*live.SessionRunner, error) {
		if !cfg.HasDoubaoCredentials() {
			return nil, errors.New("Doubao credentials are not configured; set DOUBAO_API_KEY or legacy credentials and restart")
		}
		astClient, err := ast.NewClient(ast.ClientOptions{
			APIKey:     cfg.DoubaoAPIKey,
			AppID:      cfg.DoubaoAppID,
			AppKey:     cfg.DoubaoAppKey,
			AccessKey:  cfg.DoubaoAccessKey,
			ResourceID: cfg.DoubaoASTResourceID,
			ModelID:    cfg.DoubaoASTModelID,
			Codec:      ast.ProtobufCodec{},
		})
		if err != nil {
			return nil, err
		}
		return live.NewSessionRunner(astClient, live.SessionRunnerOptions{})
	}
}
