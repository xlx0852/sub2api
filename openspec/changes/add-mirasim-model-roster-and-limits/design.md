# Design: Mirasim Model Roster And Quota Windows

## Context

参考实现（桌面客户端 `Mirasim.app/Contents/Resources/server.cjs`）在**未登录模型流量**时也会请求两类
元数据接口；本设计以该实现为唯一事实来源（逐条已取证，见下表），并把它映射到 sub2api
既有的两个面：

| 参考实现 | 行为 | sub2api 落点 |
| --- | --- | --- |
| `q1n` / `O1n='/v1/model-roster'` | `GET`，空 body，`authorization: Bearer credential`，明文签名头；非 2xx 一律 `null`；200 走 `znr` 归一化；成功结果按 JWT `sub` 缓存 `ldo=600000ms`；超时 `ddo=8000ms` | `FetchUpstreamSupportedModels` |
| `znr` | 根 `{version, agents, models?, cli?}`；`agents[key]` 必须数组；条目 `id` 非空字符串 + `contextWindow` 数字 `> 0`；`id` trim+lowercase；`label` trim+lowercase 否则回退 `id`；`default===true` 首个胜出；无有效条目 → `null` | `mirasim_roster.go` |
| `sgo` / `ePn='/v1/limits'` | `GET`，头 `x-mirasim-probe: usage`，credential = `signer.credential() ?? token`，`AbortController` 超时 | `mirasim_limits.go` |
| `pBn` | `{windows:[{name,budget,used,reset_at,model_scoped?}], paid, degraded?}` → `usedPercent` 夹逼 `[0,100]`、`≥99 → 100`、`remainingPercent=max(0,100-usedPercent)`、`status` 阈值 `100→limit_reached` / `80→warning` / 否则 `allowed`；顶层 status 先取非 `model_scoped` 窗口，为空则回退全部窗口 | `mirasim_limits.go` |
| `uBn` | `reset_at` 为 epoch **秒**，转 ISO | `mirasim_limits.go` |

已实测（只读探测）：`/v1/model-roster`、`/v1/limits`、`/v1/models`、`/v1/device/session`
不带鉴权均返回 **401**，而 `/v1/model-rosterX`、`/v1/bogus-xyz` 返回 **404** → 路由存在。

## Decisions

### D1. 明文签名头，不密封

roster / limits 只发 `x-mirasim-device` / `x-mirasim-ts` / `x-mirasim-nonce` / `x-mirasim-sig`
（+ 非空时的 `x-mirasim-client`）与 `Authorization: Bearer <credential>`，**不**发
`x-mirasim-enc`。密封只发生在被代理的模型流量路径。

理由：参考实现的 `headersFor` / `Xdt` 明确只产出这 5 个头；密封（`bct`）只在代理路径调用。

### D2. 凭据优先复用缓存 device ticket

credential 选择顺序：**缓存 device ticket → 缓存 access token → 走 `/v1/device/session`
获取 ticket**。

理由：`/auth/refresh` 会轮换 refresh token（生产已实测），任何不必要的 refresh 都会带来
「轮换后未回写 DB 即打挂账号」的风险。device ticket 已缓存，复用零联网成本。

### D3. 401/403 兜底重试一次

只在 `401`/`403` 时失效本地 ticket/access 缓存并用另一凭据重试一次；仍失败则返回错误。
非 401/403 的失败**不**失效凭据、**不**触发任何 refresh。

理由：对齐 `mirasimIsAuthError` 既有语义，避免上游 5xx 导致凭据抖动。

### D4. 缓存策略

- roster：按账号缓存 10 分钟（对齐客户端 `ldo`），**不**跨账号共享。
- limits：按账号缓存 30 秒（客户端无此缓存；这是为了挡住管理端抽屉轮询），`force`
  （用量接口的强制刷新参数）必须绕过。

### D5. 窗口映射

`5h` → `five_hour`，`7d` → `seven_day`（仅当 `budget > 0`，否则该窗口无百分比含义）；
**所有**通过校验的窗口都会出现在 `UsageInfo.mirasim_windows`。`model_scoped=true` 的窗口
只进列表，**不**参与 `five_hour`/`seven_day` 映射。顶层状态按客户端 `pBn` 语义推断：
先取非 `model_scoped` 窗口，若为空则回退用全部窗口（包括 `model_scoped`）。

理由（关键约束）：`five_hour`/`seven_day` 是既有前端 presentation（`anthropic` 平台分支）
已经渲染的字段，映射进去即可零前端改动拿到主要收益；但 relay 的 `model_scoped` 窗口语义
（按模型/按 agent 配额）未测量清楚，把它当成账号级窗口会误导调度与用户。

### D6. 失败降级为 200 + error

用量拉取失败（超时、非 2xx、解析失败）返回 `UsageInfo{error: ...}`（HTTP 200），与
`CanGetUsage()` 既有降级语义一致；**不**把整个用量接口打成 5xx。

理由：管理端用量是只读面板，可用性优先；模型同步走的是另一条链路，失败必须是明确错误。

### D7. 不做账本与调度联动

见 proposal 的 Non-Goals：`platformQuotaWindowSpecs[PlatformAnthropic]` 已被
`passive_usage_7d_*` 占用，mirasim 窗口进账本需要新的键命名空间（平台内多来源窗口），
属于独立的账本设计问题。

## Risks

- **agent 选择**：参考实现按 `agents` 对象中的 key（客户端用 `claude`），本变更默认取
  `default: true` 的首个 agent，其次 `claude`，再次字典序首个。真实 roster 的 `agents`
  结构（是否只有 `claude`）在实现时必须实测确认；若实测结构与假设不符，需回到本设计更新 D5/D6。
- **`tor`（limits 超时）常量值未复核**：实现时需回 `server.cjs` 复核，取与 roster 同量级
  的值（8–12s），并在测试里钉住。
- **429 vs limits**：relay 在超限时的响应语义（429 头、`degraded` 触发条件）尚未测量；
  本变更不据此改动网关行为。

## Open Questions（实现前需确认）

1. roster 的 `agents` 是否真有多 agent？`models` / `cli` 两段在真实响应中是否存在？（实测即可）
2. `/v1/limits` 在当前账号（`id=140`）返回的窗口名集合是什么？是否包含 `5h`？（实测即可）
3. limits 超时常量 `tor` 的确切值。
