## Context

Sub2API 是自托管 AI API 网关（Go + Gin + Ent + PostgreSQL/Redis，Vue 3 前端，单二进制嵌入交付），全仓约 34 万行。核心问题：

- **上帝包与拆包死锁**：业务几乎全部位于 `backend/internal/service` 单包（约 420 非测试文件 / 17.6 万行）；约 151 个 Repository/Cache 接口定义在 service 内，`internal/repository` 反向 import service 来实现它们（`repository/wire.go`），任何按领域拆包都会立即产生 import cycle。
- **平台垂直复制**：Claude（`gateway_forward.go` Forward 约 772 行）、OpenAI/Codex（`openai_gateway_forward.go` Forward 约 1068 行，Grok/Kimi 挂其下）、Gemini（`gemini_messages_compat_service.go` 3560 行）、Antigravity 各一套「选账号 → 改写 → 转发 → 解析流 → 计费」，5+ 份同构 `handleStreamingResponse`，指纹/模型映射在 HTTP 非透传、透传、WS v2、WS ingress 四处重复插桩。
- **双调度器**：`gateway_scheduling.go`（2513 行，Claude/Gemini/Antigravity）与 `openai_gateway_scheduling.go` + `openai_account_scheduler.go`（2611 行）并行演进，粘性会话、能力过滤、配额暂停语义各自实现。
- **装配旁路**：`SetWebSearchManager`、`SetDefaultIdempotencyCoordinator`、Codex identity 全局开关等包级 setter 绕过 Wire；`cmd/server/wire.go` 手工维护约 60 项 cleanup；`internal/middleware` 与 `internal/server/middleware` 双目录并存；`config.go` 3349 行上帝配置。
- **前端上帝组件**：`SettingsView.vue` 约 1.1 万行；`CreateAccountModal.vue`（6.4k）与 `EditAccountModal.vue`（4.8k）平台字段双份维护。

硬约束（来自 `openspec/project.md` 与部署现实）：兼容层保持输入输出语义稳定，不用大而全抽象掩盖协议差异；session/turn/transport connection 是不同层级；用户群为自托管非专业运维（一键安装、单容器 compose、后台在线升级）。

## Goals / Non-Goals

- Goals:
  - 打破拆包死锁，建立按领域划分且被 CI 锁定的模块边界。
  - 网关统一生命周期管道 + 显式 provider 适配器；**全部平台迁入，旧链路删除**（承诺而非方向）。
  - 单一调度器、单一结算路径。
  - 前后端架构/构建/契约层面彻底分离；embed 退化为可选打包，默认交付形态零变化。
  - 消除前端上帝组件与账号表单双份维护。
- Non-Goals:
  - 不拆微服务/多进程：自托管运维成本优先，水平扩展已由 Redis 锁 + pub/sub 支撑。
  - 不改变任何对外协议行为、调度/计费语义、配置文件格式、默认部署与升级形态。
  - 不重写：全部迁移为行为保持的结构性搬移，以现有约 16.6 万行测试为回归护栏。
  - ent/ 生成代码与数据库 schema 不动。

## Decisions

### D1 目标包结构与依赖方向

```
backend/internal/
├── domain/            # 领域类型/常量/错误（不 import 任何 internal 包）
├── ports/             # Repository/Cache/UpstreamClient 接口（只 import domain）
├── app/               # 应用模块（原 service 按领域拆分）
│   ├── auth/
│   ├── account/       # 账号管理 + oauth/{openai,claude,gemini,grok,kimi,antigravity}
│   ├── scheduling/    # 统一账号调度（D4）
│   ├── gateway/       # 转发核心（D3）
│   │   ├── pipeline/  # 生命周期骨架与阶段接口
│   │   ├── stream/    # SSE/WS 中继、usage extractor 注册
│   │   ├── transport/ # 上游 HTTP/WS 客户端、连接池、TLS 指纹
│   │   └── provider/  # claude/ openai/ gemini/ antigravity/ grok/ kimi/
│   ├── billing/       # 定价、计费、结算（D5）、利润
│   ├── quota/         # 限流、并发槽、配额窗口
│   ├── moderation/    # 内容审核
│   ├── ops/           # 监控、告警、聚合、备份
│   ├── payment/
│   └── notification/  # email 队列
├── infra/             # ports 实现：ent repo、redis cache、外部 HTTP（禁止 import app）
├── transport/         # HTTP 接入层：handler + routes + middleware（合并现有两处 middleware）
├── config/            # 按模块拆分的配置结构（D7）
└── pkg/               # 无状态协议/工具库（openai/claude/gemini/apicompat/logger/errors 等，保留）
```

依赖规则（CI 强制，见 D10）：`domain` 零依赖；`ports` 只依赖 `domain`；`app` 依赖 `domain`/`ports`/`pkg`；`infra` 依赖 `domain`/`ports`/`ent`/`pkg`，**禁止 import `app`**；`transport` 依赖 `app`，反向禁止。

- Alternatives considered: 微服务拆分（运维成本转嫁自托管用户，无扩展性收益，排除）；整体重写（协议怪癖编码在测试与生产行为中，必丢行为，排除）；仅拆文件不动依赖（不解决 import cycle 根因，排除）。

### D2 ports 下沉打破拆包死锁

接口从 service 迁至 `ports`，方法签名与语义不变；接口引用的参数/返回类型迁至 `domain`。`repository` 改为 import `ports` + `domain`，删除对 service 的 import。这是纯机械搬移（P1），是后续一切拆分的解锁前提。现有 `*_port.go` 命名习惯保留为 ports 内的文件组织方式。

### D3 网关统一管道 + 显式 Provider 适配器（全平台迁移）

管道只固化**生命周期与横切关注点**，协议转换保留在各 provider 显式代码中（遵守"不用大而全抽象掩盖协议差异"）：

```go
// app/gateway/pipeline —— 阶段接口草图（实现期允许微调签名）
Parse      入站协议归一（Responses/ChatCompletions/Messages/Gemini native → Exchange）
Admission  定价/余额/权限准入（现 ValidatePricingAdmission）
Select     统一调度选号（委托 app/scheduling，D4）
Rewrite    有序改写链：模型映射 → 参数覆盖/Fast policy → 指纹 → OAuth 伪装
Transport  上游往返：HTTP SSE / WS 连接池 / passthrough
Relay      流中继：keepalive、interval timeout、断连后 drain、usage 提取
Settle     结算：计费（D5）、限流回写、会话窗口更新
```

- 每个 provider 提供 `Parse`/`Rewrite` 中协议相关环节与 usage extractor；`Select`/`Admission`/`Relay` 骨架/`Settle` 为共享实现。
- 指纹、模型映射、Fast policy 收敛为 Rewrite 中间件后，HTTP 非透传/透传/WS v2/WS ingress 共用同一实现，消除四处插桩。
- WS ingress（现 1679 行单函数）拆为 session / turn / transport connection 三层显式状态机，与 project.md 约定对齐。
- **全平台迁移承诺**：试点平台验证抽象后，六平台全部迁入，每迁完一个平台即删除其旧链路，终态不留双路径。若试点暴露抽象缺陷，先修抽象再继续，不允许长期双轨。
- Alternatives considered: 按需迁移允许低频平台留旧链路（维护双路径心智负担 + 横切修复需双打，被否）；单一大 `Forward` 接口不分阶段（无法单测、重蹈覆辙，被否）。

### D4 统一账号调度

单一 `AccountSelector` 接口置于 `app/scheduling`，输入为平台族、模型、粘性键（session hash / previous_response_id）、所需能力（WS、compact 等），内部由策略链组成：优先级 → 负载感知 → 粘性会话 → 能力过滤 → 配额暂停/冷却 → 混合调度（antigravity mixed_scheduling）。平台差异（Claude Code 降级分组、Codex 配额暂停、Grok/Kimi platform override）以策略插件表达，不再分叉两套实现。迁移顺序：先抽接口并让两套现有实现各自适配（行为不变），再逐策略合并去重。

### D5 统一用量结算

合并 `GatewayService.RecordUsage` 与 `OpenAIGatewayService.RecordUsage` 为 `app/billing` 单一 settlement 入口；统一 usage 数据模型（tokens、cache、长上下文、媒体），provider usage extractor 负责把各协议事件映射到该模型。计费口径、异步落库（worker pool）与现有对账/利润逻辑保持语义不变。

### D6 装配与生命周期治理

- Wire 保留，每个 `app` 模块自带 ProviderSet；`cmd/server` 只做组合。
- 新增 Lifecycle 注册表：模块注册 `Start/Stop`，替代 `cmd/server/wire.go` 中约 60 项手工 cleanup 列表，关停顺序按注册反序。
- 全部包级可变全局（`SetWebSearchManager`、`SetDefaultIdempotencyCoordinator`、`sharedReqClients`、Codex identity 开关、`setting_gateway_runtime` 的包级 atomic 缓存）收归构造注入或模块内实例状态。

### D7 配置拆分

`Config` 上帝结构按模块拆为子结构（`config.Gateway`、`config.Billing`、…），Viper 加载与 YAML/env 键名完全兼容；模块只接收自己的子结构，禁止透传整个 `*Config`。

### D8 前后端契约分离（embed 为可选打包）

- 前端构建为**环境无关静态产物**（Vite `outDir` 回归 `frontend/dist`）：同一份产物既可静态托管，也可被 `-tags embed` 原样打进二进制；`internal/web` 退化为纯静态文件服务，删除 `FrontendServer` 的 HTML 配置注入。
- 运行时配置统一走公开配置端点（并入 `/api/v1/public/*`），两种拓扑同一实现，产物 build once, deploy anywhere。
- 控制面 API（auth/user/admin/payment）以 OpenAPI 3 规格为源，CI 校验契约与实现同步，生成 TS 客户端替代约 49 个手写 axios 模块；网关协议端点（OpenAI/Anthropic/Gemini 兼容）保持上游协议语义，不纳入 codegen，单独文档化。
- 版本偏移防护：后端暴露版本/最低兼容前端版本端点，分离部署时前端自检告警；embed 模式天然同版本。
- 部署矩阵：默认（install.sh、单容器 compose、后台单二进制在线升级、setup 向导）零变化；新增分离变体 compose（`sub2api-api` + `sub2api-web` nginx + postgres + redis）、官方 nginx 参考配置（SSE 关 buffering、WS upgrade、`underscores_in_headers on`）、Release 增发前端静态包工件、分域部署 CORS 白名单。
- Alternatives considered: 完全移除 embed（对自托管用户是部署 breaking：强制 nginx、双工件升级、版本偏移故障面，被否）；维持 HTML 注入（产物环境相关，无法 CDN/多环境复用，被否）。

### D9 前端模块化与表单引擎

- `src/features/<domain>/`（views/components/api/stores 就近），公共 UI 沉淀 `src/ui/`；生成客户端外保留薄封装（401 刷新队列等拦截逻辑）。
- 账号表单引擎：平台字段 registry（schema 驱动：字段、校验、条件显隐、i18n key），`CreateAccountModal`/`EditAccountModal`/`BulkEditAccountModal` 共用一份平台定义，新平台接入只写 registry 条目。
- `SettingsView.vue` 拆为懒加载子路由分区（每区独立组件与 store 切片）。

### D10 CI 边界防护

golangci-lint `depguard`（按 D1 规则配置各层允许的 import 前缀）+ 独立架构测试（go/packages 扫描违例，含"禁止包级可变全局"检查）；前端以 ESLint 边界规则限制跨 feature 直接深层 import。P0 先按"现状白名单 + 禁止新增违例"落地，随迁移逐步收紧至目标规则。

## Risks / Trade-offs

- 与在途 change 冲突（当前 41 个活跃 change 集中于网关区）→ 见"迁移协调"；每平台迁移为独立 change，避让密集区，先行低风险平台。
- 管道抽象可能被协议差异击穿 → 试点平台先行验证；阶段接口允许 provider 定制环节；协议转换永远保留显式代码，不强行归一。
- 机械搬移引入回归 → 每阶段全量测试 + 编译作为门禁；P1/P2 禁止任何逻辑修改（纯移动 + rename import）。
- 迁移周期长、双结构过渡期认知负担 → 每阶段独立可发布、可暂停；CI 边界规则防止过渡期腐化。
- OpenAPI 契约与 534 条路由的初次对齐成本高 → 契约先覆盖 user/auth/payment，admin 分组分批；生成客户端与手写模块可短期共存，按 feature 切换。

## Migration Plan

- P0 边界冻结：depguard/架构测试（现状白名单）、基线回归护栏、CI 接入。
- P1 ports 下沉：接口与依赖类型机械搬移至 `ports`/`domain`，repository 改指向，删除 service 反向依赖。
- P2 domain 与 config 拆分、合并双中间件目录。
- P3 网关管道：骨架 + 试点平台（建议 Antigravity 或 Gemini，链路自包含、在途 change 少）→ Claude → Kimi → Grok → OpenAI/Codex（在途最密集，最后迁）；每平台迁完删旧链路；WS ingress 状态机拆解随 OpenAI 平台迁移完成。
- P4 调度统一（先接口适配后策略合并）。
- P5 结算统一 + 全局 setter 收编 + Lifecycle 注册表。
- P6 API 契约化 + 前端模块化（OpenAPI、生成客户端、features 迁移、表单引擎、SettingsView 拆分）。
- P7 契约分离交付：公开配置端点、产物环境无关化、embed 纯静态化、分离 compose 变体与 nginx 参考配置、版本偏移防护（默认部署零变化）。
- P8 观测（管道阶段级指标/日志）与文档收尾。

回滚策略：P0–P2 为纯结构搬移，git revert 即可；P3 起每平台迁移独立 change，出现协议回归可单平台回退（旧链路删除发生在该平台回归测试通过后的同一 change 内，回退即恢复旧链路）。

## 迁移协调（与在途 change）

- 每个阶段/平台迁移是独立 OpenSpec change 与原子提交，不与协议修复混提（遵守 project.md Git 约定）。
- P3 平台顺序按在途 change 密度动态调整：迁移某平台前检查 `openspec list` 中该平台活跃 change，未归档则顺延或先协调合并。
- 横切修复（限流、计费）在过渡期若涉及未迁移平台，允许在旧链路修复并在迁移时带入管道实现，由该平台迁移 change 的回归测试兜底。

## Open Questions

- OpenAPI 工具链选型：spec-first（手写 spec + oapi-codegen/openapi-typescript）还是 code-first（swag 注释生成）；倾向 spec-first、客户端生成先行、服务端绑定渐进。
- 表单引擎 schema 表达力边界（复杂联动如 OAuth 授权流内嵌步骤是否留在定制组件中）。
- `datamanagementd` 旁路进程是否纳入 `app` 模块边界或维持独立（倾向维持独立，仅共享 `domain`/`pkg`）。
