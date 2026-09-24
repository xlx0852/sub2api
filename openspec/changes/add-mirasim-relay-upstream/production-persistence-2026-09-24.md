# Mirasim Relay 上游接入：生产持久化方案（2026-09-24 勘察）

## 0. 问题

代码已经写好并在真实链路上验证通过（本变更 `tasks.md` 6.7 / 6.8：本地直连 4/4 通过，生产出口
+ 真实凭据 4/4 通过）。但**能力目前只存在于本地工作区**：线上三个 api 容器里 `mirasim` 字符串
命中数为 0，前端 `/opt/sub2api/frontend/current` 无 `mirasim` 命中，DB 里 mirasim 账号数为 0。

"线上持久化"要回答三件事：能力怎么**发布上去**、状态**存在哪**（重启/换容器/换版本之后还在不在）、
凭据怎么**长期有效**。本文件是这三件事的勘察结论与执行方案；操作细节同时写进
`docs/MIRASIM_RELAY_UPSTREAM.md` §9（§9 是长期维护文档，本文件是本次取证的记录）。

## 1. 结论

1. **不需要为持久化新增任何机制。** 唯一持久状态是 Postgres `accounts.credentials` 里的
   `mirasim_refresh_token` / `mirasim_device_key`，以及 `extra.mirasim_relay_enabled`：
   无新表、无新列、无迁移。进程内 `mirasimRelayRuntimeAcquire` 的 `accountID → client` 缓存
   冷启动即为空，首个 relay 请求按 DB 凭据自动走一次 `/v1/device/session` 换 ticket
   ⇒ 重启容器、换二进制、换副本都不需要预热或状态搬迁。Redis 只放调度器快照 `sched:acc:<id>`，
   由 `UpdateCredentials` 触发的 `syncSchedulerAccountSnapshot` 自动刷新。
2. **发布 = 二进制热替换 + 前端 release 目录 + 管理端录入账号**，沿用仓库既有约定
   （`AGENTS.md`：bind-mount 二进制、前端由 nginx 直接 serve、`current`/`rollback` 软链）。
   构建侧已在本机验证（见 §3.2），本机 10.6s 产出 118 MiB linux/amd64 静态 ELF。
3. **长期有效的真正风险不是"存储"，而是"谁持有 refresh token"。** refresh token 是账号级的，
   同一 token 不能同时被两个 sub2api 账号 / 两个进程 / "sub2api + 桌面客户端"持有，否则互相踢
   成 401（`tasks.md` 7.4 / 8.2）。发布前必须先决定这一点（见 §5）。

## 2. 状态模型（勘察结论）

| 层 | 内容 | 重启 / 换容器 / 换版本后 |
| --- | --- | --- |
| Postgres `accounts.credentials` | `mirasim_refresh_token`（必需）、`mirasim_device_key`（可空，服务端自动生成并落库）、`base_url` | **唯一持久状态**，原样存活 |
| `accounts.extra` | `mirasim_relay_enabled` 等开关与可选覆盖 | 原样存活 |
| 账号变更事件传播 | scheduler outbox + 快照 `sched:acc:<id>` | 新建账号由 `Create` 的 outbox 事件带进调度器（`account_repo.go:153`），无需重启 |
| 进程内 `mirasimRelayRuntimeAcquire` | `accountID → client`（TTL 1h、上限 512、single-flight） | 清空后自愈，**无需预热** |
| Redis `sched:acc:<id>` | 仅调度器快照 | 随凭据写回自动刷新；**直接改 SQL 时必须手工删该键**（既有坑） |

轮换回写链：client `OnRotate` → `mirasimPersistCallbacks` → 异步 goroutine → `GetByID` 重读防脏写
→ `UpdateCredentials` → 快照刷新；写失败删去重记录，下次请求重试。残留风险是异步落盘窗口内崩溃
最坏落后一次轮换，此时表现为持续 401（不会发错请求），需要重新取证覆盖凭据。

生产不依赖 macOS keychain 或 `MIRASIM_APP_SECRET_KEY`——那层 `mrs1:` 密封存储只存在于桌面客户端
（`/Applications/Mirasim.app` → app-bound store）。sub2api 侧凭据就是 `accounts.credentials`
这个 JSONB 列（`ent/schema/account.go:79`）；未发现对该列的字段级加密（`aes_encryptor.go`
服务于 TOTP 秘钥），敏感键只靠读取/回显阶段脱敏（`account_credentials_redact.go`）
⇒ **备份 / 迁移数据库即备份 / 迁移了该能力**，同时也意味着 DB 导出物含 refresh token，
必须按凭据处理。

## 3. 勘察证据

### 3.1 线上现状（只读）

- 运行中 api 容器 3 个，均 `0.1.391`：`-noinject-a33b47ef9`（8080→127.0.0.1:**18373**）、
  `-ocprotocol-ccef37122`（→18372）、`-ocsession2-d2f2b9bf9`（→18371）。
- 线上流量只走 18373：`/etc/nginx/conf.d/code-sicts-shop.conf` 的 `upstream sub2api_api`
  → `server 127.0.0.1:18373`；`18372` 不出现在任何 live conf；`18371` 只出现在
  `code-sicts-shop.conf.bak-ocprotocol-084645` 这类**备份**文件里。
- 三个容器**共用同一个 Postgres 与 Redis**（`DATABASE_HOST/PORT/DB/USER`、`REDIS_HOST/PORT/DB`
  逐键比对一致）。
- 二进制挂载：`/opt/sub2api/api/releases/0.1.391-noinject-a33b47ef9/sub2api → /app/sub2api`
  （123,547,810 B）；三个线上二进制 `strings | grep mirasim` 命中数 **0**。
- 前端：`/opt/sub2api/frontend/current/` 下 `grep -rl mirasim` **无命中**。
- DB：`select count(*) from accounts where credentials::text like '%mirasim%'` → **0**。
- 白名单：`SECURITY_URL_ALLOWLIST_ENABLED=false`、`..._ALLOW_PRIVATE_HOSTS=true`、
  `..._ALLOW_INSECURE_HTTP=true`，未设 `..._UPSTREAM_HOSTS`；`/opt/sub2api/data/config.yaml`
  里没有 `security:` 段 ⇒ **关闭状态不校验 host，生产无需改配置**（启用时才必须补
  `api.anthropic.com` 与 `relay.mirasim.ai`）。
- 生产机有 Go 1.25.8（低于本仓 go.mod 要求），因此产物应在本机交叉编译后上传；
  生产 `/opt` 可用 106 G。

### 3.2 构建与产物自检（本机）

```bash
cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags embed \
  -ldflags "-s -w -X main.Version=0.1.391 -X main.MinFrontendVersion=0.1.380" \
  -trimpath -o /tmp/sub2api ./cmd/server
```

结果：10.6s、123,654,306 B（比线上二进制 +106 KB）、静态 ELF。产物内检出
`mirasim_refresh_token`(3)、`mrs-seal-v1`(1)、`mirasim_relay_persist_failed`(1)、
`mirasim_relay_credential_updated`(1)，并且已 embed 本次前端 dist
（`AccountsView-E83-4CKV.js` 命中）。前端 `backend/internal/web/dist` 构建时间晚于最后一次前端
源码改动（18:34:20 vs 18:33:21），且 dist 内含 `mirasim_refresh_token` ⇒ dist 新鲜，可直接 embed。

> 注：若重新构建前端，需 `pnpm --dir frontend run build`（本机 pnpm 11.5.3、依赖已装）。

### 3.3 设备身份（关键前提，已实测）

用 `openssl genpkey -algorithm ed25519` 新生成一把 relay **从未注册过**的 device key
（`deviceID=j4CdJBupXYECUxXtVrcxoP`），配合调用方提供的真实短期 access token 请求
`/v1/messages`、`/v1/messages/count_tokens` → **均 200**。⇒ 生产 `mirasim_device_key` 留空，
由服务端首请求生成并落库即可，**不需要从桌面客户端导出任何状态**。

### 3.4 凭据获取（管理端已内置授权登录，详见 `docs/MIRASIM_RELAY_UPSTREAM.md` §10）

- `POST {auth}/auth/code {"email"}` → `POST {auth}/auth/verify {"email","code"}` → 响应含
  `access_token` / `refresh_token`。依据 `mirasim2api` 的 `internal/admin/oauth.go:332` / `:366`。
- 或 OAuth（GitHub/Google）回调链接携带 `refresh_token`（`internal/admin/oauth.go:171`）。
- 既然本项目已把三条取凭据路径实现成管理端功能（`/admin/mirasim/oauth/*`），生产发布**不需要**
  在服务器上手工调端点：管理员在后台账号表单里自己登录/收验证码即可（邮件发到管理员自己的邮箱）。
- 只读实测（2026-09-24）：`GET /auth/oauth/providers` → `{"providers":["github","google"]}`；
  `POST /auth/oauth/{provider}/login` 返回带 `state` 的授权链接；非 loopback `redirect_uri` → 400。
- ⚠️ `/auth/code`（会真发邮件）与 `/auth/verify`（消耗 OTP）**有意未触发**；端点形状来自上游源码。
- ⚠️ relay 权益是否绑定账号订阅、新账号能否用 relay：**未验证**。

## 4. 发布步骤（待批准后执行）

1. 本机构建（§3.2），上传到 `/opt/sub2api/api/releases/0.1.391-mirasimrelay-<sha256前9位>/sub2api`。
2. `docker stop <容器>` → 替换挂载路径上的二进制 → `docker start`（运行中会 `Text file busy`）。
   只替换 **18373** 那个容器；另外两个候选容器**不要**同时服务 relay 账号（§5）。
3. 前端：把新 `dist` 解到 `/opt/sub2api/frontend/releases/<ver>-<hash>/`，`current` 重指过去，
   `rollback` 指向原目录。
4. 管理端录入：平台 Anthropic、类型 API Key（Key 可留空）、打开 "Mirasim Relay"，
   **直接用内置的「授权登录获取凭据」拿 refresh token**（GitHub/Google 浏览器授权、邮箱验证码、
   或粘贴 token），`base_url = https://relay.mirasim.ai`、**配模型映射**（如
   `claude-sonnet-4-5 → kimi-k3`，否则 `claude-*` 全部 422）。新建账号**不需要重启容器**：
   `Create` 会 enqueue scheduler outbox 事件（`account_repo.go:153`）把它带进调度器。
5. 回归：管理端授权登录成功一次 → 账号"测试账号"通过 → 真实请求 200 → 观察
   `mirasim_relay_credential_updated` 落库 → 重启容器再发一次请求确认冷启动自愈。

回滚：删 `credentials.mirasim_refresh_token` 即退化回普通 Anthropic apikey 账号；二进制/前端回滚
不需要任何数据修补。

## 5. 发布前需要用户决策

1. **用哪个 Mirasim 账号**：生产独占现有账号（则桌面客户端需登出/停用，否则互相踢），
   还是新开一个生产专用账号（则要先确认新账号是否有 relay 权益——未验证）。
2. **刷新凭据的取得方式**：已不需要外部决策——管理端内置授权登录（GitHub/Google、邮箱验证码、
   粘贴 token 三条路径）随本次发布上线，由管理员自己在后台完成。⚠️ 登录环节**不会**调
   `/auth/refresh`，不会轮换凭据。
3. **发布窗口**：是否现在就做这次热替换（含前端），或先只发后端。

## 6. 明确未验证的项

- 轮换后旧 refresh token 是否**立即**失效（上游语义暗示是，未直接测量）。
- `/auth/code`、`/auth/verify` 的真实请求/响应形状（均来自读上游源码，未触发）。
- 管理端授权登录的端到端人工验证（需管理员在浏览器完成授权，作为发布后回归第 1 步）。
- 新账号的 relay 权益与订阅绑定关系。
- 双实例同时服务同一账号的具体表现（按 §2 单持有者约束推断为互相踢）。

---

## 7. 真机落号结果（**2026-09-24 13:40–13:45 UTC 已执行**）

本节是上面第 4 节「发布后回归」清单的实际结果，也是第 5 节两个待决策项的收口。

- **决策落地**：走的是「服务端自有会话族」——管理员在生产后台用授权登录入口（Google）取了
  一份**同账号、不同 `sid`** 的凭据。既不是生产独占现有账号（那会踢掉桌面客户端），
  也没有另开账号（relay 权益是否随订阅绑账号仍未验证，故不去赌）。
- **回归结果**：`oauth/complete` → 200；`POST /admin/accounts` → 200，**账号 `id=140`
  （`mirasim-relay`，`anthropic`/`apikey`，`active`，未挂分组）**；管理端「测试账号」真实打到
  上游并返回内容（`kimi-k3` 15s、`deepseek-flash` 9s 有 `content` + `test_complete`）。
- **持久化在线验证**：refresh token 被上游轮换（哈希前缀 `6b201126`→`0e738a9c`，长度仍 539）并
  由进程回调写回 `credentials`；`credentials.mirasim_device_key` 由服务端自生成（118 字节 PKCS8
  Ed25519 PEM）后落库；`base_url` 自动补齐为 `https://relay.mirasim.ai`。
- **仍在第 6 节「未验证」里的项**：旧 refresh token 是否立即失效（未测）；`/auth/code`、
  `/auth/verify` 形状（未触发）；双实例同账号表现（未测）。新增一条：上游**模型可用性随时间窗变化**
  （同一批探测中 `kimi-code/k3`、`grok-4.6` 稳定 422，而 `kimi-k3`、`deepseek-flash` 正常）
  ⇒ 接流前必须实测，不能固化默认模型。
- 详表见 `docs/MIRASIM_RELAY_UPSTREAM.md` §11。
