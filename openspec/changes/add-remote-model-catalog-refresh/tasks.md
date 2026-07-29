## 1. Configuration and validation
- [ ] 1.1 增加 `model_catalog` 配置结构、默认值、环境覆盖和配置测试
- [x] 1.2 为 catalog 增加严格业务校验、大小限制、版本策略和 SHA-256 校验测试
- [x] 1.3 增加安全远端 URL、超时、有限重试和错误摘要处理

## 2. Snapshot storage and dynamic consumers
- [x] 2.1 将 modelcatalog 重构为不可变快照、来源与状态元数据模型
- [x] 2.2 实现本地最后可用文件的临时写入、fsync/close、原子 rename 和启动恢复
- [x] 2.3 审计并替换 OpenAI、Claude、Gemini、Antigravity、Grok、Bedrock 的启动期静态目录副本
- [x] 2.4 补充并发读取和原子切换测试，验证不会观察到部分更新

## 3. Refresh lifecycle and invalidation
- [x] 3.1 实现启动异步刷新、定时 hash 检查、周期更新、随机抖动和优雅停止
- [x] 3.2 实现刷新 singleflight/互斥与重复版本短路
- [x] 3.3 实现平台模型、映射、展示元数据、fallback pricing、UI preset 的语义差异检测
- [ ] 3.4 按差异类型失效模型目录、`/v1/models`、映射、模型广场和定价派生缓存
- [ ] 3.5 验证刷新不会修改账号白名单、渠道定价、租户路由和账号实时模型探测结果

## 4. Administration and observability
- [x] 4.1 增加管理员 catalog 状态查询和手动刷新 API
- [x] 4.2 增加结构化日志、刷新成功/失败计数、当前版本和最近成功时间指标
- [x] 4.3 在管理端展示 catalog 来源、版本、更新时间和最近错误；是否加入刷新按钮按提案确认结果执行

## 5. Verification and rollout
- [ ] 5.1 增加远端成功、hash 不符、非法 JSON、业务校验失败、超时、超限、旧版本和本地恢复测试
- [ ] 5.2 增加热更新后模型、默认映射、fallback pricing 与 UI API 生效的集成测试
- [x] 5.3 运行相关 Go、前端测试及 `openspec validate add-remote-model-catalog-refresh --strict`
- [x] 5.4 补充部署配置、禁用开关、数据源切换和紧急回滚文档
