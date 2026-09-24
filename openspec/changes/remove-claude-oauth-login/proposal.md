# Change: 完整移除 Claude OAuth 登录（Anthropic 上游 OAuth / Setup Token 授权接入）

## Why

生产 Anthropic 供应已是 100% APIKey：三个账号（`deepseek`、`kimi`、`国模`）全部 `type=apikey`，`extra` 为空，`session_window_*` 为 `NULL`，Anthropic 无任何 OAuth / Setup Token 账号。上一个 change `remove-anthropic-subscription-quota-control` 已删掉只对 Anthropic OAuth/SetupToken 生效的配额控制面，留下的 Claude OAuth 授权链路（浏览器授权码、Setup Token、cookie 授权、token 刷新、Redis token 缓存、CRS 导入分支、后台刷新协程）只服务于生产不存在的账号类型。

这条链路仍完整保留着 Anthropic 授权端点、凭据协议与刷新调度面：任何人只要拿到一个 Anthropic OAuth/Setup Token 凭据就能通过控制面重新建立一种本仓库不再维护的供给方式，同时它持续占用 handler、service、repository、wire 注入、后台协程与前端授权向导。本 change 按完整供应退役处理：控制面 fail-closed 拒绝新增与再授权，删除全部生产执行路径，历史行继续保留。

## What Changes

- **BREAKING**：控制面不再接受 Anthropic 平台 `type=oauth` / `type=setup-token` 的新增、类型转换、重新授权（apply/re-auth）、token 刷新、导入或批量启用；拒绝发生在 service/domain 层之下，HTTP 与前端不可绕过。
- 删除 Claude OAuth 授权接口及其实现：`/admin/accounts/generate-auth-url`、`/generate-setup-token-url`、`/exchange-code`、`/exchange-setup-token-code`、`/cookie-auth`、`/setup-token-cookie-auth`（路由、`admin.OAuthHandler`、`OAuthService`、`repository.ClaudeOAuthClient`、`ports.ClaudeOAuthClient`、`domain.ClaudeOAuthTokenResponse`、wire 注入）。
- 删除 Anthropic token 供给与刷新面：`ClaudeTokenProvider`、`ClaudeTokenRefresher`、`ClaudeProviderRefreshPolicy`、`ClaudeTokenCacheKey`、`TokenRefreshService` 中的 Anthropic 分支与 token 缓存失效的 `PlatformAnthropic` 分支，以及 `cleanup.go` 中的 `oauth.Stop()` 会话清理协程。
- 删除 CRS 同步中的 Claude OAuth/Setup Token 导入分支、`crsClaudeAccount` DTO 与预览分类项，以及只服务于该分支的 `CRSSyncService.oauthService` 字段/构造参数（`refreshOAuthToken` 的其它平台分支保留）；`claudeConsoleAccounts`（Anthropic APIKey）与其他平台导入保持不变。
- 网关、账号测试、上游模型同步的 Anthropic 分支收敛为「仅 APIKey」：删除 `getOAuthToken` 的 `ClaudeTokenProvider` 快路径与 `upstream_models.go` 的 `account.IsOAuth()` 分支；`AccountTypeServiceAccount` 的既有 retired 语义保持不变。
- 删除 `pkg/oauth` 中仅 Claude 使用的常量（`ClientID`/`AuthorizeURL`/`TokenURL`/`RedirectURI`/scope）、`SessionStore`、`OAuthSession`、`TokenResponse`、`BuildAuthorizationURL`；共享的 `GenerateState` / `GenerateCodeVerifier` / `GenerateCodeChallenge`（用户登录 OAuth 使用）保持不变。
- 前端移除 Anthropic 的 `oauth` / `setup-token` 选项与 Claude 授权向导：`useAccountOAuth`、Anthropic cookie/Setup Token 入口、`credentialsBuilder` 与 registry 中的 Anthropic oauth 类别、类型与 i18n 文案；共享的 `OAuthAuthorizationFlow` 组件保留给 OpenAI/Gemini/Antigravity/Grok。
- 存量 Anthropic `type=oauth|setup-token` 行不删除、不迁移、不改写凭据；本 change 只做 fail-closed（不可调度、不可刷新、不可再授权），保留可识别与可删除。无 schema 变更。

## Impact

- Affected specs:
  - `provider-supply-lifecycle`（新增「Claude OAuth/SetupToken 供应无生产执行路径」与「退役保留历史记录」两条 requirement）
- Affected code:
  - `backend/internal/pkg/oauth/oauth.go`
  - `backend/internal/domain/{account.go,oauth_pkg_aliases.go}`、`backend/internal/ports/oauth_client_port.go`
  - `backend/internal/repository/{claude_oauth_service.go,wire.go}`
  - `backend/internal/service/{oauth_service.go,claude_token_provider.go,token_refresher.go,refresh_policy.go,token_cache_key.go,token_cache_invalidator.go,token_refresh_service.go,crs_sync_service.go,gateway_service.go,upstream_models.go,account_test_service.go,admin_account.go,account_service.go,wire.go}`
  - `backend/internal/handler/admin/account_handler.go`、`backend/internal/handler/{handler.go,wire.go}`、`backend/internal/handler/routes/admin.go`
  - `backend/cmd/server/{wire_gen.go,cleanup.go}`
  - `frontend/src/composables/useAccountOAuth.ts`、`frontend/src/components/account/{CreateAccountModal,EditAccountModal,OAuthAuthorizationFlow}.vue`、`frontend/src/components/admin/account/ReAuthAccountModal.vue`、`frontend/src/features/admin-accounts/form/registry.ts`、账号类型/API/i18n
- Preserved behavior:
  - Anthropic APIKey 供给、模型路由、TLS 指纹、cache-TTL、自定义 base URL、warmup 拦截与请求体改写不变。
  - OpenAI / Antigravity / Gemini / Grok / Kimi / Mirasim 的上游 OAuth（含设备码 / setup-token / cookie 各自实现的授权）与后台刷新不变。
  - sub2api 用户登录 OAuth（GitHub / Google / LinuxDo / WeChat / DingTalk / OIDC）不变。
  - usage、结算、账单、审计与历史 Anthropic OAuth 行不删除。

## Non-Goals

- 不退役 Anthropic APIKey 供应，也不改 Anthropic 协议转发/计费语义。
- 不删除历史账号、凭据密文、usage、settlement、billing 或 audit 数据，不做破坏性迁移。
- 不删除其他平台仍然使用的共享代码（`MergeCredentials`、`AccountTypeSetupToken` 常量、共享 PKCE/state 生成器、`OAuthRefreshAPI`）。
- 不合并 `retire-vertex-service-account-supply` / `remove-aws-bedrock-supply` 的剩余工作，也不改动 `kimi-oauth-accounts` 的 Anthropic Messages 转 Kimi 适配。
