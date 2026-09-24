# Tasks: 完整移除 Claude OAuth 登录

> 状态约定：`[ ]` 未完成，`[x]` 已完成并验证。实施前需提案获批。

## 1. Inventory（实施前复核）

- [x] 1.1 复核引用清单：`rg -n "OAuthService|ClaudeOAuthClient|ClaudeTokenProvider|ClaudeTokenRefresher|ClaudeProviderRefreshPolicy|ClaudeTokenCacheKey|GenerateAuthURL|GenerateSetupTokenURL|BuildClaudeAccountCredentials|useAccountOAuth|crsClaudeAccount" backend frontend`，确认无遗漏调用点（Redis/DB 无 schema 依赖）。
- [x] 1.2 确认保留边界：`ApplyOAuthCredentials`、`CleanupOAuthEmailDuplicates`、`MergeCredentials`、`OAuthRefreshAPI`、`AccountTypeSetupToken`、`IsOAuth()`、`OAuthAuthorizationFlow.vue`、`gateway_claude_oauth_body.go`、`ClaudeConsoleAccounts` 有其它平台或 APIKey 调用点，必须存活。
- [x] 1.3 生产数据复核：`SELECT id, platform, type, name FROM accounts WHERE platform='anthropic' AND type IN ('oauth','setup-token');` 预期 0 行；并确认 Anthropic 供给仍为 `type=apikey`。
- [x] 1.4 记录回滚面：确认目标环境当前 binary 与前端 release（`go version -m <binary>` → `vcs.revision`）作为回滚基线。

## 2. Backend removal — 授权接入链路

- [x] 2.1 `backend/internal/pkg/oauth/oauth.go`：删除 Claude 常量（`ClientID`/`AuthorizeURL`/`TokenURL`/`RedirectURI`/scope 常量）、`SessionTTL`、`OAuthSession`、`SessionStore`（含 `NewSessionStore`/`Stop`/`Set`/`Get`/`Delete`/`cleanup`）、`GenerateSessionID`、`BuildAuthorizationURL`、`TokenResponse`、`OrgInfo`、`AccountInfo`；保留 `GenerateRandomBytes`/`GenerateState`/`GenerateCodeVerifier`/`GenerateCodeChallenge`/`base64URLEncode` 与所需 import。
- [x] 2.2 删除 `backend/internal/handler/admin/account_handler.go` 的 OAuth 授权段：`type OAuthHandler`、`NewOAuthHandler`、`GenerateAuthURLRequest`/`GenerateAuthURL`、`GenerateSetupTokenURL`、`ExchangeCodeRequest`/`ExchangeCode`、`ExchangeSetupTokenCode`、`CookieAuth`、`SetupTokenCookieAuth`；保留 `ApplyOAuthCredentials`、`CleanupOAuthEmailDuplicates`。
- [x] 2.3 `AccountHandler` 结构体与 `NewAccountHandler` 移除 `oauthService *service.OAuthService` 参数与字段，并修复残留引用。
- [x] 2.4 删除 `backend/internal/service/oauth_service.go`（及测试）与其类型 `OAuthService`/`GenerateAuthURLResult`/`BuildClaudeAccountCredentials`。
- [x] 2.5 删除 `backend/internal/repository/claude_oauth_service.go`（及测试）与 `NewClaudeOAuthClient`；`backend/internal/repository/wire.go:128` 同步移除。
- [x] 2.6 删除 `backend/internal/ports/oauth_client_port.go` 的 `ClaudeOAuthClient` 接口，保留其它 OAuth client 端口。
- [x] 2.7 删除 `backend/internal/domain/oauth_pkg_aliases.go` 的 `type ClaudeOAuthTokenResponse = oauth.TokenResponse`（若文件因此为空则删除文件）。
- [x] 2.8 `backend/internal/handler/handler.go`、`backend/internal/handler/wire.go` 移除 `OAuth *admin.OAuthHandler` 字段、`admin.NewOAuthHandler` 注入与对应赋值；保留其它 6 个 `*OAuthHandler`。
- [x] 2.9 `backend/internal/handler/routes/admin.go` 删除 `// Claude OAuth routes` 段（`generate-auth-url`、`generate-setup-token-url`、`exchange-code`、`exchange-setup-token-code`、`cookie-auth`、`setup-token-cookie-auth`）；保留 `/import/codex-session`、`/sync/crs`、`/sync/crs/preview`、`/apply-oauth-credentials`。

## 3. Backend removal — Token 供给与刷新

- [x] 3.1 删除 `backend/internal/service/claude_token_provider.go`（`ClaudeTokenProvider`、`NewClaudeTokenProvider`、缓存/锁常量、`ClaudeTokenCache`）。
- [x] 3.2 删除 `backend/internal/service/token_refresher.go` 的 `ClaudeTokenRefresher` 与 `NewClaudeTokenRefresher`；保留 `OpenAITokenRefresher`。
- [x] 3.3 删除 `backend/internal/service/refresh_policy.go` 的 `ClaudeProviderRefreshPolicy` 及其引用。
- [x] 3.4 删除 `backend/internal/service/token_cache_key.go` 的 `ClaudeTokenCacheKey`，保留 `OpenAITokenCacheKey` 等。
- [x] 3.5 `backend/internal/service/token_cache_invalidator.go` 删除 `PlatformAnthropic` 分支（含相关测试断言）。
- [x] 3.6 `backend/internal/service/token_refresh_service.go`：`NewTokenRefreshService` 移除 `oauthService *OAuthService` 参数、字段与 Anthropic 刷新/注册分支；保留 Kimi/Grok 等注册入口。
- [x] 3.7 `backend/internal/service/gateway_service.go`：删除 `claudeTokenProvider` 字段、构造器参数与赋值；删除 `getOAuthToken` 中的 `PlatformAnthropic && AccountTypeOAuth && claudeTokenProvider != nil` 快路径（`AccountTypeServiceAccount` 的 retired 错误路径保留）；`GetAccessToken` 对 Anthropic `oauth`/`setup-token` 走 fail-closed 拒绝。
- [x] 3.8 `backend/internal/service/upstream_models.go`：删除 `accessToken == "" && claudeTokenProvider != nil` 的 Anthropic 上游模型同步分支。
- [x] 3.9 `backend/internal/service/account_test_service.go`：删除未使用的 `claudeTokenProvider` 字段与构造参数。
- [x] 3.10 `backend/internal/service/account_service.go` / `admin_account.go`：新增并接入 `domain` 层 fail-closed 校验（`rejectDeprecatedClaudeOAuthAccount(platform, accountType)`，判定 `platform==anthropic && type in (oauth,setup-token)`），覆盖单条/批量创建、更新（含类型转换）、导入与重新授权；`refreshSingleAccount` 删除 Claude 的 `oauthService.RefreshAccountToken` 分支，改为显式拒绝。

## 4. Backend removal — CRS 导入与网关收敛

- [x] 4.1 `backend/internal/service/crs_sync_service.go`：删除 `crsExportResponse.ClaudeAccounts`（`:111`）、`crsClaudeAccount` DTO（`:128-142`）、Claude OAuth/Setup Token 导入分支（`:284`+）、`PreviewFromCRS` 的 `ClaudeAccounts` 分类循环（`:1455-1461`）与 `refreshOAuthToken` 的 `PlatformAnthropic` case（`:1313-1317`）；保留 `refreshOAuthToken` 的其它平台分支与 `ClaudeConsoleAccounts`（Anthropic APIKey）、OpenAI、Gemini 分支。
- [x] 4.2 删除 `CRSSyncService.oauthService` 字段（`:24`）、`NewCRSSyncService` 参数（`:33`/`:41`）与 `:51-52` 的 Claude 平台守卫注释，并同步 `service.NewCRSSyncService` 调用点与 wire 注入（确认 `oauthService` 在 CRS 服务内仅被 `:1314-1317` 使用）。
- [x] 4.3 复核 `admin_account.go` 中通用 OAuth 合并逻辑（`:345-360`、`:444`、`:478`、`:534`、`:652`）在移除后仍对 Kimi/Grok/Antigravity 生效，无 Anthropic 特化残留。

## 5. Backend removal — Wire、启动与生成代码

- [x] 5.1 `backend/internal/service/wire.go`：删除 `ProvideClaudeTokenProvider`；调整 `ProvideTokenRefreshService`、`ProvideAccountTestService`、`ProvideGatewayService` 参数签名。
- [x] 5.2 `backend/internal/repository/wire.go`：删除 `NewClaudeOAuthClient` 注册。
- [x] 5.3 `backend/internal/handler/wire.go`：删除 `admin.NewOAuthHandler` 注册与 `Handler` 结构体赋值。
- [x] 5.4 `backend/cmd/server/wire_gen.go`：删除 `claudeOAuthClient`/`oAuthService`/`claudeTokenProvider` 构造与传参（`NewGatewayService`、`ProvideAccountTestService`、`NewCRSSyncService`、`NewAccountHandler`），保留 `ProvideOAuthRefreshAPI`。
- [x] 5.5 `backend/cmd/server/cleanup.go`：删除 `oauth *service.OAuthService` 字段与 `oauth.Stop()` 调用及其它 Anthropic OAuth 清理项，保留其它 worker 清理。
- [x] 5.6 同步 `archtest/baseline_imports.txt`（若受影响）与任何 wire 快照测试。

## 6. 测试与文档同步（后端）

- [x] 6.1 删除 Claude OAuth 专项测试：`oauth_service_test.go`、`claude_oauth_service_test.go`、Claude token provider/refresher 用例，以及 `oauth_refresh_api_test.go` 中的 `BuildClaudeAccountCredentials`（`:347-380`）与 `TestClaudeProviderRefreshPolicy`（`:628+`）用例（保留该文件其余 `OAuthRefreshAPI` 用例）。
- [x] 6.2 修复共享测试夹具：`gateway_handler_warmup_intercept_unit_test.go:167`（`claudeTokenProvider` 参数）、`token_refresh_service_test.go`、`admin_account_oauth_merge_test.go:187`、`token_cache_invalidator_test.go:184`、`gateway_body_order_test.go:203`、`account_vertex_onboarding_test.go:30`、`account_repo_integration_test.go:1127`。
- [x] 6.3 确认 `antigravity_rate_limit_test.go` 的 `AccountTypeSetupToken` 用例保持通过（Antigravity setup-token 不受影响）。
- [x] 6.4 新增回归测试：Anthropic `oauth`/`setup-token` 创建与更新被拒、批量入口被拒、`GetAccessToken` fail-closed、Claude 授权路由 404、CRS 导入跳过 `claudeAccounts` 但保留 `claudeConsoleAccounts`。
- [x] 6.5 文档同步：删除/更新提及 Claude OAuth / Setup Token 的运维与开发文档。

## 7. Frontend removal

- [x] 7.1 删除 `frontend/src/composables/useAccountOAuth.ts`；将 `AddMethod` / `AuthInputMethod` 类型迁移到共享位置并修复引用。
- [x] 7.2 `frontend/src/components/account/CreateAccountModal.vue`：移除 Anthropic 的 `useAccountOAuth` 与 `<OAuthAuthorizationFlow>` 使用分支，保留其它平台用法。
- [x] 7.3 `frontend/src/components/account/EditAccountModal.vue` 与 `components/admin/account/ReAuthAccountModal.vue`：移除 Anthropic OAuth/Setup Token/cookie 重新授权入口与 `useAccountOAuth` 调用。
- [x] 7.4 `frontend/src/components/account/OAuthAuthorizationFlow.vue`：仅删除 Claude 专属分支（`platform === 'anthropic'` 处理、cookie-auth/sessionKey、`isAnthropic` help 文案），保留 OpenAI/Gemini/Antigravity/Grok 行为。
- [x] 7.5 `frontend/src/features/admin-accounts/form/registry.ts`：Anthropic 项改为 `categories:['apikey']`、`types:['apikey']`，移除 oauth-only 字段（`anthropicAdvanced` 等 oauth 专用项）。
  - 偏差说明：`oauthModelRestriction` **必须保持 `true`**。它同时 gating Anthropic APIKey 账号的通用「模型限制」面板（消费点 `EditAccountModal.vue:88`/`:2093`/`:2979`），置 `false` 会让 APIKey 账号丢失模型限制编辑能力（已在 `registry.ts` 就地留「Never touch」注释）。
- [x] 7.6 清理残留引用（`AccountUsageCell.vue`/`AccountUsageDrawer.vue` 的 Anthropic OAuth 用量展示分支刻意保留——属历史数据展示、非授权接入能力，见 proposal 的非目标）：`credentialsBuilder.ts`、`types/index.ts`、`PlatformTypeBadge.vue`、`AccountTableFilters.vue`、`AccountUsageCell.vue`、`AccountActionMenu.vue`、`AccountCostConfigPanel.vue`、`AccountCostConfigDialog.vue`、`AccountUsageDrawer.vue`、`api/admin/accounts.ts`、`components/account/index.ts`。
- [x] 7.7 同步 `SyncFromCRS` 预览列表（不再列出 claude oauth/setup-token）与 i18n（`frontend/src/i18n/locales/{zh,en}/admin/accounts.ts`）中被删能力的文案；保留 Anthropic APIKey 文案。另需检查 `frontend/src/components/admin/account/SyncFromCRS*.vue` 是否按 kind/authType 渲染 Claude OAuth 条目。

## 8. Gates（验证）

- [x] 8.1 `cd backend && go build ./...`。
- [x] 8.2 `cd backend && go vet ./...`。
- [x] 8.3 `cd backend && go test ./internal/... ./cmd/...`（重点：service、handler、repository、domain、archtest）。
- [x] 8.4 `cd frontend && pnpm vue-tsc --noEmit`（或项目既有 type-check 脚本）。
- [x] 8.5 `cd frontend && pnpm test:run`：`2 failed | 174 passed (176)`、`3 failed | 1129 passed (1132)`；3 个失败（`UserPlatformQuotaModal.spec.ts` ×2、`AccountsView.sparkShadow.spec.ts` ×1）已在基线 worktree（`dc11f065b`）逐用例复现，属预存失败，非本 change 引入；本 change 触及的 spec（`registry.spec.ts`、`EditAccountModal.spec.ts`、`credentialsBuilder.spec.ts`）全绿。
- [x] 8.6 残留检索收口：`rg -n "generate-auth-url|generate-setup-token-url|exchange-setup-token-code|setup-token-cookie-auth|useAccountOAuth|ClaudeOAuthClient|ClaudeTokenProvider|claudeTokenProvider|crsClaudeAccount"` 在 `backend/`、`frontend/` 中无命中（历史数据字段与 APIKey 文案除外）。
- [x] 8.7 `openspec validate remove-claude-oauth-login --strict` 通过。

## 9. Deploy & 收尾

- [x] 9.1 构建前端（pnpm 11 需 `--dangerously-allow-all-builds`）后端（`-tags embed`），按 hot-swap 流程部署：停容器 → 替换 bind-mount binary → 启动。
- [x] 9.2 冒烟：6 条退役路由 404（控制项 401 证明鉴权面未变）；创建 `type=oauth`/`setup-token` 被拒（`CLAUDE_OAUTH_ACCOUNT_RETIRED`）且无落库行；`platform=anthropic & type=apikey` 创建路径可达（参数校验错误而非退役错误）；OpenAI/Grok/Antigravity 授权路由存活（200/400 进入 handler）；Anthropic APIKey 转发与账号探针成功（账号 140/139 `test_complete success=true`）。
  - 用户登录 OAuth：`github|google|linuxdo|oidc|wechat /start` 返回 `OAUTH_DISABLED`（`wechat oauth is disabled`）、`dingtalk/start` 302 `dingtalk_not_enabled`。经源码定位（`service/setting_oauth.go:416/438`）确认为**数据库设置未启用**的既有配置态，与本 change 无关（本 change 未触及 `routes/auth.go` 与邮件 OAuth 代码）。
- [x] 9.3 清理 Redis：删除 `claude:account:*` token 键与受影响账号的 `sched:acc:<id>` 快照键。
- [x] 9.4 部署后复核 SQL：确认无 Anthropic `oauth`/`setup-token` 行，历史 usage/结算记录未被改写。
- [x] 9.5 回勾本 tasks 完成；`openspec archive` **未执行**，留待后续批量归档（仓库既有大量未归档 change，与本 change 无关）。

## 10. 交付物

- [x] 10.1 生产发布记录：`deploy-2026-09-25-claude-oauth-retirement.md`（含 sha256、回滚点、两轮冒烟、预存失败对照、观察项）。
