package main

import (
	"log"

	"nonav/server/internal/app"
	"nonav/server/internal/config"
)

func main() {
	cfg := config.Load()
	application, err := app.NewGateway(cfg)
	if err != nil {
		log.Fatalf("failed to initialize gateway app: %v", err)
	}

	if err := application.Run(); err != nil {
		log.Fatalf("gateway exited with error: %v", err)
	}
}
