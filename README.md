# nonav

Go + Vue + SQLite 的内网导航与分享网关项目。

当前版本是可运行 MVP：

- 内网导航磁贴（新增/删除/点击统计）
- 右键卡片创建分享（默认 24h，可选密码）
- 单域名路径路由分享网关（`/s/:token`）
- 分享访问日志与状态管理（active/stopped/expired）
- 日间/夜间主题 + 自适应布局

## 架构

- `web`：Vue 3 前端（导航页）
- `server/cmd/nonav-api`：Go 导航服务 `nonav`（内网导航前后端 + 控制面）（二进制 1）
- `server/cmd/nonav-gateway`：Go 分享网关（仅提供 `/s/*` 分享访问）（二进制 2）
- `nginx`（可选）：外层 TLS 与反向代理

请求链路：

1. 用户访问 `node.sourcedream.cn/s/<token>`
2. Nginx 反代到 Go 网关（`nonav-gateway`）
3. 网关将 `/api/*` 转发到 `nonav`
4. 网关对 `/s/*` 校验分享状态和密码会话并反代到目标站点

> 当前版本已支持 FRP-only 模式，可在分享创建时动态分配 FRP 上游端口。

## 本地启动（推荐 Make）

### 1) 安装依赖

```bash
make deps
```

### 2) 开发模式（`nonav` + `nonav-gateway` + 前端同时启动）

```bash
make dev
```

- 前端 Vite: `http://localhost:5173`
- nonav: `http://localhost:8081`
- nonav-gateway: `http://localhost:8080`

`make dev` 由 `nonav-gateway` 内嵌启动 `frps`，分享创建时由 `nonav` 动态拉起 `frpc tcp` 代理进程。

开发模式默认开启 `FRP-only` 验证：新建分享会在 `NONAV_FRP_PORT_MIN` 到 `NONAV_FRP_PORT_MAX` 的端口池中动态分配上游目标。

可通过 `NONAV_FRP_PORT_MIN` / `NONAV_FRP_PORT_MAX` 配置端口池范围，后续用于多站点动态分配。

开发模式下可直接访问 `http://localhost:8080`，Gateway 会把前端请求代理到 Vite。

若出现 `connect: connection refused 127.0.0.1:5173`，说明 Vite 未成功启动（或 5173 端口被占用），请先释放端口后重试 `make dev`。

若出现 `431 Request Header Fields Too Large`，通常是浏览器里历史分享 Cookie 累积过多，请清理 `localhost` 的 Cookie 后重试，并优先通过 `http://localhost:8080` 访问。

### 3) 一体化构建并本地部署（双二进制）

```bash
make build
make run-all
```

执行后会：

- 构建前端到 `web/dist`
- 拷贝静态文件到 `server/internal/httpserver/web-dist` 并编译进 `nonav-gateway`
- 构建 nonav 二进制到 `bin/nonav`
- 构建 Gateway 二进制到 `bin/nonav-gateway`

此时运行两个二进制即可同时提供：

- 网关仅处理 `/s/*` 与分享上下文请求，访问 `/` 返回 404
- nonav `/api/*`（gateway 反代到 nonav）
- 分享网关 `/s/*`（gateway 直接处理）

## 手动启动（不使用 Make）

### 1) 启动 nonav

```bash
cd server
go mod tidy
go run ./cmd/nonav-api
```

默认监听 `:8081`。

首次在空目录运行时会自动生成 `internal.env` 模板文件（仅在文件不存在时生成），生成后程序会提示并退出，等待你修改配置后再次启动。

### 2) 启动 Gateway

```bash
cd server
go run ./cmd/nonav-gateway
```

默认监听 `:8080`。

首次在空目录运行时会自动生成 `gateway.env` 模板文件（仅在文件不存在时生成），生成后程序会提示并退出，等待你修改配置后再次启动。

可选环境变量：

- `NONAV_API_LISTEN_ADDR`：默认 `:8081`
- `NONAV_GATEWAY_LISTEN_ADDR`：默认 `:8080`
- `NONAV_API_BASE_URL`：默认 `http://127.0.0.1:8081`（分离部署建议 `http://127.0.0.1:18081`）
- `NONAV_DB_PATH`：默认 `./data/nonav.db`
- `NONAV_WEB_DIST_DIR`：默认 `./web-dist`（仅作文件系统回退，可不设置）
- `NONAV_PUBLIC_BASE_URL`：默认 `http://localhost:8080`
- `NONAV_SHARE_SUBDOMAIN_ENABLED`：是否启用泛子域分享模式（默认 `false`）
- `NONAV_SHARE_SUBDOMAIN_BASE`：泛子域根域名（如 `node.sourcedream.cn`）
- `NONAV_LOG_LEVEL`：日志级别（默认 `info`）
- `NONAV_LOG_ROUTE_TRACE`：是否打印网关路由追踪日志（默认 `true`）
- `NONAV_CORS_ORIGIN`：默认 `http://localhost:8080`
- `NONAV_DEFAULT_SHARE_TTL_HOURS`：默认 `24`
- `NONAV_FORCE_FRP`：是否强制分享仅走 FRP 上游（默认 `false`）
- `NONAV_FRP_UPSTREAM_URL`：FRP 上游地址（默认 `http://127.0.0.1:13000`）
- `NONAV_FRP_PORT_MIN`：FRP 端口池最小值（默认 `13000`）
- `NONAV_FRP_PORT_MAX`：FRP 端口池最大值（默认 `13020`）
- `NONAV_FRP_CLIENT_BIN`：frpc 可执行文件路径（默认 `../frp/frpc`）
- `NONAV_FRP_SERVER_BIN`：frps 可执行文件路径（仅 `NONAV_EMBED_FRPS=true` 时使用）
- `NONAV_EMBED_FRPS`：gateway 是否内嵌启动 frps（默认 `false`，推荐生产设为 `true`）
- `NONAV_FRP_SERVER_BIND_ADDR`：frps 监听地址（仅 `NONAV_EMBED_FRPS=true` 时使用）
- `NONAV_FRP_SERVER_ADDR`：frps 地址（默认 `127.0.0.1`）
- `NONAV_FRP_SERVER_PORT`：frps 端口（默认 `7000`）
- `NONAV_FRP_AUTH_TOKEN`：frp token（默认 `nonav-local-dev`）
- `NONAV_FRP_RECOVER_ON_START`：nonav 启动时是否自动恢复历史分享代理（默认 `true`）

分享模式说明：

- `path_ctx`（默认）：链接为 `/s/<token>`，访问后网关会跳转到 `/x/<ctx-id>/...` 隔离上下文。
- `subdomain`：链接为 `https://<slug>.<base-domain>/`，需开启 `NONAV_SHARE_SUBDOMAIN_ENABLED=true` 并配置 `NONAV_SHARE_SUBDOMAIN_BASE`。若不填写 `slug`，系统会自动生成 10 位随机前缀。

本地调试建议使用 `lvh.me`（自动解析到 `127.0.0.1`）：例如设置 `NONAV_PUBLIC_BASE_URL=http://lvh.me:8080`、`NONAV_SHARE_SUBDOMAIN_BASE=lvh.me`，即可通过 `http://<slug>.lvh.me:8080` 访问分享。

排障提示：

- 网关每个请求会返回 `X-Nonav-Gateway-Rev`、`X-Nonav-Req-Id`。
- 子域链路会附带 `X-Nonav-Route`、`X-Nonav-Reason`、`X-Nonav-Subdomain-Slug`，可直接在浏览器 Network 或 curl 响应头里定位失败原因。

### 3) 启动前端

```bash
cd web
npm install
npm run dev
```

默认地址：`http://localhost:5173`

## 生产部署（内网 + 公网）

推荐拓扑：

- 内网服务器：`nonav` + 业务站点
- 公网服务器：`nonav-gateway`（嵌入 frps）
- 网关前面可选 Nginx 做 TLS

### 1) 打包

```bash
make build
```

产物：

- `bin/nonav`（部署到内网机）
- `bin/nonav-gateway`（部署到公网机）
- `server/web-dist`（部署到公网机，给 gateway 托管前端）

### 2) 准备文件

- 把 `bin/nonav`、`frp/frpc` 传到内网机 `/opt/nonav`
- 把 `bin/nonav-gateway`、`frp/frps` 传到公网机 `/opt/nonav`
- 复制环境变量模板：
  - 内网机：`deploy/env/internal.env.example` -> `/etc/nonav/internal.env`
  - 公网机：`deploy/env/gateway.env.example` -> `/etc/nonav/gateway.env`

### 3) systemd 启动

- 内网机：`deploy/systemd/nonav-api.service`（对应 `nonav`）
- 公网机：`deploy/systemd/nonav-gateway.service`

```bash
sudo cp deploy/systemd/nonav-api.service /etc/systemd/system/
sudo cp deploy/systemd/nonav-gateway.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now nonav-api
sudo systemctl enable --now nonav-gateway
```

### 4) 关键检查

- 公网机 `7000/tcp`（frps）需要放行
- 公网机 `8080` 仅给 Nginx 回源（推荐）
- 内网机到公网机 `7000` 必须可达
- 公网机 gateway 到 `127.0.0.1:18081`（nonav frp 隧道）应可达
- `NONAV_FRP_AUTH_TOKEN` 内外网保持一致

### 5) 验证

- 内网机：`systemctl status nonav-api`
- 公网机：`systemctl status nonav-gateway`
- 分享创建后：公网机应出现对应 `13xxx` 监听（frp 代理端口）
- 停止分享后：对应监听端口应消失

## Nginx 反向代理示例

```nginx
server {
    listen 443 ssl http2;
    server_name node.sourcedream.cn;

    # ssl_certificate /path/fullchain.pem;
    # ssl_certificate_key /path/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

如果前后端分离部署：

- `/api` 与 `/s` 反代到 Go 服务
- `/` 反代到前端静态站点

## 后续计划

1. 提升 FRP 动态编排稳定性（自动恢复、故障回滚、监控）
2. 新增流量统计聚合图表
3. Natter 打洞分享作为实验功能
