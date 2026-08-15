## 1. 基线与差分护栏

- [x] 1.1 记录 Change 1 干净基线与当前调度重复/差异清单
- 2026-08-15 1.1 完成：基线为干净提交 `50f9f1fb9f8b99040b75f6801532e2835f1b011e`；当前等价边界、生产调用点和必须保留的平台差异已记录在 `scheduling-inventory-2026-08-15.md`。核对确认 sort/shuffle 当前已是两侧共用的唯一 service 实现，本 change 仅下沉归属，不声称删除了两份副本。
- [x] 1.2 为 channel restriction、account list、sort/shuffle 和 sticky decision 补新旧 characterization/differential tests
- 2026-08-15 1.2 阶段记录：channel restriction 先补 `app/scheduling` 表驱动模型基准测试和 Gateway/OpenAI 薄入口对拍；account list、sort/shuffle、sticky 随后均按“测试先于迁移”补齐。
- 2026-08-15 1.2 account-list 进度：新增 snapshot-first、grouped/ungrouped、single/multi-platform 与 filter 表驱动测试，锁定查询分支、raw count 和过滤结果。
- 2026-08-15 1.2 ordering 进度：新增 priority/LRU、秒级 LastUsedAt 分组、preferred partition 和 equal-run 不跨组 shuffle 测试；原无 build-tag Gateway sticky 回归也覆盖共享候选排序路径。
- 2026-08-15 1.2 sticky 完成：新增 no-session/no-binding/excluded/candidate-missing/mismatch/clear/ready 纯决策表驱动测试，并运行 Gateway Layer 1.5、OpenAI legacy sticky、advanced sticky/escape/transport 的无 build-tag 针对性回归，全部通过。
- [x] 1.3 确认 `harden-openai-pool-outcome-sticky-stream` 的 outcome/sticky re-eval/lowest-usage 语义不在本 change 修改集合
- 2026-08-15 1.3 完成：改动未触及 outcome class、credential generation、sticky re-eval 设置/计数、lowest-usage、TopK 或权重公式；`openai_account_scheduler.go` 只将原 binding/exclusion/candidate-clear 前置判定替换为等价纯函数，后续 epoch/credential/transport/re-eval/escape 顺序未变。

## 2. 等价原语收敛

- [x] 2.1 合并 Gateway/OpenAI `checkChannelPricingRestriction` 为单一共享函数
- 2026-08-15 2.1 完成：计费模型基准选择下沉为 `scheduling.BillingModelForRestriction`，Gateway/OpenAI receiver 只保留薄入口并共用唯一 service 检查实现。`app/scheduling` 全测试与无 build-tag 双入口对拍通过；旧 unit-tag 完整渠道用例保留，不修改已知 dirty WIP 来强行全量编译。
- [x] 2.2 抽取 snapshot-first + grouped/ungrouped 基础候选查询骨架，保留 mixed/privacy/credential 显式策略
- 2026-08-15 2.2 完成：`scheduling.ListAccounts` 统一 snapshot-first 和四类 repository 查询；Gateway 显式传入 mixed platform/filter，OpenAI 在调用前仍执行 compatible-platform normalization，privacy/credential/runtime 门槛仍在原候选链。原 Gateway debug 日志类型、raw/filtered count 与 OpenAI 错误包装保留；`app/scheduling` 全测试通过。
- 2026-08-15 2.2 回归修正：service 指针在装入 `AccountListSnapshot` 窄接口前显式判 nil，避免 Go typed-nil 让无 snapshot 路径误命中 nil receiver；该问题由无 build-tag 粘性调度回归实际捕获并修复。
- [x] 2.3 将已共用的 priority/LRU 排序和等值 run shuffle 下沉至 `app/scheduling`
- 2026-08-15 2.3 完成：`SortPriorityLRU`、`ShufflePriorityLRURuns`、`ShuffleEqualRuns`、`SameLastUsedSecond` 已下沉 `app/scheduling`；service 的原符号仅作兼容薄包装，OpenAI/Gateway 生产调用均走共享实现。`preferOAuth`分区、load-rate 组边界和秒级 LastUsedAt 相等语义保留；`app/scheduling` 全测试及针对性 service 回归通过。
- [x] 2.4 抽取粘性绑定/排除/清理/候选命中纯决策，保留 Gateway session 注册与 OpenAI epoch/escape/transport 差异
- 2026-08-15 2.4 完成：`EvaluateStickyBinding`/`EvaluateStickyCandidate` 已接入 Gateway Layer 1.5、OpenAI legacy `tryStickySessionHit`、load-aware Layer 1 和 advanced `selectBySessionHash`。帮助器只返回决策，不执行 cache/DB/concurrency 副作用；Gateway session 注册/等待队列和 OpenAI credential/epoch/transport/runtime/re-eval/escape 顺序保留。
- [x] 2.5 审计 prefetch 路径；仅在确认逐字等价时共享，否则记录差异并保留
- 2026-08-15 2.5 完成：Gateway 预取的是 window cost/RPM 计数，包含 Redis/cache/batch SQL/逐账号 fallback 与独立指标；OpenAI 预取的是候选及 shadow parent 完整账号快照，通过 `GetByIDs` 水合凭据并在失败时保留原路径。两者数据、生命周期和 fail-open/fallback 不同，不共享；差异已记入 inventory。

## 3. 回归门禁

- [x] 3.1 `app/scheduling` 全测试与 Gateway/OpenAI scheduler/sticky/failover 针对性测试全绿
- 2026-08-15 3.1 完成：`go test ./internal/app/scheduling -count=1`，service 的 `Scheduler|Schedul|Sticky|SelectAccount|Failover|ChannelRestriction|Priority|Shuffle` 无 build-tag 定向集，以及 handler `Failover|Sticky|SelectAccount|Scheduler|Scheduling|GrokRetry` 定向集全绿。
- [x] 3.2 调度差分用例证明候选集、排序、粘性命中、排除和选号结果一致
- 2026-08-15 3.2 完成：`app/scheduling` 表驱动用例对拍 snapshot/repository 候选分支、filter 结果、priority/LRU 组边界、preferred partition、sticky binding/exclusion/clear/match 决策；service/handler 多候选、sticky busy/escape/out-of-group、transport 不兼容、hard exclusion 与 failover 回归继续断言实际选号结果，全部通过。
- 2026-08-15 3.2 端到端证据：旧版从 Change 1 提交 `50f9f1fb9f8b99040b75f6801532e2835f1b011e` 构建，新版从当前 Change 2 构建；HTTP/SSE + OpenAI WS `11/11`、断连 `5/5`、passthrough/Bedrock/raw CC `6/6` 全通过，schema/usage/数据库快照相等。报告位于 `evidence/run-*.json`，隔离容器与 worktree 已清理。
- [x] 3.3 archtest、OpenSpec strict、`git diff --check` 与根 `make build` 全绿
- 2026-08-15 3.3 完成：archtest、`openspec validate refactor-scheduling-equivalent-primitives --strict`、`git diff --check`、根 `CI=true PNPM_CONFIG_DANGEROUSLY_ALLOW_ALL_BUILDS=true make build` 全绿；前端仅现有 Vite chunk/dynamic-import 警告。
- [x] 3.4 独立提交 Change 2，工作树恢复干净
- 2026-08-15 3.4 完成：Change 2 以独立 `refactor(scheduling): 收敛行为等价原语` 提交收口；仅包含明确白名单内的 scheduling/service 源码、测试、OpenSpec 和差分证据，未使用 `git add -A`。

## 4. 明确挂起的后续决策

- [x] 4.1 记录评分制转正/废弃/并存所需线上指标和设置使用数据，不在本 change 实现 Change 3
- 2026-08-15 4.1 完成：`change3-decision-gate.md` 记录了 advanced settings 实际启用、runtime stats、TTFT/error/escape、候选门槛、凭证类和 Gateway mixed 对照数据要求，以及转正/废弃/并存三条路线的准入条件。本 change 没有选择路线，也没有改任何调度设置或算法。
