# Mirasim Relay 上游接入

Mirasim（`relay.mirasim.ai`）是一个第三方 Claude 中继。与普通 Anthropic API Key
账号的区别在于：模型请求不能只发静态 `x-api-key`，必须携带一份短期 device ticket
和一份把签名头密封后的 blob。本文件记录该上游的接入方式、协议事实与运维前置条件。

代码入口：

| 文件 | 作用 |
| --- | --- |
| `backend/internal/service/mirasim_protocol.go` | 纯协议原语（签名 / 密封 / 设备身份 / PEM） |
| `backend/internal/service/mirasim_relay.go` | 账号识别、配置解析、凭据链、请求头注入 |
| `backend/internal/service/mirasim_relay_runtime.go` | 进程内 `accountID → client` 缓存与 single-flight |
| `frontend/src/features/admin-accounts/form/credentialsBuilder.ts` | 表单 → credentials/extra 写入规则 |

## 1. 账号模型

Mirasim **复用现有 `anthropic` 平台 + `apikey` 账号类型**，没有新增平台、账号类型或
数据库列。识别规则唯一实现在 `isMirasimRelayAccount`：

```
platform == "anthropic" && type == "apikey"
  && credentials.mirasim_refresh_token 非空
  && extra.mirasim_relay_enabled 未被显式置为 false
```

`tokenType` 仍然返回 `"apikey"`。原因：`gateway_anthropic_passthrough.go` 与
`gateway_count_tokens.go` 都有 `tokenType != "apikey"` 的硬门禁，返回新值会直接让这两条
路径报错；分流改由 `isMirasimRelayAccount` 完成。

凭据与 extra 键：

| 位置 | 键 | 说明 |
| --- | --- | --- |
| credentials | `mirasim_refresh_token` | 必需；等价于永久凭据，响应中永不回显 |
| credentials | `mirasim_device_key` | 可选；Ed25519 PKCS8 PEM，留空由服务端生成并落库 |
| credentials | `base_url` | relay 地址，默认 `https://relay.mirasim.ai` |
| extra | `mirasim_relay_enabled` | 显式开关；`false` 时即使凭据仍在也不走 relay |
| extra | `mirasim_relay_url` / `mirasim_auth_url` / `mirasim_client_version` / `mirasim_seal_pubkey` | 可选覆盖，留空即用内置默认值 |

`mirasim_refresh_token` 与 `mirasim_device_key` 已加入
`account_credentials_redact.go` 的 `SensitiveCredentialKeys`，因此：

- 前端拿不到原值，编辑表单恒显示空，提交时**留空即保留原值**
  （后端 `MergePreservingSensitiveCreds`）；
- 只在需要轮换时填入新值。

## 2. 凭据链

```
refresh_token --POST {auth}/auth/refresh--> access_token (+ 轮换后的 refresh_token)
access_token --POST {relay}/v1/device/session--> ticket
ticket + sealed headers --POST {relay}/v1/messages--> 模型响应
```

- 提前刷新窗口：access token 120s、ticket 60s（与参考实现一致）。
- refresh token 轮换：响应里的新值会先 `GetByID` 读最新行再合并写回，避免整列快照回写竞态。
- 缓存是**进程内、按账号**的（`mirasim_relay_runtime.go`）；缓存的配置指纹变化
  （relay/auth 地址、clientVersion、seal 公钥、refresh token、device key）时自动重建。

## 3. 请求头

模型请求（`/v1/messages`、`/v1/messages/count_tokens` 等）只发：

```
Authorization: Bearer <ticket>
x-mirasim-client: <clientVersion>
x-mirasim-enc: <base64url seal blob>
Content-Type: application/json
```

明文签名头（`x-mirasim-device` / `x-mirasim-ts` / `x-mirasim-nonce` / `x-mirasim-sig`
/ `x-mirasim-client`）**只在 `/v1/device/session` 上发送**，模型请求上它们被密封进
`x-mirasim-enc`。签名原文（`mrs-sig-v2`，10 行 `\n` 连接）覆盖方法、path、时间戳、
nonce、deviceID、clientVersion、凭据哈希、meta、body 哈希；密封 blob（`mrs-seal-v1`）
的 AAD 绑定方法与 path。

由此产生两条实现约束，改动时请勿破坏：

1. **注入必须在请求构造器的最后一步**，用构造器最终返回的 body 与最终 path 签名。
2. **构造器里必须清空 `req.URL.RawQuery`**：sub2api 会自行追加 `?beta=true`，而签名只覆盖
   path，带上未签名的 query 会造成不一致。

四个注入点：`gateway_upstream_request.go`、`gateway_anthropic_passthrough.go`、
`gateway_count_tokens.go`（两个构造器）。

## 4. 错误语义

| 上游响应 | 处理 |
| --- | --- |
| 401 | 丢弃该账号的 ticket 缓存 → 冷却（复用 `oauth_401_cooldown_minutes`，`<=0` 取 10 分钟）→ 通知调度封禁（原因 `mirasim_401`）→ `SetTempUnschedulable`。**不做 `SetError` 永久禁用**，冷却后自动重取 ticket 自愈。 |
| 403 | 同上，原因 `mirasim_403`（device 身份或密封头被拒）。 |

若 refresh token 本身失效，冷却期后仍会 401 —— 这是"凭据不可用"而不是"账号被封"，
请检查 `mirasim_401` 日志再决定是否重新签发凭据。

两个服务的错误信封不同（实测）：

| 端点 | 401 响应体 |
| --- | --- |
| `auth.mirasim.ai/auth/refresh` | `{"detail":"invalid token: DecodeError"}`（FastAPI 风格） |
| `relay.mirasim.ai/v1/messages` | `{"error":{"code":"token_invalid","message":"invalid user token","type":"authentication_error"},"request_id":"..."}` |

因此错误归类**只按 HTTP 状态码（401/403）**，不允许解析 body 结构去猜错误类型。
`auth.mirasim.ai` 与 `relay.mirasim.ai` 是两套独立服务，不要假设它们的行为一致。

### 模型命名空间（重要）

relay 只服务它自己目录里的模型（实测 `kimi-k3`、`deepseek-flash` 走 `/v1/messages` 返回 200
且是 Anthropic Messages 响应体），**不接受 Anthropic 官方模型 id**。所以要给使用
Claude 模型名的客户端配模型映射：

```
credentials.model_mapping = {"claude-sonnet-4-5": "kimi-k3", "claude-opus-4-1": "deepseek-flash"}
```

未配置映射时客户端请求 `claude-*` 会被 relay 判为模型不可用（实测 422）：

```json
{"error":{"message":"model \"claude-sonnet-4-5\" is not supported at this time","type":"invalid_request_error"},"request_id":"...","type":"error"}
```

422/400 属于请求错误，**不是**鉴权错误：不会触发 401/403 的冷却与封禁逻辑，
也会直接透传给客户端（`shouldFailoverUpstreamError` 只对 401/403/429/529/5xx 生效，
见 `gateway_forward.go:39`），不产生 failover 也不计入账号失败统计。
但它对客户端来说就是“模型不可用”，所以请把模型映射配全。

## 5. 已排除的能力

- **上游模型同步**：relay 不提供 `/v1/models`（实测返回 404 `not_found_error`），
  `buildAnthropicUpstreamModelsRequest` 直接返回 `newUpstreamModelSyncUnsupportedError`，
  模型请在账号上手动配置。
- **上游计费探针**：`/v1/sub2api/billing` 是 sub2api 私有约定，relay 上不存在；
  `isUpstreamBillingProbeAccount` 对 Mirasim 账号返回 false，避免用错误的鉴权方式打扰上游。

模型请求路径固定为 `/v1/messages`（无 query），`/v1/messages/count_tokens` 实测存在。

## 6. 运维前置条件

1. **URL 白名单**（`config_security.go` 的 `URLAllowlistConfig.UpstreamHosts`）：
   - `relay.mirasim.ai` **必须加入**。网关侧的校验对象是账号 `base_url` 解析结果
     （APIKey 账号 `base_url` 为空时回落 `https://api.anthropic.com`，见
     `account_entity.go:1007 GetBaseURL`），注入发生在校验之后会把 host 改写成 relay，
     所以网关**不会**因为白名单缺 relay 而报错；但账号测试走的是另一条路径
     （`account_test_service.go:380` 校验 `relayCfg.RelayURL`），白名单缺 relay 时
     "测试账号"会直接报 `Invalid base URL`。两条路径都要求放行，因此：
     `upstream_hosts` 需同时包含 `api.anthropic.com` 与 `relay.mirasim.ai`。
   - `auth.mirasim.ai` 走 `repository/http_upstream.go` 的 host 校验，只需公网 DNS 可达。
   - **生产现状（2026-09-24 勘察）**：线上容器 env 为 `SECURITY_URL_ALLOWLIST_ENABLED=false`、
     `SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS=true`、`SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP=true`，
     且没有 `SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS`。白名单关闭时不校验 host，所以**当前无需改配置**；
     一旦有人把 `ENABLED` 打开，就必须同时补 `UPSTREAM_HOSTS`，否则账号测试会报 `Invalid base URL`。
2. 若账号设置了代理，relay/auth 请求都会经过该代理（复用 `resolveAccountProxyURL`）。
3. 新增账号：平台 Anthropic、类型 API Key（API Key 一栏可留空），打开 "Mirasim Relay"
   开关，填入 refresh token。device key 建议留空由服务端生成。
4. 配置模型映射（见上一节），否则 Claude 模型名的请求到 relay 后会被判模型不可用。
5. 一个 refresh token 只对应一个账号：refresh 会轮换 token（上游 `OnRotate` 回调已接到
   `accountRepo` 持久化）。同一个 refresh token 配到两个账号上会互相踢掉登录。
6. **设备身份不必从客户端导出**（2026-09-24 实测）：`mirasim_device_key` 留空由服务端自生成即可，
   详见 §9.3。
7. **生产出口已验证**（2026-09-24）：在生产机（`74.82.195.84`，hostname `cool-spin-1.localdomain`）
   上分别用 bogus Bearer（无凭据）与真实短期 access token（经 SSH SOCKS 隧道把本机 e2e 的 TCP
   出口切到生产机）探测过 `relay.mirasim.ai` / `auth.mirasim.ai`：DNS 解析、TLS 校验、到
   `/v1/messages` `/v1/messages/count_tokens` `/v1/device/session` 的路径均可达，结论与本地一致，
   **不存在 IP 层封锁**。因此部署时不需要为 relay/auth 配置额外出口代理。

## 7. 回滚

删除 `credentials.mirasim_refresh_token` 即退化为普通 Anthropic apikey 账号；代码回滚
不需要任何数据修补（无新列、无迁移）。

## 8. 验证方式

- `backend/internal/service/mirasim_protocol_test.go`：协议原语用参考实现的钉住向量逐字节断言。
- `backend/internal/service/mirasim_relay_test.go`：从已注入的 `http.Request` 出发复算
  canonical、Ed25519 验签、反解 seal，再与请求头逐字段比对；并覆盖 5 类非目标账号不被改写。
- `backend/internal/service/mirasim_relay_e2e_test.go`（`//go:build e2e`）：**真实联网**验证。
  缺凭据自动 skip，凭据只从 0600 文件读取：

  ```bash
  cd backend
  MIRASIM_E2E_DEVICE_KEY_FILE=/path/device_key.pem \
  MIRASIM_E2E_ACCESS_TOKEN_FILE=/path/access_token \
  MIRASIM_E2E_MODELS='kimi-k3,deepseek-flash' \
  go test -tags e2e ./internal/service/ -run 'Mirasim' -v -timeout 10m
  ```

  覆盖：设备身份推导（可与真实客户端的 deviceID 对拍）、真实注入 + `/v1/messages` 多模型探测、
  `/v1/messages/count_tokens`、生产入口 `mirasimRelayTicketToken` 的 tokenType 断言、
  生产 builder（`buildUpstreamRequestAnthropicAPIKeyPassthrough` /
  `buildCountTokensRequestAnthropicAPIKeyPassthrough`）端到端 200、另外两个 builder 的改写断言、
  伪造 refresh token 的错误归类。

  ⚠️ **运行注意**：默认只使用已提供的 access token，不会调 `/auth/refresh`；真实客户端的
  refresh token 是单飞轮换的，测试若替它刷新会让本机客户端掉登录。务必先提供
  `MIRASIM_E2E_ACCESS_TOKEN_FILE`，不要为了省事只给 refresh token。注意测试会把 access token
  的本地过期时间虚拟设为 45 分钟后，所以**只应在 token 真实有效期内跑**，否则会静默打到
  `/auth/refresh` 造成轮换。

- **换出口验证（验证某个部署点能不能直连 relay）**：e2e 客户端用 `http.Client{}`（nil Transport
  ⇒ `http.DefaultTransport`），因此天然尊重 `HTTPS_PROXY`/`ALL_PROXY`。把出口切到目标机器即可
  在目标机器的网络视角下复跑真实链路，无需在目标机装 Go 工具链：

  ```bash
  ssh -f -N -D 127.0.0.1:11080 root@<target-host>
  curl -s --socks5-hostname 127.0.0.1:11080 https://api.ipify.org   # 复核出口 IP
  cd backend && HTTPS_PROXY=socks5h://127.0.0.1:11080 \
    MIRASIM_E2E_DEVICE_KEY_FILE=/path/device_key.pem \
    MIRASIM_E2E_ACCESS_TOKEN_FILE=/path/access_token \
    MIRASIM_E2E_MODELS='kimi-k3,deepseek-flash' \
    go test -tags e2e ./internal/service/ -run 'Mirasim' -v -timeout 10m
  ```

  2026-09-24 用该方法在生产机出口上跑通 4/4（见 `tasks.md` 6.8）。
- `frontend/.../__tests__/credentialsBuilder.spec.ts`、`EditAccountModal.spec.ts`：表单写入规则与
  弹窗提交载荷。

> 本仓库 `internal/service` 存在若干与本接入无关的陈旧测试文件（引用了已移除的 API），
> 包级 `go test` 无法直接构建。建议在临时副本中移走这些文件后运行，仓库内不要"顺手修好"它们
> —— 那属于另一个变更。

## 9. 生产持久化（2026-09-24 勘察结论与发布方案）

回答的问题是：这套能力怎么在线上**长期存活**（重启、换容器、换版本、凭据轮换之后都还有效）。

### 9.1 状态在哪

| 层 | 内容 | 重启/换容器后 |
| --- | --- | --- |
| Postgres `accounts.credentials` | `mirasim_refresh_token`、`mirasim_device_key`、`base_url`；开关在 `extra.mirasim_relay_enabled` | **唯一持久状态**，无新表 / 无新列 / 无迁移 |
| 进程内 `mirasimRelayRuntimeAcquire` 缓存 | `accountID → client`（TTL 1h、上限 512、single-flight） | 冷启动即为空，**不需要预热**：首个 relay 请求按 DB 凭据走一次 `/v1/device/session` 换 ticket。e2e 每次都是空缓存起步，已反复验证 |
| Redis | 只放调度器快照 `sched:acc:<id>` | 由 `UpdateCredentials` 触发的 `syncSchedulerAccountSnapshot` 自动刷新；**直接改 SQL 时必须手工删该键**（同 `AGENTS.md` 既有坑） |

轮换回写链（已实现，见 `mirasim_relay_runtime.go:139 mirasimPersistCallbacks`）：client `OnRotate`
→ 异步 goroutine → `GetByID` 重读防脏写 → `UpdateCredentials` → 快照刷新。写失败会删掉去重记录让
下一次请求重试。**残留风险**：异步落盘有一个极窄崩溃窗口，最坏落后一次轮换，此时 DB 里的
refresh token 已失效，需要重新取证（不会静默发错请求，表现为持续 401）。

生产**不依赖任何本机密钥库**：与桌面客户端不同，sub2api 不读 macOS keychain、不需要
`MIRASIM_APP_SECRET_KEY`；DB 就是唯一凭据存储，因此备份/迁移数据库即备份了该能力。

### 9.2 单持有者约束（最重要的一条）

refresh token 是**账号级**的：谁刷新谁拿到新值。同一个 token 同时给两个 sub2api 账号、两个
sub2api 进程，或"sub2api + 桌面客户端"共用，都会互相把对方踢成 401。

生产现状（2026-09-24 勘察）：nginx `code-sicts-shop.conf` 的 `upstream sub2api_api` **只指向**
`127.0.0.1:18373`（容器 `sub2api-api-candidate-0.1.391-noinject-a33b47ef9`）。同时还有 `18372`
`18371` 两个候选容器在跑，它们与 18373 **共用同一个 Postgres 与 Redis**（逐键比对 env 一致），
但只出现在 `.bak` 备份 conf 里，当前不接线上流量。

- 今天只有一个进程会服务 relay 账号 → 安全。
- 蓝绿切换 / 切流 / 加副本时**若两个实例同时服务同一账号，就会互相踢**。切之前先停旧实例的
  relay 流量（或临时 `mirasim_relay_enabled=false`）；必须双活时给每个实例配不同账号。
- 与桌面客户端共存：只能 ① 生产独占、客户端登出该账号，或 ② 用另一个 Mirasim 账号做生产专用。
  ⚠️ relay 权益是否随订阅绑定到账号、新账号能否使用 relay：**未验证**。

### 9.3 凭据获取

本项目只消费 refresh token，不实现登录。可得途径（按侵入性排序）：

1. 管理端表单直接粘贴 refresh token（已有字段）。
2. **管理端内置授权登录（已实现，见 §10）**：浏览器授权 / 邮箱验证码 / 直接粘贴 token
   三条路径，最终都收敛到 refresh token 并回填表单。
3. 手工从桌面客户端导出（§9.2 的单持有者约束意味着导出后不要再让客户端登录）。

上游端点形状（供核对）：`POST {auth}/auth/code {"email"}` → `POST {auth}/auth/verify
{"email","code"}` → 响应含 `access_token` / `refresh_token`。依据是上游开源实现
`mirasim2api`（`internal/admin/oauth.go:332`、`:366`，`internal/mirasim/client.go:18
DefaultAuthURL`）。截至 2026-09-24，`/auth/oauth/providers` 与
`/auth/oauth/{provider}/login` 已只读实测；`/auth/code`（会真发邮件）与
`/auth/verify`（会消耗 OTP）**有意未触发**。

**设备身份不需要从客户端导出**（2026-09-24 实测）：用 `openssl genpkey -algorithm ed25519`
新生成一把 relay **从未见过**的 device key（`deviceID=j4CdJBupXYECUxXtVrcxoP`），配合调用方
提供的真实 access token 请求 `/v1/messages` 与 `/v1/messages/count_tokens` → **均 200**。
所以 `mirasim_device_key` 留空、由服务端首请求生成并落库即可，不需要搬运客户端状态。

### 9.4 发布步骤（构建与增量取证已完成；**2026-09-24 已执行，记录见 9.4.5**）

#### 9.4.1 基线事实（2026-09-24 实测，非推测）

| 项 | 线上当前 | 本轮待发 |
| --- | --- | --- |
| 后端容器 | `sub2api-api-candidate-0.1.391-noinject-a33b47ef9`（唯一接流：nginx `upstream sub2api_api → 127.0.0.1:18373`） | 同容器热替换 |
| 挂载 | `Type=bind`、`RW=true`：`/opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api` → `/app/sub2api` | **挂载源是一个文件路径**，新 release 目录不会被自动挂载 |
| 后端二进制 | 123547810 B，sha256 `1d78d046…b1800` | 123715746 B，sha256 `924e4c5e165c9ad904130b5dab0dc1acb63a9ad73a3090ee1ef4a7e82737c6d0` |
| 后端源码差 | `a33b47ef9`（= HEAD 的祖先） | `a33b47ef9..HEAD` 的 **backend 改动只有 `fd5e71536`（mirasim）**，`git log --oneline a33b47ef9..HEAD -- backend/` 实测 1 条 |
| 前端 | `current → releases/0.1.391-ocgremove-7e1ee1c92`（`rollback → 0.1.391-noinject-a33b47ef9`） | 新 release 目录 + 原子切 `current` |
| 构建树 | — | 产物由 `0608a3402` 构建；此后追加的只有 `docs/` + `openspec/…/tasks.md`（不参与构建），所以 `0.1.391-mirasim-0608a3402` 就是这两个产物的准确来源 |
| 前端源码差 | `7e1ee1c92` | 相对 live 的**逻辑增量**：`AccountsView-*.js`、`AccountsView-*.css`、`index-*.css`(+1 条 `.bg-sky-600`)、zh/en i18n chunk（新增 `admin.accounts.mirasimOAuth.*`）、`index.html`；其余 103 个文件的差异**全部**是 chunk 名哈希级联（i18n chunk → LocaleSwitcher → AppLayout → 各视图），已用「剥哈希后逐文件 md5」证明 |
| 未认证探针 | `GET /api/v1/admin/mirasim/oauth/providers` → **404**（未部署） | 部署后应为 **401**（路由存在、未认证） |

二进制替换的坑：bind mount 绑的是 **inode**，`mv` 新文件进去旧 inode 仍挂在容器上；hot-swap 靠
`docker stop` → 覆盖同名文件 → `docker start`（start 时按路径重解析）。运行中的二进制是
`Text file busy`，必须先停。

#### 9.4.2 构建（已执行并验证，如需重建）

```bash
# 前端（注意：本机 pnpm 对 symlink 的 node_modules 会 abort，直接调 bin）
cd frontend && ./node_modules/.bin/vue-tsc -b && ./node_modules/.bin/vite build
cd .. && ls backend/internal/web/dist/assets | head   # 新 chunk：AccountsView-C9yrXbGB.js
# 交叉编译 linux/amd64（CGO_ENABLED=0，静态 ELF）
cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags embed \
  -ldflags "-s -w -X main.Version=0.1.391 -X main.MinFrontendVersion=0.1.380" \
  -trimpath -o bin/server ./cmd/server
# 产物自检：①含 mirasim 关键串 ②内嵌的是本轮 dist（新 chunk 名在、旧 chunk 名不在）
for s in mirasim_refresh_token mrs-seal-v1 mirasim_relay_persist_failed; do grep -c "$s" bin/server; done
for s in AccountsView-C9yrXbGB.js index-BCTKBti-.js; do grep -c "$s" bin/server; done   # 应 ≥1
grep -c AccountsView-DEtjWFwt.js bin/server                                                 # 应 0
```

#### 9.4.3 发布顺序（前端先、后端后；先给 runbook 并取得批准）

```bash
# 产物固定到本机稳定路径（sha256 前 9 位命名，便于与线上 sub2api.bak-* 对照）
cp /tmp/s2a-deploy-mirasim/backend/bin/server /tmp/sub2api-0.1.391-mirasim-924e4c5e1-linux-amd64
shasum -a 256 /tmp/sub2api-0.1.391-mirasim-924e4c5e1-linux-amd64

# --- 打包前端（macOS 必须禁资源叉，否则 172 文件会打成 347 条目）---
cd /tmp/s2a-deploy-mirasim/backend/internal/web/dist
COPYFILE_DISABLE=1 tar --no-mac-metadata --no-xattrs -czf /tmp/dist-0.1.391-mirasim-clean.tgz .
tar tzf /tmp/dist-0.1.391-mirasim-clean.tgz | grep -c '/\._'        # 必须 0
md5 -r /tmp/dist-0.1.391-mirasim-clean.tgz                          # 2acaf14e492652c06a78ed9c2bc71126
md5 -r index.html                                                   # 67ae44d28f5fe219d8bbadab01b1fa8e

# --- 前端（无停机；nginx root 是 symlink，逐请求解析，不需要 reload）---
R=/opt/sub2api/frontend/releases/0.1.391-mirasim-0608a3402
install -d "$R" && tar xzf /tmp/dist-0.1.391-mirasim-clean.tgz -C "$R"
find "$R" -type f | wc -l                       # 必须 172
find "$R" -name '._*' | wc -l                   # 必须 0
md5sum "$R/index.html"                         # 必须 67ae44d2…（守卫：任一不符即 ABORT）
ln -sfn "$R"                 /opt/sub2api/frontend/.current.new
ln -sfn /opt/sub2api/frontend/releases/0.1.391-ocgremove-7e1ee1c92 /opt/sub2api/frontend/.rollback.new
mv -T /opt/sub2api/frontend/.current.new  /opt/sub2api/frontend/current    # 原子替换
mv -T /opt/sub2api/frontend/.rollback.new /opt/sub2api/frontend/rollback
curl -s https://code.sicts.shop/ | grep -o 'index-[A-Za-z0-9_-]*\.js' | head   # 应为新 entry

# --- 后端（停机窗口 = stop→cp→start，实测秒级）---
C=sub2api-api-candidate-0.1.391-noinject-a33b47ef9
P=/opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9
sha256sum "$P/sub2api"                          # 记录基线：1d78d046…
cp -a "$P/sub2api" "$P/sub2api.bak-20260924-mirasim"
systemctl stop nginx                            # 可选：缩短 5xx 窗口
scp bin/server root@74.82.195.84:$P/sub2api.new
docker stop "$C" && cp "$P/sub2api.new" "$P/sub2api" && docker start "$C"
sha256sum "$P/sub2api"                          # 应 = 924e4c5e…
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18373/health          # 200
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:18373/api/v1/admin/mirasim/oauth/providers  # 401（部署前 404）
systemctl start nginx
```

1. 前端先发（纯静态、无停机、可秒回）；后端后发（唯一停机动作）。
   前端 release 目录沿用「`<ver>-<feature>-<commit>`」既有命名（`0.1.391-mirasim-0608a3402`，
   与 `0.1.391-ocgremove-7e1ee1c92` 同构）；后端**不新建 release 目录**（新目录不会被挂载），
   而是在旧目录内覆盖 + 留 `sub2api.bak-*` 回滚件——因此不要用目录名判断后端版本，
   用 `sha256sum` 判断。
2. 后端只替换**已挂载的那个文件**，保持容器与 env 不变（改 env 才需要 recreate）。
3. 管理端录入账号：平台 Anthropic、类型 API Key（Key 可留空）、打开 "Mirasim Relay" 开关、填
   refresh token、`base_url` = `https://relay.mirasim.ai`、配模型映射（如 `claude-sonnet-4-5 → kimi-k3`）。
   **不需要重启容器**：新增账号会由 `Create` 里的 scheduler outbox 事件（`account_repo.go:153`）
   自动进入调度器。
4. 验证：管理端"测试账号" → 发一条真实请求 → 看日志事件（见 9.5）。

#### 9.4.4 回滚

| 层 | 动作 | 代价 |
| --- | --- | --- |
| 前端 | `mv -T` 把 `current` 指回 `0.1.391-ocgremove-7e1ee1c92`（或直接 `ln -sfn` 旧目录到 `current`） | 秒级，无数据影响 |
| 后端 | `docker stop` → `cp sub2api.bak-20260924-mirasim sub2api` → `docker start`（或直接 `docker start` 旧候选容器 + 改 nginx upstream） | 秒级 |
| 数据 | **无需修补**：无表/列/迁移；已录入的 mirasim 账号在旧二进制下退化为普通 Anthropic apikey 账号，删 `credentials.mirasim_refresh_token` 即彻底失效 | 无 |
| 凭据 | 若已轮换，旧二进制不认新 token 也没关系（旧二进制不读这些字段）；反向（旧→新）需要 DB 里的 token 是最新的 | 无 |

风险提示（执行前必读）：

- 停机窗口内 nginx 会 502；`systemctl stop nginx` 可把 5xx 换成明确拒绝，但会让站点完全不可达，
  按需选择。
- §9.2 单持有者：发布**不引入第二个进程**，所以不触发互踢；但发布后若管理员把同一个 refresh
  token 同时用于桌面客户端，会互踢。
- 三个候选容器共用同一 Postgres/Redis，**不要**在这轮顺手切换 nginx upstream。

### 9.4.5 发布记录（**已执行**，2026-09-24 22:21–22:22 JST / 13:21–13:22 UTC）

执行顺序：先前端（无停机）→ 后后端（`docker stop` → `cp` → `docker start`）。nginx **未停**；实测停机窗口内
没有产生可观察到的 502（替换到 `health=healthy` 间隔约 20 秒，期间站点仍由旧进程服务至 stop）。

| 项 | 发布后实测值 |
| --- | --- |
| 后端二进制 | `/opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api`，123715746 B，sha256 `924e4c5e165c9ad904130b5dab0dc1acb63a9ad73a3090ee1ef4a7e82737c6d0`（= 本地产物，逐字节） |
| 备份件 | 同目录 `sub2api.bak-20260924-mirasim`（123547810 B = 旧基线 `1d78d046…`）；另有中转文件 `sub2api.new`（与现役同哈希，可直接删） |
| 容器 | `sub2api-api-candidate-0.1.391-noinject-a33b47ef9`，`StartedAt=2026-09-24T13:22:04Z`，`Restarts=0`，`healthy`，唯一接流（nginx upstream → `127.0.0.1:18373`） |
| 前端 | `current → releases/0.1.391-mirasim-0608a3402`（172 文件），`rollback → releases/0.1.391-ocgremove-7e1ee1c92` |
| 前端一致性 | 线上 release 与本地 dist **逐文件 md5 全等（172/172）**；`index.html` md5 `67ae44d28f5fe219d8bbadab01b1fa8e` |
| 差分探针 | `GET /api/v1/admin/mirasim/oauth/providers`：发布前 **404** → 发布后 **401**（路由存在、待鉴权）；对照不存在路径仍为 404 |
| 5 个管理端点 | `/oauth/providers`(GET)、`/oauth/auth-url`、`/oauth/complete`、`/oauth/email/code`、`/oauth/email/verify` 全部 **401**（未带凭据）→ 路由均已注册生效 |
| 上游出口 | 容器内 `https://auth.mirasim.ai/auth/oauth/providers` 可达（exit 0），不依赖宿主机网络 |
| 业务回归 | `GET /api/v1/settings/public` → 200；`POST /api/v1/auth/login`（空体）→ 400；网关 `account_simple_selection` / sticky 正常出流量；日志中 `mirasim` 相关错误 0 条 |

| 前端 UI 接线 | 实时服务的 `assets/AccountsView-C9yrXbGB.js`（md5 `629180038d328ddac5c33a530781b447`，与本地构建一致）内含 5 个新端点路径各 1 次、`mirasimOAuth` 44 次；HTTP 取回字节与磁盘 md5 一致 |
| 已部署二进制内核证据 | `sha256(线上 sub2api)` **完全等于**本机构建产物；长字面量命中 `mirasim_refresh_token`×3、`/admin/mirasim`×2、`/v1/device/session`×1、`mirasim_relay_persist_failed`×1、`mrs-seal-v1`×1；**旧基线上这些串均 0 命中**（对照有效） |

本轮三条方法论教训（下次发布直接复用）：

1. **macOS 打包必须禁资源叉**：默认 `tar` 会把 `._*` 一并打入（172 文件 → 347 条目）。
   部署脚本内的「文件数 + `._` 残留 + `index.html` md5」三重守卫在本轮**成功拦下一次脏包**（未产生任何线上改动）。
2. **minified bundle 不能按逻辑名比对**：本轮相对 live 发生了 chunk 重划分（`UsageView-*.js` 40951 B → 29346 B，
   `api-*.js` 由 2255 B 拆为 1061 B），「逐文件哈希归一化」与「正则抓字符串 diff」两种口径都已验证失效。
   权威口径是：①`git diff <live>..HEAD -- frontend/src`；②全站字符串字面量并集（`/api/v1/...` 路径集、
   `admin.*` key 集、CSS 规则集）＋正向对照（新端点只在新的里面）；③部署后用整树 md5 证「线上 = 本地构建」。
3. **短字符串字面量不能当存在性判据**：Go 在 amd64 上会把 ≤8 字节的常量编码进指令立即数
   （`grep -c 'mrs1:'` 在正确产物与线上二进制里都是 **0**，而该常量在 `mirasim_protocol.go:42` 确实在用）。
   存在性校验要用**长字面量**（事件名、URL 路径），并在旧基线二进制上做 0 命中对照。
   另：`AGENTS.md` 里的 `go version -m <binary>` → `vcs.revision` 在本仓**不适用**（`-trimpath` 且无 vcs stamp），
   版本判定只能用「二进制 sha256 ＋ 串比对」。

### 9.5 巡检

| 日志事件 / 现象 | 含义 | 处置 |
| --- | --- | --- |
| `mirasim_relay_credential_updated` | 轮换已落库（正常，约每次 access token 过期时） | 无 |
| `mirasim_relay_persist_failed` / `_persist_reload_failed` | 落库失败，会在下次请求重试 | 检查 DB 可用性；持续出现则重新取证 |
| 持续 401（`token_invalid`） | refresh token 已被别处刷掉，或落后于轮换 | 确认没有第二个持有者；重新取证并覆盖凭据 |
| 冷启动后首请求变慢 | 正常：空缓存要换一次 ticket | 无 |
| 新建账号后调度器没看到 | 账号 `Create` 会 enqueue scheduler outbox（`account_repo.go:153`），等事件消费即可；若直接改 SQL 则必须手工删 `sched:acc:<id>` | 无 |
| 账号测试报 `Invalid base URL` | 只可能是白名单被启用且缺 `relay.mirasim.ai` | 补 `SECURITY_URL_ALLOWLIST_UPSTREAM_HOSTS` |

### 9.6 仍未验证（不要当成事实）

- 轮换后旧 refresh token 是否**立即**失效（上游语义暗示是；本项目未直接测量）。操作上无论结论如何
  都按 §9.2 单持有者处理。
- §9.3 的 `/auth/code`（真发邮件）与 `/auth/verify`（消耗 OTP）**未实测**，请求/响应形状
  来自上游源码与文档；管理端实现做了宽容解析（`refresh_token` / `token`，兼容
  `data`/`result` 包裹）。OAuth 的 providers 列表与授权链接生成已只读实测（见 §10）。
- 新 Mirasim 账号是否有 relay 权益（与订阅绑定关系）。
- 多实例双活的具体表现未实测（按 §9.2 推断为互相踢）。

### 9.7 上线后待办（需管理员在后台操作，代码侧无未完成项）

任务 `tasks.md` 的 8.6（上线后回归）/ 8.7（回滚演练）**只能在实际存在一个 mirasim 账号后才能跑**，
而生产 DB 当前 mirasim 账号数 = 0（已实测）。步骤：

1. 后台 → 账号 → 新建，平台 **Anthropic**，类型 **API Key**（Key 可留空）。
2. 在该账号表单里找到 **Mirasim 授权登录** 面板（`MirasimOAuthOnboarding.vue`）：
   选登录方式 → 「生成授权链接」→ 在浏览器完成授权 → **从地址栏复制完整回调 URL**
   （跳到 `http://127.0.0.1:9/callback?...` 打不开是正常的）→ 粘回表单 → refresh token 自动回填，
   relay 开关自动打开；也可走邮箱验证码两条接口（**真发邮件 / 真消耗 OTP**）。
3. 填 `base_url` = `https://relay.mirasim.ai`，配模型映射（如 `claude-sonnet-4-5 → kimi-k3`）。
4. 保存（走 `POST /admin/accounts`；device key 由 relay client 首次调用时自动生成并落库，无需手填）。
5. 验证清单（即 8.6）：管理端「测试账号」应返回 200（内部打 `relay.../v1/messages`，不带 beta query）→
   真实请求一条 → 日志应出现 `mirasim_relay_credential_updated`（轮换已落库）→
   `docker restart` 容器后再发一条，确认冷启动自愈（空缓存先换 ticket）。
6. 回滚演练（即 8.7）：删掉该账号 `credentials.mirasim_refresh_token` → 该账号应退化为普通 Anthropic apikey；
   再把 `current` 指回 `0.1.391-ocgremove-7e1ee1c92` / 用 `sub2api.bak-20260924-mirasim` 回退后端，
   确认服务仍正常（无数据修补）。

> ⚠️ **不要**把你自己电脑上 mirasim 桌面客户端正在用的 refresh token 直接抄进去：§9.2 单持有者约束下会互踢，
> 你的客户端会掉登录。请用第 2 步的授权登录另取一份属于服务端的凭据。

## 10. 管理端授权登录（取 refresh token，2026-09-24）

对标 `openai_oauth_service.go` / `kimi_oauth_service.go`：管理员不必再从桌面客户端
导出凭据，直接在后台账号表单里登录 Mirasim，取回 refresh token 自动回填。

### 10.1 端点

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| GET | `/admin/mirasim/oauth/providers` | 代理上游可用登录方式（`auth_url`、`proxy_id` 可选） |
| POST | `/admin/mirasim/oauth/auth-url` | 生成授权链接（`provider`、`auth_url`、`proxy_id`） |
| POST | `/admin/mirasim/oauth/complete` | 从 `callback_url` 或 `refresh_token` 取凭据（本地解析，零上游调用） |
| POST | `/admin/mirasim/oauth/email/code` | 触发上游邮箱验证码（**会真发邮件**） |
| POST | `/admin/mirasim/oauth/email/verify` | 邮箱 + 验证码换 refresh token（**消耗 OTP**） |

全部为管理端已认证接口，响应体含 refresh token 明文——与「管理员手工粘贴」等价；
日志只记 `provider` / `has_name` / `has_plan` / `expires_at`，**绝不记 token 与邮箱**。

### 10.2 浏览器授权为什么需要手工复制回调

上游 `OAUTH_REDIRECT_ALLOWLIST` 只放行 loopback 与 `*.mirofish.ai`。sub2api 的管理
服务不是 loopback，所以**无法自己接收回调**：服务端固定用占位回调
`http://127.0.0.1:9/callback` 生成授权链接，管理员授权完成后浏览器会跳到
`http://127.0.0.1:9/callback?...&refresh_token=...`（**页面打不开是正常的**），
从地址栏复制完整链接贴回表单即可。实测：把 `redirect_uri` 换成非 loopback 地址，
上游直接返回 400。

### 10.3 三条路径（前端组件三个页签）

1. **浏览器授权**：`生成授权链接` → `打开授权页` → 复制地址栏完整链接 → `完成并回填`。
2. **邮箱验证码**：填邮箱 → `发送验证码` → 填验证码 → `完成并回填`。
3. **粘贴 Token**：直接贴 refresh token 原文（最省事，也是排障兜底）。

三条路径产出的都是同一种凭据对象，前端回填 `mirasim_refresh_token` 并自动打开
relay 开关；新建账号时还会用 JWT 里的 `name`/`email` 预填空的账号名。

### 10.4 关键约束（改动前必读）

1. **登录环节绝不调用 `/auth/refresh`**。refresh token 是单值轮换的，在这里刷一次
   就会把管理员本地 mirasim 客户端踢下线；凭据只做本地 JWT 解析（不验签，零上游调用）。
   对应代码是 `CompleteWithRefreshToken` / `ParseMirasimRefreshToken`，它们的签名里
   根本没有 `context.Context`——单测用这一点证明登录链路不发网络请求。
2. **`state` 不是 CSRF 控制**。上游会用它自己签的 `state` 覆盖我们传进去的值，
   sub2api 无法校验；`state` 只作本地关联 ID。信任边界是「已认证的管理员 API 调用 +
   管理员自己粘贴的回调 URL」。
3. **只生产凭据，不直接建号**。凭据回填账号表单后仍走既有 `POST /admin/accounts`
   （含敏感凭据合并 `MergePreservingSensitiveCreds` 与调度器快照同步）。
4. **代理由 `HTTPUpstream` 承担**：授权登录请求与会话请求一样按 `proxy_id` 走代理与
   host 校验，不新增网络出口。白名单关闭时 `auth.mirasim.ai` 无需配置。
5. **别把含 token 的回调 URL 贴进聊天/工单/日志**——它就是凭据本身。

### 10.5 验证到哪一步（区分已验证 / 未验证）

已验证：

- 上游 `GET /auth/oauth/providers` → `{"providers":["github","google"]}`（只读实测）。
- 上游 `POST /auth/oauth/{provider}/login` 返回带 `state` 的授权链接；非 loopback
  `redirect_uri` → 400（只读实测）。
- 回调链接解析、JWT 形态/`token_type`/`exp`/身份校验、错误分类（4xx 透传细节、
  5xx 不外泄）、providers 空列表 502、代理仓库缺失 400：单测 17/17。
- 前端组件 9 用例 + `vue-tsc --noEmit` exit=0 + `vitest run src/components/account
  src/features/admin-accounts` 192 用例全绿（含「登录链路只打 4 个 admin 端点」断言）。

未验证（有意为之，因为会产生真实副作用或需人工操作）：

- `/auth/code` 发信、`/auth/verify` 换凭据的真实响应形状（会真发邮件 / 消耗 OTP）。
- 端到端人工授权（需管理员在浏览器里点 GitHub/Google 授权）。

⇒ 发布后第一次真机登录请当作验证步骤：管理端账号表单 → 生成授权链接 → 浏览器授权 →
复制回调 → 回填 → 保存 → 账号测试通过。若失败，先用「粘贴 Token」路径对照，
以区分是登录链路问题还是凭据本身问题。
