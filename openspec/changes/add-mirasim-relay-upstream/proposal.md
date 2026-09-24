# Change: Add Mirasim Relay Upstream

## Why

Mirasim (`relay.mirasim.ai`) 是一个公网第三方 Claude relay，已实测在线。它的
上游协议不是裸 `x-api-key`：模型请求必须携带 `Authorization: Bearer <ticket>`、
`x-mirasim-client`、以及把 Ed25519 签名头密封后的 `x-mirasim-enc`，签名原文里包含
请求路径与 body 哈希（`mrs-sig-v2` / `mrs-seal-v1`）。现有 `anthropic + apikey`
账号只能发静态 API Key，因此无法接入该 relay。本变更把 mirasim 作为一等上游接进
现有 Anthropic 转发链路，使管理员可以用一份 refresh token 获得完整的调度、重试、
计费与可观测能力。

## What Changes

- 新增内建 Mirasim relay 协议实现（`mrs-sig-v2` 签名、`mrs-seal-v1` 密封、
  Ed25519 设备身份与 PKCS8 PEM 读写），与参考实现逐字节等价。
- 复用现有 `anthropic` 平台 + `apikey` 账号类型承载 Mirasim 账号，不新增平台、
  不新增账号类型、不改数据库结构：`credentials.mirasim_refresh_token` 存在即视为
  Mirasim relay 账号，`credentials.base_url` 指向 relay 地址。
- 新增账号级凭据链：refresh token → access token → device ticket，带提前刷新
  （access 120s / ticket 60s）与 refresh token 轮换落库。
- 在四个上游请求构造器（messages / as_chat / as_responses / anthropic passthrough /
  count_tokens）的最终阶段注入签名头，使用构造器**最终返回的 body** 与**最终 path**。
- Mirasim 账号 401 采用「失效本地 ticket + 临时不可调度冷却」而非
  `SetError` 永久禁用，冷却结束后自动重取 ticket 自愈。
- 账号测试（admin 账号连接测试）对 Mirasim 账号同样签发 ticket，使测试可用。
- 前端账号表单 Anthropic 分支新增 Mirasim relay 开关与凭据/覆盖字段。
- 新增管理端 **Mirasim 授权登录**（对标既有 `openai_oauth_service.go` /
  `kimi_oauth_service.go`）：管理员可直接走浏览器授权、邮箱验证码或直接粘贴 token
  取回 refresh token，无需手工从桌面客户端导出；该流程只生产凭据并回填账号表单，
  不直接建号，且**不调用 `/auth/refresh`**（避免轮换掉管理员本地客户端的登录）。

## Impact

- Affected specs: `mirasim-relay-upstream`（新增能力）
- Affected code:
  - `backend/internal/service/mirasim_protocol.go`、`mirasim_relay.go`、
    `mirasim_relay_runtime.go`（新增）
  - `backend/internal/service/gateway_upstream_request.go`、
    `gateway_anthropic_passthrough.go`、`gateway_count_tokens.go`（签名注入点）
  - `backend/internal/service/gateway_service.go`（`GetAccessToken` 分支）
  - `backend/internal/service/ratelimit_service.go`（401 / 403 语义）
  - `backend/internal/service/account_test_service.go`（账号连接测试）
  - `backend/internal/service/upstream_models.go`、
    `upstream_billing_probe.go`（上游模型同步 / 计费探针排除）
  - `backend/internal/service/account_credentials_redact.go`（敏感键清单新增 2 项；
    合并逻辑与 `dto` 层 `has_*` 状态标记复用既有实现，无需改动）
  - `backend/internal/service/mirasim_oauth_service.go`（新增，授权登录/取凭据）
  - `backend/internal/handler/admin/mirasim_oauth_handler.go`、
    `backend/internal/server/routes/admin.go`（`/admin/mirasim/oauth/*` 路由）、
    `backend/internal/handler/wire.go` / `handler.go` / `service/wire.go` / `cmd/server/wire_gen.go`（注入）
  - `frontend/src/api/admin/mirasim.ts`（新增 API 客户端）
  - `frontend/src/features/admin-accounts/form/credentialsBuilder.ts`（表单写入规则）
  - `frontend/src/components/account/MirasimOAuthOnboarding.vue`（新增）
  - `frontend/src/components/account/{Create,Edit}AccountModal.vue`（开关与字段 UI + 授权登录接入）
  - `frontend/src/i18n/locales/{zh,en}/admin/accounts.ts`（`mirasimRelay.*`、`mirasimOAuth.*`）
  - `docs/MIRASIM_RELAY_UPSTREAM.md`（新增，运维接入与协议说明）、
    `docs/ARCHITECTURE_CN.md`（一句话说明 relay 不新增适配器）
  - 新增单测：`backend/internal/service/mirasim_{protocol,relay,oauth_service}_test.go`、
    `frontend/.../__tests__/{credentialsBuilder,EditAccountModal,MirasimOAuthOnboarding}.spec.ts`
- 兼容性：非 Mirasim 账号行为不变；`tokenType` 仍返回 `"apikey"`，因此
  `tokenType != "apikey"` 的既有守卫与计费头逻辑不受影响。
- 无数据库迁移；若启用 `security.url_allowlist`，需把 `relay.mirasim.ai` 加入
  `upstream_hosts`（`auth.mirasim.ai` 走 `http_upstream` 的 host 校验，公网 DNS 即可）。
- 授权登录端点复用 `HTTPUpstream`（含账号代理与 host 校验），不新增网络出口；
  响应体包含 refresh token，与「管理员手工粘贴」等价，仅暴露在已认证的管理端接口上。
