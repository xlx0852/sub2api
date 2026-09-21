# 生产基线实测（2026-09-21）

只读分析，数据源为生产 4 个候选容器的容器日志聚合，**未发出任何上游请求**。

- 日志窗口：`2026-08-13T10:04:46+0800` → `2026-09-22T01:59:26+0800`（约 40 天）
- 日志行数：646,140
- 活跃实例：nginx `upstream sub2api_api` 指向的那个候选容器（具体名字与端口属内部信息，不入库）

## 一、同账号重试成功率（本变更的直接基线）

口径：`pool_mode_same_account_retry` 事件按 `request_id` 关联 `http request completed` 的 `status_code`。

| 维度 | 结果 |
|---|---|
| 同账号重试请求数 | **137**（可关联终态 137，100%） |
| 最终 200 | **45 / 137 = 32.8%** |
| 最终 502 | 92 |

按**触发状态码**拆分：

| upstream_status | 最终成功 | 说明 |
|---|---|---|
| 502 | 45/95 = **47.4%** | 传输类瞬时故障，重试有效 |
| **403** | **0/42 = 0.0%** | ⚠️ 42 次重试全部无效，见「独立发现」 |

按**打到的最高轮次**拆分：

| 最高 retry_count | 最终成功 |
|---|---|
| 1 | 27/27 = 100.0% |
| 2 | 11/11 = 100.0% |
| **3（打满预算）** | **7/99 = 7.1%** | ⚠️ 第 3 次重试近乎无用 |

**结论**：进入第 3 轮的重试几乎必然失败（7.1%）。这正是本变更的目标区间——把第 2/3 轮从「原样重发同一 thread-id」改为「轮换 thread-id 后重发」。

## 二、过载信号的观测限制（重要）

- 聚合 40 天日志中，字面量 `server_is_overloaded` 出现 **0 次**。
- **但这不是「生产不存在该错误码」的证据**：网关不打印上游错误码，只打印清洗后的 message 与状态码。因此本项**无法**从现有日志判定。
- 本变更新增的 `overload_path` 字段（`http_503` / `stream_failed`）正是为消除这个盲区而加的；灰度期即可得到真实占比。
- 可参考的代理指标：`openai.upstream_failover_switching` 中 `upstream_status=503` 有 **179** 次，这是 `server_is_overloaded` 最可能所在的集合（也可能是通用 503）。

## 三、独立发现：终态过载被透传（非本变更范围）

40 天内共 **151** 个请求以 SSE 终态错误结束，message 为
`Our servers are currently overloaded. Please try again later.`：

- **151/151 全部** `upstream_error_response_already_written=true`
- 时间高度集中：**2026-09-21 单日 148 条**（其余 2 天共 3 条）
- 集中于少数账号（2 个账号合计约 95%）与少数用户（前 8 个用户覆盖大部分）——
  具体账号/用户标识属内部信息，不在库内记录
- 其中 20 个请求此前已发生过 failover 动作（切号或同账号重试）后仍失败

### 为什么没有恢复（不是 bug，是设计边界）

两条流式路径的降载恢复闸门都**只在「尚未向客户端输出可见内容」时**求值：

`openai_gateway_response_handling.go:410-431`（非 passthrough）

```go
if !openAIStreamClientOutputStarted(c, clientOutputStarted) {
    ...
    if openAIStreamFailedEventShouldFailover(dataBytes, failedMessage) {
        streamEarlyErr = s.newOpenAIStreamFailoverError(...)   // ← 仅此处可切号重放
        return
    }
}
s.markOpenAIStreamTerminalError(...)   // 已输出 → 原样透传，sawFailedEvent=true
```

`openai_gateway_passthrough.go:1360`（passthrough）同构。

之后 `finalizeStream` 遇 `sawFailedEvent` 直接返回 `upstream response failed: ...`，handler 侧
`openAIForwardErrorAlreadyCommunicated`（`openai_gateway_handler.go:2674-2681`）识别
`upstream response failed:` 前缀后不再追加 fallback —— 于是错误被用户看到。

这与既有规格一致：**已输出可见内容后不重放**。因此这 151 条属于**结构性不可恢复**，
不是本次可修的缺陷。它们的正确对策是「让下一次请求更容易成功」，即：

1. 过载后立即丢弃被钉住的票（本变更已覆盖可重放的过载路径）；
2. 下一条请求使用不同的会话身份（**见第四节的局限**）。

补充：被判为不可恢复而返回的是**普通 error（非 `*UpstreamFailoverError`）**，
因此 `noteOpenAICodexUpstreamOutcome` 不会触发 —— 这条支路目前**不会**清票、也不记爆发窗口。

## 四、本变更的覆盖局限（自我纠正）

`thread-id` 的轮换轮次存在 **gin context（即单个 HTTP 请求）** 内。因此：

- ✅ **覆盖**：同一请求内的同账号重试（`pool_mode_retry_count` 次，生产实测最多 3 轮）。
- ❌ **未覆盖**：**跨请求**复用同一客户端 `session-id` 的重试。客户端下一次发起的是新 HTTP 请求，
  `CodexFingerprintAttempt` 归零，派生回**同一个** `thread-id`。

而本地实测里 3.36× 的收益（同 thread 19.6% vs 新 thread 65.7%）来自**客户端跨请求**复用同一
会话标识的场景。也就是说：**当前实现覆盖的是收益较小的一段，收益最大的那段尚未覆盖。**

若要让跨请求也生效，需要一个**按 (account, client session) 维度的过载 epoch**：
过载后该组合的派生进入新 epoch，持续到爆发窗口（30 分钟）过期。代价是客户端的正常续聊
在窗口内失去 thread 亲和（丢 prompt cache），属于需要用 A/B 验证的权衡。

## 五、复现命令

```bash
# 需要在生产读取容器日志（只读）；具体主机别名/容器名按内部运维文档执行
#   for C in <候选容器...>; do docker logs "$C" 2>&1; done | python3 -c '<单遍关联>'
```

聚合脚本见本文件「一、」与「三、」小节的口径说明（按 `request_id` 关联
`pool_mode_same_account_retry` → `http request completed`）。
