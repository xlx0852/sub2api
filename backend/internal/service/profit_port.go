package service

const (
	// AccountCostTypeSubscription 订阅制（固定周期费用）
	AccountCostTypeSubscription = "subscription"
	// AccountCostTypeMetered 按量付费（成本 = usage_logs 账号侧成本汇总）
	AccountCostTypeMetered = "metered"
)
