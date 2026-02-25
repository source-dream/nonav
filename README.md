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
- `server/cmd/nonav-api`：Go 控制面 API（二进制 1）
- `server/cmd/nonav-gateway`：Go 分享网关 + 静态站点托管（二进制 2）
- `nginx`（可选）：外层 TLS 与反向代理

请求链路：

1. 用户访问 `node.sourcedream.cn/s/<token>`
2. Nginx 反代到 Go 网关（`nonav-gateway`）
3. 网关将 `/api/*` 转发到 `nonav-api`
4. 网关对 `/s/*` 校验分享状态和密码会话并反代到目标站点

> 下一阶段可接入 frpc/frps，把目标 URL 替换为动态隧道地址。

## 本地启动（推荐 Make）

### 1) 安装依赖

```bash
make deps
```

### 2) 开发模式（API + 网关 + 前端 同时启动）

```bash
make dev
```

- 前端 Vite: `http://localhost:5173`
- API: `http://localhost:8081`
- Gateway: `http://localhost:8080`
- FRP Server: `127.0.0.1:7000`
- FRP 映射（home:3000）: `127.0.0.1:13000`

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
- 拷贝静态文件到 `server/web-dist`
- 构建 API 二进制到 `bin/nonav-api`
- 构建 Gateway 二进制到 `bin/nonav-gateway`

此时运行两个二进制即可同时提供：

- 前端页面 `/`（gateway 托管）
- API `/api/*`（gateway 反代到 api）
- 分享网关 `/s/*`（gateway 直接处理）

## 手动启动（不使用 Make）

### 1) 启动 API

```bash
cd server
go mod tidy
go run ./cmd/nonav-api
```

默认监听 `:8081`。

### 2) 启动 Gateway

```bash
cd server
go run ./cmd/nonav-gateway
```

默认监听 `:8080`。

可选环境变量：

- `NONAV_API_LISTEN_ADDR`：默认 `:8081`
- `NONAV_GATEWAY_LISTEN_ADDR`：默认 `:8080`
- `NONAV_API_BASE_URL`：默认 `http://127.0.0.1:8081`
- `NONAV_DB_PATH`：默认 `./data/nonav.db`
- `NONAV_WEB_DIST_DIR`：默认 `./web-dist`
- `NONAV_PUBLIC_BASE_URL`：默认 `http://localhost:8080`
- `NONAV_CORS_ORIGIN`：默认 `http://localhost:8080`
- `NONAV_DEFAULT_SHARE_TTL_HOURS`：默认 `24`

### 3) 启动前端

```bash
cd web
npm install
npm run dev
```

默认地址：`http://localhost:5173`

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

1. 接入 frpc/frps 进程编排（创建分享时自动建隧道）
2. 新增流量统计聚合图表
3. Natter 打洞分享作为实验功能
