package service

import (
	"context"
	"math"
	"sort"
	"time"
)

const (
	SupplyForecastDefaultHorizonDays  = 30
	SupplyForecastDefaultSafetyMargin = 0.20
)

type SupplyForecastResponse struct {
	GeneratedAt          time.Time                 `json:"generated_at"`
	HistoryStart         time.Time                 `json:"history_start"`
	HistoryEnd           time.Time                 `json:"history_end"`
	Timezone             string                    `json:"timezone"`
	HorizonDays          int                       `json:"horizon_days"`
	SafetyMargin         float64                   `json:"safety_margin"`
	SpendableBalance     float64                   `json:"spendable_balance"`
	FrozenBalance        float64                   `json:"frozen_balance"`
	EligibleUsers        int64                     `json:"eligible_users"`
	DailyBurn7           float64                   `json:"daily_burn_7"`
	DailyBurn30          float64                   `json:"daily_burn_30"`
	BaseDailyDemand      float64                   `json:"base_daily_demand"`
	PlanningDailyDemand  float64                   `json:"planning_daily_demand"`
	ProjectedConsumption float64                   `json:"projected_consumption"`
	PlanningConsumption  float64                   `json:"planning_consumption"`
	RunwayDays           *float64                  `json:"runway_days,omitempty"`
	Available            bool                      `json:"available"`
	UnavailableReason    string                    `json:"unavailable_reason,omitempty"`
	Platforms            []*PlatformSupplyForecast `json:"platforms"`
}

type PlatformSupplyForecast struct {
	Platform                      string   `json:"platform"`
	DemandShare                   float64  `json:"demand_share"`
	ProjectedConsumption          float64  `json:"projected_consumption"`
	PlanningConsumption           float64  `json:"planning_consumption"`
	SubscriptionShare             float64  `json:"subscription_share"`
	SubscriptionPlanningDaily     float64  `json:"subscription_planning_daily"`
	AccountDailyCapacityP75       *float64 `json:"account_daily_capacity_p75,omitempty"`
	RequiredSubscriptionAccounts  *int     `json:"required_subscription_accounts,omitempty"`
	CurrentSubscriptionAccounts   int      `json:"current_subscription_accounts"`
	SubscriptionAccountGap        *int     `json:"subscription_account_gap,omitempty"`
	SubscriptionAccountSurplus    *int     `json:"subscription_account_surplus,omitempty"`
	SampleAccounts                int      `json:"sample_accounts"`
	SampleAccountDays             int      `json:"sample_account_days"`
	Confidence                    string   `json:"confidence"`
	SubscriptionUnavailableReason string   `json:"subscription_unavailable_reason,omitempty"`
	MeteredShare                  float64  `json:"metered_share"`
	MeteredCostRatio              *float64 `json:"metered_cost_ratio,omitempty"`
	MeteredProcurementBudget      *float64 `json:"metered_procurement_budget,omitempty"`
	MeteredUnavailableReason      string   `json:"metered_unavailable_reason,omitempty"`
}

type platformForecastAccumulator struct {
	revenue              float64
	subscriptionRevenue  float64
	meteredRevenue       float64
	meteredCost          float64
	subscriptionDays     []float64
	subscriptionAccounts map[int64]struct{}
}

func (s *ProfitService) GetSupplyForecast(ctx context.Context, horizonDays int, safetyMargin float64, tzName string) (*SupplyForecastResponse, error) {
	if horizonDays <= 0 {
		horizonDays = SupplyForecastDefaultHorizonDays
	}
	if safetyMargin < 0 {
		safetyMargin = 0
	}
	if tzName == "" {
		tzName = "Asia/Shanghai"
	}
	location, err := time.LoadLocation(tzName)
	if err != nil {
		location = time.UTC
		tzName = "UTC"
	}
	now := time.Now()
	localNow := now.In(location)
	historyEnd := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location).AddDate(0, 0, 1)
	historyStart := historyEnd.AddDate(0, 0, -30)

	balance, err := s.profitRepo.GetStoredValueSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	samples, err := s.profitRepo.GetSupplyForecastUsageSamples(ctx, historyStart, historyEnd, tzName)
	if err != nil {
		return nil, err
	}
	currentSupply, err := s.profitRepo.GetSchedulableSubscriptionSupply(ctx)
	if err != nil {
		return nil, err
	}
	return calculateSupplyForecast(now.UTC(), historyStart, historyEnd, tzName, horizonDays, safetyMargin, balance, samples, currentSupply), nil
}

func calculateSupplyForecast(
	generatedAt, historyStart, historyEnd time.Time,
	tzName string,
	horizonDays int,
	safetyMargin float64,
	balance *StoredValueSnapshot,
	samples []*SupplyForecastUsageSample,
	currentSupply map[string]int,
) *SupplyForecastResponse {
	if balance == nil {
		balance = &StoredValueSnapshot{}
	}
	resp := &SupplyForecastResponse{
		GeneratedAt:      generatedAt,
		HistoryStart:     historyStart,
		HistoryEnd:       historyEnd,
		Timezone:         tzName,
		HorizonDays:      horizonDays,
		SafetyMargin:     roundMoney(safetyMargin),
		SpendableBalance: roundMoney(balance.SpendableBalance),
		FrozenBalance:    roundMoney(balance.FrozenBalance),
		EligibleUsers:    balance.EligibleUsers,
	}

	last7Start := historyEnd.AddDate(0, 0, -7).Format(time.DateOnly)
	platforms := make(map[string]*platformForecastAccumulator)
	for _, sample := range samples {
		if sample == nil || sample.Revenue <= 0 || sample.Platform == "" {
			continue
		}
		resp.DailyBurn30 += sample.Revenue
		if sample.Date >= last7Start {
			resp.DailyBurn7 += sample.Revenue
		}
		acc := platforms[sample.Platform]
		if acc == nil {
			acc = &platformForecastAccumulator{subscriptionAccounts: make(map[int64]struct{})}
			platforms[sample.Platform] = acc
		}
		acc.revenue += sample.Revenue
		if isSubscriptionAccountType(sample.AccountType) {
			acc.subscriptionRevenue += sample.Revenue
			acc.subscriptionDays = append(acc.subscriptionDays, sample.Revenue)
			acc.subscriptionAccounts[sample.AccountID] = struct{}{}
		} else {
			acc.meteredRevenue += sample.Revenue
			acc.meteredCost += sample.MeteredCost
		}
	}
	resp.DailyBurn7 = roundMoney(resp.DailyBurn7 / 7)
	resp.DailyBurn30 = roundMoney(resp.DailyBurn30 / 30)
	resp.BaseDailyDemand = math.Max(resp.DailyBurn7, resp.DailyBurn30)
	resp.PlanningDailyDemand = roundMoney(resp.BaseDailyDemand * (1 + safetyMargin))
	resp.ProjectedConsumption = roundMoney(math.Min(resp.SpendableBalance, resp.BaseDailyDemand*float64(horizonDays)))
	resp.PlanningConsumption = roundMoney(math.Min(resp.SpendableBalance, resp.PlanningDailyDemand*float64(horizonDays)))
	if resp.BaseDailyDemand <= 0 {
		resp.UnavailableReason = "no_recent_balance_usage"
		return resp
	}
	runway := roundMoney(resp.SpendableBalance / resp.BaseDailyDemand)
	resp.RunwayDays = &runway
	resp.Available = true

	totalRevenue := 0.0
	for _, acc := range platforms {
		totalRevenue += acc.revenue
	}
	if totalRevenue <= 0 {
		resp.Available = false
		resp.UnavailableReason = "no_platform_mix"
		return resp
	}

	platformNames := make([]string, 0, len(platforms))
	for platform := range platforms {
		platformNames = append(platformNames, platform)
	}
	sort.Strings(platformNames)
	for _, platform := range platformNames {
		acc := platforms[platform]
		item := &PlatformSupplyForecast{
			Platform:                    platform,
			DemandShare:                 roundRatio(acc.revenue / totalRevenue),
			CurrentSubscriptionAccounts: currentSupply[platform],
			SampleAccounts:              len(acc.subscriptionAccounts),
			SampleAccountDays:           len(acc.subscriptionDays),
		}
		item.ProjectedConsumption = roundMoney(resp.ProjectedConsumption * item.DemandShare)
		item.PlanningConsumption = roundMoney(resp.PlanningConsumption * item.DemandShare)
		if acc.revenue > 0 {
			item.SubscriptionShare = roundRatio(acc.subscriptionRevenue / acc.revenue)
			item.MeteredShare = roundRatio(acc.meteredRevenue / acc.revenue)
		}
		item.SubscriptionPlanningDaily = roundMoney(resp.PlanningDailyDemand * item.DemandShare * item.SubscriptionShare)
		item.Confidence = supplyForecastConfidence(item.SampleAccounts, item.SampleAccountDays)
		if item.SubscriptionShare > 0 {
			capacity := percentile75(acc.subscriptionDays)
			if capacity <= 0 {
				item.SubscriptionUnavailableReason = "no_subscription_capacity_sample"
			} else {
				capacity = roundMoney(capacity)
				item.AccountDailyCapacityP75 = &capacity
				required := int(math.Ceil(item.SubscriptionPlanningDaily / capacity))
				item.RequiredSubscriptionAccounts = &required
				gap := required - item.CurrentSubscriptionAccounts
				if gap < 0 {
					gap = 0
				}
				surplus := item.CurrentSubscriptionAccounts - required
				if surplus < 0 {
					surplus = 0
				}
				item.SubscriptionAccountGap = &gap
				item.SubscriptionAccountSurplus = &surplus
			}
		}
		if item.MeteredShare > 0 {
			if acc.meteredRevenue <= 0 {
				item.MeteredUnavailableReason = "no_metered_cost_sample"
			} else {
				ratio := roundRatio(acc.meteredCost / acc.meteredRevenue)
				budget := roundMoney(item.PlanningConsumption * item.MeteredShare * ratio)
				item.MeteredCostRatio = &ratio
				item.MeteredProcurementBudget = &budget
			}
		}
		resp.Platforms = append(resp.Platforms, item)
	}
	return resp
}

func percentile75(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	index := int(math.Ceil(float64(len(sorted))*0.75)) - 1
	if index < 0 {
		index = 0
	}
	return sorted[index]
}

func supplyForecastConfidence(accounts, accountDays int) string {
	switch {
	case accounts >= 5 && accountDays >= 50:
		return "high"
	case accounts >= 2 && accountDays >= 14:
		return "medium"
	default:
		return "low"
	}
}

func roundRatio(value float64) float64 {
	return math.Round(value*10000) / 10000
}
