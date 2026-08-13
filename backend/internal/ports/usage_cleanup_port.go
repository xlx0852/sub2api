// Package ports —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package ports

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// UsageCleanupRepository 定义清理任务持久层接口
type UsageCleanupRepository interface {
	CreateTask(ctx context.Context, task *domain.UsageCleanupTask) error
	ListTasks(ctx context.Context, params domain.PaginationParams) ([]domain.UsageCleanupTask, *domain.PaginationResult, error)
	// ClaimNextPendingTask 抢占下一条可执行任务：
	// - 优先 pending
	// - 若 running 超过 staleRunningAfterSeconds（可能由于进程退出/崩溃/超时），允许重新抢占继续执行
	ClaimNextPendingTask(ctx context.Context, staleRunningAfterSeconds int64) (*domain.UsageCleanupTask, error)
	// GetTaskStatus 查询任务状态；若不存在返回 sql.ErrNoRows
	GetTaskStatus(ctx context.Context, taskID int64) (string, error)
	// UpdateTaskProgress 更新任务进度（deleted_rows）用于断点续跑/展示
	UpdateTaskProgress(ctx context.Context, taskID int64, deletedRows int64) error
	// CancelTask 将任务标记为 canceled（仅允许 pending/running）
	CancelTask(ctx context.Context, taskID int64, canceledBy int64) (bool, error)
	MarkTaskSucceeded(ctx context.Context, taskID int64, deletedRows int64) error
	MarkTaskFailed(ctx context.Context, taskID int64, deletedRows int64, errorMsg string) error
	DeleteUsageLogsBatch(ctx context.Context, filters domain.UsageCleanupFilters, limit int) (int64, error)
}
