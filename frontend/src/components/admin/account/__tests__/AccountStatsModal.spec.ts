import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountStatsModal from '../AccountStatsModal.vue'
import type { Account, AccountUsageStatsResponse } from '@/types'

const { getStats, profitSummary } = vi.hoisted(() => ({ getStats: vi.fn(), profitSummary: vi.fn() }))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: { getStats },
    profit: { summary: profitSummary }
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const account = {
  id: 72,
  name: 'OpenAI Account',
  platform: 'openai',
  type: 'oauth',
  status: 'active'
} as Account

const stats = {
  history: [],
  models: [],
  endpoints: [],
  upstream_endpoints: [],
  summary: {
    days: 30,
    actual_days_used: 1,
    total_cost: 1,
    total_user_cost: 1,
    total_standard_cost: 1,
    total_requests: 42,
    total_tokens: 1000,
    avg_daily_cost: 1,
    avg_daily_user_cost: 1,
    avg_daily_requests: 42,
    avg_daily_tokens: 1000,
    avg_duration_ms: 10
  }
} as AccountUsageStatsResponse

describe('AccountStatsModal', () => {
  beforeEach(() => {
    getStats.mockReset()
    profitSummary.mockReset()
  })

  it('首次挂载时立即加载当前账号统计', async () => {
    getStats.mockResolvedValue(stats)
    profitSummary.mockResolvedValue({
      accounts: [{
        account_id: 72,
        account_name: 'OpenAI Account',
        cost_type: 'subscription',
        configured: true,
        requests: 42,
        revenue: 12,
        cost: 3,
        profit: 9,
        margin: 75,
        billing_window_start: '2026-07-01T00:00:00Z',
        billing_window_end: '2026-07-31T00:00:00Z',
        billing_window_revenue: 1500,
        billing_window_cost: 1200,
        billing_window_profit: 300,
        currency: 'USD'
      }]
    })

    const wrapper = mount(AccountStatsModal, {
      props: { account },
      global: {
        stubs: {
          LoadingSpinner: true,
          ModelDistributionChart: true,
          EndpointDistributionChart: true,
          Line: true,
          Icon: true
        }
      }
    })
    await flushPromises()

    expect(getStats).toHaveBeenCalledOnce()
    expect(getStats).toHaveBeenCalledWith(72, 30)
    expect(profitSummary).toHaveBeenCalledWith(expect.any(String), expect.any(String), 72)
    expect(wrapper.text()).toContain('42')
    expect(wrapper.text()).toContain('admin.profit.currentCyclePurchaseCost')
    expect(wrapper.text()).toContain('1.20K')
    expect(wrapper.text()).toContain('300.00')
    expect(wrapper.text()).not.toContain('admin.accounts.stats.noData')
  })

  it('订阅账号没有有效周期时不把期间摊销额当成采购成本', async () => {
    getStats.mockResolvedValue(stats)
    profitSummary.mockResolvedValue({
      accounts: [{
        account_id: 72,
        account_name: 'OpenAI Account',
        cost_type: 'subscription',
        configured: false,
        requests: 42,
        revenue: 12,
        cost: 3,
        profit: 9,
        margin: 75,
        currency: 'USD'
      }]
    })

    const wrapper = mount(AccountStatsModal, {
      props: { account },
      global: {
        stubs: {
          LoadingSpinner: true,
          ModelDistributionChart: true,
          EndpointDistributionChart: true,
          Line: true,
          Icon: true
        }
      }
    })
    await flushPromises()

    expect(wrapper.text()).toContain('admin.accounts.usageDetails.missingCycleHint')
    expect(wrapper.text()).not.toContain('$3.00')
  })

  it('订阅周期采购成本为零时仍显示为有效周期', async () => {
    getStats.mockResolvedValue(stats)
    profitSummary.mockResolvedValue({
      accounts: [{
        account_id: 72,
        account_name: 'OpenAI Account',
        cost_type: 'subscription',
        configured: true,
        requests: 42,
        revenue: 12,
        cost: 0,
        profit: 12,
        margin: 100,
        billing_window_start: '2026-07-10T00:00:00Z',
        billing_window_end: '2026-08-09T00:00:00Z',
        billing_window_revenue: 12,
        billing_window_cost: 0,
        billing_window_profit: 12,
        currency: 'USD'
      }]
    })

    const wrapper = mount(AccountStatsModal, {
      props: { account },
      global: {
        stubs: {
          LoadingSpinner: true,
          ModelDistributionChart: true,
          EndpointDistributionChart: true,
          Line: true,
          Icon: true
        }
      }
    })
    await flushPromises()

    expect(wrapper.text()).toContain('admin.profit.currentCyclePurchaseCost')
    expect(wrapper.text()).toContain('$0.0000')
    expect(wrapper.text()).toContain('+$12.00')
    expect(wrapper.text()).not.toContain('admin.accounts.usageDetails.missingCycleHint')
  })
})
