# Design: thinking 形态与 effort 阶梯

## D1 形态来源：roster 的 adaptive 标记，不猜模型名

`adaptive` 只在 roster **显式出现**时才算数（缺字段 ≠ false），否则会把所有模型误判成预算型。
默认（无 roster / 无该模型 / 形态未知）是自适应型——relay 上所有 Claude 模型都是这一形态，
未来新模型也因此默认正确。

## D2 请求路径不拉 roster

roster 只在只读元数据路径（`AvailableModels`）刷新并写入运行期客户端缓存，请求路径**只读**该缓存。
冷缓存 = 默认形态。这样 roster 慢/不可达永远不会给推理请求加延迟（与官方客户端同语义）。

## D3 折衷而不是拒绝

- 自适应型 + 固定预算：折成最近档位（`budgetToLevel`，`minimal → low`）；
- 预算型 + effort：转成 `budget_tokens`（`low=1024 / medium=8192 / high=24576 / xhigh=32768 / max=128000`），
  并夹紧到 `< max_tokens`；`max_tokens` 太小（无法满足 `≥1024 且 < max_tokens`）才 400。
- 只有「档位不在该模型阶梯上」「预算型模型收到固定预算」「GLM/Kimi 请求 off」才本地 400。

## D4 400 的落地方式

新增 `MirasimThinkingConfigError`（带 `StatusCode() = 400`），在 messages handler 里与
`BetaBlockedError` 同一模式处理；错误消息只含模型名与阶梯说明，不含凭据或上游正文。

## D5 与既有 Chinese-LLM thinking 补正的关系

`NormalizeChineseLLMThinking` 仍先跑（它处理 MiniMax 等只接受 adaptive 的上游）；relay 归一在其后、
用**映射后**的模型名执行，避免与前者相互覆盖。
