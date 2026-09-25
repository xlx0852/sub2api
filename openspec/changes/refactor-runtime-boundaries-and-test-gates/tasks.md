# Tasks

## 1. Baseline and gates

- [x] 1.1 记录当前 `archtest`、默认测试、`-tags unit`、`go vet` 的失败集合与耗时
- [x] 1.2 修复 unit-tag helper/文件隔离，使 `go test -tags unit ./internal/service/...` 与 `go vet -tags unit ./internal/service/...` 可编译
- [x] 1.3 将 unit/vet/archtest 门禁纳入 CI 或等价的仓库验证脚本
- [x] 1.4 补 archtest：禁止新增 service→handler、service→Gin transport、handler→repository/Ent 的依赖

## 2. Runtime state and injection

- [x] 2.1 将 `mirasimRelayClients` 改为注入式 `RelayClientRegistry`，覆盖并发、轮换、TTL/eviction、测试隔离
- [x] 2.2 将 `openAICodexTurnStateProbeWatch` 改为注入式 watcher/lifecycle component
- [x] 2.3 清理新增包级可变全局，令 `TestNoNewPackageLevelMutableGlobals` 通过

## 3. Transport boundary

- [x] 3.1 定义 transport-neutral stream event/sink contract 与 HTTP/SSE adapter
- [ ] 3.2 迁移一个 Claude/OpenAI HTTP/SSE 主路径，补旧/新 body、事件、错误、usage、settlement 差分测试
- [ ] 3.3 迁移 WS、图片和兼容入口，删除已验证的 service 直接 writer/Flush/SSE 编码
- [ ] 3.4 验证所有 provider 仍通过统一 pipeline 阶段执行，禁止新增并行转发路径

## 4. Persistence and shared contracts

- [ ] 4.1 将 OAuth handler 的直接 Ent 查询迁入 identity service/repository/ports
- [ ] 4.2 将 service 中的裸 SQL/直接 Ent 访问按域迁入 repository，保持事务边界和错误语义
- [ ] 4.3 集中 Redis key builder、版本与 snapshot invalidation，补 DB mutation 后缓存一致性测试
- [x] 4.4 抽取 usage log 单一 column schema/builder，覆盖 single/batch/best-effort/no-result 四种写入模式
- [ ] 4.5 将 settings parser 按领域拆分为类型化 schema/defaults，保持历史 key/value 兼容

## 5. Service package boundaries

- [x] 5.1 将 AccountSelector 公共 primitives 与平台策略分离，补候选集/排序/sticky/cooldown 差分测试
- [ ] 5.2 建立 gateway、scheduling、billing、identity、settings 子包边界与 import archtest
- [ ] 5.3 将超大测试文件/测试函数按能力拆分，默认测试与 unit 测试按目录运行
- [ ] 5.4 为 `GatewayService`、`OpenAIGatewayService`、`SystemSettings` 制定拆分清单，禁止继续增加字段

## 6. Repository hygiene and final verification

- [x] 6.1 更新 `.gitignore`，覆盖 artifact、tar/zip、`.env`、`.venv`、本地 dist 与临时文件
- [x] 6.2 清理当前未跟踪构建产物与敏感/临时目录（保留必要源代码）
- [ ] 6.3 运行后端构建、unit/vet/archtest、受影响协议/计费/调度测试
- [ ] 6.4 运行前端 typecheck/Vitest；确认 routes.golden、OpenAPI 契约无未预期变化
- [ ] 6.5 更新部署/回滚说明，确认无公开 API、计费、调度语义回归
