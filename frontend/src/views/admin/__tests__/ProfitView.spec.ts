import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ProfitView from '../ProfitView.vue'

const { overview, showError } = vi.hoisted(() => ({
  overview: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { profit: { overview } }
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
  PointElement: {},
  LineElement: {},
  Tooltip: {},
  Legend: {},
  Filler: {}
}))

vi.mock('vue-chartjs', () => ({
  Line: { template: '<div data-testid="profit-chart" />' }
}))

describe('ProfitView', () => {
  beforeEach(() => {
    overview.mockReset()
    showError.mockReset()
  })

  it('在全局趋势下方展示与汇总日期一致的账号明细', async () => {
    overview.mockResolvedValue({
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
          currency: 'USD'
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
        }
        ]
      },
      points: [{ date: '2026-07-22', revenue: 180, cost: 75, profit: 105 }]
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
          Icon: true
        }
      }
    })
    await flushPromises()

    expect(overview).toHaveBeenCalledTimes(1)
    expect(overview).toHaveBeenCalledWith(expect.any(String), expect.any(String))
    expect(wrapper.text()).toContain('admin.profit.accountDetails')
    expect(wrapper.find('[data-transparent="true"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('GPT-自有 1')
    expect(wrapper.text()).toContain('GROK-外接')
    expect(wrapper.text()).toContain('12,345')
    expect(wrapper.text()).toContain('$50.00')
    expect(wrapper.text()).toContain('+$70.00')
    expect(wrapper.findAll('[data-testid="account-profit-item"]')).toHaveLength(2)
    expect(wrapper.findAll('[data-testid="revenue-bar"]')).toHaveLength(2)
    expect(wrapper.findAll('[data-testid="cost-bar"]')).toHaveLength(2)
    expect(wrapper.find('[data-testid="revenue-bar"]').attributes('style')).toContain('width: 100%')
  })
})
