## 1. Preflight and data safety

- [x] 1.1 增加只读审计命令，统计订阅分组、有效订阅、余额不足用户、订阅计划、待履约订单和未使用兑换码。
- [x] 1.2 新增前向迁移，将订阅分组转为 standard、终止有效用户订阅并禁用订阅创建来源；保留历史表和外键。
- [x] 1.3 增加迁移回归测试，验证历史 usage/order/subscription 数据仍可读取。

## 2. Backend removal

- [x] 2.1 从 API Key 鉴权和网关上下文移除 UserSubscription 加载、配额窗口校验及订阅优先分支，统一余额校验。
- [x] 2.2 从用量结算移除 subscription billing type、订阅额度增量和 subscription_id 写入；历史查询兼容保留。
- [ ] 2.3 停止注册 admin/user subscription handlers 和 routes，并移除服务器依赖注入中的运行时 SubscriptionService。
- [x] 2.4 移除订阅分配、续期、恢复、配额重置、过期任务、通知和缓存维护的生产调用链。
- [x] 2.5 移除支付、兑换码、OAuth 首次绑定和默认策略创建订阅的路径；余额及非订阅履约保持不变。
- [x] 2.6 分组服务强制标准模式，拒绝新的 subscription 类型输入，并移除订阅专属校验。

## 3. Frontend removal

- [x] 3.1 删除后台/用户订阅路由、侧栏入口、页面和 API/store。
- [x] 3.2 从分组列表与创建/编辑表单移除订阅模式、有效期和日/周/月额度配置。
- [x] 3.3 移除用户订阅进度、平台订阅配额和订阅分配操作；余额与平台普通限额展示保留。
- [x] 3.4 从支付与兑换界面移除用户订阅商品/类型，保留历史订单展示所需只读标签。
- [ ] 3.5 清理仅供用户订阅模式使用的 types、i18n 和测试。

## 4. Verification and rollout

- [x] 4.1 增加余额鉴权、余额扣费、无 subscription context、usage subscription_id=NULL 的后端回归测试。
- [x] 4.2 增加路由不存在、侧栏无入口、分组无订阅模式的前端回归测试。
- [ ] 4.3 运行 Go 全量测试、前端测试、类型检查、生产构建和 OpenSpec 严格校验。
- [x] 4.4 蓝绿部署并执行迁移前后只读核验，确认新订阅数量不再增长且利润账号订阅成本报表不变。
