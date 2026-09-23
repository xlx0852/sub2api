## 1. 可观测性：空候选拒绝明细

- [x] 1.1 `selectByLoadBalance` 过滤循环始终收集 `CandidatePolicyTrace`（去掉 shadow 门控）。
- [x] 1.2 `filtered==0` 时输出 `openai_scheduler_no_candidates_reject_trace`，含逐账号被拒 policy、require_compact、required_capability、模型。

## 2. transport 错误与票失效解耦

- [x] 2.1 `UpstreamFailoverError` 增加 `TransportOnly` 字段；`handleOpenAIUpstreamTransportError` 置 true。
- [x] 2.2 `NoteOpenAICodexTurnStateUpstreamResult` 加 `transportOnly` 参数，true 时直接返回不清票。
- [x] 2.3 4 个 handler 调用点透传 `failoverErr.TransportOnly`。
- [x] 2.4 新增 `TestTurnStateTransportErrorDoesNotInvalidate` 回归测试。

## 3. 无候选错误分级

- [x] 3.1 `noAvailableOpenAIAccountError` 增加 `reason` 字段与 `noAvailableOpenAISelectionErrorWithReason`。
- [x] 3.2 各返回点标记 `platform_empty` / `filtered_all` / `model_filtered` / `acquire_or_recheck_failed`。

## 4. 验证门禁

- [x] 4.1 `go build ./internal/...` 通过。
- [x] 4.2 `go vet` + `gofmt` 干净。
- [x] 4.3 `go test ./internal/service/ -run 'TurnState|TransportError|Failover|SelectAccount'` 通过；3 个失败为基线遗留（已用 git stash 对照确认非本次引入）。

## 5. 不做（评估后移出，见 proposal）

- [ ] ~~拉黑合一删内存块~~ —— 内存/DB 是互补分层，删除会开"刚失败又选中"窗口；已通过 reject_trace 让拦截可观测。
- [ ] ~~票迁 Redis~~ —— 与 `harden-openai-pool-outcome-sticky-stream` 撞车，待其收口后单独立项。
- [ ] ~~三层粘性合并~~ —— 归 `promote-unified-scoring-scheduler`。
