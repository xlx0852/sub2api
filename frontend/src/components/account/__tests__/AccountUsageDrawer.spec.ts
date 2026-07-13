import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountUsageDrawer from '../AccountUsageDrawer.vue'
import type { Account } from '@/types'

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
  credentials: {}
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
  })

  const mountDrawer = () => mount(AccountUsageDrawer, {
    attachTo: document.body,
    props: { show: true, account },
    global: {
      stubs: {
        teleport: true,
        AccountUsageCell: { template: '<div data-testid="quota-content">quota</div>' },
        PlatformTypeBadge: true,
        MetricItem: { props: ['label', 'value'], template: '<div class="metric-stub">{{ label }}:{{ value }}</div>' }
      }
    }
  })

  it('按额度、统计、性能和诊断展示页签并默认打开额度', () => {
    const wrapper = mountDrawer()
    const tabs = wrapper.findAll('[role="tab"]')
    expect(tabs).toHaveLength(4)
    expect(tabs[0]?.attributes('aria-selected')).toBe('true')
    expect(wrapper.find('[data-testid="quota-content"]').exists()).toBe(true)
  })

  it('在同一个抽屉中展示统计视图', async () => {
    const wrapper = mountDrawer()
    await wrapper.findAll('[role="tab"]')[1]?.trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="statistics-content"]').exists()).toBe(true)
  })

  it('切换账号后更新头部并回到额度页签', async () => {
    const wrapper = mountDrawer()
    await wrapper.findAll('[role="tab"]')[1]?.trigger('click')
    await flushPromises()

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
})
