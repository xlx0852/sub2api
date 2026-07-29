## Context

当前用户订阅模式不是孤立的 CRUD 页面。`groups.subscription_type=subscription` 会让 API Key 鉴权加载 `user_subscriptions`，跳过余额路径，并在网关结算时累计日/周/月配额。支付、兑换码、OAuth 首次绑定和后台管理又会创建或延长订阅。直接隐藏页面会留下仍可使用的后门路径，直接删表则会破坏历史订单和用量审计。

## Goals / Non-Goals

- Goals:
  - 用户请求统一使用余额授权与余额扣费。
  - 移除所有可创建、延长或消费用户订阅的运行时路径。
  - 保留历史财务、订单、兑换和用量审计数据。
  - 保持账号利润分析中的上游订阅采购成本不变。
- Non-Goals:
  - 不移除 OAuth / Setup Token 账号类型。
  - 不移除利润分析的 `account_subscription_cycles`。
  - 不删除历史 `user_subscriptions`、`payment_orders` 或 `usage_logs` 数据。

## Decisions

- Decision: 采用“停止运行时使用、保留历史结构”的退役方式，不直接删除订阅表和历史外键列。
  - Rationale: 历史用量的 `subscription_id`、订阅订单和兑换记录仍是财务审计事实；删表会丢失归因并扩大迁移风险。
- Decision: 发布迁移把 `groups.subscription_type='subscription'` 改为 `standard`，并把仍为 active/suspended 的用户订阅改为 `expired`。
  - Rationale: 仅改代码会让回滚实例或旧任务继续把历史配置视为有效订阅；数据状态也必须明确终止授权。
- Decision: 所有新请求使用余额阈值和余额扣费，网关上下文不再设置 `UserSubscription`，新用量日志的 `subscription_id` 保持 NULL。
  - Rationale: 单一计费路径才能证明订阅模式真正被移除，而不是仅从界面隐藏。
- Decision: 停止注册订阅管理和用户订阅 API，而不是返回伪成功兼容响应。
  - Rationale: 旧客户端必须显式发现能力已下线，避免继续认为订阅写入有效。
- Decision: 订阅型支付计划、兑换码和默认订阅策略在迁移时禁用；历史订单和已使用兑换记录只读保留。
  - Rationale: 如果任一入口还能创建订阅，系统仍存在订阅模式。
- Decision: 分组 schema 字段先保留但服务层强制标准模式，前端不再暴露订阅字段；待运行至少一个稳定版本后再单独评估物理删列。
  - Rationale: Ent 生成代码、历史查询和回滚版本仍依赖字段，分阶段退役比同版本物理删列更安全。
- Decision: 名称中涉及“账号订阅”“上游订阅”的逻辑不删除，必须依据数据对象区分：`UserSubscription` 属于本次范围，`AccountSubscriptionCycle` 不属于。
  - Rationale: 两者同名但业务含义相反，误删会破坏利润核算。

## Risks / Trade-offs

- 原订阅用户切换后可能因余额不足立即无法请求。
  - Mitigation: 部署前只读统计活跃订阅用户及余额，生成影响清单；切换动作本身不自动赠送余额。
- 已售但未履约的订阅订单或兑换码可能无法继续兑现。
  - Mitigation: 部署前统计 pending/paid 订阅订单和未使用订阅兑换码，先完成退款或人工补偿，再禁用入口。
- 旧实例在蓝绿切换期间可能短暂继续接受订阅请求。
  - Mitigation: 数据迁移在新实例健康后、流量切换前执行；切流后立即停止旧实例写流量，并验证新订阅数量不再增长。
- 大量测试和依赖注入仍引用 SubscriptionService。
  - Mitigation: 先从生产调用链移除，再删除构造参数与测试桩；使用编译错误作为依赖清单，避免留下隐式路径。

## Migration Plan

1. 只读盘点订阅分组、活跃订阅用户、余额不足用户、订阅支付计划、未履约订单和未使用兑换码。
2. 发布新代码但暂不切流，验证余额鉴权和普通分组请求。
3. 在事务中把订阅分组改为 standard、终止活跃用户订阅，并禁用所有可创建订阅的计划/兑换/默认策略。
4. 蓝绿切流到新版本，验证订阅 API 为 404、普通 API Key 只走余额路径、新 usage log 的 `subscription_id` 为 NULL。
5. 保留旧表和字段至少一个稳定版本；如未来需要物理删除，另立数据库清理变更。

## Rollback

- 代码可切回旧蓝绿实例，但数据状态不会自动恢复。
- 若确需恢复订阅能力，必须从迁移前导出的影响清单恢复分组类型和订阅状态，并重新启用计划/兑换策略；禁止用全表无条件 UPDATE 猜测原状态。

## Open Questions

- 无。默认不把既有订阅额度折算为余额；如需要补偿，应按部署前影响清单单独制定财务补偿方案。
