# 架构与部署矩阵

Sub2API 是**模块化单体**：一个 Go 二进制、一份 Vue 控制台、PostgreSQL + Redis。不拆微服务。重构不改变对外协议 I/O、调度与计费语义。

## 后端分层

依赖方向（CI：`backend/internal/archtest` + golangci depguard）：

`domain` → `ports` → `app` / `service` → `handler` / `repository`

| 包 | 职责 |
|----|------|
| `internal/domain` | 实体、常量、错误。不 import 任何 `internal` 包。 |
| `internal/ports` | Repository / cache / 上游客户端接口。只依赖 `domain`。 |
| `internal/app` | 已抽出的应用模块：`gateway/pipeline`、`scheduling`、`billing`、`lifecycle`。禁止 import `service` / `repository` / `handler`。 |
| `internal/service` | 尚未拆完的业务与 `app` 端口适配器。 |
| `internal/repository` | ports 的 Ent / HTTP / OAuth 实现。禁止 import `service`。 |
| `internal/handler` + `internal/server` | HTTP 接入、路由、中间件。 |
| `internal/config` | Viper 加载。YAML/env **键名不变**。模块访问器在 `modules.go`。 |
| `internal/web` | 可选 `-tags embed` 静态文件服务。不再做 HTML 配置注入。 |

禁止新增包级 `Set*` 装配旁路。

## 网关管道

入站转发编排为：

Parse → Admission → Select → Rewrite → Transport → Relay → Settle

协议差异留在各平台显式适配器（Claude、OpenAI/Codex、Gemini、Antigravity、Grok、Kimi）。共享阶段负责调度、准入、中继与结算。入口短路径（透传、Bedrock、Grok/Kimi raw CC、Codex CLI-only 等）仍在管道外。

生产选号入口是 `AccountSelector`（`app/scheduling`）。两套调度器仍在适配器后；等待计划 / 槽位 / lookup 已共享。

`app/billing.Settlement` 是用量结算门面，扣费核心仍复用原实现。

装配由 Wire + `app/lifecycle.Registry` 负责；`cmd/server/cleanup.go` 由注册表生成。

## 前后端契约

| 面 | 契约 |
|----|------|
| 控制面（`/api/v1/auth`、`/user`、`/keys` …） | `openapi/control-plane.yaml` + `frontend/src/api/generated/*` |
| 网关（`/v1/*`、`/v1beta/*`、Codex、Antigravity …） | 保持上游协议语义，不走 codegen |
| 公开运行时配置 | 规范端点 `GET /api/v1/public/config`；`GET /api/v1/settings/public` 为兼容别名 |
| 版本偏移 | `GET /api/v1/public/version` → `{ version, min_frontend_version }` |

SPA 启动时拉取公开配置。`window.__APP_CONFIG__` 只作首屏缓存（Vite 开发注入）。embed 的 `index.html` 是纯静态壳。

前端模块在 `frontend/src/features/`（API 与部分 store）。基础控件在 `frontend/src/ui/`。账号平台字段走 `features/admin-accounts/form/registry.ts`。设置页路由为 `/admin/settings/:tab?`，多数分区仍在 `SettingsView.vue`。

构建产物仍输出到 `backend/internal/web/dist`，供 `-tags embed` 打进同一份二进制。Release 另发 `sub2api-frontend-<version>.zip`。

## 部署矩阵

默认一键安装、单容器 compose、后台自更新、setup 向导**零变化**。分离拓扑为可选。

| 拓扑 | 方式 | 前端 | 公开配置 | 升级 | 说明 |
|------|------|------|----------|------|------|
| **二进制 + embed**（默认） | `deploy/install.sh` + systemd | Go 进程内静态托管 | `/api/v1/public/config` | 后台自更新 / 换二进制 | 与后端同版本。向导不变。 |
| **单容器 compose**（默认） | `docker-compose.yml` 或 `docker-compose.local.yml` | 镜像内嵌 | 同上 | 拉镜像 / 重建 | Auto-setup。推荐 `docker-deploy.sh` + `docker-compose.local.yml`。 |
| **分离 compose**（可选） | overlay `docker-compose.separated.yml` | nginx 托管解压后的前端 zip | 同上 | **同时**升级 API 镜像与前端 zip | 宿主机端口在 nginx，API 只内网。见 `deploy/nginx/README.md`。 |
| **分域**（可选） | 任意静态站 + API 进程 | 同一份 zip | 同上 | 可独立升级 | 设置 `CORS_ALLOWED_ORIGINS`（或 `cors.allowed_origins`）为控制台源。 |

反向代理要求（官方配置：`deploy/nginx/sub2api.conf`）：

- `underscores_in_headers on`（Codex / OpenAI 的 `session_id`、`x-stainless-*`）
- WebSocket `Upgrade` / `Connection`
- SSE / 流式网关关闭 `proxy_buffering`

版本偏移：控制台构建号（`__APP_VERSION__`，来自 `backend/cmd/server/VERSION`）低于 `min_frontend_version` 时显示可关闭横幅。embed 天然同版本。版本端点缺失不阻断启动。

## 相关文件

- OpenSpec 变更：`openspec/changes/refactor-architecture/`
- 控制面规格：`openapi/README.md`
- 分离代理：`deploy/nginx/README.md`
- Compose overlay：`deploy/docker-compose.separated.yml`
