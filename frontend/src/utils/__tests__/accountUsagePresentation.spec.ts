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

  it('Devin external bridge 的日窗口应显示 24h 而不是 5h', () => {
    const presentation = buildAccountUsagePresentation({
      account: makeAccount({
        type: 'apikey',
        extra: { external_bridge: true, external_bridge_kind: 'devin' }
      }),
      usageInfo: baseUsage({
        source: 'active',
        five_hour: {
          utilization: 18,
          resets_at: '2026-09-14T08:00:00Z',
          remaining_seconds: 3600,
          window_minutes: 24 * 60,
          window_label: '24h'
        },
        seven_day: {
          utilization: 41,
          resets_at: '2026-09-20T08:00:00Z',
          remaining_seconds: 86400,
          window_minutes: 7 * 24 * 60,
          window_label: '7d'
        }
      }),
      todayStats: today,
      t
    })

    expect(presentation.windows.map(window => window.label)).toEqual(['24h', '7d'])
  })

  it('OpenAI 订阅等级使用大写展示', () => {
    const presentation = buildAccountUsagePresentation({
      account: makeAccount(),
      usageInfo: baseUsage({ subscription_tier: 'pro' }),
      todayStats: today,
      t
    })

    expect(presentation.plan).toBe('PRO')
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
    const stats: WindowStats = {
      requests: 20,
      tokens: 5000,
      cost: 2,
      full_requests: 72,
      full_tokens: 18000,
      full_cost: 7.2,
      full_estimate: { method: 'cumulative', confidence: 'low', sample_count: 1, used_percent_span: 28 }
    }
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
            reset_time: '2026-07-12T05:00:00Z',
            window_stats: stats
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
    expect(presentation.windows[0]?.stats).toEqual(stats)
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

  it('Mirasim relay 优先渲染归一化后的全部窗口，不重复展示 five_hour/seven_day', () => {
    const presentation = buildAccountUsagePresentation({
      account: makeAccount({ platform: 'anthropic', type: 'apikey', extra: { mirasim_relay_enabled: true } }),
      usageInfo: baseUsage({
        source: 'active',
        // 后端会把 5h/7d 同时映射到 five_hour/seven_day，这里刻意重复，确保不会渲染两次
        five_hour: { utilization: 98.4, resets_at: '2026-07-12T05:00:00Z', remaining_seconds: 0 },
        seven_day: { utilization: 12.5, resets_at: '2026-07-18T00:00:00Z', remaining_seconds: 0 },
        mirasim_paid: true,
        mirasim_windows: [
          {
            name: '5h',
            label: '5h',
            budget: 40_000_000,
            used: 39_360_000,
            utilization: 98.4,
            remaining_percent: 1.6,
            status: 'warning',
            reset_at: '2026-07-12T05:00:00Z',
            window_key: 'five_hour'
          },
          {
            name: '7d',
            label: '7d',
            budget: 400_000_000,
            used: 50_000_000,
            utilization: 12.5,
            remaining_percent: 87.5,
            status: 'allowed',
            reset_at: '2026-07-18T00:00:00Z',
            window_key: 'seven_day'
          },
          {
            name: 'claude-opus-4-1',
            label: 'claude-opus-4-1',
            budget: 0,
            used: 1234,
            unlimited: true,
            status: 'allowed',
            model_scoped: true
          }
        ]
      }),
      todayStats: today,
      t
    })

    expect(presentation.windows.map(window => window.label)).toEqual(['5h', '7d', 'claude-opus-4-1'])
    expect(presentation.windows).toHaveLength(3)
    expect(presentation.windows[0].utilization).toBe(98.4)
    expect(presentation.windows[0].color).toBe('indigo')
    expect(presentation.windows[0].resetsAt).toBe('2026-07-12T05:00:00Z')
    expect(presentation.windows[0].detail).toBe('39.4M / 40M')
    expect(presentation.windows[1].color).toBe('emerald')
    expect(presentation.windows[1].detail).toBe('50M / 400M')
    // 不限量窗口不能伪造成满额
    expect(presentation.windows[2].utilization).toBe(0)
    expect(presentation.windows[2].color).toBe('cyan')
    expect(presentation.windows[2].detail).toBe('admin.accounts.usageWindow.mirasimUnlimited · admin.accounts.usageWindow.mirasimModelScoped')
    // 唯一 key，避免上游同名 model_scoped 窗口互相覆盖
    expect(new Set(presentation.windows.map(window => window.key)).size).toBe(3)
    expect(presentation.statusTone).toBe('warning')
    expect(presentation.diagnostics).toContainEqual({
      label: 'admin.accounts.usageDetails.mirasimPlan',
      value: 'admin.accounts.usageWindow.mirasimPaid'
    })
    expect(presentation.diagnostics.some(item => item.label === 'admin.accounts.usageDetails.mirasimDegraded')).toBe(false)
  })

  it('Mirasim 免费且上游降级时诊断里透出 free 与降级告警', () => {
    const presentation = buildAccountUsagePresentation({
      account: makeAccount({ platform: 'anthropic', type: 'apikey', extra: { mirasim_relay_enabled: true } }),
      usageInfo: baseUsage({
        source: 'active',
        mirasim_paid: false,
        mirasim_degraded: true,
        error: 'upstream',
        mirasim_windows: []
      }),
      todayStats: today,
      t
    })

    expect(presentation.diagnostics).toContainEqual({
      label: 'admin.accounts.usageDetails.mirasimPlan',
      value: 'admin.accounts.usageWindow.mirasimFree'
    })
    expect(presentation.diagnostics).toContainEqual({
      label: 'admin.accounts.usageDetails.mirasimDegraded',
      value: 'admin.accounts.usageWindow.mirasimDegradedHint',
      tone: 'warning'
    })
    // 降级不得影响窗口渲染路径（空窗口时回落到 0 个窗口，而不是报错）
    expect(presentation.windows).toEqual([])
  })

  it('非 mirasim 账号不受 mirasim_windows 分支影响', () => {
    const presentation = buildAccountUsagePresentation({
      account: makeAccount({ platform: 'anthropic', type: 'oauth' }),
      usageInfo: baseUsage({
        source: 'active',
        five_hour: { utilization: 10, resets_at: '2026-07-12T05:00:00Z', remaining_seconds: 0 },
        seven_day: { utilization: 20, resets_at: '2026-07-18T00:00:00Z', remaining_seconds: 0 }
      }),
      todayStats: today,
      t
    })

    expect(presentation.windows.map(window => window.label)).toEqual(['5h', '7d'])
    expect(presentation.diagnostics.some(item => String(item.label).includes('mirasim'))).toBe(false)
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
