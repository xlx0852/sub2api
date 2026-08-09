//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestGetAccountWindowEconomics_HistoryCurrentUpcoming(t *testing.T) {
	now := time.Now()
	curStart := now.Add(-3 * 24 * time.Hour).UTC().Truncate(time.Second)
	curEnd := now.Add(4 * 24 * time.Hour).UTC().Truncate(time.Second)
	pastStart := curStart.Add(-7 * 24 * time.Hour)
	pastEnd := curStart
	futureStart := curEnd
	futureEnd := curEnd.Add(7 * 24 * time.Hour)

	repo := &windowEconRepoStub{
		profitRepoStub: profitRepoStub{
			cycles: map[int64][]*AccountSubscriptionCycle{
				// Long open cycle covering past/current/future windows.
				69: {{AccountID: 69, StartsAt: now.Add(-60 * 24 * time.Hour).UTC(), PeriodFee: 210, PeriodDays: 120}},
			},
		},
		byRange: map[string]*ProfitUsageStats{},
	}
	pastKey := fmt.Sprintf("%d|%s|%s", 69, pastStart.UTC().Format(time.RFC3339Nano), pastEnd.UTC().Format(time.RFC3339Nano))
	repo.byRange[pastKey] = &ProfitUsageStats{Requests: 800, Revenue: 120}
	repo.currentStart = curStart
	repo.currentStats = &ProfitUsageStats{Requests: 1200, Revenue: 331.13}

	svc := &ProfitService{
		profitRepo: repo,
		accountRepo: &profitAccountRepoStub{byID: map[int64]*Account{
			69: {ID: 69, Name: "GPT-自有-1", Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		}},
	}

	resp, err := svc.GetAccountWindowEconomics(context.Background(), 69, []ProfitWindowEconomicsQuery{
		{StartAt: pastStart, EndAt: pastEnd, Kind: "7d", Label: "7d"},
		{StartAt: curStart, EndAt: curEnd, Kind: "7d", Label: "7d"},
		{StartAt: futureStart, EndAt: futureEnd, Kind: "7d", Label: "7d"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Windows) != 3 {
		t.Fatalf("windows=%d want 3: %+v", len(resp.Windows), resp.Windows)
	}
	if resp.Windows[0].Status != "upcoming" || resp.Windows[0].Revenue != 0 {
		t.Fatalf("future=%+v", resp.Windows[0])
	}
	if resp.Windows[0].Cost <= 0 {
		t.Fatalf("future planned cost should be >0, got %+v", resp.Windows[0])
	}
	if resp.Windows[1].Status != "current" || resp.Windows[1].Revenue != 331.13 || resp.Windows[1].Requests != 1200 {
		t.Fatalf("current=%+v", resp.Windows[1])
	}
	if resp.Windows[1].Cost <= 0 || resp.Windows[1].Cost >= 50 {
		t.Fatalf("current amortized cost out of range: %+v", resp.Windows[1])
	}
	if resp.Windows[2].Status != "ended" || resp.Windows[2].Revenue != 120 || resp.Windows[2].Requests != 800 {
		t.Fatalf("past=%+v", resp.Windows[2])
	}
}

type windowEconRepoStub struct {
	profitRepoStub
	byRange      map[string]*ProfitUsageStats
	currentStart time.Time
	currentStats *ProfitUsageStats
}

func (s *windowEconRepoStub) GetAccountUsageStatsForRanges(_ context.Context, ranges []ProfitAccountUsageRange) (map[int64]*ProfitUsageStats, error) {
	s.rangeStatsCalls++
	out := make(map[int64]*ProfitUsageStats, len(ranges))
	for _, r := range ranges {
		key := fmt.Sprintf("%d|%s|%s", r.AccountID, r.Start.UTC().Format(time.RFC3339Nano), r.End.UTC().Format(time.RFC3339Nano))
		if st, ok := s.byRange[key]; ok {
			out[r.AccountID] = st
			continue
		}
		if !s.currentStart.IsZero() && r.Start.Equal(s.currentStart) && s.currentStats != nil {
			out[r.AccountID] = s.currentStats
			continue
		}
		out[r.AccountID] = &ProfitUsageStats{}
	}
	return out, nil
}
