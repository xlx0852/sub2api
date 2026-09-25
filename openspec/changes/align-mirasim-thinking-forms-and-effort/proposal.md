# Change: 对齐 mirasim relay 的 thinking 形态与 effort 阶梯

## Why

官方客户端只允许两种 thinking 形态，且由账号签名 roster 的 `adaptive` 标记决定：

- 自适应型：`thinking.type=adaptive` + `output_config.effort`；
- 预算型：`thinking.type=enabled` + `thinking.budget_tokens`。

把固定 token 预算发给自适应型模型（或反之）会被上游**每一轮**拒绝；形态不能按模型名猜。
effort 也有阶梯：Claude/GPT 为 `low/medium/high/xhigh/max`，DeepSeek 额外接受 `off`（实际允许
`low/high/max`），GLM/Kimi 只允许 `low/high/max`；官方客户端的 `ultra` 单请求形态等于 `max`。

我们此前对 relay 账号不做这套归一：请求体里带 `output_config.effort` 或预算时原样透传，
既可能被上游按形态/档位拒绝，也把「本地能提前判定」的 400 变成了上游错误。

## What Changes

- 新增 relay 账号的 thinking 归一（`/v1/messages` 路径，在模型映射之后执行）：
  - 形态取自**本地缓存的**签名 roster 标记（冷缓存/未声明 = 默认自适应型），请求路径不拉 roster、不阻塞；
  - 自适应型收到固定预算 → 折成最接近档位（不是拒绝）；预算型收到 `effort` → 转 `budget_tokens`（并夹紧到 `< max_tokens`）；
  - `ultra` 折成 `max`（`reasoning.effort`/`reasoning_effort`/`output_config.effort` 三处）；
  - 请求体没有任何 thinking/effort 控制时**原样放行**。
- effort 阶梯校验：不在该模型阶梯上的档位返回本地 **400**（`MirasimThinkingConfigError`），
  不再转给上游；DeepSeek 的 `off`、GLM/Kimi 拒绝 `off` 的边界与官方一致。
- 签名 roster 的拉取并入既有只读元数据路径（与 `/v1/models` 同节奏、`404/501` 等失败只记 debug），
  只解析 `adaptive` 显式标记（`agents[family]` 覆盖 `models` 共享定义）。
- 该 roster **不参与**模型白名单（白名单仍以账号维度 `/v1/models` 为唯一来源）。

## Impact

- Affected spec: `mirasim-relay-upstream`
- 代码：`internal/service/mirasim_thinking.go`（新）、`mirasim_roster.go`（新）、`mirasim_relay.go`
  （roster 拉取/缓存/形态读取）、`mirasim_metadata.go`（元数据路径触发刷新）、
  `gateway_pipeline_claude.go`（归一挂点）、`internal/handler/gateway_handler.go`（400 通道）
- 现网风险评估：relay 账号近 24h `/v1/messages` 的 effort 只有 `high`(320)/`(null)`(224)/`low`(4)，
  全在阶梯内；`/v1/responses` 路径本次不改。
- 回滚：纯请求体改写 + 新错误类型，回滚即恢复透传；无数据迁移。
