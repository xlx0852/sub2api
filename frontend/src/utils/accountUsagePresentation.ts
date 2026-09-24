import type { Account, AccountUsageInfo, MirasimQuotaWindow, WindowStats } from '@/types'

export type AccountUsageTone = 'neutral' | 'success' | 'warning' | 'danger'

export interface AccountUsageWindowPresentation {
  key: string
  label: string
  utilization: number
  resetsAt: string | null
  detail?: string | null
  displayPercent?: string | null
  clockOffsetMs?: number
  stats?: WindowStats | null
  color: 'indigo' | 'emerald' | 'purple' | 'amber' | 'cyan'
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

const clampPercent = (value: number): number => Math.max(0, Math.min(100, Math.round(value * 100) / 100))

const makeQuotaWindow = (
  key: string,
  label: string,
  utilization: number,
  resetsAt: string | null | undefined,
  color: AccountUsageWindowPresentation['color'],
  stats?: WindowStats | null,
  detail?: string | null,
  displayPercent?: string | null,
  clockOffsetMs = 0
): AccountUsageWindowPresentation => ({
  key,
  label,
  utilization: clampPercent(utilization),
  resetsAt: resetsAt || null,
  color,
  stats: stats || null,
  detail: detail || null,
  displayPercent: displayPercent || null,
  clockOffsetMs
})

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

const antigravityWindowLabel = (label: string, window: string | undefined): string => {
  const normalized = `${window || ''} ${label || ''}`.trim().toLowerCase()
  if (normalized.includes('weekly') || normalized.includes('week')) {
    return '7d'
  }
  if (normalized.includes('5h') || normalized.includes('5 hour') || normalized.includes('5-hour')) {
    return '5h'
  }
  if (normalized.includes('daily') || normalized.includes('day')) {
    return '1d'
  }
  return window || label || 'quota'
}

const getAntigravityWindowOrder = (label: string): number => {
  const norm = label.toLowerCase()
  if (norm.startsWith('5h') || norm.includes('5 hour') || norm.includes('5-hour')) return 1
  if (norm.startsWith('1d') || norm.includes('day') || norm.includes('daily')) return 2
  if (norm.startsWith('7d') || norm.includes('week') || norm.includes('weekly')) return 3
  return 4
}

const formatWindowMinutesLabel = (minutes?: number | null, fallback = 'quota'): string => {
  if (typeof minutes !== 'number' || minutes <= 0) return fallback
  if (minutes >= 20 * 24 * 60) return `${Math.round(minutes / (24 * 60))}d`
  if (minutes >= 36 * 60) {
    const days = minutes / (24 * 60)
    return Number.isInteger(days) ? `${days}d` : `${days.toFixed(1)}d`
  }
  if (minutes >= 90) {
    const hours = minutes / 60
    return Number.isInteger(hours) ? `${hours}h` : `${hours.toFixed(1)}h`
  }
  return `${minutes}m`
}

const resolveUsageProgressLabel = (
  progress: NonNullable<AccountUsageInfo['five_hour']>,
  fallback: string
): string => {
  if (progress.window_label) return progress.window_label
  return formatWindowMinutesLabel(progress.window_minutes, fallback)
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
    resolveUsageProgressLabel(progress, label),
    progress.utilization,
    progress.resets_at,
    color,
    progress.window_stats
  ))
}

const formatCompactNumber = (value: number): string => {
  if (!Number.isFinite(value)) return '0'
  const abs = Math.abs(value)
  if (abs >= 1_000_000) return `${(value / 1_000_000).toFixed(abs % 1_000_000 === 0 ? 0 : 1)}M`
  if (abs >= 1_000) return `${(value / 1_000).toFixed(abs % 1_000 === 0 ? 0 : 1)}K`
  return String(Math.round(value * 100) / 100)
}

// Mirasim relay 额度窗口：上游窗口名不可预测（5h/7d/model_scoped 都可能出现），
// 因此标签优先用后端归一化出的 window_key，其余按上游原样展示。
const mirasimWindowLabel = (window: MirasimQuotaWindow): string => {
  if (window.window_key === 'five_hour') return '5h'
  if (window.window_key === 'seven_day') return '7d'
  return window.name || window.label || 'quota'
}

const mirasimWindowColor = (window: MirasimQuotaWindow): AccountUsageWindowPresentation['color'] => {
  if (window.window_key === 'five_hour') return 'indigo'
  if (window.window_key === 'seven_day') return 'emerald'
  if (window.model_scoped) return 'cyan'
  return 'purple'
}

const mirasimWindowDetail = (window: MirasimQuotaWindow, t: Translator): string => {
  const amount = window.unlimited
    ? t('admin.accounts.usageWindow.mirasimUnlimited')
    : `${formatCompactNumber(window.used)} / ${formatCompactNumber(window.budget)}`
  return window.model_scoped ? `${amount} · ${t('admin.accounts.usageWindow.mirasimModelScoped')}` : amount
}

const addMirasimWindows = (
  windows: AccountUsageWindowPresentation[],
  quotaWindows: MirasimQuotaWindow[],
  t: Translator
) => {
  quotaWindows.forEach((window, index) => {
    windows.push(makeQuotaWindow(
      // 上游同名窗口可能重复（尤其 model_scoped），用下标保证 key 唯一。
      `mirasim-${index}-${window.window_key || window.name}`,
      mirasimWindowLabel(window),
      // 不限量窗口没有百分比语义，固定 0 而不是伪造成满额。
      window.unlimited ? 0 : window.utilization ?? 0,
      window.reset_at || null,
      mirasimWindowColor(window),
      null,
      mirasimWindowDetail(window, t)
    ))
  })
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
  if (typeof plan !== 'string') return null
  const normalized = plan.trim()
  if (account.platform === 'kimi' && normalized.startsWith('LEVEL_')) {
    return normalized
      .slice('LEVEL_'.length)
      .toLowerCase()
      .replace(/(^|_)([a-z])/g, (_, prefix: string, letter: string) => `${prefix ? ' ' : ''}${letter.toUpperCase()}`)
  }
  if (account.platform === 'openai') {
    return normalized.toUpperCase()
  }
  return normalized
}

export const buildAccountUsagePresentation = ({
  account,
  usageInfo,
  todayStats,
  t
}: BuildOptions): AccountUsagePresentation => {
  const windows: AccountUsageWindowPresentation[] = []
  const diagnostics: AccountUsageDiagnostic[] = []

  if (account.platform === 'openai' || account.platform === 'anthropic' || account.platform === 'kimi') {
    // Mirasim relay 账号（platform 仍是 anthropic）后端会把 /v1/limits 的**全部**窗口
    // 归一化到 mirasim_windows（5h/7d 同时映射到 five_hour/seven_day 以兼容其它视图）。
    // 这里优先渲染上游真实窗口，避免与 five_hour/seven_day 重复展示。
    const mirasimWindows = account.platform === 'anthropic' ? usageInfo?.mirasim_windows || [] : []
    if (mirasimWindows.length > 0) {
      addMirasimWindows(windows, mirasimWindows, t)
    } else {
      addUsageProgress(windows, 'five-hour', '5h', usageInfo?.five_hour || null, 'indigo')
      addUsageProgress(windows, 'seven-day', '7d', usageInfo?.seven_day || null, 'emerald')
      addUsageProgress(windows, 'seven-day-sonnet', '7d S', usageInfo?.seven_day_sonnet || null, 'purple')
      addUsageProgress(windows, 'seven-day-fable', '7d F', usageInfo?.seven_day_fable || null, 'amber')
    }
  } else if (account.platform === 'grok') {
    const billing = usageInfo?.grok_billing
    if (billing?.period_type?.toLowerCase() === 'weekly' && billing.usage_percent != null) {
      windows.push(makeQuotaWindow(
        'grok-weekly',
        t('admin.accounts.usageWindow.grokWeekly'),
        billing.usage_percent,
        billing.period_end,
        'indigo',
        usageInfo?.grok_local_usage || todayStats
      ))
    }
  } else if (account.platform === 'gemini') {
    addUsageProgress(windows, 'gemini-shared', '1d', usageInfo?.gemini_shared_daily || null, 'indigo')
    addUsageProgress(windows, 'gemini-pro', t('admin.accounts.usageWindow.geminiProDaily'), usageInfo?.gemini_pro_daily || null, 'indigo')
    addUsageProgress(windows, 'gemini-flash', t('admin.accounts.usageWindow.geminiFlashDaily'), usageInfo?.gemini_flash_daily || null, 'emerald')
  } else if (account.platform === 'antigravity') {
    const quotaGroups = usageInfo?.antigravity_quota_groups || []
    const serverTime = usageInfo?.antigravity_server_time
      ? new Date(usageInfo.antigravity_server_time).getTime()
      : Number.NaN
    const observedAt = usageInfo?.updated_at ? new Date(usageInfo.updated_at).getTime() : Number.NaN
    const clockOffsetMs = Number.isFinite(serverTime) && Number.isFinite(observedAt)
      ? serverTime - observedAt
      : 0
    const byWindow = new Map<string, { utilization: number; resetsAt: string | null; stats: WindowStats | null }>()
    for (const group of quotaGroups) {
      for (const bucket of group.buckets || []) {
        const remaining = Math.max(0, Math.min(1, Number(bucket.remaining_fraction) || 0))
        const label = antigravityWindowLabel(bucket.label, bucket.window)
        const utilization = (1 - remaining) * 100
        const current = byWindow.get(label)
        if (!current || utilization > current.utilization) {
          byWindow.set(label, { utilization, resetsAt: bucket.reset_time || null, stats: bucket.window_stats || null })
        } else if (!current.resetsAt || (bucket.reset_time && bucket.reset_time < current.resetsAt)) {
          current.resetsAt = bucket.reset_time || null
        }
      }
    }
    const sortedWindows = Array.from(byWindow.entries()).sort(
      ([labelA], [labelB]) => getAntigravityWindowOrder(labelA) - getAntigravityWindowOrder(labelB)
    )
    for (const [label, value] of sortedWindows) {
      windows.push(makeQuotaWindow(
        `antigravity-${label}`,
        label,
        value.utilization,
        value.resetsAt,
        label === '7d' ? 'emerald' : 'indigo',
        value.stats,
        null,
        null,
        clockOffsetMs
      ))
    }

    // Compatibility fallback for API versions before quota-summary groups.
    const legacyGroups = [
      {
        key: 'antigravity-pro',
        label: t('admin.accounts.usageWindow.gemini3Pro'),
        models: ['gemini-3.1-pro', 'gemini-3.1-pro-high', 'gemini-3.1-pro-low'],
        color: 'indigo' as const
      },
      {
        key: 'antigravity-flash',
        label: t('admin.accounts.usageWindow.gemini3Flash'),
        models: [
          'gemini-3.8-flash',
          'gemini-3.8-flash-high',
          'gemini-3.7-flash',
          'gemini-3.7-flash-high',
          'gemini-3.6-flash',
          'gemini-3.6-flash-high'
        ],
        color: 'emerald' as const
      }
    ]
    if (windows.length === 0) {
      for (const group of legacyGroups) {
        const quota = maxAntigravityQuota(usageInfo, group.models)
        if (!quota) continue
        windows.push(makeQuotaWindow(group.key, group.label, quota.utilization, quota.resetsAt, group.color))
      }
    }
  }

  if (account.type === 'apikey' && windows.length === 0) {
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
		`${quota.used.toFixed(2)} / ${quota.limit.toFixed(2)}`
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
  // Mirasim 只读元数据：paid/degraded 由上游显式给出时透出，缺失就不猜。
  const mirasimWindows = usageInfo?.mirasim_windows || []
  if (mirasimWindows.length > 0 || usageInfo?.mirasim_paid != null || usageInfo?.mirasim_degraded) {
    if (usageInfo?.mirasim_paid != null) {
      diagnostics.push({
        label: t('admin.accounts.usageDetails.mirasimPlan'),
        value: usageInfo.mirasim_paid
          ? t('admin.accounts.usageWindow.mirasimPaid')
          : t('admin.accounts.usageWindow.mirasimFree')
      })
    }
    if (usageInfo?.mirasim_degraded) {
      diagnostics.push({
        label: t('admin.accounts.usageDetails.mirasimDegraded'),
        value: t('admin.accounts.usageWindow.mirasimDegradedHint'),
        tone: 'warning'
      })
    }
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
