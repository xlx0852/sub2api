import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import CodexTicketPanel from '../CodexTicketPanel.vue'
import type { Account } from '@/types'

// 列表接口不含 extra，所以面板打开时按 id 取详情；单测里把详情接口打桩。
const accountsMocks = vi.hoisted(() => ({ getById: vi.fn() }))
vi.mock('@/api/admin/accounts', () => ({ accountsAPI: { getById: accountsMocks.getById } }))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => (params ? `${key}:${JSON.stringify(params)}` : key) })
  }
})

const baseAccount = {
  id: 1,
  name: 'OpenAI Primary',
  platform: 'openai',
  type: 'oauth',
  status: 'active',
  extra: {},
  credentials: {}
} as unknown as Account

const withSummary = (summary: Record<string, unknown>, extra: Record<string, unknown> = {}) =>
  ({ ...baseAccount, extra, codex_ticket: summary }) as unknown as Account

// 列表里的账号（无 extra / 无摘要）
const listAccount = { ...baseAccount } as unknown as Account
// 详情接口返回的账号（带摘要）
const detailAccount = (summary: Record<string, unknown> | undefined) =>
  ({ ...baseAccount, codex_ticket: summary }) as unknown as Account

beforeEach(() => {
  accountsMocks.getById.mockReset()
})

describe('CodexTicketPanel', () => {
  it('renders each pool slot with its shape, freshness and served model', async () => {
        accountsMocks.getById.mockResolvedValue(detailAccount({
      replay_ttl_seconds: 3600,
      pools: [
        { pool: 'gpt-6-astra', model: 'gpt-6-astra', shape: 292, age_seconds: 120, fresh: true, captured_at: '2026-09-22T15:00:00Z' },
        { pool: 'gpt-5.6-luna', model: 'gpt-5.6-luna', shape: 312, age_seconds: 4000, fresh: false, captured_at: '2026-09-22T14:00:00Z' }
      ]
    }))
const wrapper = mount(CodexTicketPanel, {
      props: { account: listAccount, active: true }
    })
    await flushPromises()
    const text = wrapper.text()
    expect(text).toContain('gpt-6-astra')
    expect(text).toContain('292')
    expect(text).toContain('gpt-5.6-luna')
    expect(text).toContain('312')
    // 可回放 / 已过期 两种状态都要出现（i18n 被 mock 成 key）
    expect(text).toContain('admin.accounts.usageDetails.tickets.replayable')
    expect(text).toContain('admin.accounts.usageDetails.tickets.expired')
    // 回放窗口按秒换算成分钟展示
    expect(text).toContain('"minutes":60')
  })

  it('renders cookie count, egress fingerprint and does not invent a refresh time when absent', async () => {
        accountsMocks.getById.mockResolvedValue(detailAccount({
      replay_ttl_seconds: 3600,
      pools: [{ pool: 'gpt-5.6-luna', shape: 312, age_seconds: 60, fresh: true }],
      cookies: { count: 3, fresh: true, egress: '5ee7a937' }
    }))
const wrapper = mount(CodexTicketPanel, {
      props: { account: listAccount, active: true }
    })
    await flushPromises()
    const text = wrapper.text()
    expect(text).toContain('"count":3')
    expect(text).toContain('5ee7a937')
    expect(text).not.toContain('cookieRefreshed')
  })

  it('falls back to the empty state when the account has no summary', async () => {
    accountsMocks.getById.mockResolvedValue(detailAccount(undefined))
    const wrapper = mount(CodexTicketPanel, { props: { account: listAccount, active: true } })
    await flushPromises()
    expect(wrapper.text()).toContain('admin.accounts.usageDetails.tickets.empty')
  })

  it('does not request the detail until the tab is active', async () => {
    accountsMocks.getById.mockResolvedValue(detailAccount(undefined))
    mount(CodexTicketPanel, { props: { account: listAccount, active: false } })
    await flushPromises()
    expect(accountsMocks.getById).not.toHaveBeenCalled()
  })

  it('surfaces a load failure instead of rendering a broken panel', async () => {
    accountsMocks.getById.mockRejectedValue(new Error('boom'))
    const wrapper = mount(CodexTicketPanel, { props: { account: listAccount, active: true } })
    await flushPromises()
    expect(wrapper.text()).toContain('admin.accounts.usageDetails.tickets.loadFailed')
  })

  it('never renders ticket or cookie material from extra', async () => {
    const secret = 'gAAAAABqq_20UDF7-SECRET-TICKET-VALUE'
    const wrapper = mount(CodexTicketPanel, {
      props: {
        account: withSummary(
          { replay_ttl_seconds: 3600, pools: [{ pool: 'gpt-5.6-luna', shape: 312, age_seconds: 5, fresh: true }] },
          {
            openai_codex_turn_state: secret,
            openai_codex_turn_state_pool_gpt_5_6_luna: secret,
            openai_codex_turn_state_cookies: '[{"n":"__oailb","v":"SECRET-COOKIE-VALUE"}]'
          }
        ),
        active: true
      }
    })
    await flushPromises()
    const text = wrapper.text()
    expect(text).not.toContain(secret)
    expect(text).not.toContain('SECRET-COOKIE-VALUE')
  })
})
