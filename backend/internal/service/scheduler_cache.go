package service

import (
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/ports"
)

const (
	SchedulerModeSingle = domain.SchedulerModeSingle
	SchedulerModeMixed  = domain.SchedulerModeMixed
	SchedulerModeForced = domain.SchedulerModeForced
)

// SchedulerCache 负责调度快照与账号快照的缓存读写。
type SchedulerCache = ports.SchedulerCache
