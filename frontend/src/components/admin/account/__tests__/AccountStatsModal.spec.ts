import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountStatsModal from '../AccountStatsModal.vue'
import type { Account, AccountUsageStatsResponse } from '@/types'

const { getStats } = vi.hoisted(() => ({ getStats: vi.fn() }))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: { getStats }
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
  })

  it('首次挂载时立即加载当前账号统计', async () => {
    getStats.mockResolvedValue(stats)

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
    expect(wrapper.text()).toContain('42')
    expect(wrapper.text()).not.toContain('admin.accounts.stats.noData')
  })
})
