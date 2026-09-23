// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

// GroupAccountCapacityRow is the lightweight account projection needed for
// capacity summary aggregation.
type GroupAccountCapacityRow struct {
	GroupID     int64
	AccountID   int64
	Concurrency int
}
