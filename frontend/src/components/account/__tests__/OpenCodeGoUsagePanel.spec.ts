import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OpenCodeGoUsagePanel from '../OpenCodeGoUsagePanel.vue'
import type { Account } from '@/types'

const accountsMocks = vi.hoisted(() => ({ getOpenCodeGoUsage: vi.fn() }))
vi.mock('@/api/admin/accounts', () => ({ accountsAPI: { getOpenCodeGoUsage: accountsMocks.getOpenCodeGoUsage } }))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => (params ? `${key}:${JSON.stringify(params)}` : key)
    })
  }
})

const account = (baseUrl: string): Account =>
  ({
    id: 120,
    name: 'OpenCode Go',
    platform: 'openai',
    type: 'apikey',
    status: 'active',
    extra: {},
    credentials: { base_url: baseUrl }
  }) as unknown as Account

beforeEach(() => {
  accountsMocks.getOpenCodeGoUsage.mockReset()
})

describe('OpenCodeGoUsagePanel', () => {
  it('renders the three official windows with their percentages', async () => {
    accountsMocks.getOpenCodeGoUsage.mockResolvedValue({
      applicable: true,
      usage: {
        rolling: { percent: 12.5 },
        weekly: { percent: 55, resets_at: '2026-09-30T00:00:00Z' },
        monthly: { percent: 20 }
      }
    })
    const wrapper = mount(OpenCodeGoUsagePanel, { props: { account: account('https://opencode.ai/zen/go/v1'), active: true } })
    await flushPromises()
    const text = wrapper.text()
    expect(text).toContain('admin.accounts.usageDetails.openCodeGo.rolling')
    expect(text).toContain('admin.accounts.usageDetails.openCodeGo.weekly')
    expect(text).toContain('admin.accounts.usageDetails.openCodeGo.monthly')
    expect(text).toContain('12.5%')
    expect(text).toContain('55.0%')
    expect(text).toContain('20.0%')
  })

  it('does not query or render for accounts that are not OpenCode Go', async () => {
    const wrapper = mount(OpenCodeGoUsagePanel, {
      props: { account: account('https://relay.example.com/v1'), active: true }
    })
    await flushPromises()
    expect(accountsMocks.getOpenCodeGoUsage).not.toHaveBeenCalled()
    expect(wrapper.text()).toBe('')
  })

  it('rejects base urls that only look like the official host', async () => {
    const wrapper = mount(OpenCodeGoUsagePanel, {
      props: { account: account('https://opencode.ai.evil.example/zen/go/v1'), active: true }
    })
    await flushPromises()
    expect(accountsMocks.getOpenCodeGoUsage).not.toHaveBeenCalled()
    expect(wrapper.text()).toBe('')
  })

  it('shows the failure reason instead of hiding it', async () => {
    accountsMocks.getOpenCodeGoUsage.mockResolvedValue({
      applicable: true,
      usage: { percent: 0, error: 'Go subscription required (403)' } as never
    })
    const wrapper = mount(OpenCodeGoUsagePanel, { props: { account: account('https://opencode.ai/zen/go/v1'), active: true } })
    await flushPromises()
    expect(wrapper.text()).toContain('Go subscription required (403)')
  })

  it('hides itself when the server says the account is not applicable', async () => {
    accountsMocks.getOpenCodeGoUsage.mockResolvedValue({ applicable: false })
    const wrapper = mount(OpenCodeGoUsagePanel, { props: { account: account('https://opencode.ai/zen/go/v1'), active: true } })
    await flushPromises()
    expect(accountsMocks.getOpenCodeGoUsage).toHaveBeenCalledWith(120)
    expect(wrapper.text()).toBe('')
  })
})
