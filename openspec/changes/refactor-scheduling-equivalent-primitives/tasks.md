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
- 2026-08-15 staging 部署前备注：日本测试机仍为独立 Nginx 前端 + 内网 API/PostgreSQL/Redis，四容器 healthy，入口 `/health` 200。已在 `/opt/sub2api-staging/backups/pre-merge-20260815T082314Z` 保存当前 API/前端成对引用、Compose/Nginx、受限权限环境副本与 PostgreSQL custom dump；`pg_restore -l` 验证 1040 条 TOC 成功。尚未替换容器或切流，美国生产机未访问。
- 2026-08-15 staging 候选构建备注：从提交 `80f6f41850557719f3a0de559dd34db7fe14056f` 的 detached worktree 构建，`vue-tsc -b` 与 Vite production build 通过；后端以无 embed 的 Linux/amd64 静态二进制构建，版本 `0.1.319-staging-80f6f41850`。候选仅含二进制、4 个 resources 文件、171 个独立前端文件、运行时 Dockerfile 与 `SHA256SUMS`；46MiB archive SHA256 为 `460bbf2a2f7bb21626683a533b25874a5f192b1d78705b094f7f84282d84663c`。首次打包因误取 `frontend/dist` 在上传前失败，修正为项目真实输出 `backend/internal/web/dist` 后重建成功；远程仍未切换。
- 2026-08-15 staging 远端候选备注：archive SHA 与 payload `SHA256SUMS` 全通过，候选 API 镜像为 `sub2api-staging:merge-80f6f41850`，前端 release 为 `/opt/sub2api-staging/releases/merge-80f6f41850-20260815T082522Z/frontend`。清理 macOS tar 扩展属性产生的候选 `._*` 元数据后，真实前端文件数为 171；候选 Compose 经 `docker-compose config` 验证，diff 仅包含 API image 与 frontend mount 两行。现网旧 staging `/health` 仍为 200，尚未切换。
- 2026-08-15 staging 首轮切换备注：候选 API 容器已启动并达到 healthy，但远程版本 JSON 的 Python 单行解析因 shell 引号丢失触发校验失败；自动回滚按设计恢复旧 API `settlement-f926ae06c-v3` 与匹配旧前端，旧 pair `/health` 200、镜像和挂载均核对正确。该失败发生在前端切换前，属于部署命令问题，不归因为候选代码；未放宽版本断言，修正解析方式后再切换。
- 2026-08-15 staging 部署完成备注：修正版本解析后按 API healthy/精确版本门禁先切后端，再切独立前端挂载。日本 staging 当前运行 `sub2api-staging:merge-80f6f41850` + 匹配 release 前端，PostgreSQL/Redis 未替换；四容器 healthy，`/health`、`/`、`/login`、`/api/v1/public/config` 均 200，公开版本精确为 `0.1.319-staging-80f6f41850`，启动日志未发现 panic、fatal、migration/database error。美国生产机未访问；部署备注暂不提交。
- 2026-08-15 staging 登录/数据面备注：管理员 email/password 登录、JWT `/auth/me`、admin settings 均 200，角色保持 admin；无效 API key 为 401，有效 staging API key `/v1/models` 为 200。真实上游烟测中 Anthropic/OpenAI/Kimi 的 JSON+SSE 6/6 成功并返回 terminal usage；Grok 与 Antigravity 仍按测试机既有账号状态分别 503，与部署前基线一致。唯一 marker 落 `usage_logs` 恰好 6 条、其中 stream 3 条，input/output token 合计 8952/16、`actual_cost=0.0015350000`；durable mode 当前未为这些请求创建 attempt，未完成 attempt 为 0。首轮汇总 SQL 因误查不存在的 `platform` 列停止，改用实际字段后严格断言通过。
- 2026-08-15 staging 最终健康备注：独立前端首个哈希 JS 资产 200，四容器持续 healthy，API image/前端挂载仍精确指向本候选；后端最近日志 fatal/panic/migration/database error 为 0。Nginx 最近窗口的 4 条 5xx 已逐条归因为本次主动发送的 Grok/Antigravity 预期 503，未发现额外 5xx。回滚点保留在 `pre-merge-20260815T082314Z`，未执行 commit 或访问美国生产。
