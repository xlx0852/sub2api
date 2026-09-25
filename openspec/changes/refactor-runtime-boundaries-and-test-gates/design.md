# Design: 运行时边界、状态与测试门禁

## Context

现有代码已经具备 `domain`、`ports`、`service`、`repository`、`handler`、pipeline 和 archtest，但历史演进把多个职责重新集中到扁平的 `internal/service`。本设计优先修复可验证的边界违例，不进行微服务拆分，也不一次性重写所有 provider。

## Goals

- 让应用层不依赖 Gin、HTTP writer 或具体持久化实现。
- 让所有可变运行态由构造函数/生命周期注册表持有，禁止新增包级单例。
- 让 unit/vet、archtest、routes golden 成为实际 CI 门禁。
- 保持既有协议输出、调度候选语义、计费和结算不变。
- 为后续按能力拆分 service 建立可执行的依赖边界。

## Non-Goals

- 不拆成微服务。
- 不改变公开 API、数据库业务字段含义、价格/倍率和默认生产部署。
- 不在本 change 中删除仍被线上流量使用的 provider adapter；只有完成差分验证后才删除重复旧路径。

## Decisions

### 1. StreamSink，而不是把 Gin 抽象泄漏到 ports

service 只产生结构化流事件（thinking、content、tool、usage、error、done）并通过最小的 `StreamSink`/`RelayResult` 接口交付；SSE、JSON、WebSocket frame 的编码和 Flush 由 transport adapter 负责。HTTP handler 通过适配器实现该接口，WS 通过另一个适配器实现。

### 2. RelayClientRegistry 注入

Mirasim client registry、Codex turn-state watcher 等进程态对象作为构造依赖注入 GatewayService/对应 provider service。registry 内部可有 mutex、TTL 和 eviction，但不能由 package-level var 持有。测试可传入 fake/空 registry，不依赖全局 reset。

### 3. Redis key 与失效必须成对

新增 `internal/pkg/rediskeys`（或等价 ports 层 helper）集中生成 key；repository 的 DB mutation 方法负责触发对应 snapshot/version invalidation。直接 SQL 只允许迁移/只读脚本使用，业务 service 不得自行拼写调度 key。

### 4. Usage log 单一 schema

维护一个有序 column descriptor/typed insert model；single、batch、best-effort、no-result 只共享 column builder，执行方式不同。新增字段必须通过编译/测试同时覆盖所有写入模式。

### 5. Settings 分域类型化但兼容 KV

保留现有 settings key/value 表，新增按领域的 parser/schema/defaults（auth、gateway、billing、ops 等），由总装配器组合成 public/private view。旧 key 继续可读，迁移期间不得直接删除未知 key。

### 6. 渐进式 service 子包化

优先抽 `gateway/stream`、`scheduling`、`billing/settlement`、`settings`、`identity` 等低耦合边界。每次迁移以接口作为边界，禁止新子包反向依赖 handler；暂时保留兼容 facade，但标注删除条件。

## Verification Gates

每个阶段必须执行：

- `go test ./internal/archtest`
- `go test -tags unit ./internal/service/...`
- `go vet -tags unit ./internal/service/...`
- 受影响模块的行为/协议差分测试
- `gofmt -l`、`git diff --check`
- gateway 路由变化时同步 `backend/internal/server/testdata/routes.golden`

## Risks / Trade-offs

- 流事件抽象可能暴露协议差异；通过 provider-specific encoder 保留差异，不强行统一 wire format。
- service 拆包期间会暂时保留 facade；通过依赖 archtest 和删除清单防止 facade 永久化。
- settings/usage 的集中 schema 可能暴露历史字段不一致；先以 snapshot/列集回归测试锁定现状，再迁移。
- registry 从全局改为注入后，冷启动缓存行为会变化；以账号级并发、凭据轮换和热替换回归测试覆盖。

## Rollback

每个阶段独立提交；发生协议、计费或调度差异时回滚对应阶段提交，不需要数据库迁移回滚。Redis key builder 迁移期间保留旧 key 读取兼容，确认全部 writer 切换后再删除旧路径。
