# Change: 移除用户订阅与配额分配模式

## Why

项目同时维护余额计费和用户订阅配额两套授权、计费与运营链路。用户订阅模式贯穿分组类型、API Key 鉴权、配额窗口、支付履约、兑换码和通知任务，增加了维护成本，也容易与账号侧 OAuth 订阅采购成本混淆。系统将收敛为单一的用户余额计费模式。

## What Changes

- **BREAKING** 移除后台“订阅管理”和用户“我的订阅”页面、导航入口及管理 API。
- **BREAKING** 分组不再支持 `subscription` 类型；所有分组按标准余额计费，创建和编辑界面移除订阅类型、有效期和日/周/月配额配置。
- **BREAKING** API Key 鉴权不再加载用户订阅或校验订阅窗口，所有正常请求统一执行用户余额检查和余额扣费。
- 新产生的用量记录不再写入 `subscription_id` 或 `subscription` billing type；历史记录保留原值用于审计。
- 停止订阅分配、续期、撤销、恢复、配额重置、过期扫描、通知和订阅缓存维护任务。
- 停止通过支付订单、兑换码、OAuth 首次绑定或默认策略创建/延长用户订阅；仅保留余额充值、并发包等非订阅能力。
- 发布迁移将现有订阅分组切换为标准分组，并终止活跃用户订阅的授权效果；历史订阅、订单、兑换和用量数据不删除。
- 订阅相关旧 API 路由不再注册；旧客户端访问返回 404，不再提供兼容写入路径。
- 账号利润分析中的 OAuth / Setup Token 固定采购成本、充值周期账本和“订阅制”成本标签完整保留，它们属于上游账号采购核算，不是本次移除的用户订阅业务。

## Impact

- Affected specs: `balance-only-user-billing`（新增收敛规格）
- Affected frontend: 路由、侧栏、管理/用户订阅页面、分组表单、支付订阅卡片、用户订阅 store、订阅配额展示和相关 i18n/types。
- Affected backend: admin/user subscription handlers、API Key auth middleware、gateway usage billing、subscription service/repository/expiry worker/cache、group validation、payment fulfillment、redeem/auth auto-assignment、wire/server startup。
- Affected database: `groups.subscription_type` 及订阅配额字段停止参与运行时；`user_subscriptions`、`usage_logs.subscription_id` 和历史订单字段保留为审计数据。
- Operational impact: 切换后原本仅依赖订阅额度的用户必须有可用余额才能继续请求。
