## 1. P0 边界冻结与防护

- [x] 1.1 配置 golangci-lint depguard 依赖规则（现状白名单 + 禁止新增违例）
- [x] 1.2 新增架构测试：扫描层间 import 违例与包级可变全局
- [x] 1.3 标记基线回归护栏（路由清单、关键网关/调度/计费测试集）并接入 CI

## 2. P1 ports 下沉（打破拆包死锁）

- [x] 2.1 创建 `internal/ports` 与 `internal/domain` 骨架
- [x] 2.2 将 service 内 Repository/Cache 接口及其依赖类型机械搬移（签名不变）：核心实体 Account/Group/User/APIKey 等迁入 domain，HTTPUpstream/OAuth/UsageFetcher 迁入 ports；service 以类型别名与函数转发保持调用点零改动
- [x] 2.3 `repository` 改为实现 ports 接口，删除对 service 的生产代码 import（`repository -> service` 基线边已清零；depguard 生产代码禁止回退）
- [x] 2.4 全量编译 + 既有测试回归通过（`go build ./...`、archtest、route 清单、repository/domain/server 测试与 service 抽样均通过；全量 go test 交由 CI）

## 3. P2 domain 与 config 拆分

- [x] 3.1 领域类型/常量/错误迁入 `domain`（P1 已完成核心实体与方法；残余以 service 类型别名过渡，随模块拆分删除）
- [x] 3.2 `Config` 按模块拆为子结构（YAML/env 键名兼容），模块只接收子结构（子结构与 `GatewayModule()` 等访问器已落地；构造函数全面改为只收子结构仍随模块拆分推进）
- [x] 3.3 合并 `internal/middleware` 与 `internal/server/middleware`（RateLimiter 并入 `server/middleware`，独立包已删除）

## 4. P3 网关统一管道（全平台迁移，每平台独立 change）

- [x] 4.1 落地 `app/gateway/{pipeline,stream,transport}` 骨架与阶段接口（尚未接入生产转发；平台迁移见 4.2+）
- [x] 4.2 试点平台迁移（Antigravity 或 Gemini）并验证抽象与回归（Antigravity Claude/Gemini 与 Gemini Messages `Forward` 已管道化；handler 选号已切 `AccountSelector`）
- [x] 4.3 Claude 链路迁移，删除旧 `GatewayService.Forward` 路径（主 Messages、ForwardAsResponses / ForwardAsChatCompletions、CountTokens 已管道化；Web Search / API Key passthrough / Bedrock 短路径仍在入口外）
- [x] 4.4 Kimi、Grok 迁移（Grok Responses、Grok Chat Responses 桥、Kimi/API-key Responses→CC 回退已管道化；Grok/Kimi raw CC 仍为入口短路径，Grok Messages patch 挂在 Messages Rewrite）
- [x] 4.5 OpenAI/Codex 迁移（HTTP 主路径已管道化；指纹/模型映射/Fast policy 在 Rewrite；passthrough 仍为入口短路径；WSv2 在 Rewrite 之后的 Transport 短路径，状态机拆解见 4.6）
- [x] 4.6 WS ingress 拆为 session/turn/connection 三层状态机（`ProxyResponsesWebSocketFromClient` 为编排入口；passthrough 仍为入口短路径；session/turn/connection 方法分文件，共享 `openAIWSIngressSession` 状态）
- [x] 4.7 删除全部旧转发链路与同构流式实现，确认单一路径（Gemini Messages `Forward` / `ForwardNative` / `ForwardAsChatCompletions` 与 Grok Chat Responses 桥已管道化；Web Search / API Key passthrough / Bedrock / Codex CLI-only / Grok·Kimi raw CC / WSv2 passthrough 等入口短路径保留）

## 5. P4 调度统一

- [x] 5.1 定义 `AccountSelector` 接口与策略插件模型（接口已落在 `app/scheduling`；现有调度器适配见 5.2）
- [x] 5.2 两套现有调度器适配接口（过渡期先由 `GatewayService` / `OpenAIGatewayService` 适配 `AccountSelector`，Claude/Gemini/Antigravity handler 生产选号已切换；终态由 5.3 的单一 Engine + Strategy 取代双实现）
- [x] 5.3 逐策略合并去重，删除双实现（`WaitPlan`、槽位 fail-open、排除集判断/复制/写时追加、group membership、hydrate/lookup/选号结果构造、priority+LRU 公共比较器已收敛到 `app/scheduling`；Gateway/OpenAI 候选门槛已按显式 `CandidatePolicy` 插件接入，`GroupIDs` 快照差异以显式参数保留；终态仅 `scheduling.Engine` 实现 `AccountSelector`，Gateway/OpenAI 为互斥平台 `Strategy`，原公开方法仅作反向调用 Engine 的兼容包装；OpenAI 失败结果的 schedule decision 也保持不变。Gateway/OpenAI 全部调度定向回归、全仓编译与 archtest 通过）

## 6. P5 结算统一与装配治理

- [x] 6.1 合并两套 `RecordUsage` 为单一 settlement，统一 usage 模型（`app/billing.Settlement` 门面 + 两套适配器；扣费核心仍复用原实现）
- [x] 6.2 包级 setter/全局状态收归 Wire 注入（Idempotency / Codex identity / WebSearch manager / setting runtime cache / ReqClientPool 已收归构造注入；基线 Set* 装配旁路已清零）
- [x] 6.3 Lifecycle 注册表替代手工 cleanup 列表（`app/lifecycle.Registry` 已承接 wire cleanup）

## 7. P6 API 契约化与前端模块化

- [x] 7.1 控制面 OpenAPI 规格（user/auth/payment 先行，admin 分批）+ CI 同步校验（`openapi/control-plane.yaml` 覆盖 116 条用户/认证/支付路由；`TestControlPlaneOpenAPIMatchesRoutes` 双向对齐；字段级 schema 随 7.2 收紧）
- [x] 7.2 生成 TS 客户端，按 feature 替换手写 api 模块（`npm run generate:api` 产出 path/method 类型；user/auth/payment 及同批用户面模块改走 `controlPlaneRequest`；admin 仍手写；字段级 schema 待后续收紧）
- [x] 7.3 前端迁移至 `features/` 结构，公共 UI 沉淀 `src/ui/`（控制面 API + auth/payment/announcements store、基础控件、账号凭据构造已迁移；auth/public/user/admin 路由视图均以 `features/` 为实体路径，旧 `views/` 路径保留默认导出兼容；`vue-tsc --noEmit` 通过，user/admin/router 定向回归 29 个文件通过，另有既存 CSV 期望差异未在本项修改）
- [x] 7.4 账号表单引擎（平台字段 registry），Create/Edit/Bulk 三 Modal 收敛（平台选择、全部平台子类型、API Key 元数据及三 Modal 平台块可见性统一由 registry 驱动；Gemini 保持 `showInCreatePicker: false`，仅保留既有编辑/兼容路径；`vue-tsc --noEmit` 与 registry/三 Modal/账号页定向回归 93 项通过）
- [x] 7.5 `SettingsView` 拆为懒加载子路由（`/admin/settings/:tab?` 已路由化；9 个 tab 均已抽出。backup / agreement / email / features / general 为 `defineAsyncComponent`；payment / users / security / gateway 因单测需同步导入。父组件仍持有表单与保存逻辑）

## 8. P7 契约分离交付

- [x] 8.1 公开配置端点取代 HTML 注入，前端产物环境无关化（`GET /api/v1/public/config` 为规范端点，`/settings/public` 兼容；前端 fetch 后后台刷新；embed 注入已在 8.2 删除）
- [x] 8.2 embed 退化为纯静态打包（默认交付与升级流程不变；`serveIndexHTML` 不再拉取/注入 `__APP_CONFIG__` 或改写 `<title>`，ETag 只跟 base HTML）
- [x] 8.3 分离部署 compose 变体 + 官方 nginx 参考配置 + Release 前端静态包工件（`deploy/docker-compose.separated.yml` overlay、`deploy/nginx/`、Release 上传 `sub2api-frontend-<version>.zip`；默认 compose/install/自更新未改）
- [x] 8.4 版本偏移防护端点与前端自检（`GET /api/v1/public/version` 返回 `version` + `min_frontend_version`；分离部署启动时比对 `__APP_VERSION__` 并横幅告警，失败不阻断启动）

## 9. P8 观测与收尾

- [x] 9.1 管道阶段级指标与日志规范（`Exchange.StageDuration` 记录各阶段毫秒耗时）
- [x] 9.2 更新部署矩阵与架构文档（`docs/ARCHITECTURE.md` / `ARCHITECTURE_CN.md`；`openspec/project.md`、README、`deploy/README.md` 已指向）
- [x] 9.3 运行 `openspec validate refactor-architecture --strict` 并归档（7.3 / 7.4 / 5.3 已收口，严格校验通过，已归档为 `2026-08-14-refactor-architecture`）

### 归档后回归修复记录

- [x] 2026-08-14 Grok 同账号重试抢槽：初始化未注入的模型并发预算 provider，避免 `atomic.Value` 首次 `Load` panic；固定账号重试仅重新抢槽而不重复绑定已选账号。未配置预算、重试、惩罚与 failover 语义保持不变；定向集成回归通过。
- [x] 2026-08-14 WSv2 passthrough usage reasoning：显式 effort 仍按映射后上游模型归一化；无显式 effort 时改用映射前请求模型后缀推导，仅修正 usage 元数据归因，不改上游帧。
- [x] 2026-08-14 Profit 保本窗口：当 quota 已提供有效 `WindowMinutes` 或可识别窗口类型时，不再强制要求 `StartAt` / `EndAt`；恢复 7d 优先于 5h / 24h 的既定选择规则，不改计费公式。
- [x] 2026-08-14 后端验证：上述三项定向回归与 `go build ./...` 通过，本地 `/health` 仍返回 `ok`；handler / service / server middleware 包级回归仅剩既存 OAuth 邮箱归属安全断言、旧模型默认值、未分组 Google key 断言，以及本变更硬约束明确排除的 fingerprint / WSv2 originator 测试；未为消除旧断言改动生产语义。
- [x] 2026-08-14 上线门禁补充：前端 34 个 `vue-tsc` 类型错误已清零，根目录 `make build` 完整通过（后端二进制 + 前端 production bundle）；调度、Grok 同账号重试、WS usage 归因与 Profit 保本窗口定向回归通过。当前工作树 319 项变更，禁止直接作为发布输入；staging / 灰度待受管主机与独立测试凭证就绪后执行。
- [x] 2026-08-14 staging 前后端分离部署：`103.150.8.121:8180` 由独立 Nginx 提供 production frontend 并代理 API / SSE / WS，API-only 容器仅暴露 Docker 内网 `8080`；`/` / `/login` / `/health` / 版本端点均通过，前端资产 162 个，Nginx 无严重错误。未采用 `-tags embed`，保持 8.1–8.4 的分离交付目标。
- [x] 2026-08-15 归档后机械清理第一批：删除 `.pnpm-store/`；移除前端 `getPlans`/`fetchPlans` 死调用（后端路由已退役）；`service.ProviderSet` 补 `KimiQuotaQuerier`/`GrokQuotaProber` 两条 `wire.Bind`，`wire` 恢复自动生成 `wire_gen.go`；删除 64 个再导出桩（48 视图 + 13 UI 控件 + credentialsBuilder + paymentUx/paymentWechatResume），全仓引用改指 `@/features` 与 `@/ui`（含 vi.mock 路径与动态/双引号导入）；`config.go` 按模块拆出 6 个文件（connect/gateway/security/server_store/ops_cache/products，3354→1959 行，纯搬移）。验证：`go build ./...`、archtest、`vue-tsc --noEmit` 通过；前端 1017 通过，剩余 20 失败均为 HEAD 基线既有（UsageTable/AccountTestModal 的 Pinia 装配与 user UsageView CSV 期望，经 stash 对照确认）。285 条 service 类型别名经引用计数均 ≥4 次（无死别名），整体迁移需逐别名改调用点，另起批次。
- [x] 2026-08-14 staging 隔离测试基线：管理员合规确认完成；建立 Anthropic / OpenAI / Grok / Kimi / Antigravity 五个独立分组、低额度用户与每组 API Key，从美国生产以只读、不落盘管道导入 4 个现有上游凭证，并发固定为 1。Gemini 新建按已有“平台退役”规则被拒绝，与 Create Picker 隐藏约束一致；生产无 Gemini / Antigravity 凭证，不伪造真实连通结果。
- [x] 2026-08-14 staging 数据面与安全回归：Anthropic / OpenAI / Grok / Kimi 普通请求与 SSE 均返回 200、流式终止事件和 usage；OpenAI WS 完成 101 握手、`response.completed` 与 usage，Grok WS 按既有协议在握手前返回 426 并要求回退 HTTP/SSE。非安全 HTTP 与 `127.0.0.1` 临时账号分别被 URL scheme / host allowlist 拒绝，请求仍选中健康账号；并发 3 请求全部完成后用户/账号并发计数均归零。
- [x] 2026-08-14 staging 计费对账：首轮 9 条普通/SSE/WS 成功请求的 `actual_cost` 合计 `0.008057` USD，用户余额从 `2.000000` 精确变为 `1.991943`，五枚 API Key 的 `quota_used` 合计同为 `0.008057`；账号/分组倍率均为 1，未见少扣、重扣或负余额。
- [x] 2026-08-14 干净候选回归：显式文件集候选的根 `make build` 通过，7 个前端 registry / Edit / Bulk / router / user-admin view 文件共 59 项测试通过。Go 定向回归仍有 4 个已归属失败：Google 未分组 Key 旧断言、两个 OAuth 已注册邮箱安全断言，以及 WSv2 localhost fixture 被当前 SSRF 私网防护拒绝；未修改这些测试或生产语义来刷绿。
- [x] 2026-08-15 staging 成对回滚演练：以生产前一版对应的 `f94204d8a` 源码构建旧 Linux 后端与旧前端，三层 SHA256 校验后构建旧镜像；切换前的 PostgreSQL custom dump 已通过 `pg_restore -l` 检查。旧后端+旧前端成对切换后 health / root / admin login / 真实 OpenAI 请求均为 200，随后恢复候选后端+候选前端，同样全部 200，版本恢复为 `0.1.319-staging-refactor-20260814`，数据、余额与 usage 保留。美国生产始终保持 `sub2api-app-0.1.319-18243` / Nginx `18243` / health 200，未切流。
- [x] 2026-08-15 staging TOTP / 时钟回归：首次 2FA 实测发现日本主机比本地和美国生产慢约 160 秒，且未配置 NTP，会导致 TOTP 必然失败。测试机已安装并启用 chrony，`chronyc tracking` 为 Normal，时差收敛到 0。随后临时启用 TOTP，完成 setup、密码登录进入 2FA、TOTP 二次登录、disable 全链路，最后恢复全局开关为缺省关闭、清空测试用户 TOTP 字段，普通登录恢复。
- [x] 2026-08-15 新旧版双打计费对账：日本机上以同一数据库快照启动 `f94204d8a` 旧版与 refactor 候选版的独立 PostgreSQL / Redis / data 实例，四平台 JSON/SSE 8 用例与 OpenAI WS 同请求并行发送。真实上游 8/8 HTTP/SSE 为 200，WS 两边均 101 + `response.completed`；上游 usage 相同时新旧扣费精确相同，缓存/生成 token 随机差异均被各自 usage 忠实落盘和计费。再以私有 CA HTTPS 确定性回放消除上游随机性，8/8 响应 schema、规范化协议字段、usage 全等，两库 usage 字段摘要均为 `f86a5768f30c71571e19cf87b6f28a94`，余额变化 = `SUM(actual_cost)` = API Key `quota_used` 均为 `0.000272`。脚本与报告见 `tools/refactor_dual_version_{reconcile,fake_upstream}.py` 和 `openspec/changes/archive/2026-08-14-refactor-architecture/dual-version-reconciliation-2026-08-15.md`；临时容器、克隆卷、凭证快照、CA 私钥、端口与隧道均已清理，美国生产未发送测试流量。
