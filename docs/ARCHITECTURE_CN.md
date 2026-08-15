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

## 持久化 usage 结算（上线门禁中）

终态 usage 可以通过加法 PostgreSQL 表 `usage_settlement_attempts` 持久交接。这条路径不替换两套平台费用计算器：它保存现有计算器产生的标量 billing command 与 usage-log 快照，再在同一事务内提交账务幂等、余额/配额变更、usage log 和终态 `settled`。

`started` → `ready` → `processing` → `settled`

其他终态为 `failed_retryable`、`failed_terminal` 和 `reconciliation_required`。processing 租约过期后可由其他实例接管；Redis 和进程内队列只是唤醒加速器。事务后的缓存刷新与通知不属于账务原子性边界，可从 PostgreSQL 真相源重建。

启动 setting `usage_settlement_durability_mode` 有三个值：

| 模式 | 写 journal | Durable claim | 扣费权威 |
|------|-------------|---------------|----------|
| `off`（默认） | 否 | 否 | 现有非持久路径 |
| `shadow` | 是 | 否 | 仍由现有路径扣费，journal 只用于对比 |
| `authoritative` | 是 | 是 | PostgreSQL durable worker |

`shadow` 或 `authoritative` 还必须配置第二个数据库 setting `usage_settlement_durability_group_ids`。它接受正整数 group ID 的 JSON 数组；显式 `*` 只供隔离环境的整实例测试。同一 scope 同时约束 handler handoff、journal 写入、是否跳过旧路径和 PostgreSQL claim。未选中 group 不写 journal，并保持现有 worker-pool 结算路径。active scope 缺失、读取失败、空、格式非法或含非正整数都 fail-closed 为 `off`。

每条 attempt 还会在创建时固化不可变的 claim 资格。shadow 与迁移前历史行均为 `authoritative=false`，永久只作差分证据；同一 group 后续切到 authoritative 不会升级或 claim 这些行。只有在该 group 已处于 authoritative 时新建的 `authoritative=true` attempt 才可被 worker claim。group scope 是第二层过滤，不能替代这条逐事件门禁。

两个 setting 都只在实例启动时读取一次，不是 YAML/env 键，不改变默认部署。生产应从一个低风险测试 group 开始，禁止用 `*` 冒充生产 canary。

默认对账窗口为 24 小时。terminal 前的精确恢复是条件式能力：仅官方 OpenAI API-key 和 xAI API-key 的 `store=true` stored Responses 可按 response ID 取回。`store=false`、OAuth/Codex、Chat Completions、Anthropic、Gemini、Antigravity、Kimi 及其他不支持路径进入 `reconciliation_required`，绝不猜测 partial/zero usage。journal payload 只保存版本化计费/审计标量与 hash，不保存提示词、响应正文或凭据。

当前没有 Prometheus/OTel 或公网 settlement 端点。现有结构化日志每分钟输出积压数、最老积压时间、重试/终止失败累计、待对账累计和 provider 恢复累计。运维流程见 `docs/USAGE_SETTLEMENT_OPERATIONS_CN.md`。

## 前后端契约

| 面 | 契约 |
|----|------|
| 控制面（`/api/v1/auth`、`/user`、`/keys` …） | `openapi/control-plane.yaml` + `frontend/src/api/generated/*` |
| 网关（`/v1/*`、`/v1beta/*`、Codex、Antigravity …） | 保持上游协议语义，不走 codegen |
| 公开运行时配置 | 规范端点 `GET /api/v1/public/config`；`GET /api/v1/settings/public` 为兼容别名 |
| 版本偏移 | API：`GET /api/v1/public/version` → `{ version, min_frontend_version }`；前端产物：`sub2api-build.json` → `{ frontend_version, min_backend_version }` |

SPA 启动时拉取公开配置。`window.__APP_CONFIG__` 只作首屏缓存（Vite 开发注入）。embed 的 `index.html` 是纯静态壳。

前端模块在 `frontend/src/features/`（API 与部分 store）。基础控件在 `frontend/src/ui/`。账号平台字段走 `features/admin-accounts/form/registry.ts`。设置页路由为 `/admin/settings/:tab?`，多数分区仍在 `SettingsView.vue`。

构建产物仍输出到 `backend/internal/web/dist`，供兼容用 `-tags embed` 构建复用。前后端版本分别来自 `frontend/VERSION` 与 `backend/cmd/server/VERSION`。Release 分别发布 API 镜像、独立前端镜像/zip 和 embedded 兼容镜像。

## 部署矩阵

生产默认采用同域前后端分离：Nginx 占用公网端口，API 只在内部网络监听。embedded 交付仍供既有安装和回滚使用，但不再是生产默认。

| 拓扑 | 方式 | 前端 | 公开配置 | 升级 | 说明 |
|------|------|------|----------|------|------|
| **分离 compose**（默认） | `docker-compose.yml` 或 `docker-compose.local.yml` | `sub2api-frontend` nginx 镜像 | `/api/v1/public/config` | API 与前端可独立升级 | 公网端口在 nginx，API 只在 compose 网络。 |
| **分离宿主机发布**（生产） | `component_deploy.py` + nginx/systemd/container hook | 不可变目录 + 原子 symlink | 同上 | 显式 `frontend`、`backend` 或 `both` | 每端各保留 current/rollback。 |
| **二进制 + embed**（兼容） | `docker-compose.embedded.yml` 或 `-tags embed` | Go 进程内静态托管 | 同上 | 成对替换 | 用于既有安装和迁移回滚。 |
| **分域**（可选） | 任意静态站 + API 进程 | 同一份 zip | 同上 | 可独立升级 | 设置 `CORS_ALLOWED_ORIGINS`（或 `cors.allowed_origins`）为控制台源。 |

反向代理要求（官方配置：`deploy/nginx/sub2api.conf`）：

- `underscores_in_headers on`（Codex / OpenAI 的 `session_id`、`x-stainless-*`）
- WebSocket `Upgrade` / `Connection`
- SSE / 流式网关关闭 `proxy_buffering`

版本偏移双向检查：SPA 运行时会提示“前端低于 API 门槛”或“API 低于前端门槛”；部署预检对缺失、畸形或不兼容元数据 fail-closed。跨端契约变更必须使用 `both`。

## 相关文件

- OpenSpec 变更：`openspec/changes/refactor-architecture/`
- 控制面规格：`openapi/README.md`
- 分离代理：`deploy/nginx/README.md`
- 默认 Compose：`deploy/docker-compose.yml`
- embedded 兼容 overlay：`deploy/docker-compose.embedded.yml`
- 组件发布入口：`deploy/component_deploy.py`
- 持久结算运维手册：`docs/USAGE_SETTLEMENT_OPERATIONS_CN.md`
