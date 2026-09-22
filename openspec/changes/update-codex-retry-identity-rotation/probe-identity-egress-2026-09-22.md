# 十五、采票探针的出口与身份对齐（2026-09-22 实施）

## 15.1 改了什么

| # | 缺陷 | 改法 | 提交 |
|---|---|---|---|
| 1 | 采票用裸 `http.Client{}` 直连服务器 IP，业务请求走账号代理 → **有代理的号采票与回放不同 IP** | 探针改走 `s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)`，`proxyURL = account.Proxy.URL()`；并加上与业务相同的 `HTTPUpstreamProfileOpenAI`（否则同一个 httpUpstream 下仍走不同传输策略）；保留 45s 上下文超时（httpUpstream 默认响应头超时是 300s，后台探针不该挂钩） | `6498b6a94` `2ee5be9b0` |
| 2 | 探针只发 **6 个头**（`originator=codex_cli_rs` + 裸 UA），业务发 **19 个**、镜像真实客户端 → 同一账号、**同一张票**出现两种请求形态 | 按 2026-09-22 全量抓包补齐同一套头 | `6498b6a94` |
| 3 | 探针体未压缩，业务体 zstd | 复用 `compressOpenAIUpstreamBodyZstd` | `2263ddf0f` |

**探针补齐的头**（含取值依据）：

| 头 | 取值 | 依据 |
|---|---|---|
| `session-id` / `thread-id` / `x-client-request-id` | 账号级稳定 UUID（三者同值） | 实测三者相等 **123/123** |
| `x-codex-window-id` | `<session>:0` | 实测形态 |
| `x-codex-beta-features` | `prevent_idle_sleep,remote_compaction_v2` | 实测恒定值 123/123 |
| `x-codex-routing-hint` | `model=<body.model>` | 与业务同规则、自洽 |
| `x-codex-turn-metadata` | 合成 21 键 | 抓包里 23 个键，键名一个不造；枚举值取实测出现过的组合 |
| `originator` / `user-agent` | `codexIdentity.enforceHeadersWithUA` + `codexIdentityCandidateUA(nil, account)` | 与业务同一身份入口 |

## 15.2 用生产实测否掉了一个头（本阶段最有价值的发现）

初版给探针加了 `x-openai-internal-codex-responses-lite: true`——理由是真实客户端 123/123 都发。
**上线后探针全部返回 HTTP 400**：

```
初版（带 lite 头）：  probe_no_ticket status=400 completed=false   ← 上游拒绝请求本身
旧探针基线：         probe_no_ticket status=200 completed=false   ← 请求被接受、流未完成
```

原因与本仓已有注释一致：**该头是「本体是 Lite 形态」的声明，必须与体形状配套**。
探针体是标准 Responses 形态（tools/include/instructions），声明 Lite 即被拒。
真实客户端发它是因为**它的体就是 Lite 形态**，不是「所有请求都该带它」。

⇒ 探针不发该头。这条同时说明：**逐头对齐"真实客户端发过的头"不能机械照搬，必须看语义是否适用于该请求体**。

## 15.3 生产验证（新容器 0.1.391-ticketalign2-54a87a938 @18353）

**采票成功、零 400**：

```
18:25:38 seeded account=120 requested_model=gpt-6-astra  pool=gpt-5.6-luna  egress=direct
18:25:49 seeded account=120 requested_model=gpt-5.5      pool=gpt-5.5       egress=direct
18:26:00 seeded account=120 requested_model=gpt-5.6-sol   pool=gpt-5.6-sol   egress=direct
18:26:22 seeded account=120 requested_model=gpt-5.6-terra pool=gpt-5.6-terra egress=direct
18:31:21 seeded account=69  requested_model=gpt-5.6-luna  pool=gpt-5.6-luna  egress=direct
```

**出口标签**：`egress=direct` 对 120/69 **是正确的**（两号 `proxy_id` 为空，业务请求同样直连）。
`egress=account-proxy` 的场景当前生产不存在：唯一配代理且可调度的号（98 代理6 / 102 代理3）
**模型集不含采票模型**（被 `IsModelSupported` 排除）；111（代理7）是 `schedulable=f` 被跳过。
出口一致性的直接证据是单测断言 `upstream.lastProxyURL == acc.Proxy.URL()`。

**探针出站差异**（`TestCodexTurnStateProbeOutboundFingerprint`，用业务测试同一份真实客户端 fixture）：

```
[新增] = []                                          ← 一条不多
[丢失] = [cookie, x-oai-attestation, x-openai-internal-codex-responses-lite]
```

前两项是结构性天花板（cookie 需账号 web 会话；attestation 只能转发），第三项是 15.2 的实测结论。

## 15.4 健康度复核（排除误判）

上线后 `/responses` 出现 502×145，**全部来自账号 89（API-key）**：

```
upstream http bridge error: status=403 message=insufficient balance
（影响模型 gpt-6-astra ×21、gpt-5.6-sol ×6）
```

与本改动**无关**：探针只走 OAuth 号，从不碰 API-key 号；且旧容器 15:59 起就已报
`INSUFFICIENT_BALANCE` 24 次（计费状态早于本次部署）。502 是网关如实转述上游 403。
归一化对比反而更好：`forward_failed` 4.2/k（新） vs 10.1/k（旧）。

## 15.5 顺带发现与处置

**候选容器堆积导致行为不确定**：当时有 **9 个**候选容器各自运行调度器与采票器，
连闲置容器都在 `seeded`，彼此抢占采票、重复压 PostgreSQL
（观测到 `pq: could not resize shared memory segment ... No space left on device`）。

已停止 8 个陈旧候选（`ticketalign-2ee5be9b0`/`zstd`/`idmirror`/`overloadepoch`/`subtract`×2/
`itemid`/`allplatforms`），保留：
- `ticketalign2-54a87a938` @18353 ← **当前线上**
- `ageprobe-493602d0c` @18351 ← 回滚目标（nginx 备份 `bak-ticketalign2-*`）

清理后 PG 共享内存告警消失。
