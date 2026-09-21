## 1. 规格与现状审计

- [x] 1.1 确认 `update-openai-overload-failover` 与 `harden-openai-pool-outcome-sticky-stream` 的既有需求边界，避免需求名与语义重叠（前者管跨账号恢复与流重放安全，后者管亲和元数据；本次只加「重试时的会话身份轮换」与「即时清票」，需求名不重叠）
- [x] 1.2 确认 `thread-id` 的完整消费链：`resolveCodexFingerprintIDsFromContext` → `applyCodexFingerprintHeaders` / `applyCodexFingerprintJSONBytes` / `applyCodexFingerprintClientMetadata`，覆盖 HTTP 与 WS 两条出站路径（共 9 个调用点，均走同一 gin context 缓存）
- [x] 1.3 确认同一请求内指纹 ID 只解析一次并在头改写与体改写间共享（原实现直接缓存 `*codexFingerprintIDs`；改为缓存 `(attempt, ids)` 条目，同一 attempt 内仍是同一实例）

## 2. 实现

- [x] 2.1 `resolveConvergedThreadID` 接受 attempt 维度；attempt<=0 时保持现有确定性派生（种子 `codex:thread-id:v1:<acc>:<sess>`），attempt>0 时追加 `:<attempt>`
- [x] 2.2 同账号重试递增 attempt 并向下传递：handler 侧 4 个重试点调用 `rotateCodexSessionIdentityForSameAccountRetry`；WS 重连重试循环在 `reason == "server_overloaded"` 时调用 `BumpCodexFingerprintAttempt`
- [x] 2.3 每次重试重新解析指纹 ID（`BumpCodexFingerprintAttempt` 清除 context 缓存），同一 attempt 内头/体仍共享同一实例；WS ingress session 的指纹快照增加 `fingerprintAttempt` 校验，轮次变化时刷新并重置 turn 序号，避免同一请求的头/体携带不同 thread_id
- [x] 2.4 `server_is_overloaded` 走 `DiscardOpenAICodexTurnStateForOverload`：立即清票，绕过 `openAICodexTurnStateUpstream5xxInvalidateThreshold` 与 2 分钟冷却；爆发窗口记录与票据复用解耦
- [x] 2.5 流内 `response.failed` + `error.code == "server_is_overloaded"` 归入同一恢复判定（复用既有 `isOpenAIUpstreamCapacityShedEvent` → `openAIStreamFailedEventRetryableOnSameAccount`），并补上 `ServerOverloaded` / `OverloadPath=stream_failed` 标记；仍受既有 `writerSizeBeforeForward` 边界约束
- [x] 2.6 过载爆发窗口预算：`openai_codex_overload_burst.go` 按账号记录 30 分钟窗口，第 2 次及以后的过载在同账号重试上限上 +2（`OpenAICodexOverloadRetryBudget`），窗口外/非过载保持基线
- [x] 2.7 结构化观测字段：`overload_path`、`fingerprint_attempt`、`thread_id_rotated`、`thread_id_rotated`（WS 重连日志）、`burst_attempt`（清票日志）

- [x] 2.9 **跨请求过载 epoch**：新增按 (账号, 客户端会话) 的过载 epoch（`rotateOpenAICodexOverloadSessionEpoch` / `openAICodexOverloadSessionEpoch`），过载后该组合的 `thread-id` 派生在 30 分钟窗口内使用新种子（`v2:<acc>:<session>:<epoch>[:<attempt>]`），窗口过期回到 `v1` 确定性派生；epoch 单调递增不回退，键为会话标识摘要不保留原文，窗口外零锁快速路径
- [x] 2.10 过载入口统一为 `HandleOpenAICodexOverload(account, path, clientSessionID)`（爆发窗口 + epoch + 清票三合一）；WS ingress session 快照按 (attempt, epoch) 双维度校验刷新
- [x] 2.8 流内降载但**不可重放**（已输出可见内容）时也要清票 + 记爆发窗口：两条流式路径此前返回普通 error（非 `*UpstreamFailoverError`），过载处理完全不会触发（生产实测 151 例）

## 3. 验证

- [x] 3.1 单测：`TestResolveConvergedThreadID_AttemptZeroIsLegacySeed` 断言 attempt=0（及负值）与历史种子逐字节一致
- [x] 3.2 单测：`TestResolveConvergedThreadID_AttemptRotatesThread` + `TestBumpCodexFingerprintAttempt_RotatesContextIDs`（thread_id 变化、session_id 不变、同 attempt 复用实例）
- [x] 3.3 单测：`TestDiscardOpenAICodexTurnStateForOverload_ClearsPinnedTicket`（首次过载即清票）+ `..._RespectsDisableSwitch`
- [x] 3.4 单测：`TestIsOpenAIServerOverloadedError`、`TestNewOpenAIUpstreamFailoverError_MarksHTTP503Overload`、`TestClassifyOpenAIWSErrorEventFromRaw_ServerOverloaded`、`TestClassifyOpenAIWSReconnectReason_ServerOverloadedIsRetryable` 覆盖两条通道进入同一恢复分支
- [x] 3.5 单测：`TestBumpCodexFingerprintAttempt_FullModeKeepsThreadEqualToSession`（full 模式语义不被轮换破坏）+ 既有流重放安全测试保持通过
- [x] 3.6 运行定向 Go 测试与构建（见下方「验证记录」）
- [x] 3.9 单测：跨请求轮换（`TestOverloadEpoch_RotatesThreadIDAcrossRequests`，禁用 epoch 查找后该用例失败，确认覆盖有效）、未过载会话保持历史派生逐字节一致、窗口过期回退、账号/会话隔离、开关关闭不写 epoch
- [x] 3.8 单测：流内降载「已输出可见内容」时清票、不重放；对照组「非降载流内失败」不清票（删掉新代码块后该用例失败，确认覆盖有效）
- [x] 3.7 `openspec validate update-codex-retry-identity-rotation --strict`

## 4. 灰度与验收

- [x] 4.1 设计 A/B：同账号重试分两组，A 组 `disable_codex_overload_identity_rotation=true`（保留确定性 `thread-id`），B 组默认（带 attempt 序号）；对比重试成功率与用户可见 503
- [x] 4.2 记录基线指标（实测见 `production-baseline-2026-09-21.md`：同账号重试最终成功 32.8%=45/137；打满 3 轮仅 7.1%=7/99；403 触发 0/42；40 天日志内字面量 `server_is_overloaded` 0 次，但网关不打印上游 code，故仅可由新增 `overload_path` 判定）：`openai.pool_mode_same_account_retry` / `openai_messages.pool_mode_same_account_retry` / `openai_chat_completions.pool_mode_same_account_retry` / `openai_alpha_search.pool_mode_same_account_retry` 的 `thread_id_rotated`、`overload_path`、`fingerprint_attempt`；WS 侧 `reconnect_retry` 的同名字段；票侧 `[CodexTurnStateSeed] reason=server_overloaded`
- [x] 4.3 定义验收阈值（预期 B 组重试成功率从约 20% 升至 60%+）
- [x] 4.4 文档化回滚方式与停止条件：回滚开关 `gateway.disable_codex_overload_identity_rotation=true`；停止条件 = 重复计费、prompt cache 命中率异常下降、或转号后账号健康指标恶化

灰度与验收的执行细节见 `rollout.md`（A/B 分组、日志字段、验收指标、回滚与停止条件）。

生产基线实测与观测限制见 `production-baseline-2026-09-21.md`；其中第四节记录了当前实现「仅覆盖请求内重试、未覆盖跨请求」的局限。

## 验证记录

```bash
# 构建
cd backend && go build ./...                       # OK
go vet ./internal/handler/ ./internal/service/ ./internal/config/   # OK

# 定向用例
go test ./internal/service/ -run 'Overload|CodexFingerprint|ThreadID' -count=1   # ok

# 全量回归：与基线对比「无新增失败」
go test ./internal/service/ -count=1   # 6 个失败均为改动前既有（rate-limit / image-bridge / compact），
                                       # comm -13 基线失败集 与改动后失败集 = 空
go test ./internal/handler/ ./internal/config/ -count=1                # ok
```
