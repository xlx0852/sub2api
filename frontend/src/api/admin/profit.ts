/**
 * Admin Profit API endpoints
 * 账号利润分析：成本绑定、利润汇总、趋势
 */

import { apiClient } from '../client'

export interface AccountCostConfig {
  id: number
  account_id: number
  cost_type: 'subscription' | 'metered'
  period_fee: number
  period_days: number
  period_start_at?: string | null
  currency: string
  window_baseline_revenue: number | null
  notes: string
  created_at: string
  updated_at: string
}

export interface AccountSubscriptionCycle {
  id: number
  account_id: number
  starts_at: string
  period_fee: number
  period_days: number
  currency: string
  notes: string
  created_at: string
  updated_at: string
}

export interface SubscriptionCycleListResponse {
  cycles: AccountSubscriptionCycle[]
  subscription_expires_at?: string
  oauth_token_expires_at?: string
}

export interface AccountProfitSummary {
  account_id: number
  account_name: string
  platform: string
  account_type: string
  cost_type: 'subscription' | 'metered'
  configured: boolean
  requests: number
  revenue: number
  cost: number
  profit: number
  margin: number
  five_hour_utilization?: number
  seven_day_utilization?: number
  window_efficiency?: number
  window_baseline_source?: string
  billing_window_start?: string
  billing_window_end?: string
  billing_window_progress?: number
  billing_window_revenue?: number
  billing_window_cost?: number
  billing_window_profit?: number
  billing_window_source?: 'manual' | 'subscription_expiry'
  requires_cycle_start?: boolean
  currency: string
}

export interface ProfitSummaryResponse {
  start: string
  end: string
  total_revenue: number
  total_cost: number
  total_profit: number
  accounts: AccountProfitSummary[]
}

export interface ProfitTrendPoint {
  date: string
  revenue: number
  cost: number
  profit: number
}

export interface ProfitOverviewResponse {
  generated_at: string
  summary: ProfitSummaryResponse
  points: ProfitTrendPoint[]
}

export type SupplyForecastConfidence = 'high' | 'medium' | 'low'

export interface PlatformSupplyForecast {
  platform: string
  demand_share: number
  projected_consumption: number
  planning_consumption: number
  subscription_share: number
  subscription_planning_daily: number
  account_daily_capacity_p75?: number
  required_subscription_accounts?: number
  current_subscription_accounts: number
  subscription_account_gap?: number
  subscription_account_surplus?: number
  sample_accounts: number
  sample_account_days: number
  confidence: SupplyForecastConfidence
  subscription_unavailable_reason?: string
  metered_share: number
  metered_cost_ratio?: number
  metered_procurement_budget?: number
  metered_unavailable_reason?: string
}

export interface SupplyForecastResponse {
  generated_at: string
  history_start: string
  history_end: string
  timezone: string
  horizon_days: number
  safety_margin: number
  spendable_balance: number
  frozen_balance: number
  eligible_users: number
  daily_burn_7: number
  daily_burn_30: number
  base_daily_demand: number
  planning_daily_demand: number
  projected_consumption: number
  planning_consumption: number
  runway_days?: number
  available: boolean
  unavailable_reason?: string
  platforms: PlatformSupplyForecast[]
}

export interface UpsertCostConfigRequest {
  cost_type?: 'subscription' | 'metered'
  period_fee: number
  period_days: number
  period_start_at?: string | null
  currency: string
  window_baseline_revenue: number | null
  notes: string
}

function rangeParams(startDate: string, endDate: string) {
  return {
    start_date: startDate,
    end_date: endDate,
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone
  }
}

/** GET /api/v1/admin/profit/overview */
export async function overview(startDate: string, endDate: string, refresh = false): Promise<ProfitOverviewResponse> {
  const { data } = await apiClient.get<ProfitOverviewResponse>('/admin/profit/overview', {
    params: { ...rangeParams(startDate, endDate), ...(refresh ? { refresh: true } : {}) }
  })
  return data
}

export async function supplyForecast(horizonDays = 30, safetyMargin = 0.2, refresh = false): Promise<SupplyForecastResponse> {
  const { data } = await apiClient.get<SupplyForecastResponse>('/admin/profit/supply-forecast', {
    params: {
      horizon_days: horizonDays,
      safety_margin: safetyMargin,
      timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
      ...(refresh ? { refresh: true } : {})
    }
  })
  return data
}

/** GET /api/v1/admin/profit/summary */
export async function summary(startDate: string, endDate: string, accountID?: number): Promise<ProfitSummaryResponse> {
	const { data } = await apiClient.get<ProfitSummaryResponse>('/admin/profit/summary', {
		params: { ...rangeParams(startDate, endDate), ...(accountID ? { account_id: accountID } : {}) }
	})
  return data
}

/** GET /api/v1/admin/profit/trend */
export async function trend(startDate: string, endDate: string, accountID?: number): Promise<ProfitTrendPoint[]> {
  const { data } = await apiClient.get<{ points: ProfitTrendPoint[] }>('/admin/profit/trend', {
    params: { ...rangeParams(startDate, endDate), ...(accountID ? { account_id: accountID } : {}) }
  })
  return data.points || []
}

/** GET /api/v1/admin/profit/configs */
export async function listConfigs(): Promise<AccountCostConfig[]> {
  const { data } = await apiClient.get<AccountCostConfig[]>('/admin/profit/configs')
  return data || []
}

/** PUT /api/v1/admin/profit/configs/:account_id */
export async function upsertConfig(accountID: number, req: UpsertCostConfigRequest): Promise<AccountCostConfig> {
  const { data } = await apiClient.put<AccountCostConfig>(`/admin/profit/configs/${accountID}`, req)
  return data
}

/** DELETE /api/v1/admin/profit/configs/:account_id */
export async function deleteConfig(accountID: number): Promise<void> {
  await apiClient.delete(`/admin/profit/configs/${accountID}`)
}

export async function listSubscriptionCycles(accountID: number): Promise<SubscriptionCycleListResponse> {
  const { data } = await apiClient.get<SubscriptionCycleListResponse>(`/admin/profit/configs/${accountID}/cycles`)
  return data
}

export async function createSubscriptionCycle(accountID: number, cycle: Pick<AccountSubscriptionCycle, 'starts_at' | 'period_fee' | 'period_days' | 'currency' | 'notes'>): Promise<AccountSubscriptionCycle> {
  const { data } = await apiClient.post<AccountSubscriptionCycle>(`/admin/profit/configs/${accountID}/cycles`, cycle)
  return data
}

export async function deleteSubscriptionCycle(id: number): Promise<void> {
  await apiClient.delete(`/admin/profit/cycles/${id}`)
}

export interface BatchSubscriptionConfigRequest {
  period_fee: number
  period_days?: number
  currency?: string
}

export interface BatchSubscriptionConfigResult {
  updated: number
  account_ids: number[]
}

/** POST /api/v1/admin/profit/configs/batch 批量为未配置的订阅类（OAuth）账号绑定订阅费用 */
export async function batchConfigureSubscription(req: BatchSubscriptionConfigRequest): Promise<BatchSubscriptionConfigResult> {
  const { data } = await apiClient.post<BatchSubscriptionConfigResult>('/admin/profit/configs/batch', req)
  return data
}

export default { overview, supplyForecast, summary, trend, listConfigs, upsertConfig, deleteConfig, listSubscriptionCycles, createSubscriptionCycle, deleteSubscriptionCycle, batchConfigureSubscription }
