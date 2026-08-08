import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ProfitView from '../ProfitView.vue'

const { overview, supplyForecast, showError } = vi.hoisted(() => ({
  overview: vi.fn(),
  supplyForecast: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { profit: { overview, supplyForecast } }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

vi.mock('chart.js', () => ({
  Chart: { register: vi.fn() },
  CategoryScale: {},
  LinearScale: {},
  BarElement: {},
  Tooltip: {},
  Legend: {}
}))

vi.mock('vue-chartjs', () => ({
  Bar: { template: '<div data-testid="profit-chart" />' }
}))

describe('ProfitView', () => {
  beforeEach(() => {
    overview.mockReset()
    supplyForecast.mockReset()
    showError.mockReset()
  })

  it('在全局趋势下方展示与汇总日期一致的账号明细', async () => {
    overview.mockResolvedValue({
      generated_at: '2026-07-28T10:30:00Z',
      summary: {
        start: '2026-07-22',
        end: '2026-07-28',
        total_revenue: 180,
        total_cost: 75,
        total_profit: 105,
        accounts: [
        {
          account_id: 69,
          account_name: 'GPT-自有 1',
          platform: 'openai',
          account_type: 'oauth',
          cost_type: 'subscription',
          configured: true,
          requests: 12345,
          revenue: 120,
          cost: 50,
          profit: 70,
          margin: 58.3,
          currency: 'USD',
          seven_day_utilization: 46,
          quota_windows: [{
            id: '7d',
            label: '7d',
            kind: '7d',
            used_percent: 46,
            start_at: '2026-07-30T10:33:00Z',
            end_at: '2026-08-06T10:33:00Z',
            window_minutes: 10080
          }]
        },
        {
          account_id: 90,
          account_name: 'GROK-外接',
          platform: 'grok',
          account_type: 'apikey',
          cost_type: 'metered',
          configured: false,
          requests: 200,
          revenue: 60,
          cost: 25,
          profit: 35,
          margin: 58.3,
          currency: 'USD'
        },
        {
          account_id: 91,
          account_name: '无活动账号',
          platform: 'openai',
          account_type: 'apikey',
          cost_type: 'metered',
          configured: true,
          requests: 0,
          revenue: 0,
          cost: 0,
          profit: 0,
          margin: 0,
          currency: 'USD'
        }
        ]
      },
      points: [{
        date: '2026-07-22',
        revenue: 180,
        cost: 75,
        profit: 105,
        accounts: [
          { account_id: 69, account_name: 'GPT-自有 1', revenue: 120, cost: 50, profit: 70 },
          { account_id: 90, account_name: 'GROK-外接', revenue: 60, cost: 25, profit: 35 }
        ]
      }]
    })
    supplyForecast.mockResolvedValue({
      generated_at: '2026-07-29T08:00:00Z',
      history_start: '2026-06-30T00:00:00Z',
      history_end: '2026-07-30T00:00:00Z',
      timezone: 'UTC',
      horizon_days: 30,
      safety_margin: 0.2,
      spendable_balance: 1000,
      frozen_balance: 50,
      eligible_users: 3,
      daily_burn_7: 20,
      daily_burn_30: 15,
      base_daily_demand: 20,
      planning_daily_demand: 24,
      projected_consumption: 600,
      planning_consumption: 720,
      runway_days: 50,
      available: true,
      platforms: []
    })

    const wrapper = mount(ProfitView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            props: { transparent: Boolean },
            template: '<div :data-transparent="String(transparent)"><slot name="filters" /><slot name="table" /></div>'
          },
          LoadingSpinner: true,
          Icon: true,
          QuotaWindowPanel: { template: '<div data-testid="profit-quota-window-panel" />' }
        }
      }
    })
    await flushPromises()

    expect(overview).toHaveBeenCalledTimes(1)
    expect(supplyForecast).not.toHaveBeenCalled()
    expect(overview).toHaveBeenCalledWith(expect.any(String), expect.any(String), false)
    expect(wrapper.find('[data-testid="profit-snapshot-time"]').exists()).toBe(true)
    expect(wrapper.find('[data-transparent="true"]').exists()).toBe(true)
    // 账号明细已改为点配额窗口条 → 抽屉展示；不再内联渲染。
    expect(wrapper.text()).not.toContain('admin.profit.accountDetails')
    expect(wrapper.find('[data-testid="profit-quota-window-panel"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="profit-trend-chart"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="profit-chart"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="account-profit-item"]')).toHaveLength(0)

    await wrapper.find('[data-testid="profit-refresh"]').trigger('click')
    await flushPromises()
    expect(overview).toHaveBeenCalledTimes(2)
    expect(overview).toHaveBeenLastCalledWith(expect.any(String), expect.any(String), true)

    await wrapper.find('[data-testid="profit-forecast-tab"]').trigger('click')
    await flushPromises()
    expect(supplyForecast).toHaveBeenCalledTimes(1)
    expect(supplyForecast).toHaveBeenCalledWith(30, 0.2, false)
  })
})
