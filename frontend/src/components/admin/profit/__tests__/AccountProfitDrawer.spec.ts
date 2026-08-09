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

vi.mock('@/components/common/LoadingSpinner.vue', () => ({
  default: { template: '<div data-testid="spinner" />' }
}))

const baseAccount = {
  account_id: 69,
  account_name: 'GPT-自有-1',
  platform: 'openai',
  account_type: 'oauth',
  cost_type: 'subscription' as const,
  configured: true,
  requests: 34000,
  revenue: 9999,
  cost: 999,
  profit: 9000,
  margin: 90,
  currency: 'USD'
}

describe('AccountProfitDrawer', () => {
  it('shows selected window economics instead of page-range totals', () => {
    const wrapper = mount(AccountProfitDrawer, {
      props: {
        show: true,
        account: baseAccount,
        selectedWindow: {
          start_at: '2026-08-07T10:09:00+08:00',
          end_at: '2026-08-14T10:09:00+08:00',
          kind: '7d',
          status: 'current',
          requests: 1200,
          revenue: 331.13,
          cost: 65.56,
          profit: 265.57
        },
        windowHistory: [
          {
            start_at: '2026-08-14T10:09:00+08:00',
            end_at: '2026-08-21T10:09:00+08:00',
            kind: '7d',
            status: 'upcoming',
            requests: 0,
            revenue: 0,
            cost: 49,
            profit: -49
          },
          {
            start_at: '2026-08-07T10:09:00+08:00',
            end_at: '2026-08-14T10:09:00+08:00',
            kind: '7d',
            status: 'current',
            requests: 1200,
            revenue: 331.13,
            cost: 65.56,
            profit: 265.57
          },
          {
            start_at: '2026-07-31T10:09:00+08:00',
            end_at: '2026-08-07T10:09:00+08:00',
            kind: '7d',
            status: 'ended',
            requests: 800,
            revenue: 120,
            cost: 49,
            profit: 71
          }
        ]
      }
    })

    const text = wrapper.text()
    expect(text).toContain('331.13')
    expect(text).toContain('65.56')
    expect(text).toContain('265.57')
    expect(text).not.toContain('9999')
    expect(text).toContain('admin.profit.windowHistoryTitle')
    expect(text).toContain('120.00')
    expect(text).toContain('71.00')
  })

  it('emits select-window when a history row is clicked', async () => {
    const past = {
      start_at: '2026-07-31T10:09:00+08:00',
      end_at: '2026-08-07T10:09:00+08:00',
      kind: '7d',
      status: 'ended',
      requests: 800,
      revenue: 120,
      cost: 49,
      profit: 71
    }
    const wrapper = mount(AccountProfitDrawer, {
      props: {
        show: true,
        account: baseAccount,
        selectedWindow: {
          start_at: '2026-08-07T10:09:00+08:00',
          end_at: '2026-08-14T10:09:00+08:00',
          kind: '7d',
          status: 'current',
          requests: 1,
          revenue: 1,
          cost: 1,
          profit: 0
        },
        windowHistory: [past]
      }
    })
    const buttons = wrapper.findAll('button')
    expect(buttons.length).toBeGreaterThan(0)
    await buttons[0].trigger('click')
    expect(wrapper.emitted('select-window')?.[0]?.[0]).toMatchObject(past)
  })
})
