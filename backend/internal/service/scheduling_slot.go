package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
)

type concurrencySlotAcquirer struct {
	svc *ConcurrencyService
}

func (a concurrencySlotAcquirer) AcquireModelAwareAccountSlot(ctx context.Context, requestedModel string, accountID int64, maxConcurrency int) (*scheduling.SlotResult, error) {
	res, err := a.svc.AcquireModelAwareAccountSlot(ctx, requestedModel, accountID, maxConcurrency)
	return slotResultFromAcquire(res), err
}

func optionalSlotAcquirer(svc *ConcurrencyService) scheduling.ModelAwareSlotAcquirer {
	if svc == nil {
		return nil
	}
	return concurrencySlotAcquirer{svc: svc}
}

func slotResultFromAcquire(res *AcquireResult) *scheduling.SlotResult {
	if res == nil {
		return nil
	}
	return &scheduling.SlotResult{
		Acquired:     res.Acquired,
		Release:      res.ReleaseFunc,
		ModelLimited: res.ModelLimited,
	}
}

func acquireResultFromSlot(res *scheduling.SlotResult) *AcquireResult {
	if res == nil {
		return nil
	}
	return &AcquireResult{
		Acquired:     res.Acquired,
		ReleaseFunc:  res.Release,
		ModelLimited: res.ModelLimited,
	}
}

func tryAcquireAccountSlot(ctx context.Context, concurrency *ConcurrencyService, requestedModel string, accountID int64, maxConcurrency int) (*AcquireResult, error) {
	if shouldSkipAccountAcquire(ctx) {
		concurrency = nil
	}
	res, err := scheduling.TryAcquireAccountSlot(ctx, optionalSlotAcquirer(concurrency), requestedModel, accountID, maxConcurrency)
	return acquireResultFromSlot(res), err
}
