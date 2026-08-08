/**
 * Admin Profit API endpoints
 * 账号利润分析：成本绑定、利润汇总、趋势
 */

import { apiClient } from '../client'

export interface AccountCostConfig {
  auto_renew?: boolean
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
  termination?: AccountSubscriptionTermination
  refunds?: AccountSubscriptionRefund[]
  loss_summary?: AccountSubscriptionLossSummary
  created_at: string
  updated_at: string
}

export interface AccountSubscriptionTermination {
  id: number
  cycle_id: number
  account_id: number
  effective_at: string
  reason: string
  notes: string
  reversed_at?: string
  reversal_reason?: string
  created_at: string
  updated_at: string
}

export interface AccountSubscriptionRefund {
  id: number
  termination_id: number
  cycle_id: number
  account_id: number
  amount: number
  currency: string
  received_at: string
  notes: string
  voided_at?: string
  void_reason?: string
  created_at: string
  updated_at: string
}

export interface AccountSubscriptionLossSummary {
  purchase_cost: number
  revenue_before_ban: number
  refund_total: number
  net_purchase_cost: number
  recovered_amount: number
  recovery_progress: number
  realized_profit: number
  realized_loss: number
}

export interface SubscriptionCycleSettlementResult {
  cycle: AccountSubscriptionCycle
  disabled_account_ids?: number[]
}

export interface SubscriptionCycleListResponse {
  cycles: AccountSubscriptionCycle[]
  auto_renew?: boolean
  subscription_expires_at?: string
  oauth_token_expires_at?: string
  account_expires_at?: string | null
}

export interface ProfitQuotaWindow {
  id: string
  label: string
  kind: '5h' | '7d' | '24h' | 'session' | 'other' | string
  used_percent?: number
  start_at?: string
  end_at?: string
  window_minutes?: number
  recurring_until_at?: string
  recurring_from_at?: string
}

export interface AccountProfitSummary {
  account_id: number
  account_name: string
  platform: string
  account_type: string
  cost_type: 'subscription' | 'metered'
  configured: boolean
  deleted?: boolean
  requests: number
  revenue: number
  cost: number
  profit: number
  margin: number
  five_hour_utilization?: number
  seven_day_utilization?: number
  quota_windows?: ProfitQuotaWindow[]
  window_efficiency?: number
  window_baseline_source?: string
  billing_window_start?: string
  billing_window_end?: string
  billing_window_progress?: number
  billing_window_revenue?: number
  billing_window_cost?: number
  billing_window_profit?: number
  billing_window_source?: 'cycle' | 'quota_window' | 'manual' | 'subscription_expiry' | string
  billing_window_kind?: string
  billing_window_requests?: number
  billing_window_terminated_at?: string
  billing_window_termination_reason?: string
  billing_window_original_cost?: number
  billing_window_refund_total?: number
  billing_window_recovered_amount?: number
  billing_window_recovery_progress?: number
  billing_window_loss?: number
  requires_cycle_start?: boolean
  // 订阅 OAuth 账号：最低保本售卖倍率（按当前额度窗外推满负荷）
  break_even_rate?: number
  break_even_window_kind?: string
  break_even_window_minutes?: number
  break_even_used_percent?: number
  break_even_full_window_revenue?: number
  break_even_windows_per_period?: number
  break_even_capacity_revenue?: number
  break_even_current_rate?: number
  break_even_period_fee?: number
  break_even_period_days?: number
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

export interface ProfitTrendAccountSlice {
  account_id: number
  account_name: string
  revenue: number
  cost: number
  profit: number
}

export interface ProfitTrendPoint {
  date: string
  revenue: number
  cost: number
  profit: number
  /** Per-account contribution for stacked bar composition. */
  accounts?: ProfitTrendAccountSlice[]
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
  // 额度驱动的订阅号供给（账号自身额度视角）
  quota_accounts: number
  quota_remaining_pct?: number
  quota_exhausted: boolean
  quota_snapshot_stale: boolean
  account_daily_capacity_quota?: number
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
  auto_renew?: boolean
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

/** PUT /api/v1/admin/profit/configs/:account_id/auto-renew */
export async function setSubscriptionAutoRenew(accountID: number, auto_renew: boolean): Promise<AccountCostConfig> {
  const { data } = await apiClient.put<AccountCostConfig>(`/admin/profit/configs/${accountID}/auto-renew`, { auto_renew })
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

export interface CreateSubscriptionTerminationRequest {
  effective_at: string
  reason: string
  notes: string
  initial_refund_amount?: number
  initial_refund_received_at?: string
}

export async function terminateSubscriptionCycle(id: number, req: CreateSubscriptionTerminationRequest): Promise<SubscriptionCycleSettlementResult> {
  const { data } = await apiClient.post<SubscriptionCycleSettlementResult>(`/admin/profit/cycles/${id}/termination`, req)
  return data
}

export async function previewSubscriptionTermination(id: number, req: CreateSubscriptionTerminationRequest): Promise<AccountSubscriptionLossSummary> {
  const { data } = await apiClient.post<AccountSubscriptionLossSummary>(`/admin/profit/cycles/${id}/termination-preview`, req)
  return data
}

export async function addSubscriptionRefund(terminationID: number, req: { amount: number; received_at: string; notes: string }): Promise<SubscriptionCycleSettlementResult> {
  const { data } = await apiClient.post<SubscriptionCycleSettlementResult>(`/admin/profit/terminations/${terminationID}/refunds`, req)
  return data
}

export async function voidSubscriptionRefund(id: number, reason: string): Promise<SubscriptionCycleSettlementResult> {
  const { data } = await apiClient.post<SubscriptionCycleSettlementResult>(`/admin/profit/refunds/${id}/void`, { reason })
  return data
}

export async function reverseSubscriptionTermination(id: number, reason: string): Promise<SubscriptionCycleSettlementResult> {
  const { data } = await apiClient.post<SubscriptionCycleSettlementResult>(`/admin/profit/terminations/${id}/reverse`, { reason })
  return data
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

export default {
  overview, supplyForecast, summary, trend, listConfigs, upsertConfig, setSubscriptionAutoRenew, deleteConfig,
  listSubscriptionCycles, createSubscriptionCycle, deleteSubscriptionCycle,
  previewSubscriptionTermination, terminateSubscriptionCycle, addSubscriptionRefund, voidSubscriptionRefund,
  reverseSubscriptionTermination, batchConfigureSubscription
}
