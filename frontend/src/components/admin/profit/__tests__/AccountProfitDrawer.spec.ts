import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AccountProfitDrawer from '../AccountProfitDrawer.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => {
        if (!params) return key
        return `${key}:${Object.values(params).join(',')}`
      }
    })
  }
})

vi.mock('@/components/common/BaseDialog.vue', () => ({
  default: {
    props: ['show', 'title'],
    template: '<div data-testid="drawer"><slot /></div>'
  }
}))

describe('AccountProfitDrawer', () => {
  it('prefers quota-window economics over page-range revenue/cost', () => {
    const wrapper = mount(AccountProfitDrawer, {
      props: {
        show: true,
        account: {
          account_id: 69,
          account_name: 'GPT-自有-1',
          platform: 'openai',
          account_type: 'oauth',
          cost_type: 'subscription',
          configured: true,
          requests: 34000,
          revenue: 9999,
          cost: 999,
          profit: 9000,
          margin: 90,
          currency: 'USD',
          billing_window_source: 'quota_window',
          billing_window_kind: '7d',
          billing_window_start: '2026-08-07T10:09:00+08:00',
          billing_window_end: '2026-08-14T10:09:00+08:00',
          billing_window_revenue: 331.13,
          billing_window_cost: 65.56,
          billing_window_profit: 265.57,
          billing_window_requests: 1200
        }
      }
    })

    const text = wrapper.text()
    expect(text).toContain('331.13')
    expect(text).toContain('65.56')
    expect(text).toContain('265.57')
    expect(text).not.toContain('9999')
    expect(text).toContain('1,200')
    expect(text).toContain('admin.profit.drawerQuotaHint')
    expect(text).toContain('admin.profit.drawerQuotaWindow:7d')
  })
})
