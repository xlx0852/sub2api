import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountUsageDrawer from '../AccountUsageDrawer.vue'
import type { Account } from '@/types'

const scheduledTestsMocks = vi.hoisted(() => ({
  listByAccount: vi.fn(),
  listResults: vi.fn(),
  ensureDefault: vi.fn()
}))

vi.mock('@/api/admin/scheduledTests', () => ({
  scheduledTestsAPI: scheduledTestsMocks,
  default: scheduledTestsMocks
}))

vi.mock('@/components/admin/account/AccountStatsModal.vue', () => ({
  default: { props: ['account'], template: '<div data-testid="statistics-content">statistics</div>' }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const account = {
  id: 1,
  name: 'OpenAI Primary',
  platform: 'openai',
  type: 'oauth',
  status: 'active',
  extra: {},
  credentials: {},
  codex_ticket: {
    replay_ttl_seconds: 3600,
    pools: [{ pool: 'gpt-5.6-luna', model: 'gpt-5.6-luna', shape: 312, age_seconds: 90, fresh: true }],
    cookies: { count: 3, fresh: true, egress: '5ee7a937' }
  }
} as Account

const secondAccount = {
  ...account,
  id: 2,
  name: 'Grok Secondary',
  platform: 'grok'
} as Account

describe('AccountUsageDrawer', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    vi.clearAllMocks()
  })

  const mountDrawer = () => mount(AccountUsageDrawer, {
    attachTo: document.body,
    props: { show: true, account },
    global: {
      stubs: {
        teleport: true,
        AccountUsageCell: { template: '<div data-testid="quota-content">quota</div>' },
        PlatformTypeBadge: true,
        ScheduledTestsPanel: true,
        MetricItem: { props: ['label', 'value'], template: '<div class="metric-stub">{{ label }}:{{ value }}</div>' }
      }
    }
  })

  // 按 key 定位页签，避免新增页签时索引位移把无关测试点错。
  // 本 spec 的 i18n mock 把 label 渲染成 key，所以用 tabs.<key> 匹配。
  const openTab = async (wrapper: ReturnType<typeof mountDrawer>, key: string) => {
    const tab = wrapper.findAll('[role="tab"]').find((node) => node.text().includes(`tabs.${key}`))
    expect(tab, `tab ${key} not found`).toBeTruthy()
    await tab?.trigger('click')
    await flushPromises()
  }

  it('按额度、统计、性能、打票和诊断展示页签并默认打开额度', () => {
    const wrapper = mountDrawer()
    const tabs = wrapper.findAll('[role="tab"]')
    expect(tabs).toHaveLength(5)
    expect(tabs[0]?.attributes('aria-selected')).toBe('true')
    expect(wrapper.find('[data-testid="quota-content"]').exists()).toBe(true)
  })

  it('切到打票页签展示票与 Cookie 状态', async () => {
    const wrapper = mountDrawer()
    await openTab(wrapper, 'ticket')

    const text = wrapper.text()
    expect(text).toContain('admin.accounts.usageDetails.tickets.title')
    // 账号上的 codex_ticket 摘要要被渲染出来（形态与池位）。
    expect(text).toContain('gpt-5.6-luna')
    expect(text).toContain('312')
    // 本测试的 i18n mock 不渲染插值参数，只断言 Cookie 区块确实出现。
    expect(text).toContain('admin.accounts.usageDetails.tickets.cookieCount')
  })

  it('在同一个抽屉中展示统计视图', async () => {
    const wrapper = mountDrawer()
    await openTab(wrapper, 'statistics')

    expect(wrapper.find('[data-testid="statistics-content"]').exists()).toBe(true)
  })

  it('切换账号后更新头部并回到额度页签', async () => {
    const wrapper = mountDrawer()
    await openTab(wrapper, 'statistics')

    await wrapper.setProps({ account: secondAccount })
    await flushPromises()

    expect(wrapper.text()).toContain('Grok Secondary')
    expect(wrapper.findAll('[role="tab"]')[0]?.attributes('aria-selected')).toBe('true')
  })

  it('按 Escape 关闭抽屉', async () => {
    const wrapper = mountDrawer()
    await flushPromises()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('用最近定时测试结果展示可用性概览', async () => {
    scheduledTestsMocks.listByAccount.mockResolvedValue([{ id: 8, account_id: 1, model_id: 'gpt-test', enabled: true, max_results: 50 }])
    scheduledTestsMocks.listResults.mockResolvedValue([
      { id: 1, plan_id: 8, status: 'success', started_at: '2026-08-04T10:00:00Z' },
      { id: 2, plan_id: 8, status: 'failed', started_at: '2026-08-04T10:10:00Z' },
      { id: 3, plan_id: 8, status: 'running', started_at: '2026-08-04T10:20:00Z' }
    ])

    const wrapper = mountDrawer()
    await openTab(wrapper, 'diagnostics')

    const availability = wrapper.find('[role="list"]')
    expect(availability.exists()).toBe(true)
    expect(availability.findAll('[role="listitem"]')).toHaveLength(3)
    expect(wrapper.text()).toContain('admin.accounts.usageDetails.scheduledTestsSuccess 1')
    expect(wrapper.text()).toContain('admin.accounts.usageDetails.scheduledTestsFailed 1')
    expect(wrapper.text()).toContain('50%')
  })

  it('可用性概览最多展示最近 200 条', async () => {
    scheduledTestsMocks.listByAccount.mockResolvedValue([{ id: 9, account_id: 1, model_id: 'gpt-test', enabled: true, max_results: 250 }])
    scheduledTestsMocks.listResults.mockResolvedValue(Array.from({ length: 200 }, (_, index) => ({
      id: index + 1,
      plan_id: 9,
      status: 'success',
      started_at: new Date(Date.UTC(2026, 7, 4, 0, index)).toISOString()
    })))

    const wrapper = mountDrawer()
    await openTab(wrapper, 'diagnostics')

    expect(wrapper.find('[role="list"]').findAll('[role="listitem"]')).toHaveLength(200)
  })
})
