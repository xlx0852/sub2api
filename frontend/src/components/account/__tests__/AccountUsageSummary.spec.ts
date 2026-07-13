import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountUsageSummary from '../AccountUsageSummary.vue'
import type { AccountUsagePresentation } from '@/utils/accountUsagePresentation'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})
const presentation: AccountUsagePresentation = {
  windows: [
    { key: '5h', label: '5h', utilization: 12, resetsAt: null, color: 'indigo' },
    { key: '7d', label: '7d', utilization: 82, resetsAt: null, color: 'emerald' },
    { key: 'extra', label: 'extra', utilization: 20, resetsAt: null, color: 'purple' }
  ],
  today: { requests: 282, tokens: 56_000_000, cost: 49.09, user_cost: 49.09 },
  plan: 'Pro',
  statusLabel: 'admin.accounts.usageDetails.statusNearLimit',
  statusTone: 'warning',
  sourceLabel: 'admin.accounts.usageDetails.sourceActive',
  updatedAt: null,
  diagnostics: [],
  error: null
}

describe('AccountUsageSummary', () => {
  it('只显示两个主要窗口和紧凑的今日统计', () => {
    const wrapper = mount(AccountUsageSummary, { props: { presentation } })

    expect(wrapper.text()).toContain('5h')
    expect(wrapper.text()).toContain('7d')
    expect(wrapper.text()).not.toContain('extra')
    expect(wrapper.text()).toContain('282 req')
    expect(wrapper.text()).toContain('A $49.09')
  })

  it('点击摘要会请求打开详情', async () => {
    const wrapper = mount(AccountUsageSummary, { props: { presentation } })
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('open')).toHaveLength(1)
  })
})
