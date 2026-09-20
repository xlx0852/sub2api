## 1. 基线与契约

- [x] 1.1 固定 durable settlement 基线提交并确认工作树干净
- 2026-08-15 基线：durable settlement 已作为独立提交 `6373e92fac25ebc65897182fb6a8d8929ea54363` 收口；提交前结算针对性测试、archtest、OpenSpec strict、diff check 与根 `make build` 全绿。提交未包含 frontend/fingerprint/WS originator 等无关 WIP，提交后工作树干净；原 change 的 6.5/6.6 仍保持未完成，不声称已归档。
- [x] 1.2 盘点两套输入、费用计算、usage-log 字段与五类必须保留差异
- 2026-08-15 盘点：输入逐字段映射、重复函数、共享持久化下半场、五类显式差异及现有测试覆盖已记录在 `billing-core-inventory-2026-08-15.md`。另确认 Gateway 缺价当前语义是记录计算错误后以零费用继续落 usage，不是向 handler 返回错误；合并将保持真实代码行为。
- [x] 1.3 为未被现有测试锁定的平台差异补 characterization tests
- 2026-08-15 characterization：新增无 build tag 的 Gateway 测试，锁定未知模型“记录错误日志但继续落零费用 usage”以及 `ForceCacheBilling` 把全部 input 移到 cache-read bucket、input cost 归零的真实行为；两例通过。其余 long-context、OpenAI 缺价、WS identity、互斥桶、cyber/media/tier 差异已有现成测试覆盖。

## 2. 单一费用与日志核心

- [x] 2.1 扩展 `app/billing.RecordInput` 为统一纯标量输入与显式平台 policy
- 2026-08-15 2.1 阶段记录：canonical `Usage` 补齐 5m/1h cache buckets，`UsageTokens` 与 `CostBreakdown` 移入 `app/billing`；service 只保留无方法的类型别名，未给别名加方法。billing/service 费用与 RecordUsage 针对性测试、archtest 全绿。
- 2026-08-15 2.1 完成：`app/billing.RecordInput` 已补齐请求、媒体、channel、compact、WS、cyber 与 domain 快照；`RecordPolicy` 显式命名 provider、缺价、cost-error、token 桶、long-context 与 WS request-ID 策略。Gateway/OpenAI provider extractor 均已构造该统一输入，针对性编译/测试通过。
- 2026-08-15 2.1 policy 复核：统一核心现在拒绝零值/错配 provider policy，Gateway/OpenAI 必须显式匹配 token-bucket、cost-error 与 pricing-missing 策略；缺价零成本分支实际读取 `PricingMissingPolicy`，不再只作标记。
- [x] 2.2 合并 usage normalization、channel pricing、group multiplier 与 token 费用计算
- 2026-08-15 2.2 完成：normalization 已下沉为 `app/billing.NormalizeUsage`，channel pricing/group multiplier/token cost 已收敛到共享核心；修复并锁定 Gateway 直接调用 `opts == nil` 的历史兼容。费用/RecordUsage characterization 针对性测试全绿。
- [x] 2.3 合并 image/video 费用计算并保留平台缺价/long-context 差异
- 2026-08-15 2.3 完成：image/video 计算已进入共享费用核心；Gateway 图片 channel 路径仍只传入 input/output/image-output 三个旧 token 桶，避免 cache 桶改变旧语义。完整媒体/缺价/long-context 定向测试集已复核。
- 2026-08-15 2.2/2.3 复核：`app/billing` normalization 全包测试通过，service 的 RecordUsage/cost/channel/rate/media/long-context 定向测试通过，五类平台策略分支仍显式保留。扩大正则误命中的 OAuth originator 旧断言仍失败，已按约束排除，未修改该 dirty WIP 测试或代码。
- [x] 2.4 合并 Gateway/OpenAI usage-log 构造器及可选 ServiceTier/video/compact/WS/cyber 字段
- 2026-08-15 2.4 完成：Gateway 与 OpenAI 均改用单一 `app/billing.BuildUsageLog`；ServiceTier、video、compact、WS、cyber、cache-TTL 均是显式输入，费用与 RecordUsage 针对性测试通过。同步修正 design/spec 中与 1.2 实测冲突的 Gateway 缺价描述，不改代码语义。
- 2026-08-15 2.4 边界复核：新增显式 `VideoUsage`，保留旧路径在视频模型上游 `VideoCount == 0` 时仍写入 video resolution/duration 并选择 video multiplier 的异常响应归因；已补 app/billing characterization test。

## 3. 入口迁移与清理

- [x] 3.1 让 Gateway/OpenAI 入口共用单一结算 orchestration，保持 durable/legacy 分支
- 2026-08-15 3.1 阶段记录：durable handoff、authoritative 短路返回、legacy `applyUsageBilling` 与 best-effort usage-log 写入先收敛为单一 `persistUsageBilling`，Gateway/OpenAI 针对性 settlement/RecordUsage 测试通过。
- 2026-08-15 3.1 完成：provider extractor 下半场已统一进入 `billing.RecordInput -> settlePreparedUsage`，calculator、`app/billing.BuildUsageLog`、durable/legacy 选择均只有一份主体。
- [x] 3.2 迁移 handler、WS、media、recovery 与 pipeline 调用点到统一输入
- 2026-08-15 3.2 完成：上述生产调用点保留 provider-specific forward-result 薄入口，但费用/日志/持久化前均映射为同一 `app/billing.RecordInput`；无调用点可再绕回两套结算主体。
- [x] 3.3 删除 `settlement_adapters.go` 与已无调用的重复 calculator/log builder
- 2026-08-15 3.3 完成：删除 `settlement_adapters.go`、Gateway/OpenAI 旧 calculator 方法与 service 内 log builder；媒体测试改为直接锁定共享 calculator，`rg` 确认旧符号无残留。
- [x] 3.4 证明没有给 service 类型别名加方法，也没有修改协议/config/调度语义
- 2026-08-15 3.4 完成：类型检索只发现 `UsageTokens`/`CostBreakdown` 的无方法别名，未发现别名 receiver；改动集合仅限 app/billing、service usage 结算、相关测试与 OpenSpec，无 handler 协议、config、scheduler 文件改动；archtest 通过。

## 4. 回归门禁

- [x] 4.1 结算、repository、durable settlement 与 handler 针对性测试全绿
- 2026-08-15 4.1 完成：`app/billing` 全包，service 的 Gateway/OpenAI RecordUsage、缺价、媒体、long-context、usage settlement，repository 的 usage billing/settlement 原子与幂等测试，以及 handler 的 durable 提交、quota platform、WS usage 与 Grok retry 针对性测试全绿。
- [x] 4.2 `run-diff`、`run-disconnect-diff`、`run-short-path-diff` 全绿
- 2026-08-15 4.2 完成：旧版从 `6373e92fac25ebc65897182fb6a8d8929ea54363` 隔离 worktree 构建，新版从当前 Change 1 工作树构建；HTTP/SSE + OpenAI WS `11/11`、断连 `5/5`、passthrough/Bedrock/raw CC 短路径 `6/6` 全通过，响应 schema、usage 与数据库快照一致。证据位于 `evidence/run-*.json`，测试容器、临时数据与 worktree 已清理。
- [x] 4.3 archtest、OpenSpec strict、`git diff --check` 与根 `make build` 全绿
- 2026-08-15 4.3 完成：archtest、`openspec validate unify-usage-billing-core --strict`、`git diff --check`、根 `CI=true PNPM_CONFIG_DANGEROUSLY_ALLOW_ALL_BUILDS=true make build` 全绿；前端仅有现有 Vite chunk/dynamic-import 警告，无 vue-tsc/build 错误。
- [x] 4.4 独立提交 Change 1，工作树恢复干净
- 2026-08-15 4.4 完成：Change 1 以独立 `refactor(billing): 合并 usage 结算核心` 提交收口；仅包含明确白名单内的 billing/service 源码、测试、OpenSpec 与差分证据，未使用 `git add -A`。
- 2026-08-15 staging 部署前备注：日本测试机仍为独立 Nginx 前端 + 内网 API/PostgreSQL/Redis，四容器 healthy，入口 `/health` 200。已在 `/opt/sub2api-staging/backups/pre-merge-20260815T082314Z` 保存当前 API/前端成对引用、Compose/Nginx、受限权限环境副本与 PostgreSQL custom dump；`pg_restore -l` 验证 1040 条 TOC 成功。尚未替换容器或切流，美国生产机未访问。
- 2026-08-15 staging 候选构建备注：从提交 `80f6f41850557719f3a0de559dd34db7fe14056f` 的 detached worktree 构建，`vue-tsc -b` 与 Vite production build 通过；后端以无 embed 的 Linux/amd64 静态二进制构建，版本 `0.1.319-staging-80f6f41850`。候选仅含二进制、4 个 resources 文件、171 个独立前端文件、运行时 Dockerfile 与 `SHA256SUMS`；46MiB archive SHA256 为 `460bbf2a2f7bb21626683a533b25874a5f192b1d78705b094f7f84282d84663c`。首次打包因误取 `frontend/dist` 在上传前失败，修正为项目真实输出 `backend/internal/web/dist` 后重建成功；远程仍未切换。
- 2026-08-15 staging 远端候选备注：archive SHA 与 payload `SHA256SUMS` 全通过，候选 API 镜像为 `sub2api-staging:merge-80f6f41850`，前端 release 为 `/opt/sub2api-staging/releases/merge-80f6f41850-20260815T082522Z/frontend`。清理 macOS tar 扩展属性产生的候选 `._*` 元数据后，真实前端文件数为 171；候选 Compose 经 `docker-compose config` 验证，diff 仅包含 API image 与 frontend mount 两行。现网旧 staging `/health` 仍为 200，尚未切换。
- 2026-08-15 staging 首轮切换备注：候选 API 容器已启动并达到 healthy，但远程版本 JSON 的 Python 单行解析因 shell 引号丢失触发校验失败；自动回滚按设计恢复旧 API `settlement-f926ae06c-v3` 与匹配旧前端，旧 pair `/health` 200、镜像和挂载均核对正确。该失败发生在前端切换前，属于部署命令问题，不归因为候选代码；未放宽版本断言，修正解析方式后再切换。
- 2026-08-15 staging 部署完成备注：修正版本解析后按 API healthy/精确版本门禁先切后端，再切独立前端挂载。日本 staging 当前运行 `sub2api-staging:merge-80f6f41850` + 匹配 release 前端，PostgreSQL/Redis 未替换；四容器 healthy，`/health`、`/`、`/login`、`/api/v1/public/config` 均 200，公开版本精确为 `0.1.319-staging-80f6f41850`，启动日志未发现 panic、fatal、migration/database error。美国生产机未访问；部署备注暂不提交。
- 2026-08-15 staging 登录/数据面备注：管理员 email/password 登录、JWT `/auth/me`、admin settings 均 200，角色保持 admin；无效 API key 为 401，有效 staging API key `/v1/models` 为 200。真实上游烟测中 Anthropic/OpenAI/Kimi 的 JSON+SSE 6/6 成功并返回 terminal usage；Grok 与 Antigravity 仍按测试机既有账号状态分别 503，与部署前基线一致。唯一 marker 落 `usage_logs` 恰好 6 条、其中 stream 3 条，input/output token 合计 8952/16、`actual_cost=0.0015350000`；durable mode 当前未为这些请求创建 attempt，未完成 attempt 为 0。首轮汇总 SQL 因误查不存在的 `platform` 列停止，改用实际字段后严格断言通过。
- 2026-08-15 staging 最终健康备注：独立前端首个哈希 JS 资产 200，四容器持续 healthy，API image/前端挂载仍精确指向本候选；后端最近日志 fatal/panic/migration/database error 为 0。Nginx 最近窗口的 4 条 5xx 已逐条归因为本次主动发送的 Grok/Antigravity 预期 503，未发现额外 5xx。回滚点保留在 `pre-merge-20260815T082314Z`，未执行 commit 或访问美国生产。
