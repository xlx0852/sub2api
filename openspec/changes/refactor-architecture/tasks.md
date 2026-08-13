## 1. P0 边界冻结与防护

- [x] 1.1 配置 golangci-lint depguard 依赖规则（现状白名单 + 禁止新增违例）
- [x] 1.2 新增架构测试：扫描层间 import 违例与包级可变全局
- [x] 1.3 标记基线回归护栏（路由清单、关键网关/调度/计费测试集）并接入 CI

## 2. P1 ports 下沉（打破拆包死锁）

- [ ] 2.1 创建 `internal/ports` 与 `internal/domain` 骨架
- [ ] 2.2 将 service 内 Repository/Cache 接口及其依赖类型机械搬移（签名不变）
- [ ] 2.3 `repository` 改为实现 ports 接口，删除对 service 的 import
- [ ] 2.4 全量编译 + 既有测试回归通过

## 3. P2 domain 与 config 拆分

- [ ] 3.1 领域类型/常量/错误迁入 `domain`
- [ ] 3.2 `Config` 按模块拆为子结构（YAML/env 键名兼容），模块只接收子结构
- [ ] 3.3 合并 `internal/middleware` 与 `internal/server/middleware`

## 4. P3 网关统一管道（全平台迁移，每平台独立 change）

- [ ] 4.1 落地 `app/gateway/{pipeline,stream,transport}` 骨架与阶段接口
- [ ] 4.2 试点平台迁移（Antigravity 或 Gemini）并验证抽象与回归
- [ ] 4.3 Claude 链路迁移，删除旧 `GatewayService.Forward` 路径
- [ ] 4.4 Kimi、Grok 迁移（按在途 change 密度安排窗口）
- [ ] 4.5 OpenAI/Codex 迁移（HTTP/passthrough/WS），指纹/模型映射/Fast policy 收敛为 Rewrite 中间件
- [ ] 4.6 WS ingress 拆为 session/turn/connection 三层状态机
- [ ] 4.7 删除全部旧转发链路与同构流式实现，确认单一路径

## 5. P4 调度统一

- [ ] 5.1 定义 `AccountSelector` 接口与策略插件模型
- [ ] 5.2 两套现有调度器适配接口（行为不变）
- [ ] 5.3 逐策略合并去重，删除双实现

## 6. P5 结算统一与装配治理

- [ ] 6.1 合并两套 `RecordUsage` 为单一 settlement，统一 usage 模型
- [ ] 6.2 包级 setter/全局状态收归 Wire 注入
- [ ] 6.3 Lifecycle 注册表替代手工 cleanup 列表

## 7. P6 API 契约化与前端模块化

- [ ] 7.1 控制面 OpenAPI 规格（user/auth/payment 先行，admin 分批）+ CI 同步校验
- [ ] 7.2 生成 TS 客户端，按 feature 替换手写 api 模块
- [ ] 7.3 前端迁移至 `features/` 结构，公共 UI 沉淀 `src/ui/`
- [ ] 7.4 账号表单引擎（平台字段 registry），Create/Edit/Bulk 三 Modal 收敛
- [ ] 7.5 `SettingsView` 拆为懒加载子路由

## 8. P7 契约分离交付

- [ ] 8.1 公开配置端点取代 HTML 注入，前端产物环境无关化
- [ ] 8.2 embed 退化为纯静态打包（默认交付与升级流程不变）
- [ ] 8.3 分离部署 compose 变体 + 官方 nginx 参考配置 + Release 前端静态包工件
- [ ] 8.4 版本偏移防护端点与前端自检

## 9. P8 观测与收尾

- [ ] 9.1 管道阶段级指标与日志规范
- [ ] 9.2 更新部署矩阵与架构文档
- [ ] 9.3 运行 `openspec validate refactor-architecture --strict` 并归档
