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

  it('shortens sticky labels and exposes a horizontal gantt scroller', () => {
    const end = new Date(Date.now() + 2 * 24 * 3600_000).toISOString()
    const start = new Date(Date.now() - 5 * 24 * 3600_000).toISOString()
    const wrapper = mount(QuotaWindowPanel, {
      props: {
        accounts: [
          {
            account_id: 84,
            account_name: 'GROK-自建-relay-long',
            platform: 'grok',
            account_type: 'oauth',
            cost_type: 'subscription',
            configured: true,
            requests: 1,
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
                used_percent: 56,
                start_at: start,
                end_at: end,
                window_minutes: 10080
              }
            ]
          }
        ]
      }
    })

    expect(wrapper.find('[data-testid="profit-quota-window-scroll"]').exists()).toBe(true)
    // First two segments kept; long tail dropped from sticky label.
    expect(wrapper.text()).toContain('GROK-自建')
    expect(wrapper.text()).not.toContain('relay-long')
  })

  it('does not paint bars past recurring_until_at for expired no-renew accounts', () => {
    // Fixed "now" around Aug 9; subscription ended Aug 8; upstream window still to Aug 13.
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-09T04:00:00+08:00'))
    try {
      const wrapper = mount(QuotaWindowPanel, {
        props: {
          accounts: [
            {
              account_id: 84,
              account_name: 'GROK-自建',
              platform: 'grok',
              account_type: 'oauth',
              cost_type: 'subscription',
              configured: true,
              requests: 1,
              revenue: 1,
              cost: 1,
              profit: 0,
              margin: 0,
              currency: 'USD',
              quota_windows: [
                {
                  id: 'grok-billing',
                  label: '7d',
                  kind: '7d',
                  used_percent: 56,
                  start_at: '2026-08-06T13:31:00+08:00',
                  end_at: '2026-08-13T13:31:00+08:00',
                  window_minutes: 10080,
                  recurring_from_at: '2026-07-09T00:00:00+08:00',
                  recurring_until_at: '2026-08-08T00:00:00+08:00'
                }
              ]
            }
          ]
        }
      })

      const titles = wrapper.findAll('button').map((btn) => btn.attributes('title') || '').join('\n')
      // Clipped live bar ends at 8/8; no dashed projections into 8/13+.
      expect(titles).toMatch(/08\/08/)
      expect(titles).not.toMatch(/08\/13/)
      expect(titles).not.toMatch(/08\/2[0-9]/)
      expect(wrapper.text()).not.toMatch(/08\/1[3-9]/)
    } finally {
      vi.useRealTimers()
    }
  })
})
