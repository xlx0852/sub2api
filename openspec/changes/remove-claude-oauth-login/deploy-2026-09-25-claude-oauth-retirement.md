# 生产发布记录：remove-claude-oauth-login（Claude OAuth 授权接入退役）

- 日期：2026-09-25（记录用 +08:00；生产机时区 JST +09:00，同一时刻 = 04:37 JST）
- 代码：`9535ad651 fix(frontend): 恢复被误删的平台配额看板与 OAuth 模型限制面板`
  - 同一批变更：`c1d5ac415 feat: 网关空池强制 DB 复核 + 移除 Claude OAuth 登录能力`、`378f920ca feat: 前端收口移除 Claude OAuth 授权入口`、`b8a8b66ad fix(scheduling): exclude retired Anthropic OAuth accounts`
- change：`openspec/changes/remove-claude-oauth-login/`
- 生产机：`sub2api-prod`（74.82.195.84），域名 `code.sicts.shop`
- 审计基线（过度删除比对）：`dc11f065b`
- 无数据库 schema 变更；无新增/修改环境变量。

## 0. 语义与边界（本次发布的定义）

「Claude OAuth」= **Anthropic 上游账号**通过 OAuth / `setup-token` / cookie-auth 授权接入的能力，**不是** sub2api 用户登录方式。

- 退役：Anthropic 账号的 `type=oauth` / `type=setup-token` 全链路（授权 URL 生成、code / setup-token 兑换、cookie 授权、授权后建号、token 刷新与 session 存储、CRS 的 `ClaudeAccounts` 导入、对应管理端路由）。
- 保留：Anthropic **APIKey** 供给（含 CRS `ClaudeConsoleAccounts` 导入）、模型路由、TLS 指纹、cache-TTL、自定义 base URL、warmup 拦截与请求体改写；其它 provider（OpenAI / Antigravity / Gemini / Grok / Kimi / Mirasim / Cursor）的 OAuth 全链路；sub2api 用户登录 OAuth（GitHub / Google / LinuxDo / WeChat / DingTalk / OIDC）。

## 1. 发布顺序与产物

前端先、后端后。后端沿用既有约定：**同容器热替换 bind-mount 二进制**（`docker stop` → 覆盖文件 → `docker start`），非 compose 重建。

### 1.1 前端

| 项 | 值 |
| --- | --- |
| 构建 | `pnpm --dir frontend run build`（= `vue-tsc -b && vite build`）；`pnpm vue-tsc --noEmit` exit 0 |
| dist | 172 个文件；聚合 sha256（`find . -type f \| LC_ALL=C sort \| xargs shasum -a 256 \| shasum -a 256`）= `b0b68fa4206c5ec8628bf98e8675e9db5d98ad4416bd3d68fd6c0ceebd5d8d79` |
| 打包 | `frontend-0.1.391-9535ad651.tgz`，17,396,847 B，`sha256=5d3a5c73a7578a2e673fca258df0acf15d60074f66627ecfdf1bb5b569ce74b7`；上传生产 `/tmp/` 后双向校验 sha256 一致 |
| 发布目录 | `/opt/sub2api/frontend/releases/0.1.391-9535ad651/`（解包后清理 `._*` / `.DS_Store`，清理后计数 **0**；`chmod -R a+rX`） |
| `current` | → `releases/0.1.391-9535ad651`（发布前 → `0.1.391-claudeoauthretire-378f920ca`） |
| `rollback` | → `releases/0.1.391-claudeoauthretire-378f920ca` |
| 一致性 | 生产 release 目录 172 文件，聚合 sha256 = `b0b68fa4…`，与本地 `backend/internal/web/dist` **逐文件一致** |
| 线上产物 | `https://code.sicts.shop/` 引用 `/assets/index-DIL5AhZl.js`、`/assets/index-BMxnFjyh.css`；`index.html` sha256 = `44cb30c911016c42315de28ad4e0b577011282d4505df3219f64abe1ee4f4faf` |
| 内容自检 | `AccountsView-Dxubk-Lb.js` 含 `create-oauth-model-restriction`（通用 OAuth 模型限制面板健在）；`api-CXB2dIF6.js` 含 `getMyPlatformQuotas`（平台配额看板健在）；6 条退役 API 路径在 dist 中 **0** 命中 |
| 服务方式 | nginx `code-sicts-shop.conf`：`root /opt/sub2api/frontend/current`，`upstream sub2api_api { server 127.0.0.1:18373; }`（前端不来自二进制内嵌 dist） |

### 1.2 后端

| 项 | 值 |
| --- | --- |
| 构建 | `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -tags embed ./cmd/server`（go1.26.4），随后剥离符号 |
| 构建指纹 | `go version -m` → `-tags=embed`、`-trimpath=true`、`CGO_ENABLED=0`、`GOOS=linux`、`GOARCH=amd64`、无 `-ldflags` 记录；`go tool nm` → `no symbol section` |
| 版本来源 | 内嵌 `backend/cmd/server/VERSION` = `0.1.391`、`MIN_FRONTEND_VERSION` = `0.1.380`；线上 `/api/v1/public/version` 实测 `{"version":"0.1.391","min_frontend_version":"0.1.380"}` |
| 二进制 | `sha256=6e017abd1dbaafce1767d53dd8e97ec4e64055a55f24d9ba4eae21d9a39ff163`，123,682,978 B（本地 `/tmp/s2a-deploy/sub2api` 与线上一致） |
| 退役守卫在位 | 线上二进制中 `CLAUDE_OAUTH_ACCOUNT_RETIRED` 命中 1 次 |
| 容器 | `sub2api-api-candidate-0.1.391-noinject-a33b47ef9`（`127.0.0.1:18373` → nginx upstream `sub2api_api`） |
| 挂载点 | `/opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api → /app/sub2api` |
| 热替换 | `docker stop` → `install -m 0755` 覆盖 → `docker start`；`StartedAt=2026-09-24T19:37:21Z`（= 03:37 +08:00），`RestartCount=0`，`healthy` |
| 发布前版本 | `vcs.revision=378f920ca`（前端收口那一次） |
| 发布后版本 | `go version -m` → `vcs.revision=9535ad65175b85aeb918f3ac87eb1a8eb4d4188f`、`vcs.time=2026-09-24T19:35:45Z`、`vcs.modified=true` |
| 回滚点 | `/opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api.bak-20260925-claudeoauthretire2`（123,674,786 B，`sha256=91b7047740e33ac44ffb1b10fd24166ac751bb10667ace68a32a2933d5eda5f1`，`vcs.revision=378f920ca`） |
| 健康检查 | `/health` → 200；`/api/v1/public/version` → 0.1.391 / min_frontend 0.1.380 |

## 2. 发布后核验（只读 + 幂等拒绝路径）

冒烟脚本：`/tmp/smoke.sh`、`/tmp/smoke2.sh`（发布期在工作机 `/tmp/s2a-deploy/`，生产 `/tmp/`），发布后重跑，`EXIT=0` 全绿。

### 2.1 退役面（期望消失）

| 核验 | 结果 |
| --- | --- |
| `POST /api/v1/admin/accounts/{cookie-auth, setup-token-cookie-auth, exchange-code, exchange-setup-token-code, generate-auth-url, generate-setup-token-url}` | 6/6 **404** |
| 控制项 `POST /api/v1/admin/accounts/data`（带 admin key） | **400**（路由存在，参数校验失败 → 鉴权与路由面未变） |
| 控制项 `GET /api/v1/admin/accounts`（无认证） | **401** |
| 创建 `platform=anthropic, type=oauth` | 400 `{"reason":"CLAUDE_OAUTH_ACCOUNT_RETIRED","message":"anthropic claude oauth/setup-token sign-in has been retired; use anthropic api-key instead"}` |
| 创建 `platform=anthropic, type=setup-token` | 同上 400（同一守卫；`rejectRetiredSupplyAccount` 在 handler / service / repository 三层，且双判 `platform==anthropic`，Antigravity 的 setup-token 不受影响） |
| 落库检查 | `SELECT count(*) FROM accounts WHERE name LIKE 'smoke-claude-oauth-retire%'` = **0** |
| 对照：`platform=anthropic, type=apikey` 且缺 `credentials` | 400 `Invalid request: Key: 'CreateAccountRequest.Credentials' … 'required'`（APIKey 建号路径可达，未被守卫误拒） |

### 2.2 保留面（期望健在）

| 核验 | 结果 |
| --- | --- |
| `POST /api/v1/admin/openai/generate-auth-url`（admin key） | **200**，返回 `auth_url: https://auth.openai.com/oauth/authorize?client_id=…` |
| `POST /api/v1/admin/grok/oauth/exchange-code` | 400 `GrokExchangeCodeRequest.SessionID` 校验失败（已进入 handler，非 404） |
| `POST /api/v1/admin/antigravity/oauth/exchange-code` | 400 `AntigravityExchangeCodeRequest.SessionID` 校验失败（已进入 handler，非 404） |
| `GET /api/v1/auth/oauth/{github, google, linuxdo, oidc}/start` | 404 `{"code":404,"message":"oauth login is disabled","reason":"OAUTH_DISABLED"}` |
| `GET /api/v1/auth/oauth/wechat/start` | 404 `wechat oauth is disabled`（`OAUTH_DISABLED`） |
| `GET /api/v1/auth/oauth/dingtalk/start` | 302 → `/auth/dingtalk/callback#error=dingtalk_not_enabled` |
| 归因（非回归） | 上述 404/302 为**数据库设置未启用**的既有配置态：`internal/server/routes/auth.go:60-101` 无条件注册路由；`internal/handler/auth_email_oauth.go:49/61/469` → `internal/service/setting_oauth.go:416/438-439` 在 `!cfg.Enabled` 时返回 `NotFound("OAUTH_DISABLED", …)`。本 change 未改动 `routes/auth.go`、邮件 OAuth、用户登录 OAuth 任何代码 |

### 2.3 Anthropic APIKey 供给端到端

| 核验 | 结果 |
| --- | --- |
| 线上 Anthropic 账号（`deleted_at IS NULL`） | 3 个，全部 `type=apikey`、`status=active`、`schedulable=t`：81 `国模 https://hk-api.x5m5x.com`、139 `OpenCode Go https://opencode.ai/zen/go`、140 `mirasim-relay https://relay.mirasim.ai`（均属 group 34 `国模集合`） |
| 真实转发 | `POST /v1/messages`（group 34 用户 API key，`model=glm-5.3-flash`，`max_tokens=16`）→ **200**，返回真实上游内容（`"ok"`，含 thinking 块）；日志 `sticky.account_selected selected_account_id=81` → `http request completed latency_ms=3009` |
| 透传分支 | 同窗口 `[Anthropic 自动透传] 命中 API Key 透传分支` 正常打印（如 `account=139 name=OpenCode Go`） |
| 账号探针 | 账号 140（`kimi-k3`）、139（`deepseek-v4.1-flash`）→ SSE `{"type":"test_complete","success":true}` |
| 探针质量线索 | 140 的诊断探针答案判为不正确（上游质量线索，属既有行为，本次不改；139 因 `max_tokens=1024` 被 thinking 耗尽而截断） |

### 2.4 DB / Redis 一致性

| 核验 | 结果 |
| --- | --- |
| Anthropic 账号类型分布 | `apikey` 9 行（其中 3 行 `deleted_at IS NULL`）、`oauth` **0** 行、`setup-token` **0** 行 |
| 其它 provider 供给 | antigravity `oauth` 2、grok `apikey` 1 / `oauth` 9、openai `apikey` 2 / `oauth` 4（未被本 change 影响；另有 cursor/devin/kimi 0 存量） |
| 历史记录 | `usage_logs` = 2,579,790 行，最新 `created_at` = `2026-09-25 03:55:15 (+08:00)`；历史 usage / 结算记录未被改写 |
| Redis Claude token 键 | `env -u REDISCLI_AUTH redis-cli --scan --pattern "claude*"` = **0**（发布前存量亦为 0，无需清理）；`oauth:token:*` 仅剩 openai 8 / grok 4 / ag 1 / cursor 1 前缀，无 `claude` |
| `sched:acc:*` 快照键 | 34 个（scheduler 正常态）。本次**未做任何直连 SQL 账号编辑**（无 Claude OAuth 存量行需处理），按约定无需删除快照键 |

> 注意：该 Redis 容器带空 `REDISCLI_AUTH=` 环境变量，直接 `docker exec … redis-cli --scan` 会报
> `AUTH failed: … called without any password configured` 并**静默返回 0 行**（假阴性）。
> 必须写成 `docker exec sub2api-redis env -u REDISCLI_AUTH redis-cli …`。本节结论已用该方式复测。

### 2.5 运行面 / 日志

| 项 | 结果 |
| --- | --- |
| 容器近 15 分钟 | `Up (healthy)`、`RestartCount=0`、无 `panic` / `fatal` |
| 45 分钟窗口日志 | 5,049 行；`ERROR` **19** 条，全部为既有定时任务 / 账号探针噪声：Grok Responses 402 额度（8）、`SubscriptionAutoRenew ACCOUNT_NOT_FOUND`（4）、`Account test error`（Account not found 2、上游 500 2、404 1、400 1、422 1）；**无一条**引用 `CLAUDE_OAUTH_ACCOUNT_RETIRED` 或退役守卫 |
| 退役守卫误触发 | 0（`rejectRetired…` 未在 Anthropic APIKey 请求路径上触发：2.3 的真实转发与探针均 200/成功） |

## 3. 本地门禁与预存失败对照

| 门禁 | 结果 |
| --- | --- |
| `go build ./...`、`go vet` | 通过 |
| `make test`（= `go test ./...`） | 失败包 = `internal/archtest` + `internal/service`，与基线 worktree（`git worktree add /tmp/s2a-base dc11f065b`）**完全一致** |
| `internal/service` 失败用例 | 6 个，与基线同名同数量：`TestIsModelRateLimited`、`TestGetModelRateLimitRemainingTime`、`TestGetRateLimitRemainingTime`、`TestOpenAIGatewayServiceForward_CodexImageInjectionRespectsGroupCapability`、`TestOpenAIGatewayService_OAuthPassthrough_CompactUsesJSONAndKeepsNonStreaming`、`TestOpenAIGatewayService_ProxyResponsesWebSocketFromClient_CodexImageBridgeRespectsResponsesLite` |
| `internal/archtest` | 仅 `TestNoNewPackageLevelMutableGlobals`（2 条基线外全局：`service/mirasim_relay_runtime.go:35`、`service/openai_codex_turn_state_watch.go:38`；两文件在 `dc11f065b..HEAD` 未被改动） |
| `internal/handler`、`repository`、`domain` | 全部 `ok`（含新增 `claude_oauth_retirement_test.go`、`claude_oauth_retirement_unit_test.go`、`account_schedulable_retirement_test.go`） |
| 前端 `pnpm vue-tsc --noEmit` / `pnpm build` | 通过 |
| 前端 `pnpm test:run` | `2 failed \| 174 passed (176)`、`3 failed \| 1129 passed (1132)`；3 个失败（`UserPlatformQuotaModal.spec.ts` ×2、`AccountsView.sparkShadow.spec.ts` ×1）在基线 worktree 逐用例复现，属**预存失败**；本 change 触及的 spec 全绿 |
| 过度删除审计 | `git diff --name-status dc11f065b..HEAD` 共 90 文件（14 A / 8 D / 68 M，+1,402 / −5,316）逐文件核对；过程中已修复 4 处前端误删/回归（`showRefreshTokenOption`、重复 `useOpenAIOAuth` import、平台配额看板、通用 OAuth 模型限制面板、openai/gemini 创建默认类别） |

## 4. 回滚

1. 后端：`docker stop <container>` → `install -m 0755 /opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api.bak-20260925-claudeoauthretire2 /opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api` → `docker start`；校验 `sha256=91b70477…` 与 `vcs.revision=378f920ca`。
2. 前端：`ln -sfn /opt/sub2api/frontend/releases/0.1.391-claudeoauthretire-378f920ca /opt/sub2api/frontend/current`（`rollback` 保留指向更早 release 视需要调整）。
3. 无 schema / 无数据迁移，无需 DB 回滚；退役守卫在旧二进制中不存在，回滚后 Claude OAuth 建号路径恢复可达。

## 5. 观察项与已知限制

- **同批发布**：本二进制同时包含 `add-gateway-empty-pool-db-recheck`（网关空池强制 DB 复核），见 `openspec/changes/add-gateway-empty-pool-db-recheck/deploy-2026-09-25-empty-pool-db-recheck.md`。
- **刻意保留的非目标**：`AccountUsageCell.vue` / `AccountUsageDrawer.vue` 的 Anthropic OAuth 用量展示分支（历史数据展示）；`registry.ts` 的 `oauthModelRestriction` 保持 `true`（同时 gating Anthropic **APIKey** 的模型限制面板，置 false 会误伤 APIKey）；`oauth.cursor` / `oauth.devin` 死键。
- **制品可复现性**：以相同设置重建未剥离二进制 158,449,214 B，`x86_64-linux-gnu-strip -s` 后 123,680,552 B（与线上 123,682,978 B 同量级但**非字节一致**）；以线上 sha256 `6e017abd…` 为准。
- **用户登录 OAuth 的 404**：线上为「设置未启用」状态，非本次回归；如需启用属运营配置操作。
- 前端 `pnpm build` 需 `--dangerously-allow-all-builds`（pnpm 11 的 esbuild / vue-demi postinstall 限制）。

## 6. 后续修正：单一账号类型不再渲染选择器（前端，2026-09-25 05:03 JST）

**动机**：Claude OAuth / setup-token 退役后，Anthropic 的「账号类型」只剩 `Claude Console / API Key` 一个选项，弹窗里仍渲染成可点击的选择器，属于退役遗留的无效交互。

**代码**（`0c48b9f6b fix(frontend): 单一账号类型的平台不再渲染可点击的账号类型选择器`，4 文件）：

| 文件 | 改动 |
| --- | --- |
| `frontend/src/features/admin-accounts/form/registry.ts` | `ACCOUNT_CATEGORY_PICKERS.anthropic.model`：`'accountCategory'` → `'none'`（`options` 仍为 `['apikey']`，`layoutClass` 保留 `grid-cols-1`） |
| `frontend/src/features/admin-accounts/form/AccountCategoryPicker.vue` | 新增 `isSingleOption` / `isStatic`：`options.length <= 1`（或 `model === 'none'`）时走静态卡片分支（`cursor-default`，无 hover / 无选中交互），静态卡片同样带 `data-tour="account-form-type"` 以保住引导步骤锚点；静态分支沿用 `option.icon`（Anthropic 仍显示 key 图标） |
| `frontend/src/features/admin-accounts/form/registry.spec.ts` | 增加 `platformCategoryPicker('anthropic')?.model === 'none'` 断言 |
| `frontend/src/features/admin-accounts/form/__tests__/AccountCategoryPicker.spec.ts` | 新增：anthropic / kimi 无 `<button>` 且带 `data-tour` 锚点、`cursor-default`；openai（2 个类别）仍渲染按钮并照常 `emit('update:modelValue')` |

**范围**：前端单文件逻辑，无后端、无 schema、无环境变量变更；`kimi` 等其它单一类别平台顺带收敛为同一静态分支，多类别平台（openai / gemini / grok / antigravity）行为不变。Anthropic 的 `accountCategory` 仍由平台 watcher 固定为 `apikey`（`CreateAccountModal.vue:3117`），不依赖选择器事件，故不再触发 `update:modelValue` 不影响建号。

**产物与部署（前端-only，热替换 release 目录）**：

| 项 | 值 |
| --- | --- |
| 变更前 `current` | `releases/0.1.391-9535ad651` |
| 新 release | `/opt/sub2api/frontend/releases/0.1.391-0c48b9f6b` |
| 制品 | `frontend-0.1.391-0c48b9f6b.tgz`（17,393,946 B，`sha256=652665ff214bbd4144430bda78b7b5d6aa729e91b2c4adcd9fb2ad87f25f9fcf`），本地 ↔ `/tmp` 双向一致 |
| 部署后 `current` / `rollback` | `releases/0.1.391-0c48b9f6b` / `releases/0.1.391-9535ad651`（`Sep 25 05:03` JST） |
| 后端二进制 | **未重新发布**（本改动不影响后端；nginx 从 `/opt/sub2api/frontend/current` 直供 SPA，二进制内嵌 dist 仅为兜底路径） |

**发布后核验（只读）**：

| 校验 | 结果 |
| --- | --- |
| 文件数 / AppleDouble | `find -L current -type f` = **172**；`._*` / `.DS_Store` = **0** |
| 全量聚合校验 | `cd <release> && find . -type f \| LC_ALL=C sort \| xargs sha256sum \| sha256sum` = `7d8aa2609c4c8ea8e827e234be8aa5543aa7a413416876a3ac3e60d11b84d035` == 本地 `backend/internal/web/dist` 聚合值 |
| 入口文件 | `index.html` `sha256=c542596b46eee3db23d9177651388259da812d719e959b2310401c055d6e1eac`，引用 `/assets/index-BLa5x04W.js` + `/assets/index-BMxnFjyh.css`（CSS 未变） |
| 字节一致性 | 经 nginx（生产机 `--resolve code.sicts.shop:443:127.0.0.1`）与公网 `https://code.sicts.shop` 取回：`index.html` = `c542596b…`、`index-BLa5x04W.js` = `70e1583e83dcb63e4748515cc94602eb6a80b586226f62b7b9953d6b737493fe`（200，147,409 B）、`index-BMxnFjyh.css` = `61840a65…`（200）、`AccountsView-BoBP6HCR.js` = `756c4417c120bf2d3c2b01920477aae213b44e384eaf8cf14f594f17f15f58d2`（200，631,804 B）——全部与本地构建逐字节相同 |
| 行为标记 | 本地 `assets/AccountsView-BoBP6HCR.js` 含静态分支类名 `mt-2 cursor-default rounded-lg border-2 p-3`（1 处）；组件级行为由新增 spec 断言 |
| 门禁 | `pnpm vue-tsc --noEmit` 通过；`pnpm build` 通过；`registry.spec.ts` 6/6、`AccountCategoryPicker.spec.ts` 3/3 通过；`pnpm test:run` 仍为 `3 failed \| 1129 passed`（3 个失败与基线 worktree 同名同数量，属预存失败） |

**回滚**：`ln -sfn /opt/sub2api/frontend/releases/0.1.391-9535ad651 /opt/sub2api/frontend/current`（回到 9535ad651 版本的前端；后端无需动作）。
