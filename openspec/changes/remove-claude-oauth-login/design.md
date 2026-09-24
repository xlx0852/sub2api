## Context

- 目标：删除 Anthropic 上游账号通过 OAuth / Setup Token（含 cookie 授权、设备/浏览器授权码）接入的全部能力，保留 Anthropic APIKey 供给与其它所有 provider 的 OAuth。
- 生产事实（沿用 `remove-anthropic-subscription-quota-control` 的核实结论，部署前需再验一次）：Anthropic 三个账号 `deepseek` / `kimi` / `国模` 全部 `type=apikey`，`extra` 为空，无 Anthropic OAuth/SetupToken 行。
- 约束：不得影响 sub2api 用户登录 OAuth（GitHub / Google / LinuxDo / WeChat / DingTalk / OIDC），不得影响 OpenAI / Antigravity / Gemini / Grok / Kimi / Mirasim 的上游 OAuth 与后台刷新，不得删除历史数据。

## Goals / Non-Goals

- Goals:
  - 控制面 fail-closed：Anthropic `type=oauth|setup-token` 无法新增、导入、转换、再授权、刷新或被测活。
  - 生产执行路径归零：授权端点、token provider、token 缓存、会话存储、后台刷新分支全部删除。
  - 共享给其它 provider 的代码（PKCE/state 生成、`MergeCredentials`、`AccountTypeSetupToken` 常量、`OAuthRefreshAPI`）保持可用。
- Non-Goals:
  - 不做破坏性数据迁移，不删除历史账号/凭据/usage/结算/审计。
  - 不退役 Anthropic APIKey，不改动 Anthropic 协议转发、模型路由与计费。
  - 不重构共享授权组件 `OAuthAuthorizationFlow.vue`，只摘除 Claude 专属分支。

## Decisions

- Decision: 拒绝点放在 domain/service 层（`rejectDeprecated*` 家族的同构模式，如 `domain.ErrRetiredClaudeOAuthAccount`），在 `admin_account.go` 的创建与更新路径、`account_service.go` 的账号服务路径同时生效。
  - Why: 与 `rejectDeprecatedBedrockType` / `rejectDeprecatedVertexServiceAccountType` 一致，HTTP 与前端无法绕过，批量入口自动覆盖。
  - Alternatives considered: 只在路由层删端点和在 handler 里校验 —— 会被批量/内部调用（CRS 同步、import、refresh）绕过。
- Decision: 存量行保持原样、仅 fail-closed，不新增迁移。
  - Why: 生产无存量行；写入型迁移会在其它环境制造不可逆改写，且历史 usage/结算仍引用这些行。
  - Alternatives considered: 迁移把 `type` 改成 `apikey`（凭据形态不兼容，等于伪造数据）或置 inactive（会改写历史状态，且仍留下可被重新激活的入口）。
- Decision: `AccountTypeSetupToken` 常量保留（Antigravity 仍在使用），只在 Anthropic 平台维度拒绝；同样保留 `Account.IsOAuth()`、`IsAnthropicOAuthOrSetupToken()`（TLS 指纹 / 账号表单可见性仍在使用），除非确认删除后无调用点。
  - Why: 按平台收敛比按类型收敛更安全，避免误伤 Antigravity 的 setup-token 与共享 helper。
- Decision: `refreshSingleAccount` 的 Claude 分支改为显式拒绝，而不是留下不可达的 `else`。
  - Why: 保留可读的 fail-closed 语义，避免后续误认为「该分支只是暂时没有账号」。
- Decision: CRS 只删 `claudeAccounts`（oauth/setup-token）导入分支，保留 `claudeConsoleAccounts`（Anthropic APIKey）。
  - Why: Anthropic APIKey 仍是受支持供给；CRS 导入的 Anthropic APIKey 路径必须继续可用。
- Decision: `pkg/oauth` 保留 `GenerateRandomBytes` / `GenerateState` / `GenerateCodeVerifier` / `GenerateCodeChallenge` / `base64URLEncode`，删除 Claude 常量、`SessionStore`、`OAuthSession`、`TokenResponse`、`BuildAuthorizationURL`。
  - Why: 用户登录 OAuth handler 直接复用这些基础函数，删除会波及 6+ 个 auth handler。
- Decision: 前端保留共享 `OAuthAuthorizationFlow.vue`（OpenAI/Gemini/Antigravity/Grok 在用），把 `AddMethod` / `AuthInputMethod` 类型从被删的 `useAccountOAuth.ts` 迁到共享位置。
  - Why: 组件被多平台复用，整删会连带破坏其它 provider 的授权向导。

## Risks / Trade-offs

- 风险：`go build` 层面删漏一个引用点会连带阻塞整个 build（wire 注入在 `cmd/server/wire_gen.go`、`handler/wire.go`、`service/wire.go`、`repository/wire.go` 四处）。
  - Mitigation: 按 wire 依赖自底向上删除（pkg → domain/ports → repository → service → handler → cmd），每层结束跑一次 `go build ./...`。
- 风险：后台刷新与 token 缓存的残留分支会让已删能力经由 Redis 复活（`sched:acc:*` 快照 + `claude:account:*` token 键）。
  - Mitigation: 删除 `token_cache_key.go` 的 Claude 键与 `token_cache_invalidator.go` 的 `PlatformAnthropic` 分支；部署后清理 `claude:account:*` 与受影响账号的调度快照键。
- 风险：前端 `CreateAccountModal.vue` / `ReAuthAccountModal.vue` 与 registry 耦合较深，摘除 Anthropic oauth 类别容易留下 `isAnthropic` / `isOAuthFlow` 死分支。
  - Mitigation: 按 registry 的 `categories` / `types` 驱动删除，跑 `vue-tsc --noEmit` 与账号组件 Vitest 套件确认无残留引用。
- 风险：存量 Anthropic OAuth 行（其它环境）在退役后仍可被调度（凭据仍在 credentials 中）。
  - Mitigation: 网关 Anthropic 分支只在 `AccountTypeAPIKey` 下取 token，`oauth`/`setup-token` 账号走 `GetAccessToken` 的显式拒绝路径；部署前用 SQL 复核目标环境是否存在此类行并在 deploy 记录中列出。
- 风险：部署后需要回滚。
  - Mitigation: 沿用 hot-swap 流程（停容器 → 换 binary → 启动；前端 repoint `current` → `rollback`），本 change 无 schema 变更，回滚只需恢复上一个 binary 与前端 release。

## Migration Plan

1. 部署前：SQL 复核 Anthropic `type in ('oauth','setup-token')` 行数（预期 0），并核对 Anthropic 供给仍为 APIKey。
2. 部署后端 binary（hot-swap），确认启动无 panic，`generate-auth-url` 等路由返回 404。
3. 部署前端 release，确认 Anthropic 账号表单不再出现 OAuth / Setup Token，其它平台授权向导正常。
4. 清理 Redis `claude:account:*` token 键（如存在）与受影响账号的 `sched:acc:<id>` 快照键。
5. 回滚：恢复上一个 binary 与前端 release；无数据迁移需要回退。

## Open Questions

- 存量 Anthropic `oauth`/`setup-token` 行是否需要提供显式「识别并删除」运维入口？当前设计只保证可识别可删除（沿用账号删除路径），不新增专用入口。
