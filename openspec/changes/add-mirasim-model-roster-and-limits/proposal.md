# Change: Add Mirasim Model Roster And Quota Windows

## Why

`add-mirasim-relay-upstream` 只接通了**模型流量**（`/v1/messages` 及三种变体）：账号能发请求、
能被调度、能计费，但管理端拿不到 mirasim 的**模型清单**与**额度窗口**。实测表现：

- 上游模型同步对 mirasim 账号固定返回 `unsupported`（`upstream_models.go` 的
  `buildAnthropicUpstreamModelsRequest` 里写死 “Mirasim relay upstream has no model list
  endpoint”），模型白名单只能手工填。
- 账号用量（`GetUsage`）对 mirasim 账号落到 `CanGetUsage()` 兜底分支：Claude OAuth 的
  `apiUsageCache` 依赖 `api.anthropic.com/api/oauth/usage`，mirasim 账号拿不到，于是「额度窗口」
  永远为空。

而参考实现（客户端 `server.cjs`）证明 relay **确实提供**这两类只读元数据接口：

- `GET /v1/model-roster` — 返回 `{version, agents, models?, cli?}`，每条 agent 含
  `id`/`label`/`contextWindow`/`default`，客户端按 `znr` 校验归一化、按 JWT `sub` 缓存 10 分钟。
- `GET /v1/limits`（带 `x-mirasim-probe: usage`）— 返回 `{windows:[{name,budget,used,reset_at,
  model_scoped?}], paid, degraded?}`，客户端按 `pBn` 归一化成 `allowed`/`warning`/`limit_reached`
  三种状态。

两者都**只发明文签名头、不密封**（密封只发生在被代理的模型流量路径），因此可以在
sub2api 内用已实现的 `mrs-sig-v2` 原语直接调用。本变更把这两条元数据链路接进现有的
「上游模型同步」与「账号用量」两个面，前端复用既有展示组件，不新增管理页面。

## What Changes

- 新增 mirasim 只读元数据客户端：`GET /v1/model-roster`、`GET /v1/limits`
  （`x-mirasim-probe: usage`），空 body、明文签名头、**不密封**、独立超时、
  凭据优先复用缓存 device ticket（避免触发 refresh token 轮换）。
- 新增 roster 校验/归一化（对齐参考实现）：`agents[key]` 数组、条目需
  `id` 非空 + `contextWindow > 0`、`id`/`label` trim+lowercase、`default` 首个胜出；
  同时提取 `models`/`cli` 两段可选清单。
- 上游模型同步（`POST /admin/accounts/:id/models/sync-upstream` 与其 preview）对 mirasim
  账号改走 roster，**取代**上一变更 `tasks.md` 4.4 的 “unsupported” 行为；上游失败映射到既有
  `UpstreamModelSyncError`（`upstream` / `configuration`），不再返回 `unsupported`。
- 账号用量（`GetUsage`）对 mirasim 账号改走 `/v1/limits`：`5h` → `five_hour`、`7d` →
  `seven_day`，其余窗口进新增的 `mirasim_windows` 列表；透出 `paid` / `degraded`；
  失败按 HTTP 200 + `error` 降级（不把整个用量接口打成 500）。
- 新增独立缓存：roster 10 分钟/账号（对齐客户端），limits 30 秒并支持 `force` 绕过。
- 前端：`UsageProgress` 之外的 `mirasim_windows` / `paid` / `degraded` 展示（zh/en i18n）；
  模型同步按钮**无需改动**（后端分支打开即可用）。
- 文档：`docs/MIRASIM_RELAY_UPSTREAM.md` 增加「元数据接口（模型清单 / 额度窗口）」章节。

## Supersedes

- `openspec/changes/add-mirasim-relay-upstream/tasks.md` 4.4（mirasim 模型同步固定
  `unsupported`）被本变更的行为取代；该 change 的 delta spec 中没有与之对应的 requirement，
  因此**无 spec 冲突**。两个 change 归档顺序不限（两者都是对 `mirasim-relay-upstream`
  能力追加互不相交的 requirement）。

## Impact

- Affected specs: `mirasim-relay-upstream`（追加 Requirements）
- Affected code:
  - `backend/internal/service/mirasim_relay.go`（新增 roster / limits 拉取与缓存）
  - `backend/internal/service/mirasim_roster.go`（新增：roster 解析/归一化/agent 选择）
  - `backend/internal/service/mirasim_limits.go`（新增：`/v1/limits` 归一化与窗口映射）
  - `backend/internal/service/upstream_models.go`（mirasim 分支改走 roster）
  - `backend/internal/service/account_usage_service.go`（`GetUsage` 增加 mirasim 分支、
    `UsageInfo` 增加 `mirasim_windows` / `mirasim_paid` / `mirasim_degraded`）
  - `backend/internal/service/wire.go`（把 mirasim 元数据查询器注入 `AccountUsageService`）
  - `frontend/src/types/index.ts`、`frontend/src/utils/accountUsagePresentation.ts`、
    `frontend/src/i18n/locales/{zh,en}/admin/*.ts`
  - `docs/MIRASIM_RELAY_UPSTREAM.md`
  - 新增单测：`backend/internal/service/mirasim_{roster,limits,metadata}_test.go`、
    前端 `utils/__tests__/accountUsagePresentation.spec.ts` 增补

## Non-Goals

- `GET /usage/calls`（调用明细回查，必须带 `ids=`）：不是配额汇总，**明确不做**。
- 把 mirasim 窗口写入 `QuotaWindowLedger`（被动观测账本）：账本按 `platform` 分派字段命名空间
  （`platformQuotaWindowSpecs`），mirasim 账号平台为 `anthropic`，直接写会与既有
  `passive_usage_7d_*` 语义冲突；另开变更处理。
- 依据上限窗口自动置 `rate_limit_reset_at` / 停调度：relay 的 `model_scoped` 窗口与
  agent 粒度语义尚未测量清楚，本轮只做**展示**，不做调度决策。
- `/v1/models` 路径：relay 虽有路由，但客户端模型清单来自 roster，本变更不使用。
