# Change: 加固用量计费准入与并发结算

## Why

当前余额和订阅额度只在请求前读取快照，真实成本在上游返回后才扣减。并发请求可以同时通过准入并造成透支或订阅超额；无价格模型还会在上游成功后以零费用结算。

## What Changes

- 在转发前校验计费模型存在可用价格，无价格请求不进入上游。
- 为余额模式和订阅限额引入带 TTL 的请求前预留，并发准入基于“可用额度 - 已预留额度”。
- 上游完成后按真实费用捕获预留并退回差额；请求失败、取消或超时时释放预留。
- 预留、捕获、释放和原有用量幂等使用同一请求身份，支持重试且不重复扣费。
- WS 转发使用本地 turn ID 作为计费幂等主键，上游 response ID 仅用于追踪。

## Impact

- Affected specs: `usage-billing-admission`
- Affected code: 网关请求准入、`BillingCacheService`、`usageBillingRepository`、OpenAI/Claude/Gemini 各转发 handler、WS turn 计费身份
- **BREAKING**: 无定价的模型将在转发前被拒绝，不再零费用放行。
