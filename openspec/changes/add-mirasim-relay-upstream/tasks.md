# Tasks

## 1. 协议层

- [x] 1.1 新增 `backend/internal/service/mirasim_protocol.go`：常量（默认 relay/auth/clientVersion/sealPubkey）、
      `mirasimCanonicalString`、`mirasimSignHeaders`、`mirasimSealHeaders`、`mirasimDeviceIdentity`、
      `mirasimGenerateDeviceKey`、`mirasimMarshalPrivateKeyPEM`、`mirasimParsePrivateKeyPEM`
- [x] 1.2 新增 `backend/internal/service/mirasim_protocol_test.go`：搬参考实现向量
      （canonical 精确串、meta+空 body、设备身份钉住 seed `0x01..0x20`、密封往返+字段顺序+AAD 绑定、PEM 往返）

## 2. 客户端与凭据链

- [x] 2.1 新增 `backend/internal/service/mirasim_relay.go`：常量、`isMirasimRelayAccount`、
      `mirasimRelayConfigForAccount`（解析 base_url/extra 覆盖）、`mirasimRelayClient`（access token / ticket / SignedRequest 头注入）、
      `applyMirasimRelayAuth` / `applyMirasimRelayAuthToRequest`
- [x] 2.2 新增 `backend/internal/service/mirasim_relay_runtime.go`：accountID → client 缓存（fingerprint 失效）、
      per-account single-flight、`invalidateMirasimRelayClient(accountID)` /
      `invalidateMirasimRelayClientTokens(accountID)`
- [x] 2.3 网络出口走 `s.httpUpstream.Do`，proxy 取 `resolveAccountProxyURL(account)`
- [x] 2.4 refresh token 轮换：`accountRepo.GetByID` 读最新 → 合并写入 → `persistAccountCredentials`；
      device key 首次生成同样在 per-account 锁内落库（`mirasimPersistCallbacks`）

## 3. 网关接入

- [x] 3.1 `gateway_service.go GetAccessToken`：`case AccountTypeAPIKey:` 最前分支返回 ticket（`tokenType` 仍为 `"apikey"`）
- [x] 3.2 `gateway_upstream_request.go buildUpstreamRequest` 末尾注入签名
- [x] 3.3 `gateway_anthropic_passthrough.go buildUpstreamRequestAnthropicAPIKeyPassthrough` 末尾注入签名
- [x] 3.4 `gateway_count_tokens.go buildCountTokensRequestAnthropicAPIKeyPassthrough` 与
      `buildCountTokensRequest` 末尾注入签名
- [x] 3.5 注入时清空 `RawQuery`（签的 path 就是发的 path）、URI 主机改写为 relay、
      删除 `x-api-key` / `x-goog-api-key`

## 4. 错误语义与诊断

- [x] 4.1 `ratelimit_service.go` `case 401` 在 OAuth 分支前插入 Mirasim 分支：
      失效缓存 → 冷却（复用 `oauth_401_cooldown_minutes`，`<=0` 取 10）→
      `notifyAccountSchedulingBlocked(..., "mirasim_401")` → `SetTempUnschedulable` → `shouldDisable = true`
- [x] 4.1b `ratelimit_service.go handle403` 增加 Mirasim 分支（`mirasim_403`），同样只做临时不可调度
- [x] 4.2 `account_test_service.go testClaudeAccountConnection` 增加 Mirasim 分支
      （relay base URL 无 `?beta=true`、跳过 `x-api-key` 注入、缺 refresh token 时由
      `mirasimRelayConfigForAccount` 返回明确错误）
- [x] 4.3 `upstream_billing_probe.go isUpstreamBillingProbeAccount` 排除 Mirasim 账号
- [x] 4.4 `upstream_models.go buildAnthropicUpstreamModelsRequest` 对 Mirasim 返回
      `newUpstreamModelSyncUnsupportedError`（relay 无 `/v1/models`）

## 5. 前端

- [x] 5.1 `form/registry.ts`：**无需改动**。`anthropicAdvanced: true` 已覆盖 anthropic 全部账号类型，
      Mirasim 区块靠 `isFieldVisible('anthropicAdvanced', …) && account.type === 'apikey'` 门控，
      复用既有 advanced 页签；在此新增字段声明反而会与弹窗内的手工区块重复。
- [x] 5.2 `credentialsBuilder.ts`：新增 `applyMirasimRelay(...)`、`validateMirasimRelayInput(...)`、
      `mirasimRelayInputFromAccount(...)` 与 `MIRASIM_*` 常量（写入 credentials + extra，含
      「敏感键留空 = 保留原值」「禁用时 edit 恒写 `mirasim_relay_enabled=false` 并回退官方 base_url」规则）
- [x] 5.3 `CreateAccountModal.vue` / `EditAccountModal.vue`：Mirasim relay 开关与字段 UI（开关关闭保留 refresh token）
- [x] 5.4 i18n（zh/en）新增 `admin.accounts.mirasimRelay.*`（21 个键，含 4 个校验错误文案）
- [x] 5.5 前端单测：`credentialsBuilder.spec.ts` 覆盖纯函数；`EditAccountModal.spec.ts` 覆盖
      「载入既有账号 / 新建启用并提交 / 缺 refresh token 报错 / 显式禁用 + base_url 回退」

## 5.6 文档

- [x] 5.6.1 新增 `docs/MIRASIM_RELAY_UPSTREAM.md`（账号模型、凭据链、请求头约束、
  错误语义、已排除能力、运维前置条件、回滚、验证方式）
- [x] 5.6.2 `docs/ARCHITECTURE_CN.md` 补一句：relay 作为 Anthropic apikey 的一种鉴权模式，
  不新增适配器

## 6. 验证

- [x] 6.1 `cd backend && go build ./...`
- [x] 6.2 `go test -tags unit ./internal/service/ -run 'Mirasim'` 通过（18 个用例，含黄金向量）
      - 注意：本仓库 `internal/service` 存在与本变更无关的陈旧测试文件
        （`auth_oauth_email_flow_test.go`、`auth_service_register_test.go`、
        `gateway_account_selection_test.go`、`account_repo_gemini_test_helpers.go` 等引用了已移除的
        API），包级 `go test` 无法直接构建。验证方式为非侵入式：把 `backend/` 硬链接复制到
        `/tmp`，仅在副本中移走全部陈旧 `*_test.go`（并临时移出 `account_repo_gemini_test_helpers.go`），
        再拷入本变更的两个测试文件执行；仓库内文件未做任何改动。
- [x] 6.3 `go vet -tags unit ./internal/service/`（同样在副本上）通过，无既有实现回归
- [x] 6.4 断言测试：`TestMirasimRelayInjectionProducesVerifiableSealedHeaders` 用自验签名
      （canonical 复算 + Ed25519 验签 + seal 反解 + 解出的 4 个头与请求头逐字段比对）验证
      Mirasim 账号构造出的请求；`TestMirasimRelayInjectionSkipsOtherAccounts` 覆盖 5 类非目标账号
- [x] 6.5 前端：`npx vue-tsc --noEmit`、`npx eslint`、`npx vitest run`（credentialsBuilder /
      EditAccountModal / localeCompleteness / localesNoKeyCollision）全部通过
- [x] 6.6 `openspec validate add-mirasim-relay-upstream --strict` 通过
- [x] 6.7 **真实联网端到端验证**（新增 `backend/internal/service/mirasim_relay_e2e_test.go`，
      `//go:build e2e`，凭据只从 0600 文件读取、缺凭据 skip）：
      - 设备身份推导与真实客户端对拍一致（`MIRASIM_E2E_EXPECT_DEVICE_ID` 断言通过）
      - 真实注入 `applyMirasimRelayAuthToRequest` → `/v1/messages`：`kimi-k3` 200、`deepseek-flash` 200；
        `/v1/messages/count_tokens` 200 `{"input_tokens":8}`
      - 生产入口 `mirasimRelayTicketToken` 返回 `tokenType=apikey`、ticket 453 字节
      - 生产 builder `buildUpstreamRequestAnthropicAPIKeyPassthrough` /
        `buildCountTokensRequestAnthropicAPIKeyPassthrough` 打到真实 relay 均 200
      - 另两个 builder（`buildUpstreamRequest` / `buildCountTokensRequest`）离线断言已改写到
        `https://relay.mirasim.ai/v1/messages(|count_tokens)` 且 `RawQuery == ""`、`x-api-key` 已清空
      - 伪造 refresh token → 401 且 `mirasimIsAuthError(err)==true`（不触碰真实凭据、无轮换）
      - 实测补充：不支持的模型名返回 **422** `invalid_request_error`（Anthropic 形态），
        经 `gateway_forward.go:39 shouldFailoverUpstreamError` 判定不触发 failover；
        `auth.mirasim.ai` 的错误体是 FastAPI 风格 `{"detail":...}`，与 relay 不同 → 错误归类只用状态码
      - 运行方式与 refresh 轮换警告已写入 `docs/MIRASIM_RELAY_UPSTREAM.md` §8
- [x] 6.8 **生产出口 + 真实凭据联合验证**（2026-09-24，零副作用）：
      本地跑 e2e 但把 TCP 出口切到生产机（`ssh -f -N -D 127.0.0.1:11080 root@74.82.195.84`
      → `HTTPS_PROXY=socks5h://127.0.0.1:11080`，`curl --socks5-hostname` 复核出口 IP 为
      `74.82.195.84`；E2E 客户端是 `http.Client{}`，nil Transport 走 `DefaultTransport`，
      因此天然认代理环境变量）。结果：4/4 通过——
      - `TestMirasimRealRelayInjectionEndToEnd`：`kimi-k3` 200、`deepseek-flash` 200、
        `count_tokens` 200 `{"input_tokens":8}`
      - `TestMirasimRealGatewayBuildersEndToEnd`：生产入口 `tokenType=apikey ticketLen=453`，
        两个 passthrough builder 打到 relay 均 200
      - `TestMirasimRealRelayRefreshRejected`：伪造 refresh → 401 且归类为 auth 错误
      - `TestMirasimBuildersRewriteToRelay`：离线改写断言
      另在无凭据阶段已用 bogus Bearer 在生产机上单独探测：DNS/TLS/路由/鉴权路径全通
      （`/v1/messages` `/v1/messages/count_tokens` → 401 同一信封、`/auth/refresh` → 401），
      **生产出口无 IP 层封锁**，与本地结论一致。
      ⚠️ 全程只用短期 access token 缓存，未调用 `/auth/refresh`，未轮换 refresh token。

## 7. 运维前置条件（代码之外，部署前需确认）

- [x] 7.1 上游 URL 白名单（`config_security.go` 的 `URLAllowlistConfig.UpstreamHosts`）：
      **2026-09-24 已确认生产无需配置**——线上 env 为 `SECURITY_URL_ALLOWLIST_ENABLED=false`
      且未设 `UPSTREAM_HOSTS`，关闭时不校验 host（但静态 scheme/私网校验仍生效，
      `upstream_base_url_validation.go:23`）。若日后启用白名单，**必须同时包含
      `api.anthropic.com` 与 `relay.mirasim.ai`**：网关侧校验的是账号 `base_url` 解析结果
      （apikey 账号 `base_url` 为空时回落 `api.anthropic.com`，`account_entity.go:1007`），
      注入在其后改写 host，所以缺 relay 不会拦网关请求，但账号测试会直接报 `Invalid base URL`
      （`account_test_service.go:380` 校验 `relayCfg.RelayURL`）；`auth.mirasim.ai` 走
      `repository/http_upstream.go` 的 host 校验（公网 DNS 即可）
- [ ] 7.2 新增账号：平台 Anthropic + 类型 API Key，填 refresh token（可选填 device key PEM，
      留空则由服务端生成并落库）；`credentials.base_url` 默认写 `https://relay.mirasim.ai`
- [ ] 7.3 **配置模型映射**（`credentials.model_mapping`）：relay 只服务自己的模型目录，
      不接受 Anthropic 官方模型 id（实测 `claude-sonnet-4-5` → 422 not supported）；
      不配映射则客户端的 `claude-*` 请求全部 422
- [ ] 7.4 一个 refresh token 只对一个账号：refresh 会轮换 token（`OnRotate` 已接账号持久化），
      重复配置会互相踢登录
- [x] 7.5 设备身份不必从客户端导出：`mirasim_device_key` 留空由服务端生成即可（2026-09-24 实测：
      全新 ed25519 key `deviceID=j4CdJBupXYECUxXtVrcxoP` + 真实 access token → `/v1/messages`
      与 `/v1/messages/count_tokens` 均 200）

## 8. 生产持久化（2026-09-24 勘察 + 方案，详见 `docs/MIRASIM_RELAY_UPSTREAM.md` §9）

- [x] 8.1 **唯一持久状态落在 Postgres `accounts.credentials`**（`mirasim_refresh_token` /
      `mirasim_device_key` / `base_url`，开关在 `extra.mirasim_relay_enabled`）：无新表、无新列、无迁移；
      进程内 `mirasimRelayRuntimeAcquire` 缓存冷启动即为空，首个 relay 请求自动换 ticket
      ⇒ 重启/换容器**无需预热、无需额外持久化**；生产不依赖 macOS keychain 或
      `MIRASIM_APP_SECRET_KEY`（只有桌面客户端才有那层密封存储）
- [x] 8.2 **单持有者约束已取证**：refresh token 是账号级的，同一 token 不能被两个 sub2api 账号 /
      两个进程 / "sub2api + 桌面客户端"同时持有，否则互相踢成 401。生产现状：nginx
      `code-sicts-shop.conf` 的 `sub2api_api` 只指向 `127.0.0.1:18373`；另两个候选容器
      （`18372`/`18371`）与它**共用同一个 Postgres 与 Redis**（env 逐键一致）但不接线上流量
      ⇒ 今天安全，**蓝绿/切流/加副本前必须先确认 relay 账号只有一个活跃持有者**
- [x] 8.3 **构建已本机验证**：`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags embed
      -ldflags "-s -w -X main.Version=0.1.391 -X main.MinFrontendVersion=0.1.380" -trimpath`
      → 静态 ELF；产物内 `mirasim_refresh_token` / `mrs-seal-v1` / `mirasim_relay_persist_failed`
      均可检出，且已 embed 本轮前端 dist（新 chunk `AccountsView-C9yrXbGB.js` 在、旧
      `AccountsView-DEtjWFwt.js` 不在）。**最终产物：**
      `924e4c5e165c9ad904130b5dab0dc1acb63a9ad73a3090ee1ef4a7e82737c6d0`（123715746 B，
      本机 `/tmp/sub2api-0.1.391-mirasim-924e4c5e1-linux-amd64`）；前端 dist 包
      `/tmp/dist-0.1.391-mirasim.tgz`（17397514 B，md5 `0fbf8558a853ae04db107429c36ccb86`，172 个文件）。
      线上后端为 `1d78d046…`（123547810 B，源码 `a33b47ef9`），后端源码差只有 `fd5e71536`（mirasim）。
- [x] 8.3b **前端增量已证明受控**：相对线上 `0.1.391-ocgremove-7e1ee1c92`，逻辑变更仅
      `AccountsView-*.js` / `AccountsView-*.css` / `index-*.css`(+`.bg-sky-600`) / zh-en i18n chunk
      / `index.html`，其余 159 个文件的差异全是 chunk 哈希级联（剥哈希后逐文件 md5 相同）；
      线上 AccountsView 中 `mirasim` 命中数 0，新 chunk 中为 1。
- [x] 8.4 前置事实：三个线上 api 容器里 `mirasim` 字符串命中数为 **0**、
      `/opt/sub2api/frontend/current` 下无 `mirasim` 命中、DB 中 mirasim 账号数 **0**
      ⇒ 能力尚未上线，本次发布是一次全新的功能上线（而不是修复）
- [ ] 8.5 **执行发布**（runbook 已固定：`docs/MIRASIM_RELAY_UPSTREAM.md` §9.4；二进制热替换 +
      前端 release 目录 + 管理端录入账号与模型映射）。前置探针：未认证
      `GET /api/v1/admin/mirasim/oauth/providers` 现为 404，发布后应为 401（无需凭据的差分校验）
- [ ] 8.6 上线后回归：管理端"测试账号"通过 → 真实请求 200 → 观察
      `mirasim_relay_credential_updated` 落库事件 → 重启容器再发一次请求确认冷启动可自愈
- [ ] 8.7 回滚演练确认：删 `credentials.mirasim_refresh_token` 即退化回普通 Anthropic apikey 账号，
      二进制回滚无需数据修补

## 9. 授权登录（取 refresh token，2026-09-24）

- [x] 9.1 新增 `backend/internal/service/mirasim_oauth_service.go`：
      `ListProviders`（代理 `/auth/oauth/providers`，剔除非法/重复 ID，空列表 502）、
      `GenerateAuthURL`（loopback 占位 `redirect_uri` + 每次新 `state`）、
      `StartEmailCode`（`/auth/code`）、`CompleteEmailCode`（`/auth/verify`）、
      `CompleteWithCallbackURL`、`CompleteWithRefreshToken`、`ParseMirasimRefreshToken`
      （纯本地 JWT 校验：形态 / `token_type` / `exp` / 身份）、
      `ExtractMirasimRefreshTokenFromCallbackURL`（完整 URL / `?`/`#` 裸参数串 / token 原文）
- [x] 9.2 新增 `backend/internal/handler/admin/mirasim_oauth_handler.go`：
      `ListProviders` / `GenerateAuthURL` / `Complete` / `StartEmailCode` / `CompleteEmailCode`，
      全部走 `response.ErrorFrom` 统一错误映射
- [x] 9.3 路由与注入：`internal/server/routes/admin.go` 新增
      `/admin/mirasim/oauth/{providers,auth-url,complete,email/code,email/verify}`；
      `handler/wire.go`、`handler/handler.go`、`service/wire.go`、`cmd/server/wire_gen.go` 接线
- [x] 9.4 新增 `backend/internal/service/mirasim_oauth_service_test.go`（`//go:build unit`）17 个用例；
      断言纪律：HTTP 状态（`appErr.Code`）与业务码（`appErr.Reason`）分开校验，
      5xx 必须 `NotContains` 上游细节，token 全部现场构造假 JWT
- [x] 9.5 前端：新增 `src/api/admin/mirasim.ts`、
      `src/components/account/MirasimOAuthOnboarding.vue`（三个页签：浏览器授权 / 邮箱验证码 /
      粘贴 Token），并接入 `CreateAccountModal.vue` / `EditAccountModal.vue`
      （授权成功即回填 refresh token、自动开启 relay 开关；create 时用 `identity.name|email`
      预填空账号名）
- [x] 9.6 i18n：zh/en `admin.accounts.mirasimOAuth.*`
- [x] 9.7 前端单测：`MirasimOAuthOnboarding.spec.ts` 9 个用例
- [x] 9.8 验证：`go build ./...` 通过；单测 17/17（非侵入式临时拷贝方式，见 6.2）；
      `npx vue-tsc --noEmit` exit=0；`npx vitest run src/components/account src/features/admin-accounts`
      → 14 文件 / 192 用例全绿
- [ ] 9.9 **真实上游授权登录端到端未执行**（有意为之）：`/auth/code` 会真发邮件、
      `/auth/verify` 会消耗 OTP、`/auth/oauth/*/login` 需人工在浏览器完成授权，
      无法在自动化里零副作用复现；因此该路径的正确性依据是「上游只读探测 + 单测 + 前端 spec」，
      真机验证留到发布后由管理员首次登录时确认（记录在 `docs/MIRASIM_RELAY_UPSTREAM.md` §10.5）
- [ ] 9.10 发布后回归：管理员在管理端用 GitHub/Google 授权登录一次 → 凭据回填 →
      保存账号 → 账号测试通过（与 8.5 / 8.6 同批次执行）
