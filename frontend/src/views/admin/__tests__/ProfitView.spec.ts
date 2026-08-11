import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ProfitView from '../ProfitView.vue'

const { overview, summary, windowEconomics, showError } = vi.hoisted(() => ({
  overview: vi.fn(),
  summary: vi.fn(),
  windowEconomics: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { profit: { overview, summary, windowEconomics } }
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
    summary.mockReset()
    windowEconomics.mockReset()
    showError.mockReset()
    windowEconomics.mockResolvedValue({ account_id: 1, account_name: 'x', cost_type: 'subscription', windows: [] })
    summary.mockResolvedValue({ accounts: [] })
  })

  it('在全局趋势下方展示与汇总日期一致的账号明细', async () => {
    overview.mockResolvedValue({
      generated_at: '2026-07-28T10:30:00Z',
      current_user_balance: 4321.09,
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
          QuotaWindowPanel: { template: '<div data-testid="profit-quota-window-panel" />' },
          AccountProfitDrawer: { template: '<div data-testid="profit-account-drawer" />' }
        }
      }
    })
    await flushPromises()

    expect(overview).toHaveBeenCalledTimes(1)
    expect(overview).toHaveBeenCalledWith(expect.any(String), expect.any(String), false)
    expect(wrapper.find('[data-testid="profit-snapshot-time"]').exists()).toBe(true)
    expect(wrapper.find('[data-transparent="true"]').exists()).toBe(true)
    // 账号明细已改为点配额窗口条 → 抽屉展示；不再内联渲染。
    expect(wrapper.text()).not.toContain('admin.profit.accountDetails')
    expect(wrapper.find('[data-testid="profit-quota-window-panel"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="profit-trend-chart"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="profit-chart"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="profit-current-user-balance"]').text()).toContain('$4321.09')
    expect(wrapper.findAll('[data-testid="account-profit-item"]')).toHaveLength(0)

    await wrapper.find('[data-testid="profit-refresh"]').trigger('click')
    await flushPromises()
    expect(overview).toHaveBeenCalledTimes(2)
    expect(overview).toHaveBeenLastCalledWith(expect.any(String), expect.any(String), true)
  })
})
