
.PHONY: help deps dev dev-api dev-gateway dev-frontend dev-frps dev-frpc build build-web prepare-webdist build-api build-gateway run-api run-gateway run-all clean

help:
	@printf "Available targets:\n"
	@printf "  make deps          Install backend and frontend dependencies\n"
	@printf "  make dev           Run api + gateway + frontend + frps + frpc\n"
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
	$(MAKE) dev-frps & pids="$$pids $$!"; \
	$(MAKE) dev-frpc & pids="$$pids $$!"; \
	$(MAKE) dev-api & pids="$$pids $$!"; \
	$(MAKE) dev-gateway & pids="$$pids $$!"; \
	$(MAKE) dev-frontend & pids="$$pids $$!"; \
	wait $$pids

dev-frps:
	@./frp/frps -c ./frp/frps.toml

dev-frpc:
	@sleep 1 && ./frp/frpc -c ./frp/frpc.toml

dev-api:
	@cd server && NONAV_API_LISTEN_ADDR=:8081 NONAV_DB_PATH=./data/nonav.db NONAV_CORS_ORIGIN=http://localhost:5173 NONAV_FORCE_FRP=true NONAV_FRP_UPSTREAM_URL=http://127.0.0.1:13000 go run ./cmd/nonav-api

dev-gateway:
	@cd server && NONAV_GATEWAY_LISTEN_ADDR=:8080 NONAV_DB_PATH=./data/nonav.db NONAV_API_BASE_URL=http://127.0.0.1:8081 NONAV_FRONTEND_DEV_PROXY_URL=http://127.0.0.1:5173 NONAV_WEB_DIST_DIR=../web/dist NONAV_FORCE_FRP=true NONAV_FRP_UPSTREAM_URL=http://127.0.0.1:13000 go run ./cmd/nonav-gateway

dev-frontend:
	@cd web && npm run dev

build: build-web prepare-webdist build-api build-gateway

build-web:
	@cd web && npm run build

prepare-webdist:
	@rm -rf server/web-dist
	@mkdir -p server/web-dist
	@cp -R web/dist/. server/web-dist/

build-api:
	@mkdir -p bin
	@rm -f bin/nonav-api
	@cd server && go build -o ../bin/nonav ./cmd/nonav-api

build-gateway:
	@mkdir -p bin
	@cd server && go build -o ../bin/nonav-gateway ./cmd/nonav-gateway

run-api:
	@NONAV_API_LISTEN_ADDR=:8081 NONAV_DB_PATH=server/data/nonav.db NONAV_FORCE_FRP=true NONAV_FRP_UPSTREAM_URL=http://127.0.0.1:13000 ./bin/nonav

run-gateway:
	@NONAV_GATEWAY_LISTEN_ADDR=:8080 NONAV_DB_PATH=server/data/nonav.db NONAV_API_BASE_URL=http://127.0.0.1:8081 NONAV_WEB_DIST_DIR=server/web-dist NONAV_FORCE_FRP=true NONAV_FRP_UPSTREAM_URL=http://127.0.0.1:13000 ./bin/nonav-gateway

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
