import type { Account, AccountUsageInfo, WindowStats } from '@/types'

export type AccountUsageTone = 'neutral' | 'success' | 'warning' | 'danger'

export interface AccountUsageWindowPresentation {
  key: string
  label: string
  utilization: number
  resetsAt: string | null
  detail?: string | null
  stats?: WindowStats | null
  color: 'indigo' | 'emerald' | 'purple' | 'amber'
}

export interface AccountUsageDiagnostic {
  label: string
  value: string
  tone?: AccountUsageTone
}

export interface AccountUsagePresentation {
  windows: AccountUsageWindowPresentation[]
  today: WindowStats | null
  plan: string | null
  statusLabel: string
  statusTone: AccountUsageTone
  sourceLabel: string
  updatedAt: string | null
  diagnostics: AccountUsageDiagnostic[]
  error: string | null
  needsReauth: boolean
}

type Translator = (key: string, params?: Record<string, unknown>) => string

interface BuildOptions {
  account: Account
  usageInfo: AccountUsageInfo | null
  todayStats: WindowStats | null
  t: Translator
}

const clampPercent = (value: number): number => Math.max(0, Math.min(100, value))

const makeQuotaWindow = (
  key: string,
  label: string,
  utilization: number,
  resetsAt: string | null | undefined,
  color: AccountUsageWindowPresentation['color'],
  stats?: WindowStats | null,
  detail?: string | null
): AccountUsageWindowPresentation => ({
  key,
  label,
  utilization: clampPercent(utilization),
  resetsAt: resetsAt || null,
  color,
  stats: stats || null,
  detail: detail || null
})

const makeRateLimitWindow = (
  key: string,
  label: string,
  quota: { limit?: number | null; remaining?: number | null; reset_at?: string | null } | null | undefined,
  color: AccountUsageWindowPresentation['color']
): AccountUsageWindowPresentation | null => {
  if (!quota || quota.limit == null || quota.remaining == null || quota.limit <= 0) return null
  const used = Math.max(0, quota.limit - quota.remaining)
  return makeQuotaWindow(key, label, (used / quota.limit) * 100, quota.reset_at, color)
}

const formatUsdFromCents = (value: number | null | undefined): string | null => {
  if (value == null || !Number.isFinite(value)) return null
  return new Intl.NumberFormat(undefined, { style: 'currency', currency: 'USD' }).format(value / 100)
}

const quotaResetAt = (account: Account, kind: 'daily' | 'weekly'): string | null => {
  const extra = account.extra as Record<string, unknown> | undefined
  const resetAt = extra?.[`quota_${kind}_reset_at`]
  if (typeof resetAt === 'string' && resetAt) return resetAt

  const start = extra?.[`quota_${kind}_start`]
  if (typeof start !== 'string' || !start) return null
  const startedAt = new Date(start).getTime()
  if (!Number.isFinite(startedAt)) return null
  const duration = kind === 'daily' ? 24 * 60 * 60 * 1000 : 7 * 24 * 60 * 60 * 1000
  return new Date(startedAt + duration).toISOString()
}

const maxAntigravityQuota = (
  usageInfo: AccountUsageInfo | null,
  models: string[]
): { utilization: number; resetsAt: string | null } | null => {
  const quota = usageInfo?.antigravity_quota
  if (!quota) return null

  const entries = models.map(model => quota[model]).filter(Boolean)
  if (entries.length === 0) return null
  return {
    utilization: Math.max(...entries.map(entry => entry.utilization || 0)),
    resetsAt: entries
      .map(entry => entry.reset_time)
      .filter(Boolean)
      .sort()[0] || null
  }
}

const addUsageProgress = (
  windows: AccountUsageWindowPresentation[],
  key: string,
  label: string,
  progress: AccountUsageInfo['five_hour'],
  color: AccountUsageWindowPresentation['color']
) => {
  if (!progress) return
  windows.push(makeQuotaWindow(
    key,
    label,
    progress.utilization,
    progress.resets_at,
    color,
    progress.window_stats
  ))
}

const readPlan = (account: Account, usageInfo: AccountUsageInfo | null): string | null => {
  const extra = account.extra as Record<string, unknown> | undefined
  const credentials = account.credentials as Record<string, unknown> | undefined
  const grokBilling = usageInfo?.grok_billing
  const candidates = [
    grokBilling?.plan,
    usageInfo?.subscription_tier,
    extra?.subscription_tier,
    credentials?.subscription_tier,
    credentials?.plan_type,
    extra?.plan_type
  ]
  const plan = candidates.find(value => typeof value === 'string' && value.trim())
  return typeof plan === 'string' ? plan.trim() : null
}

export const buildAccountUsagePresentation = ({
  account,
  usageInfo,
  todayStats,
  t
}: BuildOptions): AccountUsagePresentation => {
  const windows: AccountUsageWindowPresentation[] = []
  const diagnostics: AccountUsageDiagnostic[] = []

  if (account.platform === 'openai' || account.platform === 'anthropic') {
    addUsageProgress(windows, 'five-hour', '5h', usageInfo?.five_hour || null, 'indigo')
    addUsageProgress(windows, 'seven-day', '7d', usageInfo?.seven_day || null, 'emerald')
    addUsageProgress(windows, 'seven-day-sonnet', '7d S', usageInfo?.seven_day_sonnet || null, 'purple')
    addUsageProgress(windows, 'seven-day-fable', '7d F', usageInfo?.seven_day_fable || null, 'amber')
  } else if (account.platform === 'grok') {
    const billing = usageInfo?.grok_billing
    if (billing?.period_type?.toLowerCase() === 'weekly' && billing.usage_percent != null) {
      windows.push(makeQuotaWindow(
        'grok-weekly',
        t('admin.accounts.usageWindow.grokWeekly'),
        billing.usage_percent,
        billing.period_end,
        'indigo'
      ))
    }
    if (billing?.used_percent != null) {
      const total = formatUsdFromCents(billing.monthly_limit_cents)
      const used = formatUsdFromCents(billing.included_used_cents ?? billing.used_cents)
      windows.push(makeQuotaWindow(
        'grok-monthly',
        t('admin.accounts.usageWindow.grokMonthly'),
        billing.used_percent,
        billing.billing_period_end || billing.period_end,
        'amber',
        null,
        used && total ? `${used} / ${total}` : null
      ))
    }
    if (windows.length === 0) {
      const requestWindow = makeRateLimitWindow(
        'grok-requests',
        t('admin.accounts.usageWindow.grokRequests'),
        usageInfo?.grok_request_quota,
        'indigo'
      )
      const tokenWindow = makeRateLimitWindow(
        'grok-tokens',
        t('admin.accounts.usageWindow.grokTokens'),
        usageInfo?.grok_token_quota,
        'emerald'
      )
      if (requestWindow) windows.push(requestWindow)
      if (tokenWindow) windows.push(tokenWindow)
    }
  } else if (account.platform === 'gemini') {
    addUsageProgress(windows, 'gemini-shared', '1d', usageInfo?.gemini_shared_daily || null, 'indigo')
    addUsageProgress(windows, 'gemini-pro', t('admin.accounts.usageWindow.geminiProDaily'), usageInfo?.gemini_pro_daily || null, 'indigo')
    addUsageProgress(windows, 'gemini-flash', t('admin.accounts.usageWindow.geminiFlashDaily'), usageInfo?.gemini_flash_daily || null, 'emerald')
  } else if (account.platform === 'antigravity') {
    const antigravityGroups = [
      {
        key: 'antigravity-pro',
        label: t('admin.accounts.usageWindow.gemini3Pro'),
        models: ['gemini-3-pro-low', 'gemini-3-pro-high', 'gemini-3.1-pro-high'],
        color: 'indigo' as const
      },
      {
        key: 'antigravity-flash',
        label: t('admin.accounts.usageWindow.gemini3Flash'),
        models: ['gemini-3-flash'],
        color: 'emerald' as const
      },
      {
        key: 'antigravity-image',
        label: t('admin.accounts.usageWindow.gemini3Image'),
        models: ['gemini-2.5-flash-image', 'gemini-3.1-flash-image', 'gemini-3-pro-image'],
        color: 'purple' as const
      },
      {
        key: 'antigravity-claude',
        label: t('admin.accounts.usageWindow.claude'),
        models: ['claude-fable-5', 'claude-sonnet-4-5', 'claude-opus-4-5-thinking', 'claude-sonnet-4-6', 'claude-opus-4-6', 'claude-opus-4-6-thinking', 'claude-opus-4-7', 'claude-opus-4-8'],
        color: 'amber' as const
      }
    ]
    for (const group of antigravityGroups) {
      const quota = maxAntigravityQuota(usageInfo, group.models)
      if (!quota) continue
      windows.push(makeQuotaWindow(group.key, group.label, quota.utilization, quota.resetsAt, group.color))
    }
  }

  if ((account.type === 'apikey' || account.type === 'bedrock') && windows.length === 0) {
    const quotaRows = [
      {
        key: 'quota-daily',
        label: '1d',
        used: account.quota_daily_used ?? 0,
        limit: account.quota_daily_limit ?? 0,
        resetAt: quotaResetAt(account, 'daily'),
        color: 'indigo' as const
      },
      {
        key: 'quota-weekly',
        label: '7d',
        used: account.quota_weekly_used ?? 0,
        limit: account.quota_weekly_limit ?? 0,
        resetAt: quotaResetAt(account, 'weekly'),
        color: 'emerald' as const
      },
      {
        key: 'quota-total',
        label: 'total',
        used: account.quota_used ?? 0,
        limit: account.quota_limit ?? 0,
        resetAt: null,
        color: 'purple' as const
      }
    ]
    for (const quota of quotaRows) {
      if (quota.limit <= 0) continue
      windows.push(makeQuotaWindow(
        quota.key,
        quota.label,
        (quota.used / quota.limit) * 100,
        quota.resetAt,
        quota.color,
        null,
        `$${quota.used.toFixed(2)} / $${quota.limit.toFixed(2)}`
      ))
    }
  }

  const maxUsage = windows.length > 0 ? Math.max(...windows.map(window => window.utilization)) : null
  let statusTone: AccountUsageTone = 'success'
  let statusLabel = t('admin.accounts.usageDetails.statusNormal')
  if (usageInfo?.needs_reauth) {
    statusTone = 'danger'
    statusLabel = t('admin.accounts.usageDetails.statusNeedsReauth')
  } else if (usageInfo?.is_forbidden) {
    statusTone = 'danger'
    statusLabel = t('admin.accounts.usageDetails.statusUnavailable')
  } else if (usageInfo?.error) {
    statusTone = 'warning'
    statusLabel = t('admin.accounts.usageDetails.statusRefreshFailed')
  } else if (maxUsage != null && maxUsage >= 100) {
    statusTone = 'danger'
    statusLabel = t('admin.accounts.usageDetails.statusExhausted')
  } else if (maxUsage != null && maxUsage >= 80) {
    statusTone = 'warning'
    statusLabel = t('admin.accounts.usageDetails.statusNearLimit')
  } else if (windows.length === 0 && !todayStats) {
    statusTone = 'neutral'
    statusLabel = t('admin.accounts.usageDetails.statusNoData')
  }

  const sourceLabel = usageInfo?.source === 'active'
    ? t('admin.accounts.usageDetails.sourceActive')
    : usageInfo?.source === 'passive'
      ? t('admin.accounts.usageDetails.sourcePassive')
      : t('admin.accounts.usageDetails.sourceLocal')

  diagnostics.push({ label: t('admin.accounts.usageDetails.dataSource'), value: sourceLabel })
  diagnostics.push({
    label: t('admin.accounts.usageDetails.lastUpdated'),
    value: usageInfo?.updated_at || usageInfo?.grok_last_quota_probe_at || usageInfo?.grok_billing?.fetched_at || '-'
  })
  if (usageInfo?.grok_last_status_code) {
    diagnostics.push({
      label: t('admin.accounts.usageDetails.responseStatus'),
      value: String(usageInfo.grok_last_status_code),
      tone: usageInfo.grok_last_status_code >= 400 ? 'warning' : 'success'
    })
  }
  if (usageInfo?.error) {
    diagnostics.push({ label: t('admin.accounts.usageDetails.lastError'), value: usageInfo.error, tone: 'warning' })
  }

  return {
    windows,
    today: usageInfo?.grok_local_usage || todayStats || null,
    plan: readPlan(account, usageInfo),
    statusLabel,
    statusTone,
    sourceLabel,
    updatedAt: usageInfo?.updated_at || usageInfo?.grok_last_quota_probe_at || usageInfo?.grok_billing?.fetched_at || null,
    diagnostics,
    error: usageInfo?.error || null,
    needsReauth: Boolean(usageInfo?.needs_reauth)
  }
}
