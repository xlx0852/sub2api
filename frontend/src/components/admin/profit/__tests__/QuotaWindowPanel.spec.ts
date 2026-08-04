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
})
