package main

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/panding999/agent-dance/backend/internal/config"
	"github.com/panding999/agent-dance/backend/internal/httpapi"
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

	api := httpapi.NewServer(st)
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
