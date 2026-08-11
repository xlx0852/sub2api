# Change: 用量计费统一六位上入

## Why

当前请求费用以浮点数进入多个账本，余额与 API Key 配额最终由 `DECIMAL(20,8)` 分别舍入，而用量日志保留十位小数。同一请求可能在余额、配额、缓存和利润收入中产生微小差异；极小正费用也可能在余额扣减时被舍为零。

## What Changes

- **BREAKING**：所有正向用量费用在价格与倍率计算完成后统一按 `0.000001 USD` 为最小单位向上取整。
- 将同一个客户应收六位上入结果用于用户余额扣费、订阅用量、API Key 配额/速率窗口、用户平台配额、计费缓存和 `usage_logs.actual_cost`；账号成本配额保留原成本口径并独立执行相同六位量化。
- 零费用保持为零；充值、退款、人工余额调整、采购成本和历史账目不应用本规则。
- 保持计费幂等：请求指纹继续由未量化的原始费用生成，避免上线前后同一请求重试发生指纹冲突。
- 增加边界、跨账本对账、幂等重试和利润收入一致性测试。

## Impact

- Affected specs: `usage-billing-precision`
- Related active change: `harden-usage-billing-admission`（预留/捕获最终也必须使用相同的规范费用）
- Affected code: `internal/service/usage_billing.go`、共享网关计费后处理、OpenAI 专用计费记录路径、用量日志与计费缓存同步
- No database schema migration: 现有金额列能够保存六位小数
- 单请求增量严格小于 `$0.000001`；按 50,000 个非零计费请求估算，每日收入上调上限小于 `$0.05`
