import type { AccountProfitSummary, ProfitQuotaWindow } from '@/api/admin/profit'

export type TimelineWindow = ProfitQuotaWindow & { startMs: number; endMs: number }

export type PreparedQuotaAccount = {
  account: AccountProfitSummary
  preferred: ProfitQuotaWindow
  ledgerSeries: TimelineWindow[]
  availabilitySpans: Array<{ startMs: number; endMs: number }>
  nowMs: number
}

export function parseTimelineTime(value?: string): number | null {
  if (!value) return null
  const ms = Date.parse(value)
  return Number.isNaN(ms) ? null : ms
}

export function inferWindowMinutes(kind?: string): number {
  if (kind === '5h') return 300
  if (kind === '7d') return 10080
  if (kind === '30d') return 30 * 24 * 60
  if (kind === '24h') return 1440
  if (kind === 'session') return 300
  return 300
}

export function prepareQuotaAccounts(
  accounts: AccountProfitSummary[],
  nowMs: number
): PreparedQuotaAccount[] {
  const prepared: PreparedQuotaAccount[] = []
  for (const account of accounts.slice(0, 24)) {
    const preferred = pickPreferredWindow(account, nowMs)
    if (!preferred) continue
    prepared.push({
      account,
      preferred,
      ledgerSeries: pickLedgerSeries(account, preferred),
      nowMs,
      availabilitySpans: (account.quota_availability_spans || [])
        .map((span) => ({
          startMs: parseTimelineTime(span.start_at),
          endMs: parseTimelineTime(span.end_at)
        }))
        .filter((span): span is { startMs: number; endMs: number } => (
          span.startMs != null && span.endMs != null && span.endMs > span.startMs
        ))
    })
  }
  return prepared
}

export function windowsForRange(
  prepared: PreparedQuotaAccount,
  rangeStartMs: number,
  rangeEndMs: number
): TimelineWindow[] {
  if (prepared.ledgerSeries.length === 0) {
    return clipToAvailabilitySpans(
      expandWindowOccurrences(prepared.preferred, rangeStartMs, rangeEndMs),
      prepared.availabilitySpans
    )
  }

  const ledgerWindows = prepared.ledgerSeries
    .filter((window) => window.endMs > rangeStartMs && window.startMs < rangeEndMs)
  const latest = prepared.ledgerSeries[prepared.ledgerSeries.length - 1]
  // A lazy-reset vacuum has no trustworthy next start. Keep the explicit gap
  // and wait for real traffic to open the next ledger window.
  if (latest?.status === 'waiting_activation') {
    return clipToAvailabilitySpans(ledgerWindows, prepared.availabilitySpans)
  }

  const latestReal = [...prepared.ledgerSeries]
    .reverse()
    .find((window) => window.status !== 'waiting_activation')
  if (!latestReal) {
    return clipToAvailabilitySpans(ledgerWindows, prepared.availabilitySpans)
  }

  const projectionAnchor: ProfitQuotaWindow = {
    ...prepared.preferred,
    ...latestReal,
    recurring_from_at: prepared.preferred.recurring_from_at || latestReal.recurring_from_at,
    recurring_until_at: prepared.preferred.recurring_until_at || latestReal.recurring_until_at
  }
  const projectionStart = Math.max(latestReal.endMs, prepared.nowMs)
  const futureWindows = expandWindowOccurrences(projectionAnchor, rangeStartMs, rangeEndMs)
    .filter((window) => window.startMs >= projectionStart)
  const windows = [...ledgerWindows, ...futureWindows]
    .sort((a, b) => a.startMs - b.startMs)
  return clipToAvailabilitySpans(windows, prepared.availabilitySpans)
}

export function windowsForDrawerSelection(
  account: AccountProfitSummary,
  nowMs: number,
  selectedStartMs: number,
  selectedEndMs: number,
  kind?: string
): TimelineWindow[] {
  const prepared = prepareQuotaAccounts([account], nowMs)[0]
  if (!prepared) return []
  const durationMs = inferWindowMinutes(kind || prepared.preferred.kind) * 60_000
  const rangeStartMs = selectedStartMs - 10 * durationMs
  const rangeEndMs = Math.max(selectedEndMs, selectedStartMs + 5 * durationMs)
  return windowsForRange(prepared, rangeStartMs, rangeEndMs)
}

export function expandWindowOccurrences(
  window: ProfitQuotaWindow,
  rangeStartMs: number,
  rangeEndMs: number
): TimelineWindow[] {
  const minutes = window.window_minutes && window.window_minutes > 0
    ? window.window_minutes
    : inferWindowMinutes(window.kind)
  const durationMs = Math.max(60_000, minutes * 60_000)
  let anchorEndMs = parseTimelineTime(window.end_at)
  let anchorStartMs = parseTimelineTime(window.start_at)
  const recurringUntilMs = parseTimelineTime(window.recurring_until_at)
  const recurringFromMs = parseTimelineTime(window.recurring_from_at)
  if (anchorEndMs == null && anchorStartMs != null) anchorEndMs = anchorStartMs + durationMs
  if (anchorStartMs == null && anchorEndMs != null) anchorStartMs = anchorEndMs - durationMs
  if (anchorStartMs == null || anchorEndMs == null || rangeEndMs <= rangeStartMs) return []
  if (recurringUntilMs != null && anchorStartMs >= recurringUntilMs) return []
  if (recurringUntilMs != null && anchorEndMs > recurringUntilMs) anchorEndMs = recurringUntilMs
  if (anchorEndMs <= anchorStartMs) return []

  // Preserve the existing end-anchored recurrence semantics even when legacy
  // start_at disagrees slightly with window_minutes.
  const baseEndMs = anchorEndMs
  const baseStartMs = baseEndMs - durationMs
  let firstIndex = Math.floor((rangeStartMs - baseEndMs) / durationMs) + 1
  let lastIndex = Math.ceil((rangeEndMs - baseStartMs) / durationMs) - 1

  if (recurringFromMs != null) {
    firstIndex = Math.max(firstIndex, Math.ceil((recurringFromMs - baseStartMs) / durationMs))
  }
  if (recurringUntilMs != null) {
    lastIndex = Math.min(lastIndex, Math.ceil((recurringUntilMs - baseStartMs) / durationMs) - 1)
  }
  if (lastIndex < firstIndex) return []

  const out: TimelineWindow[] = []
  for (let index = firstIndex; index <= lastIndex; index += 1) {
    const startMs = baseStartMs + index * durationMs
    const endMs = recurringUntilMs == null
      ? baseEndMs + index * durationMs
      : Math.min(baseEndMs + index * durationMs, recurringUntilMs)
    if (endMs > startMs && endMs > rangeStartMs && startMs < rangeEndMs) {
      out.push({
        ...window,
        id: `${window.id}-projected-${index}`,
        source: 'projected',
        closed_reason: undefined,
        status: undefined,
        is_open: undefined,
        startMs,
        endMs
      })
    }
  }
  return out
}

function pickPreferredWindow(account: AccountProfitSummary, nowMs: number): ProfitQuotaWindow | null {
  const windows = account.quota_windows || []
  let preferred: ProfitQuotaWindow | null = null
  let preferredRank = Number.POSITIVE_INFINITY
  for (const window of windows) {
    const rank = windowRank(window.kind)
    if (rank < preferredRank) {
      preferred = window
      preferredRank = rank
    }
  }
  if (preferred) return preferred
  if (account.seven_day_utilization != null) {
    return synthesizeFromUtil('7d', account.seven_day_utilization, 10080, nowMs)
  }
  if (account.five_hour_utilization != null) {
    return synthesizeFromUtil('5h', account.five_hour_utilization, 300, nowMs)
  }
  return null
}

function pickLedgerSeries(
  account: AccountProfitSummary,
  preferred: ProfitQuotaWindow
): TimelineWindow[] {
  const rows = account.quota_windows || []
  const out: TimelineWindow[] = []
  for (const window of rows) {
    if (window.kind !== preferred.kind) continue
    const startMs = parseTimelineTime(window.start_at)
    const endMs = parseTimelineTime(window.end_at)
    if (startMs == null || endMs == null || endMs <= startMs) continue
    out.push({ ...window, startMs, endMs })
  }
  if (!out.some(isLedgerBackedWindow)) return []
  out.sort((a, b) => a.startMs - b.startMs)
  return out
}

function isLedgerBackedWindow(window: ProfitQuotaWindow): boolean {
  return typeof window.is_open === 'boolean'
    || Boolean(window.closed_reason)
    || window.status === 'waiting_activation'
    || Boolean(window.source && window.source !== 'projected')
}

function clipToAvailabilitySpans(
  windows: TimelineWindow[],
  spans: Array<{ startMs: number; endMs: number }>
): TimelineWindow[] {
  if (!spans.length) return windows
  const clipped: TimelineWindow[] = []
  for (const window of windows) {
    for (const span of spans) {
      const startMs = Math.max(window.startMs, span.startMs)
      const endMs = Math.min(window.endMs, span.endMs)
      if (endMs > startMs) clipped.push({ ...window, startMs, endMs })
    }
  }
  return clipped
}

function windowRank(kind?: string): number {
  if (kind === '7d' || kind === '30d') return 0
  if (kind === '5h') return 1
  if (kind === '24h') return 2
  if (kind === 'session') return 3
  return 4
}

function synthesizeFromUtil(
  kind: string,
  usedPercent: number,
  minutes: number,
  nowMs: number
): ProfitQuotaWindow {
  const endMs = nowMs + minutes * 60_000
  return {
    id: kind,
    label: kind,
    kind,
    used_percent: usedPercent,
    start_at: new Date(endMs - minutes * 60_000).toISOString(),
    end_at: new Date(endMs).toISOString(),
    window_minutes: minutes
  }
}
