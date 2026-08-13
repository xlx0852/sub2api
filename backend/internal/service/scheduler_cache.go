package service

import (
	"github.com/Wei-Shaw/sub2api/internal/ports"
)

const (
	SchedulerModeSingle = "single"
	SchedulerModeMixed  = "mixed"
	SchedulerModeForced = "forced"
)

// SchedulerCache 负责调度快照与账号快照的缓存读写。
type SchedulerCache = ports.SchedulerCache
