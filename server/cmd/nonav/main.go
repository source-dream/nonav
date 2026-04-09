package main

import (
	"bufio"
	"log"
	"os"
	"strings"
	"sync"

	"nonav/server/internal/app"
	"nonav/server/internal/config"
)

func main() {
	createdInternal := ensureEnvFile("internal.env", internalEnvTemplate)
	createdGateway := ensureEnvFile("gateway.env", gatewayEnvTemplate)
	if createdInternal || createdGateway {
		log.Printf("default config created: internal.env=%t gateway.env=%t", createdInternal, createdGateway)
		log.Printf("please review and update both files, then run again")
		return
	}
	loadEnvFile("internal.env")
	loadEnvFile("gateway.env")

	cfg := config.Load()
	nonavApp, err := app.NewAPI(cfg)
	if err != nil {
		log.Fatalf("failed to initialize nonav: %v", err)
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
		errCh <- nonavApp.Run()
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

func loadEnvFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			continue
		}

		if os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
}

func ensureEnvFile(path string, content string) bool {
	if _, err := os.Stat(path); err == nil {
		return false
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		log.Printf("failed to generate %s: %v", path, err)
		return false
	}

	log.Printf("generated default env file: %s", path)
	return true
}

const internalEnvTemplate = `NONAV_API_LISTEN_ADDR=:8081
NONAV_DB_PATH=./data/nonav.db
NONAV_CORS_ORIGIN=http://127.0.0.1:5173
NONAV_PUBLIC_BASE_URL=http://lvh.me:8080
NONAV_SHARE_SUBDOMAIN_ENABLED=true
NONAV_SHARE_SUBDOMAIN_BASE=lvh.me
NONAV_LOG_LEVEL=info
NONAV_LOG_ROUTE_TRACE=true

NONAV_FORCE_FRP=true
NONAV_FRP_UPSTREAM_URL=http://127.0.0.1:13000
NONAV_FRP_PORT_MIN=13000
NONAV_FRP_PORT_MAX=13100

NONAV_FRP_CLIENT_BIN=../frp/frpc
NONAV_FRP_SERVER_ADDR=127.0.0.1
NONAV_FRP_SERVER_PORT=7000
NONAV_FRP_AUTH_TOKEN=change-me
NONAV_FRP_RECOVER_ON_START=true
NONAV_FRP_EXPOSE_API=true
NONAV_FRP_API_REMOTE_PORT=18081
`

const gatewayEnvTemplate = `NONAV_GATEWAY_LISTEN_ADDR=:8080
NONAV_API_BASE_URL=http://127.0.0.1:18081
NONAV_PUBLIC_BASE_URL=http://lvh.me:8080
NONAV_SHARE_SUBDOMAIN_ENABLED=true
NONAV_SHARE_SUBDOMAIN_BASE=lvh.me
NONAV_LOG_LEVEL=info
NONAV_LOG_ROUTE_TRACE=true

NONAV_FORCE_FRP=true
NONAV_FRP_UPSTREAM_URL=http://127.0.0.1:13000
NONAV_FRP_PORT_MIN=13000
NONAV_FRP_PORT_MAX=13100

NONAV_EMBED_FRPS=true
NONAV_FRP_SERVER_BIN=frps
NONAV_FRP_SERVER_BIND_ADDR=0.0.0.0
NONAV_FRP_SERVER_ADDR=127.0.0.1
NONAV_FRP_SERVER_PORT=7000
NONAV_FRP_AUTH_TOKEN=change-me
`
