import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import OpenCodeGoUsageCell from '../OpenCodeGoUsageCell.vue'
import type { Account } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => (params ? `${key}:${JSON.stringify(params)}` : key)
    })
  }
})

const account = (extra: Record<string, unknown>, baseUrl = 'https://opencode.ai/zen/go/v1', type = 'apikey'): Account =>
  ({
    id: 120,
    name: 'OpenCode Go',
    platform: 'openai',
    type,
    status: 'active',
    extra,
    credentials: { base_url: baseUrl }
  }) as unknown as Account

const withSnapshot = (payload: Record<string, unknown>) =>
  account({ opencode_go_usage: JSON.stringify(payload) })

describe('OpenCodeGoUsageCell', () => {
  it('renders 7d and 1m badges from the stored snapshot without extra requests', () => {
    const wrapper = mount(OpenCodeGoUsageCell, {
      props: { account: withSnapshot({ weekly: { percent: 55 }, monthly: { percent: 12.5 } }) }
    })
    const text = wrapper.text()
    expect(text).toContain('admin.accounts.openCodeGo.weeklyShort')
    expect(text).toContain('55.0%')
    expect(text).toContain('admin.accounts.openCodeGo.monthlyShort')
    expect(text).toContain('12.5%')
  })

  it('tone follows the highest usage band', () => {
    const hot = mount(OpenCodeGoUsageCell, { props: { account: withSnapshot({ weekly: { percent: 95 } }) } })
    expect(hot.html()).toContain('text-red-700')
    const warm = mount(OpenCodeGoUsageCell, { props: { account: withSnapshot({ weekly: { percent: 75 } }) } })
    expect(warm.html()).toContain('text-amber-700')
    const cool = mount(OpenCodeGoUsageCell, { props: { account: withSnapshot({ weekly: { percent: 10 } }) } })
    expect(cool.html()).toContain('text-emerald-700')
  })

  it('shows a failure badge carrying the reason in the tooltip', () => {
    const wrapper = mount(OpenCodeGoUsageCell, {
      props: { account: withSnapshot({ error: 'Go subscription required (403)' }) }
    })
    expect(wrapper.text()).toContain('admin.accounts.openCodeGo.failedShort')
    expect(wrapper.attributes('title')).toContain('Go subscription required (403)')
  })

  it('shows a not-fetched marker when no snapshot exists yet', () => {
    const wrapper = mount(OpenCodeGoUsageCell, { props: { account: account({}) } })
    expect(wrapper.text()).toContain('admin.accounts.openCodeGo.notFetched')
  })

  it('renders nothing for accounts that are not OpenCode Go, including look-alike hosts', () => {
    expect(mount(OpenCodeGoUsageCell, { props: { account: account({}, 'https://relay.example.com/v1') } }).text()).toBe('')
    expect(mount(OpenCodeGoUsageCell, { props: { account: account({}, 'https://opencode.ai.evil.example/zen/go/v1') } }).text()).toBe('')
    expect(mount(OpenCodeGoUsageCell, { props: { account: account({}, 'https://opencode.ai/zen/go/v1', 'oauth') } }).text()).toBe('')
  })

  it('survives a corrupted snapshot instead of throwing', () => {
    const wrapper = mount(OpenCodeGoUsageCell, { props: { account: account({ opencode_go_usage: '{not json' }) } })
    expect(wrapper.text()).toContain('admin.accounts.openCodeGo.notFetched')
  })
})
