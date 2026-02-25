package main

import (
	"log"

	"nonav/server/internal/app"
	"nonav/server/internal/config"
)

func main() {
	cfg := config.Load()
	application, err := app.NewAPI(cfg)
	if err != nil {
		log.Fatalf("failed to initialize api app: %v", err)
	}

	if err := application.Run(); err != nil {
		log.Fatalf("api exited with error: %v", err)
	}
}
