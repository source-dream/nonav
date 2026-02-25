package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"nonav/server/internal/config"
	"nonav/server/internal/httpserver"
	"nonav/server/internal/store"
)

type App struct {
	name    string
	store   *store.SQLiteStore
	server  *http.Server
	cleanup bool
}

func NewAPI(cfg config.Config) (*App, error) {
	st, err := store.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		return nil, err
	}

	handler, err := httpserver.NewAPI(cfg, st)
	if err != nil {
		_ = st.Close()
		return nil, err
	}

	return &App{
		name:  "api",
		store: st,
		server: &http.Server{
			Addr:         cfg.APIListenAddr,
			Handler:      handler,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		cleanup: true,
	}, nil
}

func NewGateway(cfg config.Config) (*App, error) {
	st, err := store.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		return nil, err
	}

	handler, err := httpserver.NewGateway(cfg, st)
	if err != nil {
		_ = st.Close()
		return nil, err
	}

	return &App{
		name:  "gateway",
		store: st,
		server: &http.Server{
			Addr:         cfg.GatewayListenAddr,
			Handler:      handler,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
	}, nil
}

func (a *App) Run() error {
	defer a.store.Close()

	if a.cleanup {
		go a.startCleanupTicker()
	}

	log.Printf("nonav %s listening on %s", a.name, a.server.Addr)
	if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen and serve: %w", err)
	}

	return nil
}

func (a *App) startCleanupTicker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		if err := a.store.PurgeExpiredShares(context.Background()); err != nil {
			log.Printf("cleanup expired shares failed: %v", err)
		}
	}
}
