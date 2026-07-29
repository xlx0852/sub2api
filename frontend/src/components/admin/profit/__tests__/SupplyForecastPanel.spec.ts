import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SupplyForecastPanel from '../SupplyForecastPanel.vue'

const { supplyForecast, showError } = vi.hoisted(() => ({
  supplyForecast: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { profit: { supplyForecast } }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => `${key}${params ? JSON.stringify(params) : ''}` })
  }
})

const response = {
  generated_at: '2026-07-29T08:00:00Z',
  history_start: '2026-06-30T00:00:00Z',
  history_end: '2026-07-30T00:00:00Z',
  timezone: 'UTC',
  horizon_days: 30,
  safety_margin: 0.2,
  spendable_balance: 1234.5,
  frozen_balance: 10,
  eligible_users: 8,
  daily_burn_7: 40,
  daily_burn_30: 30,
  base_daily_demand: 40,
  planning_daily_demand: 48,
  projected_consumption: 1200,
  planning_consumption: 1234.5,
  runway_days: 30.9,
  available: true,
  platforms: [{
    platform: 'openai',
    demand_share: 1,
    projected_consumption: 1200,
    planning_consumption: 1234.5,
    subscription_share: 0.75,
    subscription_planning_daily: 36,
    account_daily_capacity_p75: 12,
    required_subscription_accounts: 3,
    current_subscription_accounts: 2,
    subscription_account_gap: 1,
    subscription_account_surplus: 0,
    sample_accounts: 4,
    sample_account_days: 40,
    confidence: 'medium' as const,
    metered_share: 0.25,
    metered_cost_ratio: 0.1,
    metered_procurement_budget: 30.86
  }]
}

describe('SupplyForecastPanel', () => {
  beforeEach(() => {
    supplyForecast.mockReset()
    showError.mockReset()
    supplyForecast.mockResolvedValue(response)
  })

  it('展示储值、可支撑天数以及订阅账号缺口和按量预算', async () => {
    const wrapper = mount(SupplyForecastPanel, {
      global: { stubs: { Icon: true, LoadingSpinner: true } }
    })
    await flushPromises()

    expect(supplyForecast).toHaveBeenCalledWith(30, 0.2, false)
    expect(wrapper.text()).toContain('$1,234.50')
    expect(wrapper.text()).toContain('30.9')
    expect(wrapper.text()).toContain('openai')
    expect(wrapper.text()).toContain('$30.86')
    expect(wrapper.findAll('[data-testid="platform-forecast"]')).toHaveLength(1)

    await wrapper.find('[data-testid="forecast-refresh"]').trigger('click')
    await flushPromises()
    expect(supplyForecast).toHaveBeenLastCalledWith(30, 0.2, true)

    await wrapper.find('[data-testid="forecast-horizon"]').setValue('60')
    await flushPromises()
    expect(supplyForecast).toHaveBeenLastCalledWith(60, 0.2, false)
  })
})
