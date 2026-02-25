package main

import (
	"log"
	"sync"

	"nonav/server/internal/app"
	"nonav/server/internal/config"
)

func main() {
	cfg := config.Load()
	apiApp, err := app.NewAPI(cfg)
	if err != nil {
		log.Fatalf("failed to initialize api app: %v", err)
	}

	gatewayApp, err := app.NewGateway(cfg)
	if err != nil {
		log.Fatalf("failed to initialize gateway app: %v", err)
	}

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		errCh <- apiApp.Run()
	}()

	go func() {
		defer wg.Done()
		errCh <- gatewayApp.Run()
	}()

	err = <-errCh
	if err != nil {
		log.Fatalf("server exited with error: %v", err)
	}

	wg.Wait()
}
