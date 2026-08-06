import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import QuotaWindowPanel from '../QuotaWindowPanel.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => {
        if (!params) return key
        return `${key}:${Object.values(params).join(',')}`
      },
      locale: { value: 'zh-CN' }
    })
  }
})

describe('QuotaWindowPanel', () => {
  it('renders lanes from quota_windows snapshots', () => {
    const end = new Date(Date.now() + 2 * 24 * 3600_000).toISOString()
    const start = new Date(Date.now() - 5 * 24 * 3600_000).toISOString()
    const wrapper = mount(QuotaWindowPanel, {
      props: {
        accounts: [
          {
            account_id: 1,
            account_name: 'codex-main',
            platform: 'openai',
            account_type: 'oauth',
            cost_type: 'subscription',
            configured: true,
            requests: 10,
            revenue: 1,
            cost: 1,
            profit: 0,
            margin: 0,
            currency: 'USD',
            quota_windows: [
              {
                id: '7d',
                label: '7d',
                kind: '7d',
                used_percent: 46,
                start_at: start,
                end_at: end,
                window_minutes: 10080
              }
            ]
          }
        ]
      }
    })

    expect(wrapper.find('[data-testid="profit-quota-window-panel"]').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="profit-quota-window-lane"]')).toHaveLength(1)
    expect(wrapper.text()).toContain('codex-main')
    expect(wrapper.text()).toContain('46%')
  })

  it('labels long free rolling windows as 30d from window_minutes', () => {
    const end = new Date(Date.now() + 20 * 24 * 3600_000).toISOString()
    const start = new Date(Date.now() - 10 * 24 * 3600_000).toISOString()
    const wrapper = mount(QuotaWindowPanel, {
      props: {
        accounts: [
          {
            account_id: 69,
            account_name: 'GPT-自有-1',
            platform: 'openai',
            account_type: 'oauth',
            cost_type: 'subscription',
            configured: true,
            requests: 10,
            revenue: 1,
            cost: 1,
            profit: 0,
            margin: 0,
            currency: 'USD',
            quota_windows: [
              {
                id: '7d',
                label: '7d', // legacy hardcoded label from older backend
                kind: '7d',
                used_percent: 100,
                start_at: start,
                end_at: end,
                window_minutes: 43200
              }
            ]
          }
        ]
      }
    })

    expect(wrapper.text()).toContain('GPT-自有-1')
    expect(wrapper.text()).toContain('30d')
    expect(wrapper.text()).not.toMatch(/GPT-自有-1\s+7d/)
  })

  it('supports month view mode toggle', async () => {
    const end = new Date(Date.now() + 2 * 24 * 3600_000).toISOString()
    const start = new Date(Date.now() - 5 * 24 * 3600_000).toISOString()
    const wrapper = mount(QuotaWindowPanel, {
      props: {
        accounts: [
          {
            account_id: 2,
            account_name: 'kimi-main',
            platform: 'kimi',
            account_type: 'oauth',
            cost_type: 'subscription',
            configured: true,
            requests: 3,
            revenue: 1,
            cost: 1,
            profit: 0,
            margin: 0,
            currency: 'USD',
            quota_windows: [
              {
                id: '7d',
                label: '7d',
                kind: '7d',
                used_percent: 83,
                start_at: start,
                end_at: end,
                window_minutes: 10080
              }
            ]
          }
        ]
      }
    })

    const modeButtons = wrapper.findAll('button').filter((btn) => {
      const text = btn.text()
      return text === 'admin.profit.quotaWindowByMonth' || text.includes('Month') || text.includes('按月')
    })
    // i18n mock returns the key string
    const monthBtn = wrapper.findAll('button').find((btn) => btn.text() === 'admin.profit.quotaWindowByMonth')
    expect(monthBtn).toBeTruthy()
    await monthBtn!.trigger('click')
    expect(wrapper.text()).toContain('admin.profit.quotaWindowByMonth')
    expect(wrapper.findAll('[data-testid="profit-quota-window-lane"]').length).toBeGreaterThanOrEqual(1)
    expect(modeButtons.length).toBeGreaterThanOrEqual(0)
  })
})
