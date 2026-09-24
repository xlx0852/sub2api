## ADDED Requirements

### Requirement: Mirasim Relay Read-Only Metadata Requests

系统 SHALL 用一个独立的只读元数据通道请求 `GET /v1/model-roster` 与 `GET /v1/limits`：
请求体为空、携带明文 Ed25519 签名头（`x-mirasim-device` / `x-mirasim-ts` /
`x-mirasim-nonce` / `x-mirasim-sig`，以及非空时的 `x-mirasim-client`）与
`Authorization: Bearer <credential>`，且 MUST NOT 发送 `x-mirasim-enc` 密封头与
`x-mirasim-probe` 之外的自定义探测头。

`/v1/limits` 请求 SHALL 携带 `x-mirasim-probe: usage`。

credential SHALL 按「缓存 device ticket → 缓存 access token → 通过 `/v1/device/session`
获取 ticket」的顺序选择；只要缓存 ticket 或 access token 可用，MUST NOT 触发
refresh token 轮换。

任一请求 SHALL 使用独立的调用超时（不大于 10 秒），MUST NOT 复用模型转发的超时设置；
超时或网络失败 SHALL 返回错误，MUST NOT 静默重试多于一次。

#### Scenario: 只读请求不密封

- **WHEN** sub2api 为某个 mirasim 账号请求 `/v1/model-roster` 或 `/v1/limits`
- **THEN** 请求携带 `x-mirasim-sig` 等明文签名头与 `Bearer` 凭据，且不携带 `x-mirasim-enc`

#### Scenario: 复用缓存 device ticket 而不轮换凭据

- **WHEN** 账号已有未过期的 device ticket 缓存
- **THEN** 只读元数据请求直接使用该 ticket 作为 credential，且不调用 `/auth/refresh`

#### Scenario: 401 后兜底重试一次

- **WHEN** 只读元数据请求返回 401 或 403
- **THEN** 系统失效本地 ticket / access token 缓存、改用另一凭据重试一次；仍失败时返回错误

#### Scenario: 非鉴权失败不抖动凭据

- **WHEN** 只读元数据请求返回 500
- **THEN** 系统不失效任何缓存凭据、不触发 refresh，仅返回错误

### Requirement: Mirasim Model Roster Parsing

系统 SHALL 按以下规则校验并归一化 `/v1/model-roster` 响应：根必须为 JSON 对象；
`agents` 必须为对象且所取 agent 的值为数组；条目必须是非空字符串 `id` 且 `contextWindow`
为大于 0 的有限数；通过校验的条目 SHALL 保留 `id`/`label`（两者都 trim 并转小写，
`label` 缺失时回退 `id`）、`contextWindow`、`default` 与可选 `cli`/`models` 标记；
`default: true` 的首个条目 SHALL 被标记为默认；没有任何有效条目时 SHALL 视为失败。

系统 SHALL 同时提取可选的顶层 `models` 与 `cli` 清单元数据（结构未识别时忽略，
不影响 agent 解析结果）。

#### Scenario: 有效 roster 被归一化

- **WHEN** 响应为 `{"version":"1","agents":{"claude":[{"id":" Claude-Sonnet-4.5 ","contextWindow":200000,"default":true}]}}`
- **THEN** 解析结果包含 `id="claude-sonnet-4.5"`、`contextWindow=200000`、`label="claude-sonnet-4.5"` 且被标记为默认 agent

#### Scenario: 无效条目被丢弃

- **WHEN** `agents.claude` 中存在 `contextWindow=0` 或 `id` 为空的条目
- **THEN** 这些条目被丢弃；若整组都被丢弃则该解析判定为失败

### Requirement: Mirasim Upstream Model Sync

> **已被取代（2026-09-25）**：本 requirement 改用账号维度的 `GET /v1/models`，见
> `openspec/changes/restrict-mirasim-whitelist-to-available-models` 的「Mirasim Available
> Model Sync」。原因：roster 是全局目录，其默认 `claude` agent 的 6 个模型在该账号上真实调用
> 全部返回 404，且 roster 不包含 `deepseek-v4-flash` / `deepseek-v4-flash-vision-exp`。
> 下方内容仅作历史记录，**不再作为实现依据**。

`FetchUpstreamSupportedModels` 对 mirasim relay 账号 SHALL 通过 `/v1/model-roster` 获取模型
清单，返回该账号 agent 的模型 id 集合（去重、稳定排序、转小写），并 MUST NOT 再返回
`unsupported` 错误。

agent 选择顺序 SHALL 为：账号显式配置的 agent（`extra.mirasim_roster_agent`）→
`default: true` 的首个 agent → 名称为 `claude` 的 agent → 字典序首个 agent。
未在任何 agent 中找到有效模型条目时 SHALL 返回 `upstream` 类错误。

上游/网络/解析失败 SHALL 映射为既有 `UpstreamModelSyncError` 的 `upstream` kind；
账号与凭据配置缺失 SHALL 映射为 `configuration` kind；返回的 API 错误消息 MUST NOT
包含凭据、ticket 或上游响应正文。

#### Scenario: mirasim 账号同步成功

- **WHEN** 管理员对 mirasim 账号点击「同步上游模型」
- **THEN** 后端通过 `/v1/model-roster` 返回该 agent 的模型 id 列表，前端白名单可选择它们

#### Scenario: 不再返回 unsupported

- **WHEN** 管理员对一个配置完整的 mirasim 账号触发同步
- **THEN** 结果不是 `unsupported` 类错误（成功或明确的 `upstream` / `configuration` 错误）

#### Scenario: 凭据不可用

- **WHEN** mirasim 账号的 refresh token 缺失或 device key 无法构造
- **THEN** 同步返回 `configuration` 类错误，且不发起上游请求

### Requirement: Mirasim Quota Window Reporting

账号用量接口对 mirasim relay 账号 SHALL 通过 `/v1/limits` 获取额度窗口，并 SHALL 按以下
规则归一化每个窗口：`name` 必须为非空字符串，`budget`/`used` 必须为有限数且
`budget >= 0`，否则丢弃该窗口；`budget == 0` 的窗口 SHALL 标记为 unlimited 且不计算百分比；
否则 `usedPercent = clamp(round(used/budget*100), 0, 100)`，其中 `>= 99` 的百分比 SHALL
上取为 `100`；`remainingPercent = max(0, round(100 - usedPercent))`；`reset_at` SHALL 按
epoch 秒解析并输出 RFC3339；状态 SHALL 为 unlimited → `allowed`、`>= 100` →
`limit_reached`、`>= 80` → `warning`，其余 `allowed`。

系统 SHALL 把 `5h` 窗口映射到 `five_hour`、`7d` 窗口映射到 `seven_day`（仅当
`budget > 0`），并 SHALL 把所有通过校验的窗口（含 `model_scoped` 标记）暴露在
`mirasim_windows` 中。`model_scoped` 窗口 MUST NOT 参与 `five_hour` / `seven_day` 映射，
MUST NOT 决定顶层窗口状态。

系统 SHALL 透出顶层 `paid`（布尔或 null）与 `degraded`（仅当上游响应显式给出时）。
顶层状态推断 SHALL 与客户端 `pBn` 一致：先只看非 `model_scoped` 窗口，仅当
**不存在**任何非 `model_scoped` 窗口时，才回退使用**全部**窗口；参与推断的窗口集合内，
任一 `limit_reached` 则为 `limit_reached`，否则任一 `warning` 则为 `warning`，否则 `allowed`。

#### Scenario: 窗口映射到既有展示字段

- **WHEN** `/v1/limits` 返回 `{"windows":[{"name":"5h","budget":100,"used":40,"reset_at":1760000000},{"name":"7d","budget":1000,"used":950,"reset_at":1760100000}],"paid":true}`
- **THEN** 用量结果包含 `five_hour`（utilization 40、allowed）与 `seven_day`
  （utilization 95、warning），且 `mirasim_paid=true`

#### Scenario: 无限窗口

- **WHEN** 某窗口 `budget` 为 0
- **THEN** 该窗口在 `mirasim_windows` 中标记为 unlimited、不产生百分比，也不映射到 `five_hour`/`seven_day`

#### Scenario: 模型维度窗口不污染账号窗口

- **WHEN** 上游返回一个 `model_scoped: true` 且 utilization 为 100 的窗口，同时账号级窗口均为 20%
- **THEN** 顶层状态为 `allowed`，且 `five_hour`/`seven_day` 不反映该模型维度窗口

#### Scenario: 仅有模型维度窗口时回退推断

- **WHEN** 上游返回的窗口全部带 `model_scoped: true`，且其中某窗口 utilization 为 100
- **THEN** 顶层状态为 `limit_reached`（无非 `model_scoped` 窗口可用，回退用全部窗口），
  且 `model_scoped` 窗口仍不映射到 `five_hour`/`seven_day`

#### Scenario: 无有效窗口

- **WHEN** 上游返回的窗口全部未通过校验
- **THEN** 用量结果不含窗口映射，且以 `error` 字段说明无可解读的额度数据

### Requirement: Mirasim Metadata Failure Degradation

只读元数据拉取失败（超时、非 2xx、解析失败、凭据不可用）SHALL 在账号用量接口中以 HTTP 200
加 `error` 字段降级返回，MUST NOT 让用量接口返回 5xx，且 MUST NOT 影响该账号的调度状态
（不置 `rate_limit_reset_at`、不置临时不可调度）。

#### Scenario: 上游超时降级

- **WHEN** `/v1/limits` 请求超时
- **THEN** 用量接口仍返回 200，`error` 描述失败原因，账号仍可被调度

#### Scenario: 降级不写调度状态

- **WHEN** 用量拉取失败
- **THEN** 账号的限流/临时不可调度标记不因此改变

### Requirement: Mirasim Metadata Caching

系统 SHALL 按账号缓存 roster 结果 10 分钟、缓存 limits 结果至少 30 秒；缓存 MUST NOT
跨账号共享；用量接口的强制刷新参数（`force`）SHALL 绕过 limits 缓存；401/403 处理后
SHALL 失效相应缓存。

#### Scenario: 缓存命中不重复请求上游

- **WHEN** 同一账号在缓存有效期内连续两次读取用量
- **THEN** 只发生一次 `/v1/limits` 请求

#### Scenario: 强制刷新绕过缓存

- **WHEN** 管理员以强制刷新方式请求用量
- **THEN** 系统重新请求 `/v1/limits` 并刷新缓存

#### Scenario: 缓存不跨账号

- **WHEN** 两个不同 mirasim 账号各自读取用量
- **THEN** 各自发起自己的请求，不会读到对方的缓存结果

### Requirement: Mirasim Credential Safety In Metadata Paths

元数据链路 MUST NOT 记录或回显 refresh token、access token、device ticket、device private key
明文；日志 SHALL 只包含账号 ID、agent、窗口名/数量与布尔状态等非敏感字段；API 响应体
MUST NOT 包含任何凭据或上游响应正文。

#### Scenario: 日志不含凭据

- **WHEN** roster 或 limits 拉取成功或失败
- **THEN** 产生的日志字段仅包含账号 ID、agent/窗口计数与错误类别，不含 token/ticket/key

#### Scenario: 错误信息不回显上游正文

- **WHEN** 上游返回错误页面
- **THEN** 返回给前端的错误消息为固定文案，不包含上游响应正文

### Requirement: Mirasim Usage Presentation

管理端账号用量界面 SHALL 渲染 mirasim 的窗口数据：映射后的 `five_hour` / `seven_day`
沿用既有配额条，其余窗口 SHALL 以 `mirasim_windows` 中的标签与百分比单独展示，
并 SHALL 展示付费状态（`paid`）与降级提示（`degraded`）。相关文案 SHALL 提供中文与英文
两种 i18n。

#### Scenario: 抽屉展示额外窗口

- **WHEN** mirasim 账号返回包含 `5h` 与 `7d` 之外的窗口
- **THEN** 用量抽屉在既有 5h/7d 配额条之外额外展示这些窗口的名称与使用百分比

#### Scenario: 降级提示

- **WHEN** 用量结果带有 `degraded=true` 或 `error`
- **THEN** 抽屉显示提示文案，且不影响其它账号信息展示
