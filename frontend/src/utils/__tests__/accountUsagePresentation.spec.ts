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

  it('Grok 只映射官方周额度到窗口模型', () => {
    const presentation = buildAccountUsagePresentation({
      account: makeAccount({ platform: 'grok' }),
      usageInfo: baseUsage({
        grok_local_usage: {
          requests: 28,
          tokens: 2800,
          cost: 2.8,
          standard_cost: 2.8,
          user_cost: 0.56
        },
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

    expect(presentation.windows).toHaveLength(1)
    expect(presentation.windows[0]?.label).toBe('admin.accounts.usageWindow.grokWeekly')
    expect(presentation.windows[0]?.utilization).toBe(28)
    expect(presentation.windows[0]?.stats?.requests).toBe(28)
    expect(presentation.statusTone).toBe('success')
    expect(presentation.plan).toBe('SuperGrok')
  })

  it('Antigravity 使用 quota summary bucket 并将剩余额度换算为已用进度', () => {
    const presentation = buildAccountUsagePresentation({
      account: makeAccount({ platform: 'antigravity' }),
      usageInfo: baseUsage({
        antigravity_quota_groups: [{
          id: 'gemini-models',
          label: 'Gemini Models',
          buckets: [{
            id: 'five-hour',
            label: '5 Hour Limit',
            window: '5h',
            remaining_fraction: 0.72,
            reset_time: '2026-07-12T05:00:00Z'
          }]
        }]
      }),
      todayStats: today,
      t
    })

    expect(presentation.windows).toHaveLength(1)
    expect(presentation.windows[0]).toMatchObject({
      label: '5h',
      utilization: 28,
      resetsAt: '2026-07-12T05:00:00Z',
      displayPercent: null
    })
  })

  it('Antigravity 多个配额窗口必须保持 5h 在上、7d 在下', () => {
    const presentation = buildAccountUsagePresentation({
      account: makeAccount({ platform: 'antigravity' }),
      usageInfo: baseUsage({
        antigravity_quota_groups: [{
          id: 'gemini-models',
          label: 'Gemini Models',
          buckets: [
            {
              id: 'weekly',
              label: 'Weekly Limit',
              window: 'weekly',
              remaining_fraction: 0.9038,
              reset_time: '2026-07-19T05:00:00Z'
            },
            {
              id: 'five-hour',
              label: '5 Hour Limit',
              window: '5h',
              remaining_fraction: 0.4933,
              reset_time: '2026-07-12T05:00:00Z'
            }
          ]
        }]
      }),
      todayStats: today,
      t
    })

    expect(presentation.windows).toHaveLength(2)
    expect(presentation.windows[0].label).toBe('5h')
    expect(presentation.windows[1].label).toBe('7d')
    expect(presentation.windows[0].color).toBe('indigo')
    expect(presentation.windows[1].color).toBe('emerald')
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
