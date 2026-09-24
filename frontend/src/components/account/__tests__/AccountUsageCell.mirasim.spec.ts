import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountUsageCell from '../AccountUsageCell.vue'
import type { Account } from '@/types'

const { getUsage } = vi.hoisted(() => ({ getUsage: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: { getUsage } } }))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

let nextID = 951000
const account = (overrides: Partial<Account> = {}) => ({
  id: ++nextID, platform: 'anthropic', type: 'apikey',
  extra: { mirasim_relay_enabled: true }, ...overrides
}) as Account
const usage = () => ({
  source: 'active', mirasim_paid: true,
  seven_day: { utilization: 20, resets_at: null },
  mirasim_windows: [
    { name: '7d', budget: 100, used: 20, utilization: 20, window_key: 'seven_day', reset_at: '2026-10-01T00:00:00Z' },
    { name: '7d_claude', budget: 10, used: 10, utilization: 100, model_scoped: true },
    { name: 'unlimited', budget: 0, used: 10, unlimited: true }
  ]
})
const wrappers: ReturnType<typeof mount>[] = []
function render(a: Account, variant: 'detail' | 'summary' = 'detail') {
  const wrapper = mount(AccountUsageCell, {
    props: { account: a, variant },
    global: { stubs: {
      UsageProgressBar: { props: ['label', 'utilization', 'displayPercent', 'resetsAt', 'extraStats'], template: '<div class="bar">{{ label }}|{{ utilization }}|{{ displayPercent }}|{{ resetsAt }}|{{ (extraStats || []).map(item => item.label).join("|") }}</div>' },
      AccountUsageSummary: { props: ['presentation'], template: '<div class="summary">{{ JSON.stringify(presentation) }}</div>' }
    } }
  })
  wrappers.push(wrapper)
  return wrapper
}

beforeEach(() => {
  getUsage.mockReset().mockResolvedValue(usage())
  Object.defineProperty(window, 'matchMedia', { writable: true, value: vi.fn(() => ({
    matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn()
  })) })
})
afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()) })

describe('Mirasim usage component wiring', () => {
  it('详情自动请求真实额度，展示全部窗口、套餐和不限量，不重复兼容窗口', async () => {
    const a = account()
    const wrapper = render(a)
    await flushPromises()
    expect(getUsage).toHaveBeenCalledWith(a.id)
    expect(wrapper.findAll('.bar')).toHaveLength(3)
    expect(wrapper.text()).toContain('7d|20||2026-10-01T00:00:00Z')
    expect(wrapper.text()).toContain('7d_claude|100')
    expect(wrapper.text()).toContain('admin.accounts.usageWindow.mirasimModelScoped')
    expect(wrapper.text()).toContain('admin.accounts.usageWindow.mirasimPaid')
    expect(wrapper.text()).toContain('unlimited|0|admin.accounts.usageWindow.mirasimUnlimited')
    expect(wrapper.text()).toContain('20 / 100')
  })

  it('凭据已脱敏时通过存在性标记识别账号，摘要也加载额度', async () => {
    const a = account({ extra: {}, credentials_status: { has_mirasim_refresh_token: true } })
    const wrapper = render(a, 'summary')
    await flushPromises()
    expect(getUsage).toHaveBeenCalledWith(a.id)
    expect(wrapper.find('.summary').exists()).toBe(true)
    expect(wrapper.find('.summary').text()).toContain('statusNormal')
  })

  it.each([
    { extra: {} },
    { extra: { mirasim_relay_enabled: false }, credentials_status: { has_mirasim_refresh_token: true } },
    { platform: 'openai' }
  ])('普通 API Key / 显式关闭 / 其它平台不误查 mirasim: %j', async overrides => {
    render(account(overrides))
    await flushPromises()
    expect(getUsage).not.toHaveBeenCalled()
  })

  it('降级响应仍显示错误，并能主动强制刷新恢复', async () => {
    getUsage.mockResolvedValueOnce({ source: 'active', error: 'failed to fetch mirasim usage: timeout' })
    const a = account()
    const wrapper = render(a)
    await flushPromises()
    expect(wrapper.text()).toContain('failed to fetch mirasim usage: timeout')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(getUsage).toHaveBeenLastCalledWith(a.id, 'active', true)
    expect(wrapper.findAll('.bar')).toHaveLength(3)
    expect(wrapper.text()).not.toContain('failed to fetch')
  })

  it('上游显式降级与免费标志可见', async () => {
    getUsage.mockResolvedValue({ ...usage(), mirasim_paid: false, mirasim_degraded: true })
    const wrapper = render(account())
    await flushPromises()
    expect(wrapper.text()).toContain('admin.accounts.usageWindow.mirasimFree')
    expect(wrapper.text()).toContain('admin.accounts.usageWindow.mirasimDegradedHint')
  })
})
