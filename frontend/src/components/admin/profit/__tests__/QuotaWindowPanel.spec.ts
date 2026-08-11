import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import QuotaWindowPanel from '../QuotaWindowPanel.vue'
import { expandWindowOccurrences, prepareQuotaAccounts, windowsForDrawerSelection, windowsForRange } from '../quotaTimeline'

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
  it('centers today when the timeline appears after async account loading', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-09T12:00:00+08:00'))
    const scrollToDescriptor = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'scrollTo')
    Object.defineProperty(HTMLElement.prototype, 'scrollTo', {
      configurable: true,
      value(this: HTMLElement, options: ScrollToOptions) {
        this.scrollLeft = Number(options.left || 0)
      }
    })
    try {
      const wrapper = mount(QuotaWindowPanel, { props: { accounts: [] } })
      expect(wrapper.find('[data-testid="profit-quota-window-scroll"]').exists()).toBe(false)

      await wrapper.setProps({
        accounts: [{
          account_id: 1,
          account_name: 'async-account',
          platform: 'openai',
          account_type: 'oauth',
          cost_type: 'subscription',
          configured: true,
          requests: 1,
          revenue: 1,
          cost: 1,
          profit: 0,
          margin: 0,
          currency: 'USD',
          quota_windows: [{
            id: '7d', label: '7d', kind: '7d', used_percent: 10,
            start_at: '2026-08-08T00:00:00+08:00',
            end_at: '2026-08-15T00:00:00+08:00',
            window_minutes: 10080
          }]
        }]
      })
      await vi.advanceTimersByTimeAsync(20)

      const el = wrapper.find('[data-testid="profit-quota-window-scroll"]').element as HTMLElement
      expect(el.scrollLeft).toBeCloseTo((180 * 36 - 1024) / 2, 0)
    } finally {
      if (scrollToDescriptor) {
        Object.defineProperty(HTMLElement.prototype, 'scrollTo', scrollToDescriptor)
      } else {
        delete (HTMLElement.prototype as { scrollTo?: unknown }).scrollTo
      }
      vi.useRealTimers()
    }
  })

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
      const textAll = `${titles}\n${wrapper.text()}`
      // Clipped live bar ends at 8/8; no dashed projections into 8/13+.
      expect(textAll).toMatch(/08\/08/)
      expect(textAll).not.toMatch(/08\/13/)
      expect(textAll).not.toMatch(/08\/2[0-9]/)
      expect(textAll).not.toMatch(/08\/1[3-9]/)
    } finally {
      vi.useRealTimers()
    }
  })
})

describe('cross-month navigation', () => {
  it('renders continuous timeline with month markers and period buttons', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-09T12:00:00+08:00'))
    try {
      const end = new Date(Date.now() + 2 * 24 * 3600_000).toISOString()
      const start = new Date(Date.now() - 5 * 24 * 3600_000).toISOString()
      const wrapper = mount(QuotaWindowPanel, {
        props: {
          accounts: [{
            account_id: 1,
            account_name: 'GPT-A',
            platform: 'openai',
            account_type: 'oauth',
            cost_type: 'subscription',
            configured: true,
            requests: 1,
            revenue: 1,
            cost: 1,
            profit: 0,
            margin: 0,
            currency: 'USD',
            quota_windows: [{
              id: '7d', label: '7d', kind: '7d', used_percent: 50,
              start_at: start, end_at: end, window_minutes: 10080
            }]
          }]
        }
      })

      // continuous horizon covers ~±30d → month markers present
      expect(wrapper.findAll('[data-testid="quota-window-month-marker"]').length).toBeGreaterThanOrEqual(1)
      expect(wrapper.find('[data-testid="quota-window-axis-header"]').classes()).toContain('h-12')
      expect(wrapper.find('[data-testid="quota-window-month-marker"]').classes()).toContain('top-7')
      const minorTicks = wrapper.findAll('[data-testid="quota-window-axis-tick"][data-minor="true"]')
      expect(minorTicks.length).toBeGreaterThan(0)
      expect(minorTicks[0].text()).toMatch(/\d+/)
      expect(minorTicks[0].find('span:last-child').classes()).toContain('text-[8px]')
      // navigation buttons exist
      const nextBtn = wrapper.findAll('button').find((b) => b.attributes('title') === 'admin.profit.quotaWindowNext')
      const todayBtn = wrapper.findAll('button').find((b) => b.text() === 'admin.profit.quotaWindowToday')
      expect(nextBtn).toBeTruthy()
      expect(todayBtn).toBeTruthy()
      await nextBtn!.trigger('click')
      await todayBtn!.trigger('click')
    } finally {
      vi.useRealTimers()
    }
  })

  it('auto-shifts period when scrolled past the right edge', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-09T12:00:00+08:00'))
    try {
      const end = new Date(Date.now() + 2 * 24 * 3600_000).toISOString()
      const start = new Date(Date.now() - 5 * 24 * 3600_000).toISOString()
      const wrapper = mount(QuotaWindowPanel, {
        props: {
          accounts: [{
            account_id: 2,
            account_name: 'GROK-B',
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
            quota_windows: [{
              id: '7d', label: '7d', kind: '7d', used_percent: 60,
              start_at: start, end_at: end, window_minutes: 10080
            }]
          }]
        }
      })
      const scroller = wrapper.find('[data-testid="profit-quota-window-scroll"]')
      expect(scroller.exists()).toBe(true)
      const el = scroller.element as HTMLElement
      // jsdom has 0 sizes; stub metrics
      Object.defineProperty(el, 'clientWidth', { value: 500, configurable: true })
      Object.defineProperty(el, 'scrollWidth', { value: 2000, configurable: true })
      el.scrollLeft = 1500
      await scroller.trigger('scroll')
      // seamless scroll: no forced cross-month toast from hitting edge
      expect(wrapper.find('[data-testid="quota-window-cross-month"]').exists()).toBe(false)
    } finally {
      vi.useRealTimers()
    }
  })
})

describe('drifting vacuum state', () => {
  it('does not project a real ledger row into history but keeps future occurrences', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-09T12:00:00+08:00'))
    try {
      const wrapper = mount(QuotaWindowPanel, {
        props: {
          accounts: [{
            account_id: 69,
            account_name: 'GPT-自有-1',
            platform: 'openai',
            account_type: 'oauth',
            cost_type: 'subscription',
            configured: true,
            requests: 1,
            revenue: 1,
            cost: 1,
            profit: 0,
            margin: 0,
            currency: 'USD',
            quota_availability_spans: [{
              start_at: '2026-07-01T00:00:00+08:00',
              end_at: '2026-08-31T00:00:00+08:00'
            }],
            quota_windows: [{
              id: '7d-open', label: '7d', kind: '7d', used_percent: 9,
              start_at: '2026-08-09T09:41:26+08:00',
              end_at: '2026-08-16T09:41:38+08:00',
              window_minutes: 10080,
              recurring_from_at: '2026-07-01T00:00:00+08:00',
              recurring_until_at: '2026-08-31T00:00:00+08:00',
              source: 'observed',
              is_open: true
            }]
          }]
        }
      })

      const quotaBars = wrapper.findAll('button').filter((button) => (
        (button.attributes('title') || '').includes('→')
      ))
      expect(quotaBars.length).toBeGreaterThan(1)
      const currentBars = quotaBars.filter((bar) => (bar.attributes('title') || '').includes('current'))
      const upcomingBars = quotaBars.filter((bar) => (bar.attributes('title') || '').includes('upcoming'))
      expect(currentBars).toHaveLength(1)
      expect(currentBars[0].attributes('title')).toContain('08/09')
      expect(upcomingBars.length).toBeGreaterThan(0)
      expect(quotaBars.every((bar) => !(bar.attributes('title') || '').includes('07/'))).toBe(true)
    } finally {
      vi.useRealTimers()
    }
  })

  it('shows drifting style for expired open ledger row without new cycle', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-09T12:00:00+08:00'))
    try {
      const pastStart = new Date(Date.now() - 10 * 24 * 3600_000).toISOString()
      const pastEnd = new Date(Date.now() - 24 * 3600_000).toISOString()
      const wrapper = mount(QuotaWindowPanel, {
        props: {
          accounts: [{
            account_id: 97,
            account_name: 'GPT-外借-1',
            platform: 'openai',
            account_type: 'oauth',
            cost_type: 'subscription',
            configured: true,
            requests: 1,
            revenue: 1,
            cost: 1,
            profit: 0,
            margin: 0,
            currency: 'USD',
            quota_windows: [{
              id: '7d-open',
              label: '7d',
              kind: '7d',
              used_percent: 10,
              start_at: pastStart,
              end_at: pastEnd,
              window_minutes: 10080,
              source: 'observed',
              is_open: true
            }]
          }]
        }
      })
      expect(wrapper.text()).toContain('admin.profit.quotaWindowDrifting')
      // has dashed drifting bar with border-dashed class
      const bars = wrapper.findAll('button')
      const hasDrift = bars.some((b) => (b.attributes('class') || '').includes('border-dashed') && (b.attributes('title') || '').includes('drifting'))
      expect(hasDrift).toBe(true)
    } finally {
      vi.useRealTimers()
    }
  })
})

describe('lazy quota activation gap', () => {
  it('renders provider-clear to first-use time as waiting activation', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-09T12:00:00+08:00'))
    try {
      const wrapper = mount(QuotaWindowPanel, {
        props: {
          accounts: [{
            account_id: 98,
            account_name: 'GPT-lazy-week',
            platform: 'openai',
            account_type: 'oauth',
            cost_type: 'subscription',
            configured: true,
            requests: 1,
            revenue: 1,
            cost: 1,
            profit: 0,
            margin: 0,
            currency: 'USD',
            quota_availability_spans: [{
              start_at: '2026-08-01T00:00:00+08:00',
              end_at: '2026-08-31T00:00:00+08:00'
            }],
            quota_windows: [
              {
                id: '7d-old', label: '7d', kind: '7d',
                start_at: '2026-08-01T01:00:00+08:00',
                end_at: '2026-08-08T01:00:00+08:00',
                window_minutes: 10080,
                is_open: false
              },
              {
                id: '7d-waiting', label: '7d', kind: '7d',
                start_at: '2026-08-08T01:00:00+08:00',
                end_at: '2026-08-08T09:00:00+08:00',
                status: 'waiting_activation',
                is_open: false
              },
              {
                id: '7d-open', label: '7d', kind: '7d', used_percent: 1,
                start_at: '2026-08-08T09:00:00+08:00',
                end_at: '2026-08-15T09:00:00+08:00',
                window_minutes: 10080,
                is_open: true
              }
            ]
          }]
        }
      })

      expect(wrapper.text()).toContain('admin.profit.quotaWindowWaitingActivation')
      const waitingBar = wrapper.findAll('button').find((button) => (button.attributes('title') || '').includes('waiting_activation'))
      expect(waitingBar).toBeTruthy()
      expect(waitingBar!.attributes('class')).toContain('border-dashed')
    } finally {
      vi.useRealTimers()
    }
  })
})

describe('bounded timeline rendering', () => {
  it('keeps ledger history and projects only future windows after the latest real window', () => {
    const now = Date.parse('2026-08-09T12:00:00Z')
    const prepared = prepareQuotaAccounts([{
      account_id: 1,
      account_name: 'openai-ledger',
      platform: 'openai',
      account_type: 'oauth',
      cost_type: 'subscription',
      configured: true,
      requests: 1,
      revenue: 1,
      cost: 1,
      profit: 0,
      margin: 0,
      currency: 'USD',
      quota_windows: [{
        id: '7d-history', label: '7d', kind: '7d', source: 'observed', is_open: false,
        start_at: '2026-08-02T12:00:00Z', end_at: '2026-08-09T12:00:00Z', window_minutes: 10080,
        recurring_until_at: '2026-08-31T00:00:00Z'
      }, {
        id: '7d-open', label: '7d', kind: '7d', source: 'observed', is_open: true,
        start_at: '2026-08-09T12:00:00Z', end_at: '2026-08-16T12:00:00Z', window_minutes: 10080,
        recurring_until_at: '2026-08-31T00:00:00Z'
      }]
    }], now)[0]

    const windows = windowsForRange(
      prepared,
      Date.parse('2026-08-01T00:00:00Z'),
      Date.parse('2026-09-01T00:00:00Z')
    )
    expect(windows.some((window) => window.id === '7d-history')).toBe(true)
    expect(windows.some((window) => window.id === '7d-open')).toBe(true)
    expect(windows.some((window) => window.source === 'projected' && window.startMs === Date.parse('2026-08-16T12:00:00Z'))).toBe(true)
  })

  it('uses one canonical recurrence when an observed ledger end drifts by seconds', () => {
    const now = Date.parse('2026-08-10T12:00:00+08:00')
    const prepared = prepareQuotaAccounts([{
      account_id: 102,
      account_name: 'GPT-自有-3',
      platform: 'openai',
      account_type: 'oauth',
      cost_type: 'subscription',
      configured: true,
      requests: 1,
      revenue: 1,
      cost: 1,
      profit: 0,
      margin: 0,
      currency: 'USD',
      quota_availability_spans: [{
        start_at: '2026-07-26T08:00:00+08:00',
        end_at: '2026-08-25T08:00:00+08:00'
      }],
      quota_windows: [{
        id: '7d-open', label: '7d', kind: '7d', source: 'seed', is_open: true,
        start_at: '2026-08-09T04:33:47+08:00',
        end_at: '2026-08-16T04:33:59+08:00',
        window_minutes: 10080,
        recurring_until_at: '2026-08-25T08:00:00+08:00'
      }]
    }], now)[0]

    const account = prepared.account
    const windows = windowsForDrawerSelection(
      account,
      now,
      Date.parse('2026-08-09T04:33:47+08:00'),
      Date.parse('2026-08-16T04:33:59+08:00'),
      '7d'
    )
    const projected = windows.filter((window) => window.source === 'projected')

    expect(projected).toHaveLength(2)
    expect(projected.map((window) => window.startMs)).toEqual([
      Date.parse('2026-08-16T04:33:59+08:00'),
      Date.parse('2026-08-23T04:33:59+08:00')
    ])
    expect(projected[1].endMs).toBe(Date.parse('2026-08-25T08:00:00+08:00'))
  })

  it('does not invent future starts while a lazy window is waiting for activation', () => {
    const now = Date.parse('2026-08-10T12:00:00Z')
    const prepared = prepareQuotaAccounts([{
      account_id: 1,
      account_name: 'waiting-account',
      platform: 'openai',
      account_type: 'oauth',
      cost_type: 'subscription',
      configured: true,
      requests: 1,
      revenue: 1,
      cost: 1,
      profit: 0,
      margin: 0,
      currency: 'USD',
      quota_windows: [{
        id: '7d-old', label: '7d', kind: '7d', source: 'observed', is_open: false,
        start_at: '2026-08-02T12:00:00Z', end_at: '2026-08-09T12:00:00Z', window_minutes: 10080
      }, {
        id: '7d-waiting', label: '7d', kind: '7d', source: 'derived', status: 'waiting_activation',
        start_at: '2026-08-09T12:00:00Z', end_at: '2026-08-10T12:00:00Z', window_minutes: 10080
      }]
    }], now)[0]

    const windows = windowsForRange(prepared, Date.parse('2026-08-01T00:00:00Z'), Date.parse('2026-09-01T00:00:00Z'))
    expect(windows.some((window) => window.source === 'projected')).toBe(false)
  })

  it('shares one visible grid across the maximum 24 lanes', () => {
    const now = Date.now()
    const accounts = Array.from({ length: 24 }, (_, index) => ({
      account_id: index + 1,
      account_name: `account-${index + 1}`,
      platform: 'openai',
      account_type: 'oauth',
      cost_type: 'subscription' as const,
      configured: true,
      requests: 1,
      revenue: 1,
      cost: 1,
      profit: 0,
      margin: 0,
      currency: 'USD',
      quota_windows: [{
        id: `7d-${index}`,
        label: '7d',
        kind: '7d',
        used_percent: index,
        start_at: new Date(now - 2 * 86_400_000).toISOString(),
        end_at: new Date(now + 5 * 86_400_000).toISOString(),
        window_minutes: 10080
      }]
    }))
    const wrapper = mount(QuotaWindowPanel, { props: { accounts } })

    expect(wrapper.findAll('[data-testid="profit-quota-window-lane"]')).toHaveLength(24)
    const gridLines = wrapper.findAll('[data-testid="quota-window-grid-line"]')
    expect(gridLines.length).toBeGreaterThan(0)
    expect(gridLines.length).toBeLessThan(60)
  })

  it('keeps content width constant while rebasing at the right edge', async () => {
    vi.useFakeTimers()
    try {
      const now = Date.now()
      const wrapper = mount(QuotaWindowPanel, {
        props: {
          accounts: [{
            account_id: 1,
            account_name: 'bounded-account',
            platform: 'openai',
            account_type: 'oauth',
            cost_type: 'subscription',
            configured: true,
            requests: 1,
            revenue: 1,
            cost: 1,
            profit: 0,
            margin: 0,
            currency: 'USD',
            quota_windows: [{
              id: '7d', label: '7d', kind: '7d', used_percent: 10,
              start_at: new Date(now - 2 * 86_400_000).toISOString(),
              end_at: new Date(now + 5 * 86_400_000).toISOString(),
              window_minutes: 10080
            }]
          }]
        }
      })
      const scroller = wrapper.find('[data-testid="profit-quota-window-scroll"]')
      const content = wrapper.find('[data-testid="profit-quota-window-content"]')
      const el = scroller.element as HTMLElement
      const initialWidth = content.attributes('style')
      Object.defineProperty(el, 'clientWidth', { value: 500, configurable: true })
      Object.defineProperty(el, 'scrollWidth', { value: 6480, configurable: true })

      for (let index = 0; index < 5; index += 1) {
        el.scrollLeft = 5980
        await scroller.trigger('scroll')
        await vi.advanceTimersByTimeAsync(40)
      }

      expect(content.attributes('style')).toBe(initialWidth)
    } finally {
      vi.useRealTimers()
    }
  })

  it('jumps directly to visible recurrence indexes far from the source anchor', () => {
    const start = Date.parse('2030-01-01T00:00:00Z')
    const windows = expandWindowOccurrences({
      id: '5h', label: '5h', kind: '5h', window_minutes: 300,
      start_at: '2020-01-01T00:00:00Z',
      end_at: '2020-01-01T05:00:00Z'
    }, start, start + 48 * 3_600_000)

    expect(windows.length).toBeGreaterThan(0)
    expect(windows.length).toBeLessThanOrEqual(10)
    expect(windows[0].endMs).toBeGreaterThan(start)
  })

  it('strips ledger state from snapshot projections', () => {
    const start = Date.parse('2026-08-01T00:00:00Z')
    const windows = expandWindowOccurrences({
      id: '7d-open', label: '7d', kind: '7d', window_minutes: 10080,
      start_at: '2026-08-08T00:00:00Z',
      end_at: '2026-08-15T00:00:00Z',
      source: '',
      is_open: true,
      closed_reason: 'cleared'
    }, start, start + 21 * 86_400_000)

    expect(windows.length).toBeGreaterThan(1)
    expect(windows.every((window) => window.source === 'projected')).toBe(true)
    expect(windows.every((window) => window.is_open == null)).toBe(true)
    expect(windows.every((window) => window.closed_reason == null)).toBe(true)
  })
})
