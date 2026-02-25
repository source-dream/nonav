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
	name        string
	store       *store.SQLiteStore
	server      *http.Server
	cleanup     bool
	cleanupTask func(context.Context) error
	startTask   func() error
	stopTask    func() error
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
		cleanup:     true,
		cleanupTask: handler.CleanupExpiredShares,
		startTask:   handler.StartBackgroundServices,
		stopTask:    handler.StopBackgroundServices,
	}, nil
}

func NewGateway(cfg config.Config) (*App, error) {
	handler, err := httpserver.NewGateway(cfg)
	if err != nil {
		return nil, err
	}

	return &App{
		name:  "gateway",
		store: nil,
		server: &http.Server{
			Addr:         cfg.GatewayListenAddr,
			Handler:      handler,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		startTask: handler.StartBackgroundServices,
		stopTask:  handler.StopBackgroundServices,
	}, nil
}

func (a *App) Run() error {
	if a.store != nil {
		defer a.store.Close()
	}
	if a.stopTask != nil {
		defer func() {
			if err := a.stopTask(); err != nil {
				log.Printf("background stop failed: %v", err)
			}
		}()
	}

	if a.startTask != nil {
		if err := a.startTask(); err != nil {
			return fmt.Errorf("background start failed: %w", err)
		}
	}

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
		task := a.cleanupTask
		if task == nil {
			task = a.store.PurgeExpiredShares
		}

		if err := task(context.Background()); err != nil {
			log.Printf("cleanup expired shares failed: %v", err)
		}
	}
}
