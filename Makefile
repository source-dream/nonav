
.PHONY: help deps dev dev-api dev-gateway dev-frontend dev-frps dev-frpc build build-web prepare-webdist build-api build-gateway run-api run-gateway run-all clean

help:
	@printf "Available targets:\n"
	@printf "  make deps          Install backend and frontend dependencies\n"
	@printf "  make dev           Run api + gateway(embedded frps) + frontend\n"
	@printf "  make build         Build frontend and both backend binaries\n"
	@printf "  make run-all       Run built api and gateway binaries together\n"
	@printf "  make clean         Remove build artifacts\n"

deps:
	@cd server && go mod tidy
	@cd web && npm install

dev:
	@pids=""; \
	cleanup(){ \
		for pid in $$pids; do \
			kill $$pid >/dev/null 2>&1 || true; \
		done; \
	}; \
	trap cleanup INT TERM EXIT; \
	$(MAKE) dev-api & pids="$$pids $$!"; \
	$(MAKE) dev-gateway & pids="$$pids $$!"; \
	$(MAKE) dev-frontend & pids="$$pids $$!"; \
	wait $$pids

dev-frps:
	@./frp/frps -c ./frp/frps.toml

dev-frpc:
	@sleep 1 && ./frp/frpc -c ./frp/frpc.toml

dev-api:
	@cd server && NONAV_API_LISTEN_ADDR=:8081 NONAV_DB_PATH=./data/nonav.db NONAV_CORS_ORIGIN=http://localhost:5173 NONAV_FORCE_FRP=true NONAV_FRP_UPSTREAM_URL=http://127.0.0.1:13000 NONAV_FRP_PORT_MIN=13000 NONAV_FRP_PORT_MAX=13020 NONAV_FRP_CLIENT_BIN=../frp/frpc NONAV_FRP_SERVER_ADDR=127.0.0.1 NONAV_FRP_SERVER_PORT=7000 NONAV_FRP_AUTH_TOKEN=nonav-local-dev NONAV_FRP_EXPOSE_API=true NONAV_FRP_API_REMOTE_PORT=18081 go run ./cmd/nonav-api

dev-gateway:
	@cd server && NONAV_GATEWAY_LISTEN_ADDR=:8080 NONAV_API_BASE_URL=http://127.0.0.1:18081 NONAV_FRONTEND_DEV_PROXY_URL=http://127.0.0.1:5173 NONAV_WEB_DIST_DIR=../web/dist NONAV_FORCE_FRP=true NONAV_FRP_UPSTREAM_URL=http://127.0.0.1:13000 NONAV_FRP_PORT_MIN=13000 NONAV_FRP_PORT_MAX=13020 NONAV_EMBED_FRPS=true NONAV_FRP_SERVER_BIN=../frp/frps NONAV_FRP_SERVER_BIND_ADDR=0.0.0.0 NONAV_FRP_SERVER_ADDR=127.0.0.1 NONAV_FRP_SERVER_PORT=7000 NONAV_FRP_AUTH_TOKEN=nonav-local-dev go run ./cmd/nonav-gateway

dev-frontend:
	@cd web && npm run dev

build: build-web prepare-webdist build-api build-gateway

build-web:
	@cd web && npm run build

prepare-webdist:
	@rm -rf server/web-dist
	@rm -rf server/internal/httpserver/web-dist
	@mkdir -p server/web-dist
	@mkdir -p server/internal/httpserver/web-dist
	@cp -R web/dist/. server/web-dist/
	@cp -R web/dist/. server/internal/httpserver/web-dist/

build-api:
	@mkdir -p bin
	@rm -f bin/nonav-api
	@cd server && go build -o ../bin/nonav ./cmd/nonav-api

build-gateway:
	@mkdir -p bin
	@cd server && go build -o ../bin/nonav-gateway ./cmd/nonav-gateway

run-api:
	@NONAV_API_LISTEN_ADDR=:8081 NONAV_DB_PATH=server/data/nonav.db NONAV_FORCE_FRP=true NONAV_FRP_UPSTREAM_URL=http://127.0.0.1:13000 NONAV_FRP_PORT_MIN=13000 NONAV_FRP_PORT_MAX=13020 NONAV_FRP_CLIENT_BIN=./frp/frpc NONAV_FRP_SERVER_ADDR=127.0.0.1 NONAV_FRP_SERVER_PORT=7000 NONAV_FRP_AUTH_TOKEN=nonav-local-dev NONAV_FRP_EXPOSE_API=true NONAV_FRP_API_REMOTE_PORT=18081 ./bin/nonav

run-gateway:
	@NONAV_GATEWAY_LISTEN_ADDR=:8080 NONAV_API_BASE_URL=http://127.0.0.1:18081 NONAV_WEB_DIST_DIR=server/web-dist NONAV_FORCE_FRP=true NONAV_FRP_UPSTREAM_URL=http://127.0.0.1:13000 NONAV_FRP_PORT_MIN=13000 NONAV_FRP_PORT_MAX=13020 NONAV_EMBED_FRPS=true NONAV_FRP_SERVER_BIN=./frp/frps NONAV_FRP_SERVER_BIND_ADDR=0.0.0.0 NONAV_FRP_SERVER_ADDR=127.0.0.1 NONAV_FRP_SERVER_PORT=7000 NONAV_FRP_AUTH_TOKEN=nonav-local-dev ./bin/nonav-gateway

run-all:
	@pids=""; \
	cleanup(){ \
		for pid in $$pids; do \
			kill $$pid >/dev/null 2>&1 || true; \
		done; \
	}; \
	trap cleanup INT TERM EXIT; \
	$(MAKE) run-api & pids="$$pids $$!"; \
	$(MAKE) run-gateway & pids="$$pids $$!"; \
	wait $$pids

clean:
	@rm -rf bin server/web-dist web/dist
