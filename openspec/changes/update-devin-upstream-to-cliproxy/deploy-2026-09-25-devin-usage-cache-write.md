# Devin 用量：cache write 被丢掉，输入恒为 3

日期：2026-09-25（侧车替换 05:23 JST）
代码：CLIProxyAPI `v7.3.13`（`2430354330af`）+ `cli-proxy-cachewrite.patch`，无 sub2api 网关改动
生产机：74.82.195.84，容器 `sub2api-devin-cliproxy`

## 现象

账号 126（Devin）自 9 月 23 日切到 CLIProxy 后，用量行大量出现 `input_tokens = 3`。9 月 24–25 日全天每条都是 3，其中绝大多数 `cache_read_tokens` 为数万到数十万，缓存命中率显示约 100%。

## 原因

Devin 的用量帧里：field 2 是未缓存输入（缓存生效后经常只剩 3），field 4 是 cache write，field 5 是 cache read。CLIProxy 计算 OpenAI usage 时：

```go
totalInput := PromptTokens + CachedTokens // 不含 CacheWriteTokens
```

而 interactions → `/v1/responses` 的转换（`setResponsesUsageFromInteractions`）只拷贝 `total_input_tokens` 和 `total_cached_tokens`，把 `cache_write_tokens` 丢掉。于是一段从未见过的长提示被记成 `input_tokens: 3`、`cached_tokens: 0`，中间的几千 token 直接消失，不参与计费。

网关侧 `RecordUsage` 再按 OpenAI 语义做 `input - cache_read`。所以缓存命中的请求会显示成「输入 3、缓存 ~100%」：上游报的就是未缓存余量 3，其余在 `cached_tokens`。缓存读取单价是输入的 1/10（实测 `gpt-5.6-terra`：输入 3 token = $0.000030，即 $10/1M；缓存 128358 token = $0.128358，即 $1/1M）。

## 修复

`cli-proxy-cachewrite.patch`：两处 `totalInput` 改为 `PromptTokens + CachedTokens + CacheWriteTokens`。cache write 进入 `input_tokens`，不单列成 `cached_tokens`，因此网关把它按普通输入计费，而不是再减掉。

## 发布

| 项 | 值 |
| --- | --- |
| 新目录 | `/opt/sub2api/bridges/devin-cliproxy/releases/v7.3.13-cachewrite`（从 `v7.3.13` 复制后替换二进制；`config.yaml` 未改） |
| 二进制 | `cli-proxy-api`，`sha256=63fb4d2d8b6a7bc5003fe265841946a200d99823f74286cda5d5f5d5856e35eb`，linux/amd64 静态链接 |
| 容器 | 新建 `sub2api-devin-cliproxy`（同名、同网络 `sub2api_sub2api-network`、`127.0.0.1:18363:8317`、同样的 auths / CA 挂载、`--restart unless-stopped`）。旧容器改名为 `sub2api-devin-cliproxy.bak-20260925` 并停止，挂载仍指向 `releases/v7.3.13` |
| 网关 | 未重新发布。API 容器解析 `sub2api-devin-cliproxy:8317` 返回 401（无密钥，符合预期） |

## 核验

同一段约 8600 字符的全新提示，`gpt-5.6-terra`：

| | input_tokens | cached_tokens |
| --- | --- | --- |
| 补丁前 | 3 | 0 |
| 补丁后 | 2923 | 0 |

同会话追加一段新文字：`1959 / cached 0` → `3167 / cached 0`，新增部分留在输入里，不再被吃成 3。短提示补丁前后都是 `593`，`swe-2` 仍是 `675`。`/v1/models` 仍返回 40 个模型。

## 回滚

```bash
docker stop sub2api-devin-cliproxy && docker rm sub2api-devin-cliproxy
docker rename sub2api-devin-cliproxy.bak-20260925 sub2api-devin-cliproxy
docker start sub2api-devin-cliproxy
```

## 未改变的部分

Devin 自己把已缓存的历史报成 field 5（cache read）时，网关仍会把输入列显示成未缓存余量。相同长提示再发一次实测为 `input_tokens=2193, cached_tokens=2190`，扣完缓存后输入仍是 3、命中率约 100%，并按 $1/1M 计缓存。这是上游的缓存拆分，不是这次丢掉的 cache write。若希望这类命中也按全价输入计费，需要另改网关。

## 端到端验证（2026-09-25 04:32 +08，经真实网关 `https://code.sicts.shop/v1/responses`）

同一段约 10500 字节的全新提示，`gpt-5.6-terra`（group 4 里只有账号 126 提供该模型，路由确定）：

| usage_logs id | 时间 | 场景 | input_tokens | cache_read_tokens | total_cost |
| --- | --- | --- | --- | --- | --- |
| 3034022 | 03:39（修复前） | 全新提示 | **3** | 0 | 0.003230 |
| 3034346 | 04:32（修复后） | 全新提示 | **2953** | 0 | 0.005966 |
| 3034347 | 04:32（修复后） | 原样重发（上游缓存命中） | 3 | 2950 | 0.000656 |

- 侧车直连同尺寸提示：补丁前 `input_tokens 3 / cached 0`，补丁后 `2923 / 0`；新增文字会留在输入里（`1959 → 3167`）。
- 原样重发仍是上游的缓存语义（未缓存余量 + cache read），按 $1/1M 计 0.000656；首次出现按 $10/1M 计 0.005966。
- 运行中的侧车：容器创建 `2026-09-24T20:23:03Z`，`/app/release` 挂载到 `.../releases/v7.3.13-cachewrite`，二进制 `sha256=63fb4d2d8b6a7bc5003fe265841946a200d99823f74286cda5d5f5d5856e35eb`；回滚目录二进制 `8d3a465e5a8b8653ab35ca2eac6dc64e173653ec6243909e62b5deea00d971a0`，旧容器 `sub2api-devin-cliproxy.bak-20260925` 已停止但保留挂载。
- 可复跑脚本 `verify-devin-cache-write.sh`（生产机执行，断言首次出现的提示 `input_tokens >= 500`）。实测输出：`input_tokens 2953 cached_tokens 0` → `PASS`。

## 其它模型与多轮对话（2026-09-25 04:34–04:52 +08）

4 轮同一会话（网关请求带固定 `Session-Id` + `prompt_cache_key`）：

| 轮次 | 上游 input | 上游 cached | 落库 input | 落库 cache_read | 命中率 |
| --- | --- | --- | --- | --- | --- |
| 1 | 1608 | 0 | 1608 | 0 | 0% |
| 2 | 2840 | 1605 | 1235 | 1605 | 56.5% |
| 3 | 4072 | 2837 | 1235 | 2837 | 69.7% |
| 4 | 5304 | 4069 | 1235 | 4069 | 76.7% |

每轮 cached 等于上一轮整段 prompt，落库 input 是本轮新增内容（1235），不再是 3。若客户端不带会话标识，缓存不会命中（早先只写 body `conversation_id` 的两轮测试 cached 恒为 0）。

模型抽样（侧车直连，全新长提示，`cached` 均为 0）：swe-2 2562、gpt-5.6-terra 3130、gpt-5.6-luna 2997、gpt-6-astra 2870、gpt-5.6-sol 3000、claude-sonnet-5 4348、gemini-3.6-flash 4704、grok-4.6 3063（cached 128）、kimi-k3 2562；短提示 419/420。网关侧再抽 `gpt-5.6-luna`：`input_tokens=402`（usage_logs 3034364）。

全库对照（近 24h）：`input_tokens=3` 只有账号 89 出现 1 次（Devin 补丁前 69 次）；近 14 天「有输出但三类 token 全 0」= **0 行**。

已知且与本次修复无关的上游抖动：偶发**空响应**（无内容、usage 全 0，计费 0），补丁前后都有，例如 2026-09-16 28/135、09-13 4/177、09-23 9/243。

## swe 系列的缓存（2026-09-25 04:55–04:56 +08）

同一会话 6 轮，`swe-2`，网关请求带固定 `Session-Id` + `prompt_cache_key`：

| 轮次 | 上游 input | 上游 cached | 落库 input | 落库 cache_read | 命中率 |
| --- | --- | --- | --- | --- | --- |
| 1 | 1383 | 0 | 1383 | 0 | 0% |
| 2 | 2325 | 0 | 2325 | 0 | 0% |
| 3 | 3267 | 1344 | 1923 | 1344 | 41.1% |
| 4 | 4209 | 64 | 4145 | 64 | 1.5% |
| 5 | 5151 | 448 | 4703 | 448 | 8.7% |
| 6 | 6094 | 0 | 6094 | 0 | 0% |

对比 `gpt-5.6-terra` 同结构会话（1608/0 → 1235+1605 → 1235+2837 → 1235+4069）：swe-2 会建缓存，但命中不稳定，第 3 轮先命中第 1 轮的 prompt（1344），随后在 64 / 448 / 0 之间抖动。数字全部来自 Devin 的 field 5，网关按原值记账、不做修正。上一轮把输出预算压到 16 导致 `status=incomplete` 的会话里同样出现过一次 1344 命中，说明与截断无关。同时段真实流量里也有 `gpt-5.6-sol` 一条 `input=54772 / cache 0`，说明线上仍存在完全不命中缓存的客户端请求。

## 流式多轮（2026-09-25 04:58–04:59 +08，`stream=true`）

`stream` 才是线上真实路径（Codex 全部走流）。同一会话 6 轮，`swe-2`，网关请求带固定 `Session-Id` + `prompt_cache_key`：

| 轮次 | 上游 input | 上游 cached | 落库 input | 落库 cache_read | 命中率 |
| --- | --- | --- | --- | --- | --- |
| 1 | 1383 | 0 | 1383 | 0 | 0% |
| 2 | 2325 | 0 | 2325 | 0 | 0% |
| 3 | 3267 | 0 | 3267 | 0 | 0% |
| 4 | 4209 | 448 | 3761 | 448 | 10.6% |
| 5 | 5151 | 3264 | 1887 | 3264 | 63.4% |
| 6 | 6094 | 5120 | 974 | 5120 | **84.0%** |

`usage_logs` 3034495–3034500（`stream=t`）。流式下 swe-2 的缓存是递进的（第 4 轮出现，之后 10.6% → 63.4% → 84.0%），比同一模型同步会话（41.1% → 1.5% → 8.7% → 0%）稳定。

两个与缓存无关的观察：

- `gpt-6-astra` 的同类流式请求被调度到账号 89（两个账号都声明该模型），落库 `account_id=89`（3034502–3034507），不属于 Devin 侧车。
- 2026-09-25 04:56:31 账号 126 的 `model_mapping` 被更新，`gpt-5.6-terra` / `gpt-5.6-luna` / `gpt-5.6-sol` 已不在映射中，因此早先那组 terra 多轮测试无法重跑（当前可用：`swe-2`、`swe-2-high/max/medium`、`gpt-6-astra`、`gpt-5.4`、`gpt-5.4-mini`、`gpt-5.3-codex`）。
