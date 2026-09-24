# Change: Restrict Mirasim Whitelist To Available Models

## Why

`add-mirasim-model-roster-and-limits` 让 mirasim 的「同步上游模型」改走 `GET /v1/model-roster`，
按 agent 选择产出模型 id。实机验证后发现**这条路选出来的模型在当前账号上根本调不通**：

对同一份凭据逐个真实调用（`POST /v1/messages`，`max_tokens=16`）17 个 roster id，结果是：

| 结果 | 模型 | 说明 |
| --- | --- | --- |
| 200 + 真实内容 | `kimi-k3`、`deepseek-flash`、`glm-5.3-flash` | 可调用 |
| 404 | `claude-opus-5-5`、`claude-fable-5-1`、`claude-opus-5`、`claude-sonnet-5`、`claude-fable-5`、`claude-opus-4-8`、`claude-haiku-4-5`、`gpt-6-sol`、`gpt-6-astra`、`gpt-6-luna`、`gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna` | `model "X" is currently unavailable for this account; it has not been removed, and other models are unaffected` |
| 422 | `kimi-code/k3` | `model "kimi-code/k3" is not supported at this time` |

而缺省 agent 选择顺序落到 `claude` 组，于是「同步上游模型」实际同步的 6 个 Claude 模型
**全部是 404**；白名单被填成一组一调就失败的模型。

同时 relay 上有另一条**账号维度**的只读接口 `GET /v1/models`，它返回的是「本账号当前可用的
模型」：

```json
{"object":"list","data":[{"id":"deepseek-flash","object":"model","owned_by":"mirasim-router"}, ...]}
```

实测该接口列出 5 个 id，且**逐个真实调用全部 200 并返回内容**：

`deepseek-flash`、`deepseek-v4-flash`、`deepseek-v4-flash-vision-exp`、`glm-5.3-flash`、`kimi-k3`

对比 roster：它不仅把 `claude`/`gpt` 这些当前不可用的模型当作默认白名单，还**根本没有**
`deepseek-v4-flash` 与 `deepseek-v4-flash-vision-exp` 这两个可用 id。也就是说 roster 既多给了
不可用的，又少给了可用的，不适合作为白名单来源。

## What Changes

- 「同步上游模型」对 mirasim 账号改走 `GET /v1/models`（账号维度可用模型），**取代**
  `add-mirasim-model-roster-and-limits` 的 roster + agent 选择行为；同步结果只包含该账号当前
  真正可用的模型 id。
- 新增 OpenAI 风格响应解析（`{object, data:[{id}]}`）：`id` 必须为非空字符串，`trim` 后去重、
  稳定排序；列表为空视为失败（映射为既有 `UpstreamModelSyncErrorUpstream`）。
- 保留既有错误映射语义：凭据/配置缺失 → `configuration`（且不发起上游请求）；网络/超时/
  非 2xx/解析失败 → `upstream`；错误消息与日志不含凭据与上游正文。
- 显式点击「同步上游模型」时**绕过缓存**重新拉取（沿用上一轮已修的行为）。
- 按 `design.md` 的决策移除 roster 的**白名单用途**（是否同时删除解析代码见 design 决策 D2）。
- `/v1/limits` 用量链路、模型转发与调度逻辑**不变**。

## Impact

- Affected specs: `mirasim-relay-upstream`
- Affected code:
  - `backend/internal/service/mirasim_relay.go`（新增 `/v1/models` 只读通道）
  - `backend/internal/service/mirasim_metadata.go`（新增 `AvailableModels` 与缓存）
  - 新增 `backend/internal/service/mirasim_available_models.go`（解析）
  - `backend/internal/service/upstream_models.go`（mirasim 分支改源）
  - `backend/internal/service/mirasim_roster.go`（按 D2 决定去留）
- Supersedes: `add-mirasim-model-roster-and-limits` 中「Mirasim Upstream Model Sync」这条
  requirement 的 roster 来源；用量（limits）部分不受影响。
- Frontend: 无需改动（白名单是通用字符串列表，`ModelWhitelistSelector` 不受影响）。
