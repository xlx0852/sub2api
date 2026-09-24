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
