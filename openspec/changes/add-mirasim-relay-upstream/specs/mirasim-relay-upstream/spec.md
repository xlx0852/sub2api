## ADDED Requirements

### Requirement: Mirasim Relay Account Identity

系统 SHALL 把满足「平台为 `anthropic`、账号类型为 `apikey`、`credentials.mirasim_refresh_token`
非空、且 `extra.mirasim_relay_enabled` 未被显式设为 `false`」的账号识别为 Mirasim relay
账号，且 SHALL NOT 为此新增平台、账号类型或数据库结构变更。

#### Scenario: Mirasim 账号被识别

- **WHEN** 一个 anthropic/apikey 账号带有非空 `mirasim_refresh_token` 凭据
- **THEN** 网关按 Mirasim relay 协议构造上游请求（Bearer ticket + 密封签名头）

#### Scenario: 未携带凭据的普通账号

- **WHEN** 一个 anthropic/apikey 账号没有 `mirasim_refresh_token` 凭据
- **THEN** 网关沿用既有 API Key 行为，不产生任何 Mirasim 头

#### Scenario: 显式关闭

- **WHEN** 账号 `extra.mirasim_relay_enabled` 为 `false` 但仍然保留 refresh token
- **THEN** 网关不进入 Mirasim 分支，且不删除该凭据

### Requirement: Mirasim Protocol Primitives

系统 SHALL 实现与参考实现逐字节等价的 Mirasim 协议原语：`mrs-sig-v2` canonical
string（10 行、meta 为空时第 9 行为空行、body 为空时为 sha256("")）、Ed25519 签名头、
`mrs-seal-v1` 密封（HKDF-SHA256 salt=临时公钥、info=`mrs-seal-v1`、ChaCha20-Poly1305、
AAD=`mrs-seal-v1\nUPPER(method)\npath`）、以及设备身份派生
（`deviceID = base64url(sha256(base64(SPKI DER)))[:22]`）。

#### Scenario: Canonical string 精确匹配

- **WHEN** 以 `POST /v1/messages`、时间戳 `1758000000123`、nonce `nonce-abc`、
  deviceID `dev-1`、clientVersion `0.0.322`、credential `test-credential`、
  空 meta、body `hello body` 构造 canonical string
- **THEN** 结果等于参考实现钉住的 10 行字符串（含第 9 行空行）

#### Scenario: 设备身份钉住向量

- **WHEN** 使用 seed `0x01..0x20` 的 Ed25519 私钥派生设备身份
- **THEN** `publicKeyB64` 等于 `MCowBQYDK2VwAyEAebVWLo/mVPlAeLES6KmLp5AfhTrmlb7X4OORC60ElmQ=`，
  `deviceID` 等于 `JUdfAMozPES5oPh9G8P13N`

#### Scenario: 密封布局与 AAD 绑定路径

- **WHEN** 把一个签名头集合密封为 `x-mirasim-enc`
- **THEN** 明文为固定字段顺序的 4 字段 JSON，且在 relay 侧用
  `mrs-seal-v1\nUPPER(method)\npath` 作为 AAD 能解密成功；AAD 中 method 或 path 变化则解密失败

#### Scenario: 设备私钥 PEM 往返

- **WHEN** 生成的 Ed25519 私钥序列化为 PKCS8 PEM 后再解析
- **THEN** 得到与原私钥一致的密钥；非法 PEM 返回错误而不是 panic

### Requirement: Mirasim Credential Chain

系统 SHALL 维护账号级凭据链 refresh token → access token → device ticket，
access token 提前 120 秒刷新、ticket 提前 60 秒重领，并在上游返回新 refresh token 时
把最新值持久化到账号凭据（先读取最新账号再合并写入）。

#### Scenario: 取 ticket 前先取 access token

- **WHEN** 账号需要向上游发请求且本地无有效 ticket
- **THEN** 先用 refresh token 调 `{auth}/auth/refresh` 取 access token，
  再用 Bearer access token 与明文签名头调 `{relay}/v1/device/session` 取 ticket

#### Scenario: refresh token 轮换落库

- **WHEN** `/auth/refresh` 响应携带与本地不同的 `refresh_token`
- **THEN** 内存缓存更新为新值，并把新值写入该账号 `credentials.mirasim_refresh_token`

#### Scenario: 轮换落库失败不影响本次请求

- **WHEN** 持久化新 refresh token 时数据库写入失败
- **THEN** 本次请求仍使用内存中的新 token 继续转发，并记录告警日志

#### Scenario: 首次使用生成设备密钥

- **WHEN** Mirasim 账号缺少 `credentials.mirasim_device_key`
- **THEN** 系统生成 Ed25519 设备密钥、持久化 PKCS8 PEM，并在同一账号的并发请求中复用同一身份

### Requirement: Mirasim Gateway Request Signing

系统 SHALL 在 Anthropic 上游请求构造完成后的最后一步对 Mirasim 账号注入签名材料：
清空请求 query、以最终 `path` 与**最终 body** 生成签名头、密封为 `x-mirasim-enc`，
设置 `Authorization: Bearer <ticket>` 与 `x-mirasim-client`，并删除 `x-api-key`。
该行为 MUST 覆盖 messages、as_chat、as_responses、anthropic passthrough 与 count_tokens
五条请求构造路径。

#### Scenario: Messages 转发

- **WHEN** 请求被路由到 Mirasim 账号并由主请求构造器生成上游请求
- **THEN** 发出的请求带 `Authorization: Bearer <ticket>`、`x-mirasim-client`、
  `x-mirasim-enc`，不带 `x-api-key`、不带明文签名头、URL 无 query

#### Scenario: Body 改写后签名一致

- **WHEN** 构造器在生成签名前已改写请求体（模型映射、billing header、beta 处理等）
- **THEN** 签名与密封使用的 body 哈希等于最终发送的 body 哈希

#### Scenario: count_tokens 与 passthrough 路径

- **WHEN** count_tokens 或 anthropic API Key passthrough 路径命中 Mirasim 账号
- **THEN** 这两条路径同样注入 Mirasim 签名材料，且不因 `tokenType` 检查报错

### Requirement: Mirasim 401 Recovery Semantics

系统 SHALL 在 Mirasim relay 账号收到上游 401 时失效本地 ticket 与 access token 缓存、
把账号置为临时不可调度冷却（默认 10 分钟）并让本次请求失败转移，且 MUST NOT 通过
`SetError` 永久禁用该账号。

#### Scenario: 401 触发冷却而非永久禁用

- **WHEN** Mirasim 账号的上游请求返回 401
- **THEN** 账号状态保持 active 且带临时不可调度截止时间，本地凭据缓存被清除

#### Scenario: 冷却后自愈

- **WHEN** 冷却结束且账号重新被调度
- **THEN** 系统重新走 refresh token → access token → ticket 链路，不依赖任何账号状态修复操作

### Requirement: Mirasim Account Diagnostics Support

系统 SHALL 让管理员账号连接测试对 Mirasim 账号可用（使用真实 ticket 签名），
并 SHALL 把 Mirasim 账号排除在上游计费探针之外。

#### Scenario: 账号测试可用

- **WHEN** 管理员对 Mirasim 账号触发连接测试
- **THEN** 测试请求携带 ticket 与密封签名头，并返回真实的流式判定结果

#### Scenario: 计费探针不误报

- **WHEN** 一个 Mirasim 账号被评估是否可参与上游计费探针
- **THEN** 该账号被排除，不向 relay 发送 `/v1/sub2api/billing` 请求

### Requirement: Mirasim Relay Account Configuration UI

系统 SHALL 在后台账号表单的 Anthropic API Key 分支提供 Mirasim relay 开关与
refresh token、设备密钥、relay/auth 地址与 client version 输入项，并把它们写入
账号凭据与 `extra`；关闭开关时 SHALL NOT 删除已保存的 refresh token。

#### Scenario: 启用并保存

- **WHEN** 管理员开启 Mirasim relay 开关并填写 refresh token 与 relay 地址
- **THEN** 账号保存后 `credentials.mirasim_refresh_token`、`credentials.base_url` 落库，
  `extra.mirasim_relay_enabled` 为 `true`

#### Scenario: 关闭开关保留凭据

- **WHEN** 管理员关闭 Mirasim relay 开关并保存
- **THEN** `extra.mirasim_relay_enabled` 变为 `false`，已保存的 refresh token 不被清除

### Requirement: Mirasim OAuth Onboarding

系统 SHALL 在管理端提供 Mirasim 授权登录，使管理员无需手工导出 refresh token 即可
从上游登录并取回凭据；该流程 SHALL 只生产凭据并回填账号表单，SHALL NOT 直接创建
账号，也 SHALL NOT 新增上游端点之外的网络出口。

#### Scenario: 列出可用登录方式

- **WHEN** 管理员请求管理端 Mirasim OAuth providers 接口
- **THEN** 服务端代理上游 `GET /auth/oauth/providers`，剔除非法与重复的 provider ID 后返回列表，全空时返回 502 与 `MIRASIM_OAUTH_PROVIDERS_EMPTY`

#### Scenario: 生成授权链接

- **WHEN** 管理员选择 provider 请求授权链接
- **THEN** 返回 `{auth}/auth/oauth/{provider}/login?redirect_uri=…&state=…`，其中 `redirect_uri` 固定为 loopback 占位符（上游白名单只放行 loopback），`state` 为每次请求新生成的随机值

#### Scenario: 从回调链接取凭据

- **WHEN** 管理员粘贴浏览器地址栏中的完整回调链接
- **THEN** 服务端在本地解析出 `refresh_token`（不调用上游），校验 JWT 形态、`token_type` 与 `exp` 后返回凭据与本地解析出的身份信息

#### Scenario: 直接粘贴 refresh token

- **WHEN** 管理员跳过浏览器授权、直接粘贴 refresh token 原文
- **THEN** 服务端执行同样的本地校验并返回同一凭据结构

#### Scenario: 邮箱验证码路径

- **WHEN** 管理员使用邮箱验证码方式（先触发 `/auth/code`，再提交 `/auth/verify`）
- **THEN** 服务端把上游返回的 refresh token 归一化为同一凭据结构返回

#### Scenario: 经指定代理访问上游

- **WHEN** 管理员在授权登录请求中指定了代理 ID
- **THEN** 服务端经该代理访问 `auth.mirasim.ai`；代理仓库不可用或代理不存在时返回明确错误而非静默直连

#### Scenario: 非法输入

- **WHEN** provider 不匹配允许的字符集，或回调链接/refresh token 缺失、不可解析、已过期、类型错误
- **THEN** 返回 400 与该情形对应的专用业务码，且错误信息不含 token 内容

### Requirement: Mirasim OAuth Credential Safety

Mirasim 授权登录流程 SHALL 在登录环节避免调用上游 `/auth/refresh`（避免轮换掉
管理员本地客户端的登录），SHALL NOT 把 refresh token / access token 写入日志，
且 SHALL NOT 把上游 `state` 当作 CSRF 安全控制。

#### Scenario: 登录环节零刷新

- **WHEN** 管理员完成任意一条授权登录路径
- **THEN** 服务端不向上游发出 `/auth/refresh` 请求，凭据仅做本地解析

#### Scenario: 日志脱敏

- **WHEN** 授权登录流程记录日志
- **THEN** 日志只包含 `provider`、是否有 `name`/`plan`、`expires_at` 与长度这类结构性字段，不含 token 明文与邮箱

#### Scenario: state 不承担安全控制

- **WHEN** 上游用自己签发的 `state` 覆盖本地传入的 `state`
- **THEN** 服务端不因 `state` 不匹配而拒绝请求；信任边界为「已认证的管理员 API 调用 + 管理员自行粘贴的回调 URL」
