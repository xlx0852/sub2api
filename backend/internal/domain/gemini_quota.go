// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

type GeminiUsageTotals struct {
	ProRequests   int64
	FlashRequests int64
	ProTokens     int64
	FlashTokens   int64
	ProCost       float64
	FlashCost     float64
}
