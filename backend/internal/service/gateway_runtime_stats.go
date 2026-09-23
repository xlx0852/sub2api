package service

import (
	"context"
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/app/scheduling"
)

func (s *GatewayService) reportGatewayCandidateSnapshots(
	ctx context.Context,
	accounts []*Account,
	loadMap map[int64]*AccountLoadInfo,
) {
	if s == nil || s.gatewayRuntimeStats == nil {
		return
	}
	for _, account := range accounts {
		if account == nil {
			continue
		}
		feedback := scheduling.RuntimeFeedback{AccountID: account.ID}
		if loadInfo := loadMap[account.ID]; loadInfo != nil {
			loadRate := float64(loadInfo.LoadRate)
			queueDepth := float64(loadInfo.WaitingCount)
			feedback.LoadRate = &loadRate
			feedback.QueueDepth = &queueDepth
		}
		if headroom, ok := gatewayQuotaHeadroom(account); ok {
			feedback.QuotaHeadroom = &headroom
		}
		s.gatewayRuntimeStats.Report(feedback)
	}
}

func (s *GatewayService) gatewayRuntimeStatsSnapshot(accountID int64, now time.Time) scheduling.RuntimeStatsSnapshot {
	if s == nil || s.gatewayRuntimeStats == nil {
		return scheduling.RuntimeStatsSnapshot{AccountID: accountID}
	}
	return s.gatewayRuntimeStats.Snapshot(accountID, now)
}

func (s *GatewayService) ReportGatewayAccountScheduleResult(accountID int64, success bool, firstTokenMs *int) {
	if s == nil || s.gatewayRuntimeStats == nil || accountID <= 0 {
		return
	}
	errorSample := 1.0
	if success {
		errorSample = 0
	}
	feedback := scheduling.RuntimeFeedback{AccountID: accountID, ErrorSample: &errorSample}
	if firstTokenMs != nil && *firstTokenMs > 0 {
		ttft := float64(*firstTokenMs)
		feedback.TTFTMillis = &ttft
	}
	s.gatewayRuntimeStats.Report(feedback)
}

func gatewayQuotaHeadroom(account *Account) (float64, bool) {
	if account == nil || account.Type != AccountTypeAPIKey {
		return 0, false
	}
	headroom := 1.0
	configured := false
	apply := func(limit, used float64) {
		if limit <= 0 {
			return
		}
		configured = true
		remaining := 1 - used/limit
		if remaining < headroom {
			headroom = remaining
		}
	}
	apply(account.GetQuotaLimit(), account.GetQuotaUsed())
	dailyUsed := account.GetQuotaDailyUsed()
	if account.IsDailyQuotaPeriodExpired() {
		dailyUsed = 0
	}
	apply(account.GetQuotaDailyLimit(), dailyUsed)
	weeklyUsed := account.GetQuotaWeeklyUsed()
	if account.IsWeeklyQuotaPeriodExpired() {
		weeklyUsed = 0
	}
	apply(account.GetQuotaWeeklyLimit(), weeklyUsed)
	if !configured {
		return 0, false
	}
	return math.Max(0, math.Min(1, headroom)), true
}
