import { describe, expect, it } from 'vitest'
import type { Account, AccountUsageInfo, WindowStats } from '@/types'
import { buildAccountUsagePresentation } from '../accountUsagePresentation'

const t = (key: string) => key

const makeAccount = (overrides: Partial<Account> = {}): Account => ({
  id: 1,
  name: 'account',
  platform: 'openai',
  type: 'oauth',
  proxy_id: null,
  concurrency: 1,
  priority: 1,
  status: 'active',
  error_message: null,
  last_used_at: null,
  expires_at: null,
  auto_pause_on_expired: true,
  created_at: '2026-07-12T00:00:00Z',
  updated_at: '2026-07-12T00:00:00Z',
  schedulable: true,
  rate_limited_at: null,
  rate_limit_reset_at: null,
  overload_until: null,
  temp_unschedulable_until: null,
  temp_unschedulable_reason: null,
  session_window_start: null,
  session_window_end: null,
  session_window_status: null,
  ...overrides
})

const baseUsage = (overrides: Partial<AccountUsageInfo> = {}): AccountUsageInfo => ({
  updated_at: '2026-07-12T01:00:00Z',
  five_hour: null,
  seven_day: null,
  seven_day_sonnet: null,
  ...overrides
})

const today: WindowStats = {
  requests: 12,
  tokens: 34_000,
  cost: 1.23,
  user_cost: 2.46
}

describe('buildAccountUsagePresentation', () => {
  it('统一映射 OpenAI 额度窗口并按最高用量给出风险状态', () => {
    const presentation = buildAccountUsagePresentation({
      account: makeAccount(),
      usageInfo: baseUsage({
        source: 'active',
        five_hour: { utilization: 85, resets_at: '2026-07-12T05:00:00Z', remaining_seconds: 0 },
        seven_day: { utilization: 42, resets_at: '2026-07-18T00:00:00Z', remaining_seconds: 0 }
      }),
      todayStats: today,
      t
    })

    expect(presentation.windows.map(window => window.label)).toEqual(['5h', '7d'])
    expect(presentation.statusTone).toBe('warning')
    expect(presentation.statusLabel).toBe('admin.accounts.usageDetails.statusNearLimit')
    expect(presentation.today).toEqual(today)
    expect(presentation.sourceLabel).toBe('admin.accounts.usageDetails.sourceActive')
  })

  it('OpenAI 仅返回每周额度时不伪造 5h 窗口', () => {
    const presentation = buildAccountUsagePresentation({
      account: makeAccount(),
      usageInfo: baseUsage({
        source: 'active',
        five_hour: null,
        seven_day: { utilization: 3, resets_at: '2026-07-20T03:05:00Z', remaining_seconds: 0 }
      }),
      todayStats: today,
      t
    })

    expect(presentation.windows.map(window => window.label)).toEqual(['7d'])
  })

  it('Kimi 使用 membership.level 展示可读套餐等级而不是购买类型', () => {
    const presentation = buildAccountUsagePresentation({
      account: makeAccount({ platform: 'kimi' }),
      usageInfo: baseUsage({
        subscription_tier: 'LEVEL_INTERMEDIATE',
        subscription_kind: 'TYPE_PURCHASE',
        five_hour: { utilization: 26, resets_at: '2026-07-29T17:00:00Z', remaining_seconds: 0 },
        seven_day: { utilization: 89, resets_at: '2026-08-01T12:00:00Z', remaining_seconds: 0 }
      }),
      todayStats: today,
      t
    })

    expect(presentation.plan).toBe('Intermediate')
    expect(presentation.plan).not.toBe('TYPE_PURCHASE')
  })

  it('将 Grok 周限和月度积分映射到同一窗口模型', () => {
    const presentation = buildAccountUsagePresentation({
      account: makeAccount({ platform: 'grok' }),
      usageInfo: baseUsage({
        grok_billing: {
          period_type: 'weekly',
          usage_percent: 28,
          period_end: '2026-07-16T00:00:00Z',
          used_percent: 100,
          monthly_limit_cents: 150_000,
          included_used_cents: 150_000,
          billing_period_end: '2026-08-01T00:00:00Z',
          plan: 'SuperGrok'
        }
      }),
      todayStats: today,
      t
    })

    expect(presentation.windows).toHaveLength(2)
    expect(presentation.windows[1]?.utilization).toBe(100)
    expect(presentation.windows[1]?.detail).toContain('$1,500.00')
    expect(presentation.statusTone).toBe('danger')
    expect(presentation.plan).toBe('SuperGrok')
  })

  it('没有官方额度时只保留本地消耗，不伪造百分比窗口', () => {
    const presentation = buildAccountUsagePresentation({
      account: makeAccount({ type: 'apikey' }),
      usageInfo: null,
      todayStats: today,
      t
    })

    expect(presentation.windows).toEqual([])
    expect(presentation.today).toEqual(today)
    expect(presentation.statusTone).toBe('success')
  })
})
