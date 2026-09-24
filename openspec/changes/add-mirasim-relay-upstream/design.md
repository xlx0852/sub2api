## Context

Mirasim relay 的 wire protocol 由参考实现 `github.com/Essaim8/mirasim2api` 定义，
且**未公开文档化**：`docs/` 站点只描述产品能力。因此本变更的协议细节全部以参考实现
`internal/mirasim/{crypto.go,client.go}` 为唯一权威，并用其 `crypto_test.go` /
`client_test.go` 中的钉住向量作为回归基准。

已核对的协议事实（来自参考实现源码，逐字节）：

- `mrs-sig-v2` canonical string 为 10 行 `\n` 连接：
  `["mrs-sig-v2", UPPER(method), path, ts, nonce, deviceID, clientVersion,
  sha256hex(credential), metaLine, sha256hex(body)]`；`meta` 为空时第 9 行为**空行**，
  非空时按键排序拼接 `k\0v\0…` 后再 sha256。
- `nonce` = 12 字节随机数的 base64url 无填充；`x-mirasim-ts` 为毫秒时间戳。
- `deviceID` = `base64url(sha256(base64(SPKI DER)))[:22]`，签名算法 Ed25519，
  `x-mirasim-sig` = base64url(签名)。
- `mrs-seal-v1`：临时 X25519 公钥 + relay 静态 X25519 公钥 → ECDH shared；
  `key = HKDF-SHA256(ikm=shared, salt=临时公钥, info="mrs-seal-v1", L=32)`；
  ChaCha20-Poly1305，AAD = `"mrs-seal-v1\n" + UPPER(method) + "\n" + path`；
  明文是固定字段顺序的 JSON
  `{"x-mirasim-device","x-mirasim-ts","x-mirasim-nonce","x-mirasim-sig"}`；
  输出 `base64url(ephPub(32) || nonce(12) || ct || tag(16))`。
- 明文签名头（5 个，含 `x-mirasim-client`）**只在 `POST /v1/device/session` 发送**；
  模型请求只发 `Authorization: Bearer <ticket>` + `x-mirasim-client` + `x-mirasim-enc`。
- 模型请求路径为 `/v1/messages`，**无 query**；参考实现只发这一条路径。
- 默认常量：relay `https://relay.mirasim.ai`、auth `https://auth.mirasim.ai`、
  clientVersion `0.0.322`、seal 公钥
  `HlyNMMeGXryasYLJuYQ/9ksCD4AYVVy1zXKAtJdpJn4=`；access token 提前 120s、
  ticket 提前 60s 刷新；`expires_in` 缺省 access=3600s / ticket=900s。
- 参考实现的设备身份是**进程级共享**（`store.LoadOrCreateDeviceKey()`），不是
  账号级；`pool` 在 refresh token 轮换后通过 `UpdateAccountToken` 落库。

## Goals / Non-Goals

- Goals:
  - Mirasim relay 作为 `anthropic + apikey` 上游被正常调度、转发、计费、重试。
  - 协议实现与参考实现逐字节等价（签名/密封/设备身份可被单测钉住）。
  - 401 可自愈（不永久禁用账号），refresh token 轮换不丢。
  - 非 Mirasim 账号零行为变化。
- Non-Goals:
  - 不实现 `/v1/models` 模型目录拉取与上游模型发现（seal AAD 与 GET 路径未经上游确认）。
  - 不实现 upstream billing probe（`/v1/sub2api/billing` 属 sub2api 私有约定）。
  - 不新增 `mirasim` 平台或账号类型，不改 quota schema。
  - 不实现 mrs1 本地密文存储（本项目 credentials 既有惯例是明文存 api_key/base_url）。

## Decisions

- Decision: 账号识别 = `platform==anthropic && type==apikey && credentials.mirasim_refresh_token`
  非空，且未被 `extra.mirasim_relay_enabled=false` 显式关闭。
  Alternatives: 新增 `mirasim` 平台（`PlatformKimi` 足迹约 40 个后端文件 + quota schema
  同步 + 前端 registry），成本远高于收益；新增账号类型 `AccountTypeUpstream` 会落入
  `GetAccessToken` 的 default 分支报错。
- Decision: 用 `credentials.base_url` 承载 relay 地址（复用既有 Custom Base URL 机制与
  URL 白名单校验），不新增 `mirasim_relay_url` 凭据。
- Decision: `tokenType` 仍返回 `"apikey"`，靠 `isMirasimRelayAccount(account)` 分流。
  理由：`gateway_anthropic_passthrough.go:61`、`gateway_count_tokens.go:59` 对
  `tokenType != "apikey"` 直接报错，返回新值会破坏这两条路径。
- Decision: 签名注入放在每个构造器的**最后一步**（`debugLogGatewaySnapshot` 之后、
  `return` 之前），且用构造器返回的最终 body；mirasim 请求清空 `req.URL.RawQuery`
  以保证 `path` 与上游期望一致（`?beta=true` 是 sub2api 自身追加的）。
- Decision: 鉴权头统一 `Authorization: Bearer <ticket>` 并删除 `x-api-key`，不依赖
  `anthropic_apikey_auth_scheme` 配置；避免 ticket 与 api_key 同时出现。
- Decision: 设备私钥放 `credentials.mirasim_device_key`（PKCS8 PEM），首次使用时生成
  并落库；在 runtime 的 per-account 锁内生成，避免并发双写。
- Decision: 运行时缓存为进程内 `accountID → client`（含 access token / ticket 及
  fingerprint）；网络出口复用 `s.httpUpstream.Do(...)`，从而继承账号代理隔离、
  并发记账与 URL 白名单校验。
- Decision: 401 在 `HandleUpstreamError` 的 `case 401` 内、`AccountTypeOAuth` 分支
  **之前**插入 Mirasim 分支：丢弃该账号 client 缓存 + `SetTempUnschedulable(10min)`，
  `shouldDisable = true`（仅本次请求失败转移，账号仍 active）。
- Decision: 轮换落库先 `accountRepo.GetByID` 再读改写，避免整列快照回写竞态
  （`ratelimit_service.go` 已就此有明确注释）。

## Risks / Trade-offs

- [协议漂移] relay 侧升级签名/密封算法 → 协议原语集中在单文件，且用参考实现向量钉住；
  任何变更都会在单测中暴露。
- [上游未文档化行为] 模型请求必须 `path` 与签名一致，若上游要求 query 参与签名，
  我们的清空 query 策略会失配 → 参考实现本身不带 query，按同构实现，风险可控。
- [401 无法自愈] 若 refresh token 本身失效（非 ticket 过期），冷却期后仍 401 →
  管理员可见连续的临时不可调度日志，不误判为「账号被封」。
- [refresh token 轮换丢失] 进程崩溃导致新 refresh token 未落库 → 参考实现同样语义；
  落库为同步调用，失败只记日志。
- [代理/白名单] `auth.mirasim.ai` 与 `relay.mirasim.ai` 都需在 URL 白名单内（若启用）
  → 在文档与 proposal 中明示。

## Verification

- 协议层：`backend/internal/service/mirasim_protocol_test.go` 用参考实现
  `crypto_test.go` / `client_test.go` 的钉住向量（seed `0x01..0x20` →
  `pub = "MCowBQYDK2VwAyEAebVWLo/mVPlAeLES6KmLp5AfhTrmlb7X4OORC60ElmQ="`、
  `deviceID = "JUdfAMozPES5oPh9G8P13N"`、canonical 10 行精确串）做逐字节断言；
  密封测试反解 blob 校验字段顺序与 AAD 绑定（改 path / 改方法必须解不开）。
- 接入层：`mirasim_relay_test.go` 的
  `TestMirasimRelayInjectionProducesVerifiableSealedHeaders` 从**已注入的 http.Request**
  出发复算 canonical、Ed25519 验签、解 seal，再把解出的 4 个头与请求头逐字段比对，
  因此「注入点是否用了最终 body / 最终 path」是被断言过的，不靠人眼检查。
  `TestMirasimRelayInjectionSkipsOtherAccounts` 覆盖 5 类非目标账号（普通 anthropic
  apikey、anthropic oauth、openai apikey 带同名凭据、`extra` 显式关闭、缺 refresh token），
  保证不串上游。
- 错误语义：401 / 403 分支只做 `SetTempUnschedulable`，不断言「账号被封」。
- 前端：`credentialsBuilder.spec.ts` 覆盖写入规则（含「留空保留原值」「禁用写 false +
  base_url 回退」），`EditAccountModal.spec.ts` 覆盖弹窗端到端提交载荷。
- 本仓库 `internal/service` 有若干与本变更无关的陈旧测试文件引用了已移除的 API，
  包级 `go test` 无法构建。验证采用非侵入方式：把 `backend/` 硬链接复制到 `/tmp`，
  仅在副本中移走陈旧测试文件后运行；仓库内未改动这些文件。

## Migration Plan

1. 无数据库迁移。新增账号：Anthropic + API Key，`credentials.base_url` =
   `https://relay.mirasim.ai`，`credentials.mirasim_refresh_token` = 用户 refresh token。
2. 存量账号不受影响（无该凭据即不进入 Mirasim 分支）。
3. 回滚：删除该凭据即退化为普通 Anthropic apikey 账号；代码回滚无需数据修补。

## Open Questions

- relay 是否对 refresh token 轮换提供旧值宽限期（未验证）——不影响本地实现，
  只影响并发窗口下的重试成功率。

## OAuth 授权登录（补充，2026-09-24）

### 上游流程事实（只读实测）

- `GET https://auth.mirasim.ai/auth/oauth/providers` → `{"providers":["github","google"]}`；
  解析同时兼容裸数组，非法/重复 ID 被剔除。
- `POST https://auth.mirasim.ai/auth/oauth/{provider}/login` 返回上游签发的授权链接，
  其中 `state` 由**上游**生成，sub2api 无法校验。
- `redirect_uri` 走上游 `OAUTH_REDIRECT_ALLOWLIST`，只放行 loopback 与 `*.mirofish.ai`；
  换成非 loopback 地址上游直接 400 ⇒ 采用 `http://127.0.0.1:9/callback` 占位回调，
  由管理员从浏览器地址栏复制完整链接交回服务端。
- `/auth/code`（发邮件）与 `/auth/verify`（消耗 OTP）**未在验证过程中触发**
  （会产生真实副作用），其请求/响应形状按上游文档实现，属未验证项。

### 决策

- Decision: 登录环节只做**本地 JWT 解析**，绝不调用 `/auth/refresh`。
  Alternatives: 登录后立刻刷新一次以验证凭据 —— 被否，refresh token 是单值轮换的，
  会让管理员本地客户端掉登录，且与「单持有者」约束冲突。
- Decision: `state` 只作本地关联 ID/调试线索，不做 CSRF 校验，因为上游会覆盖它；
  信任边界收敛为「已认证管理员 API + 管理员自行粘贴的回调 URL」。
- Decision: 授权登录不落库、不建号，只把凭据回填账号表单，最终仍走
  `POST /admin/accounts`（复用敏感凭据合并与调度器快照同步）。
- Decision: 网络出口复用 `HTTPUpstream`（`s.httpUpstream.Do`），从而继承代理隔离与
  host 校验，不新增 HTTP 客户端。
- Decision: 复用 `mirasimHTTPError` / `mirasimTruncate` 的错误归类方式，
  只按 HTTP 状态码判定，错误信息在 5xx 时不外泄上游细节。
- Decision: provider 走白名单正则（`^[a-z0-9][a-z0-9_-]{0,31}$`）后拼接路径，
  不硬编码 github/google，便于上游扩展。

### Risks / Trade-offs

- [未验证路径] 邮箱验证码路径的形状来自上游文档而非实测 ⇒ 解析做了宽容取值
  （`refresh_token` / `token`，兼容 `data`/`result` 包裹），并在文档中标注未验证。
- [凭据出现在响应体] 管理端接口会返回 refresh token 明文 ⇒ 与手工粘贴等价，
  仅在已认证的管理端可见；日志与错误信息一律不含 token。
- [占位回调页面打不开] 属预期行为，已在组件提示文案与文档中说明。

### Verification（OAuth 部分）

- `backend/internal/service/mirasim_oauth_service_test.go`（`//go:build unit`）17 个用例：
  回调链接三种形态提取、JWT 形态/类型/过期/身份缺失校验、`state` 每次不同、
  loopback `redirect_uri`、provider/email 归一化、上游错误分类（4xx 透传细节、
  5xx 不外泄）、providers 兼容解析与空列表 502、代理仓库缺失 400、
  `CompleteWithRefreshToken` 方法签名不含 `ctx`（证明登录环节零上游调用）。
- 前端 `MirasimOAuthOnboarding.spec.ts` 9 个用例（含断言登录链路只打 4 个 admin 端点，
  不出现任何 token 明文日志）。
