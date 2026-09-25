# 请求耗时详情分段 — 生产发布

日期：2026-09-25 06:30 JST（05:30 +08）
代码：`61e5f051f`（前端 + 后端首版）、`ba5c35d19`、`46cf8ce51`（后端补埋点）
change：`add-request-timing-detail`
生产机：74.82.195.84

## 1. 产物

| 层 | 内容 |
| --- | --- |
| 前端 | release `/opt/sub2api/frontend/releases/0.1.391-61e5f051f`（171 文件，聚合 `54635e8729e9827cc0f3f976e6cfdbb44488ede3a5cb339adaf6f2f450677bab`，`index.html` = `799665e4c277389b513b2ca0bd11c1f69b8e712037e209241a7f8fe6f24a2447`）；`current` → 该版本，`rollback` → `0.1.391-01a904fb3` |
| 后端 | 最终运行二进制 `sha256=7260ed5cc0357b32f0a8da22539e722a73e9d14b3b702e613c97d7763becb61d`（由 `46cf8ce51` 构建）；备份 `sub2api.bak-20260925-timing2`（`a49fb479…`，即首版 `61e5f051f`） |
| 容器 | `sub2api-api-candidate-0.1.391-noinject-a33b47ef9`，`Up (healthy)`、`RestartCount=0` |

## 2. 线上核验（真实流式请求）

`POST https://code.sicts.shop/v1/responses`（`gpt-5.6-luna`，`stream=true`，客户端 key）→ 200，落库 `first_output_stages`：

```json
{
  "outcome": "success", "body_bytes": 108, "body_first_wait_ms": 5, "body_receive_ms": 0,
  "body_read_ms": 0, "body_complete": true, "api_key_auth_ms": 4, "user_queue_ms": 0,
  "account_select_ms": 16, "account_queue_ms": 0, "handler_body_read_ms": 0, "handler_total_ms": 2135,
  "upstream_attempts": 1, "connection_reused": false, "upstream_connect_ms": 1, "upstream_write_ms": 38,
  "upstream_first_byte_ms": 870, "upstream_response_headers_ms": 870, "upstream_stream_transfer_ms": 2134,
  "response_stream_complete": true,
  "gateway_pre_upstream_ms": 32, "upstream_first_output_ms": 1988, "retry_failover_ms": 0,
  "gateway_first_output_write_ms": 0, "gateway_first_output_total_ms": 2022
}
```

旧 6 段与新分段同时在同一个 JSON 中，历史记录缺字段时前端显示「未采集」。

## 3. 已知未采集项

- **安全审核**：当前 OpenAI 链路没有内容审核中间件，字段留空（前端显示「未采集」）。
- **大请求排队**：本仓库没有大请求队列，字段留空。
- WebSocket 透传、Anthropic/Gemini 专用分支未接入本采集器，仅旧 6 段可用。

## 4. 回滚

- 前端：`ln -sfn /opt/sub2api/frontend/releases/0.1.391-01a904fb3 /opt/sub2api/frontend/current`
- 后端：`docker stop <container>` → `install -m 0755 .../sub2api.bak-20260925-timing2 .../sub2api` → `docker start`
- 无数据库迁移；新增 JSONB 字段不会被旧版本读取。

## 5. 追加：安全审核分段、Anthropic/Gemini/WS 接入（06:45 JST）

| 提交 | 内容 |
| --- | --- |
| `f0c8b0e1b` | 新增 `security_audit_ms`（`runContentModeration` 唯一入口埋点）；`recordUsageCore` 合并采集器分段；`ForwardResult` 增加 `FirstOutputStages`；WS 两条路径与 chat completions 快照合并 |
| `059819093` | 修复：异步用量落库使用 `context.Background()`，采集器不在其中 → 改为在 handler 提交任务前用请求上下文同步合并（`/v1/messages` 等路径由此生效） |
| `295df3846` | Anthropic/Gemini 路径补结果标记 |
| `7eb5fce15` / `0fe66ec18` | 账号选择埋点：`SelectAccountForModelWithExclusions` 与网关 handler 的 5 处 `accountSelector.Select` |

**线上核验**：

- OpenAI `/v1/responses`（账号 126）：`outcome=success`、`account_select_ms=10`、`security_audit_ms=2`、`upstream_attempts=1`，其余分段齐全。
- Anthropic `/v1/messages`（账号 140，key 206）：`outcome=success`、`account_select_ms=6`、`security_audit_ms=2`、`user_queue_ms=1`、`connection_reused=true`、`upstream_attempts=3`、`upstream_connect_ms=818`、`upstream_first_byte_ms=2367`、`upstream_stream_transfer_ms=2371`、`response_stream_complete=true`。
- 容器 `Up (healthy)`、`RestartCount=0`；近 10 分钟 5xx = 0。

**仍未采集**：`account_queue_ms`（Anthropic 路径无账号槽等待）、`security_audit_ms` 之外的大请求排队（本仓库无该队列）。

**运行中的后端二进制**：`sha256=0fe66ec1869d4aadb5e8b68cc32c8fa303f2c972c0c3b4a986cefc844252b8fc`（由 `0fe66ec18` 构建）。中间态备份：`sub2api.bak-20260925-timing2`（`a49fb479…`）、`sub2api.bak-20260925-timing3`（`e8298eea…`）。

## 6. 瀑布时间线（07:15 JST，提交 `6576d0200`）

按运营要求把展示从「数值列表」升级为**横向时间线**：左端请求开始、右端请求结束，内部按阶段着色，悬停显示区间与耗时，并标注请求体体积与首 Token 位置。

- **后端**：采集器新增 `segments`（`key` / `lane` / `start_ms` / `end_ms` / `attempt`）。入站体（首读等待 + 接收区间）、鉴权、安全审核、账号选择、用户/账号并发、上游（连接获取 / 写入 / 首字节 / 响应头 / 流传输）都带起止偏移；上游按尝试编号分泳道；`httptrace` 回调内直接落区间。
- **前端**：新增 `RequestTimingWaterfall.vue`（泳道 = 客户端→网关 / 网关内部 / 上游；色块按阶段；图例汇总各阶段总耗时；首 Token 红色竖线；体积 chip）。数值分组折叠进「分段数值」details。
- **发布**：前端 release `0.1.391-6576d0200`（聚合 `2bcf0f0b9e45dd92e88c937af3917a7f1965d5f0e87833b6081545527424c5f5`，`index.html` = `82fa1151c6dc514c55bb0e612e1de04279010ab94b9463df2e67c0c488899a26`）；后端 `sha256=ee5ff59b4cadede53cc4c499bc6d26fa421af0e449fab5f65665d7c9e8055f8d`，备份 `sub2api.bak-20260925-waterfall`。容器 healthy、5xx=0。

**线上实测 segments（`/v1/responses`，总耗时 2.63s）**：

```json
[{"key":"body_wait","lane":"client","start_ms":0,"end_ms":13},
 {"key":"body_receive","lane":"client","start_ms":13,"end_ms":13},
 {"key":"api_key_auth","lane":"gateway","start_ms":0,"end_ms":13},
 {"key":"security_audit","lane":"gateway","start_ms":13,"end_ms":1426},
 {"key":"user_queue","lane":"gateway","start_ms":1435,"end_ms":1436},
 {"key":"account_select","lane":"gateway","start_ms":1443,"end_ms":1454},
 {"key":"account_queue","lane":"gateway","start_ms":1454,"end_ms":1454},
 {"key":"upstream_connect","lane":"upstream","start_ms":1454,"end_ms":1455,"attempt":1},
 {"key":"upstream_write","lane":"upstream","start_ms":1455,"end_ms":1455,"attempt":1},
 {"key":"upstream_first_byte","lane":"upstream","start_ms":1455,"end_ms":2352,"attempt":1},
 {"key":"upstream_response_headers","lane":"upstream","start_ms":1454,"end_ms":2352,"attempt":1},
 {"key":"upstream_stream","lane":"upstream","start_ms":2352,"end_ms":2631,"attempt":1}]
```

**由此发现的性能事实**：生产 `content_moderation_config` 为 `enabled=true`、`mode=pre_block`，走 `https://api.openai.com/omni-moderation-latest` 同步调用，占该请求 2.63s 中的 **1.41s**（54%），是网关侧最大单项耗时。是否需要优化（缓存 / 并行 / 改后置）由运营决定，本 change 只负责让该耗时可见。

## 7. 收尾：槽位埋点、时间刻度、审核耗时分布（14:06 JST）

| 提交 | 内容 |
| --- | --- |
| `df38e1b73` | 调度层 `TryAcquireAccountSlot` 埋点 → Anthropic/Gemini 路径补齐 `account_queue_ms`；后端 `sha256=5653ae0df1354ff1fbe1cdf0dedf942510fabf7fad47d7a6c12fe82df9bb31fe` |
| `1268f0e7d` | 瀑布加时间刻度轴（0 / 中位 / 总耗时）与块内时长标签；前端 release `0.1.391-1268f0e7d`（聚合 `bbf4ded8a029975720ab958e6c28e9c4b50dabfb04e610b5254c0f204f1777e5`） |

**线上核验**：Anthropic `/v1/messages` 的 segments 现含 `account_queue`（32→33ms）；容器 healthy、5xx=0。

**安全审核耗时分布（近 3 小时 526 条 staged 行）**：p50 = **102ms**、p95 = **634ms**、≥1100ms（打满 `timeout_ms=1200`）= **8 条**、≥100ms = 275 条（52%）。

生产审核配置：`enabled=true`、`mode=pre_block`、`sample_rate=15`、`timeout_ms=1200`、`retry_count=1`、`pre_hash_check_enabled=true`、`worker_count=4`、`base_url=https://api.openai.com`、`model=omni-moderation-latest`。即：约 15% 的请求做抽样 API 审核，命中干净 hash 缓存则跳过；仍未跳过时中位 102ms、尾部会打满 1.2s 超时。是否调 `timeout_ms` / `sample_rate` / 改后置审核属运营策略，本 change 只负责让耗时可见。

## 8. 分段数值默认折叠（14:20 JST，提交 `d1a85a25e`）

按运营要求，弹窗默认只展示瀑布时间线；数值分组收进 `<details>`（默认收起，点击「分段数值」展开）。

- 前端 release `/opt/sub2api/frontend/releases/0.1.391-d1a85a25e`（聚合 `446c4db6c648259ad24516d5d61b914436b67fc315ed400eadd47773a6c8b6d6`，`index.html` = `63e4e3d69f923981f8b89ab6ed6f7398ff88abc159700cc76828de74ded3a233`，171 文件）
- `current` → `0.1.391-d1a85a25e`，`rollback` → `0.1.391-1268f0e7d`
- 组件测试 6 例通过；`vue-tsc --noEmit` 与 `pnpm build` 通过

## 9. 两项优化（14:30–14:45 JST）

### 9.1 首字节延迟感知选号（提交 `1391cf356`、`818240e57`）

- `AccountTTFTTracker`：账号×模型近期首字节中位数（20 样本 / 30 分钟 TTL / 最少 5 样本），在两条用量记录路径写入。
- `scheduling.OrderByPriorityLoadAndLatency`：同优先级、负载差 ≤ `scheduler_ttft_max_load_delta_pct`（默认 10 个百分点）时，若某候选快至少 `scheduler_ttft_min_advantage_pct`（默认 30%）则提升到当前位置；单趟提升、不跨优先级、不跨容差。
- 配置默认开启：`gateway.scheduler_ttft_preference=true`、`min_samples=5`、`min_advantage_pct=30`、`max_load_delta_pct=10`；测试：排序 3 例 + 追踪器 3 例。
- 后端二进制 `sha256=b0f748eaa1563d29baec4da01b5073fade11783d8e30caeb882abe1901a91ea5`。
- **生产现状（重要）**：该偏好当前**不改变任何路由**。近 30 分钟网关日志中 `account_simple_selection` 的多候选（同组同模型）次数 = **0**；`gpt-6-astra` 只有账号 89 有实际供给（89 priority 10 vs Devin 126 priority 50，优先级先决）；Grok Heavy 组虽映射了 5 个账号，但实测 5 次 `grok-4.6` 请求全部落在账号 86（其余账号不满足能力/额度/冷却条件）。要让它生效，需要同一分组内出现**两个同时可用的同优先级账号**。

### 9.2 安全审核尾部收敛（风险控制配置变更，无代码改动）

| 项 | 变更前 | 变更后 |
| --- | --- | --- |
| `timeout_ms` | 1200 | **400** |
| `retry_count` | 1（最多 2 次尝试 ⇒ 最坏 ~2.4s） | **0**（单次尝试 ⇒ 最坏 ~0.4s） |
| `group_ids` | `[4, 25]`（25 号分组已删除，校验会拒绝任何配置更新） | `[4]` |

- 变更方式：`PUT /api/v1/admin/risk-control/config`（首次因 `审计分组不存在: 25` 返回 400，故同时清理已删除分组）。
- 超时/失败语义不变：仍然 **fail-open**（放行）并计入 `pre_block_errors`（风控页可见，重启后已累计 8 次）。
- 生效验证（变更后新发 20 条请求）：`security_audit_ms` p50 = **2ms**、p90 = **150ms**、max = **229ms**、≥300ms = 0；变更前同口径最大值为 **1168ms**（14:25 样本）。

## 10. 小项：大请求体 gzip 压缩（15:00 JST，提交 `f71f936da`、`a65a7f526`）

**动机（来自采集数据）**：openai apikey 路径 body p50 ≈ 5.8 MiB，`upstream_write_ms` p50 ≈ 495ms / p95 ≈ 2.5s，其中 8.9MiB 级请求占多数。

**上游能力实测**：账号 89 的上游 `o10.top` 接受 `Content-Encoding: gzip` 请求体（对照组与实验组均 HTTP 200，响应尺寸一致）。

**实现**：

- `maybeGzipOpenAIUpstreamBody`：仅 **APIKey 账号**、上游 host 命中白名单、体积 ≥ 阈值时用 `gzip(BestSpeed)` 压缩并设置 `Content-Encoding: gzip`；OAuth(Codex) 仍走 zstd，外部桥与 compact unary 协议未验证一律不压。
- 配置：`gateway.upstream_gzip_min_bytes`（默认 1 MiB，0 关闭）、`gateway.upstream_gzip_hosts`（默认 `["o10.top"]`）。**清空白名单即可即时回退，无需重新发布**；启动日志会打印生效值（`gateway_upstream_gzip_config`）。
- 新增 `upstream_request_bytes`（出站请求体字节数，压缩后）落库并在弹窗「出站请求体」行展示，同时给出相对入站体积的百分比。

**验证**：

| 指标 | 结果 |
| --- | --- |
| 压缩是否生效 | **是**：近 4 分钟 9 条 >1MiB 请求全部压缩，入站 p50 2.68 MiB → **出站 p50 0.79 MiB（29.5%）** |
| 错误面 | 近 12 分钟 5xx = 0，`/v1/responses` 4xx = 0；账号 89 期间 56 条请求全部 `outcome=success` |
| 写耗时 | 出站字节与耗时关系：0.57 MiB → 301ms、3.01 MiB → 518ms、7.28 MiB → 884ms ⇒ 约 **100–170 ms/出站 MiB + 250–300ms 固定开销**。压缩把出站字节降到约 30%，但固定开销不随之下降，故端到端收益仍属「<1s」量级（与最初估计一致），不会改变 TTFT 主因（上游模型时间）。 |

**测试**：白名单/阈值/账号类型 4 例 + 真实转发路径集成 1 例（上游收到 gzip 且载荷可还原）+ 配置默认值 2 例。

制品：后端 `sha256=a0b7c5f18264812bb88df37140db00bc883d8a61ea2425af82623b4671a5677b`，前端 release `0.1.391-a65a7f526`（聚合 `609921c7a0f9458b5385cfa99dab7f09f63b026776b9c9be9232ce92585cbb3f`）。
