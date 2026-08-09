<template>
  <section
    data-testid="profit-quota-window-panel"
    class="rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800"
  >
    <div class="flex flex-wrap items-start justify-between gap-3 border-b border-gray-100 px-4 py-3 dark:border-dark-700 sm:px-5">
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.profit.quotaWindowTitle') }}</h3>
          <span class="text-xs text-gray-400 dark:text-dark-400">{{ rangeLabel }}</span>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.profit.quotaWindowHint') }}</p>
      </div>

      <div class="flex items-center gap-1.5">
        <div class="inline-flex rounded-lg bg-gray-100 p-1 dark:bg-dark-700">
          <button
            type="button"
            class="rounded-md px-2 py-1 text-xs font-semibold text-gray-600 transition hover:text-gray-900 dark:text-dark-300 dark:hover:text-white"
            :aria-label="t('admin.profit.quotaWindowPrev')"
            :title="t('admin.profit.quotaWindowPrev')"
            @click="shiftPeriod(-1)"
          >
            ‹
          </button>
          <button
            type="button"
            class="rounded-md px-2 py-1 text-[10px] font-medium text-gray-600 transition hover:text-gray-900 dark:text-dark-300 dark:hover:text-white"
            @click="jumpToCurrent()"
          >
            {{ t('admin.profit.quotaWindowToday') }}
          </button>
          <button
            type="button"
            class="rounded-md px-2 py-1 text-xs font-semibold text-gray-600 transition hover:text-gray-900 dark:text-dark-300 dark:hover:text-white"
            :aria-label="t('admin.profit.quotaWindowNext')"
            :title="t('admin.profit.quotaWindowNext')"
            @click="shiftPeriod(1)"
          >
            ›
          </button>
        </div>
        <div class="inline-flex rounded-lg bg-gray-100 p-1 dark:bg-dark-700">
          <button
            v-for="mode in modes"
            :key="mode.key"
            type="button"
            class="rounded-md px-2.5 py-1 text-xs font-medium transition"
            :class="viewMode === mode.key
              ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white'
              : 'text-gray-500 hover:text-gray-800 dark:text-dark-300 dark:hover:text-white'"
            @click="setMode(mode.key)"
          >
            {{ mode.label }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="!lanes.length" class="px-5 py-10 text-center text-sm text-gray-400 dark:text-dark-400">
      {{ t('admin.profit.quotaWindowEmpty') }}
    </div>

    <div v-else class="px-3 py-3 sm:px-4">
      <!-- Sticky short labels + independently scrollable gantt -->
      <div class="flex min-w-0 items-stretch gap-2">
        <div class="w-[6.75rem] shrink-0 sm:w-[7.5rem]">
          <div class="mb-2 h-7" />
          <div class="space-y-2.5">
            <div
              v-for="lane in lanes"
              :key="`label-${lane.accountId}`"
              class="flex h-9 min-w-0 items-center gap-1.5"
              data-testid="profit-quota-window-lane"
            >
              <span class="h-2 w-2 shrink-0 rounded-full" :class="lane.dotClass" />
              <div class="min-w-0 flex-1">
                <div
                  class="truncate text-[11px] font-semibold leading-tight text-gray-800 dark:text-dark-100"
                  :title="lane.fullTitle"
                >
                  {{ lane.shortName }}
                </div>
                <div class="truncate text-[10px] leading-tight text-gray-400 dark:text-dark-400" :title="lane.fullTitle">
                  {{ lane.kindLabel }} · {{ lane.usedLabel }}
                </div>
              </div>
            </div>
          </div>
        </div>

        <div
          ref="scrollEl"
          class="relative min-w-0 flex-1 overflow-x-auto overscroll-x-contain"
          data-testid="profit-quota-window-scroll"
          @scroll.passive="onGanttScroll"
        >
          <div :style="{ width: `${timelineWidth}px` }">
            <div class="relative mb-2 h-7">
              <div
                v-for="tick in dayTicks"
                :key="tick.key"
                class="absolute top-0 flex -translate-x-1/2 flex-col items-center"
                :style="{ left: `${tick.left}%` }"
              >
                <span class="text-[10px] font-medium uppercase tracking-wide text-gray-400 dark:text-dark-400">{{ tick.weekday }}</span>
                <span class="text-[11px] font-semibold tabular-nums text-gray-600 dark:text-dark-200">{{ tick.day }}</span>
              </div>
              <div
                v-for="mb in monthMarkers"
                :key="`month-${mb.key}`"
                class="pointer-events-none absolute -top-0.5 z-10 flex -translate-x-1/2 flex-col items-center"
                :style="{ left: `${mb.left}%` }"
                data-testid="quota-window-month-marker"
              >
                <span class="rounded-full border border-violet-300 bg-violet-50 px-1.5 py-px text-[9px] font-semibold text-violet-700 dark:border-violet-700 dark:bg-violet-950/60 dark:text-violet-300">
                  {{ mb.label }}
                </span>
              </div>
              <div
                v-if="nowLeft != null"
                class="pointer-events-none absolute bottom-0 top-0 w-px bg-rose-400/80"
                :style="{ left: `${nowLeft}%` }"
              />
            </div>

            <div class="space-y-2.5">
              <div
                v-for="lane in lanes"
                :key="`track-${lane.accountId}`"
                class="relative h-9 rounded-lg bg-gray-50 ring-1 ring-inset ring-gray-100 dark:bg-dark-700/50 dark:ring-dark-600"
              >
                <div
                  v-for="seg in dayTicks"
                  :key="`${lane.accountId}-${seg.key}`"
                  class="pointer-events-none absolute bottom-0 top-0 w-px bg-gray-200/70 dark:bg-dark-600/80"
                  :style="{ left: `${seg.left}%` }"
                />
                <div
                  v-for="mb in monthMarkers"
                  :key="`${lane.accountId}-month-${mb.key}`"
                  class="pointer-events-none absolute bottom-0 top-0 z-10 w-px bg-violet-400/70"
                  :style="{ left: `${mb.left}%` }"
                />
                <div
                  v-if="nowLeft != null"
                  class="pointer-events-none absolute bottom-0 top-0 w-px bg-rose-400/50"
                  :style="{ left: `${nowLeft}%` }"
                />

                <button
                  v-for="bar in lane.bars"
                  :key="bar.key"
                  type="button"
                  class="absolute top-1.5 h-6 cursor-pointer overflow-hidden rounded-md border text-[10px] font-semibold tabular-nums shadow-sm transition hover:brightness-95"
                  :class="bar.className"
                  :style="bar.style"
                  :title="bar.title"
                  @click="emit('select', {
                    accountId: lane.accountId,
                    startMs: bar.startMs,
                    endMs: bar.endMs,
                    status: bar.status,
                    kind: bar.kind,
                    label: bar.labelRaw,
                    windows: lane.windowPayloads
                  })"
                >
                  <div class="flex h-full items-center gap-1 px-1.5">
                    <span class="truncate">{{ bar.label }}</span>
                  </div>
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="mt-4 flex flex-wrap items-center gap-4 text-[11px] text-gray-500 dark:text-dark-400">
        <span class="inline-flex items-center gap-1.5">
          <span class="h-2.5 w-6 rounded-sm bg-slate-500/80" />
          {{ t('admin.profit.quotaWindowCurrent') }}
        </span>
        <span class="inline-flex items-center gap-1.5">
          <span class="h-2.5 w-6 rounded-sm border border-dashed border-slate-400 bg-transparent" />
          {{ t('admin.profit.quotaWindowUpcoming') }}
        </span>
        <span class="inline-flex items-center gap-1.5">
          <span class="h-2.5 w-6 rounded-sm bg-slate-300/80 dark:bg-dark-500" />
          {{ t('admin.profit.quotaWindowEnded') }}
        </span>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AccountProfitSummary, ProfitQuotaWindow } from '@/api/admin/profit'

export type QuotaWindowSelectPayload = {
  accountId: number
  startMs: number
  endMs: number
  status: 'current' | 'upcoming' | 'ended'
  kind?: string
  label?: string
  /** All expanded occurrences currently painted for this lane (for history). */
  windows: Array<{ startMs: number; endMs: number; status: 'current' | 'upcoming' | 'ended'; kind?: string; label?: string }>
}

const emit = defineEmits<{ select: [payload: QuotaWindowSelectPayload] }>()

type ViewMode = 'week' | 'month' | '5h'

const props = defineProps<{
  accounts: AccountProfitSummary[]
}>()

const { t, locale } = useI18n()

const viewMode = ref<ViewMode>('month')
// Continuous timeline: rangeStart/rangeEnd are the rendered horizon; viewport scrolls freely.
const now0 = Date.now()
const rangeStart = ref(startOfLocalDay(new Date(now0 - 30 * 86_400_000)))
const rangeEnd = ref(new Date(now0 + 30 * 86_400_000))
const scrollEl = ref<HTMLElement | null>(null)

const modes = computed(() => [
  { key: 'week' as const, label: t('admin.profit.quotaWindowByWeek') },
  { key: 'month' as const, label: t('admin.profit.quotaWindowByMonth') },
  { key: '5h' as const, label: t('admin.profit.quotaWindowBy5h') }
])

const palette = [
  {
    solid: 'bg-slate-600/85 border-slate-700 text-white dark:bg-slate-500/80 dark:border-slate-400',
    soft: 'bg-slate-200/80 border-slate-300 text-slate-700 dark:bg-dark-600 dark:border-dark-500 dark:text-dark-100',
    upcoming: 'border-dashed border-slate-400 bg-white/40 text-slate-500 dark:bg-transparent dark:text-dark-300',
    dot: 'bg-slate-500'
  },
  {
    solid: 'bg-sky-600/85 border-sky-700 text-white dark:bg-sky-500/80 dark:border-sky-400',
    soft: 'bg-sky-100 border-sky-200 text-sky-800 dark:bg-sky-950/50 dark:border-sky-800 dark:text-sky-200',
    upcoming: 'border-dashed border-sky-400 bg-white/40 text-sky-600 dark:bg-transparent dark:text-sky-300',
    dot: 'bg-sky-500'
  },
  {
    solid: 'bg-violet-600/85 border-violet-700 text-white dark:bg-violet-500/80 dark:border-violet-400',
    soft: 'bg-violet-100 border-violet-200 text-violet-800 dark:bg-violet-950/50 dark:border-violet-800 dark:text-violet-200',
    upcoming: 'border-dashed border-violet-400 bg-white/40 text-violet-600 dark:bg-transparent dark:text-violet-300',
    dot: 'bg-violet-500'
  },
  {
    solid: 'bg-emerald-600/85 border-emerald-700 text-white dark:bg-emerald-500/80 dark:border-emerald-400',
    soft: 'bg-emerald-100 border-emerald-200 text-emerald-800 dark:bg-emerald-950/50 dark:border-emerald-800 dark:text-emerald-200',
    upcoming: 'border-dashed border-emerald-400 bg-white/40 text-emerald-600 dark:bg-transparent dark:text-emerald-300',
    dot: 'bg-emerald-500'
  },
  {
    solid: 'bg-amber-500/90 border-amber-600 text-white dark:bg-amber-500/80 dark:border-amber-400',
    soft: 'bg-amber-100 border-amber-200 text-amber-900 dark:bg-amber-950/40 dark:border-amber-800 dark:text-amber-200',
    upcoming: 'border-dashed border-amber-400 bg-white/40 text-amber-700 dark:bg-transparent dark:text-amber-300',
    dot: 'bg-amber-500'
  }
]

const viewRange = computed(() => ({ start: new Date(rangeStart.value), end: new Date(rangeEnd.value) }))

// Fixed scale: ~36px per day (week/month), ~120px per hour in 5h mode.
const PX_PER_DAY = 36
const PX_PER_HOUR = 140
const timelineWidth = computed(() => {
  const span = Math.max(1, rangeEnd.value.getTime() - rangeStart.value.getTime())
  if (viewMode.value === '5h') {
    const hours = span / 3600_000
    return Math.max(640, Math.round(hours * PX_PER_HOUR))
  }
  const days = span / 86_400_000
  return Math.max(860, Math.round(days * PX_PER_DAY))
})

const viewDurationMs = computed(() => Math.max(1, viewRange.value.end.getTime() - viewRange.value.start.getTime()))

const rangeLabel = computed(() => {
  const { start, end } = viewRange.value
  const scope = viewMode.value === '5h'
    ? t('admin.profit.quotaWindowBy5h')
    : viewMode.value === 'month'
      ? t('admin.profit.quotaWindowByMonth')
      : t('admin.profit.quotaWindowByWeek')
  return t('admin.profit.quotaWindowRange', {
    start: formatShortDate(start),
    end: formatShortDate(new Date(end.getTime() - 1)),
    scope
  })
})

const dayTicks = computed(() => {
  const ticks: Array<{ key: string; left: number; weekday: string; day: string }> = []
  const { start, end } = viewRange.value
  if (viewMode.value === '5h') {
    for (let i = 0; i <= 5; i += 1) {
      const ts = new Date(start.getTime() + i * 3600_000)
      ticks.push({
        key: `h-${i}`,
        left: (i / 5) * 100,
        weekday: '',
        day: formatHour(ts)
      })
    }
    return ticks
  }
  let cursor = startOfLocalDay(start)
  while (cursor < end) {
    const left = ((cursor.getTime() - start.getTime()) / viewDurationMs.value) * 100
    // 月视图格线仍按天，标签隔天显示（1 号必显），避免 30 个 weekday 挤成一团
    const showLabel = viewMode.value !== 'month'
      || cursor.getDate() === 1
      || cursor.getDate() % 2 === 1
    ticks.push({
      key: cursor.toISOString(),
      left,
      weekday: viewMode.value === 'month' ? '' : formatWeekday(cursor),
      day: showLabel
        ? (viewMode.value === 'month' ? formatMonthDay(cursor) : formatDay(cursor))
        : ''
    })
    cursor = new Date(cursor.getTime() + 86_400_000)
  }
  return ticks
})

const monthMarkers = computed(() => {
  const { start, end } = viewRange.value
  const markers: Array<{ key: string; left: number; label: string }> = []
  // Include the first visible day's month, plus every 1st inside the range.
  const first = startOfLocalDay(start)
  const add = (date: Date, label: string) => {
    const ms = date.getTime()
    if (ms < start.getTime() || ms >= end.getTime()) return
    const left = ((ms - start.getTime()) / viewDurationMs.value) * 100
    if (markers.some((m) => Math.abs(m.left - left) < 0.01)) return
    markers.push({ key: date.toISOString(), left, label })
  }
  add(startOfLocalMonth(start), formatMonthYearShort(start))
  let cursor = startOfLocalMonth(first)
  while (cursor < end) {
    add(cursor, formatMonthYearShort(cursor))
    cursor = addMonths(cursor, 1)
  }
  return markers
})

const nowLeft = computed(() => {
  const now = Date.now()
  const { start, end } = viewRange.value
  if (now < start.getTime() || now > end.getTime()) return null
  return ((now - start.getTime()) / viewDurationMs.value) * 100
})

setTimeout(() => scrollToNow(false), 0)

const lanes = computed(() => {
  const now = Date.now()
  const { start, end } = viewRange.value
  const rows = (props.accounts || [])
    .map((account, index) => {
      const preferred = pickPreferredWindow(account)
      if (!preferred) return null
      // Real ledger: multiple same-kind windows with concrete start/end → paint as-is.
      // Snapshot-only: single live window → expand calendar projections.
      const ledgerSeries = pickLedgerSeries(account, preferred)
      const expanded = ledgerSeries.length > 1
        ? ledgerSeries
        : expandWindowOccurrences(preferred, start, end)
      const bars = expanded
        .map((win, winIndex) => buildBar(win, start, end, now, index, winIndex))
        .filter((bar): bar is NonNullable<typeof bar> => bar != null)
      if (!bars.length) return null
      const windowPayloads = bars.map((bar) => ({
        startMs: bar.startMs,
        endMs: bar.endMs,
        status: bar.status,
        kind: bar.kind,
        label: bar.labelRaw
      }))
      const used = preferred.used_percent
      const usedLabel = used == null || Number.isNaN(Number(used)) ? '—' : `${Math.round(Number(used))}%`
      const colors = palette[index % palette.length]
      const accountName = account.account_name || `#${account.account_id}`
      return {
        accountId: account.account_id,
        accountName,
        // Narrow sticky column + CSS truncate; keep readable head of the name.
        shortName: shortenAccountName(accountName),
        kindLabel: formatWindowKindLabel(preferred),
        usedLabel,
        fullTitle: `${accountName} · ${account.platform} · ${usedLabel}`,
        meta: `${account.platform} · ${usedLabel}`,
        dotClass: colors.dot,
        bars,
        windowPayloads
      }
    })
    .filter((row): row is NonNullable<typeof row> => row != null)

  return rows.slice(0, 24)
})

function horizonCenter(): number {
  return (rangeStart.value.getTime() + rangeEnd.value.getTime()) / 2
}

function setHorizon(centerMs: number, spanMs?: number) {
  const span = spanMs ?? (rangeEnd.value.getTime() - rangeStart.value.getTime())
  rangeStart.value = new Date(centerMs - span / 2)
  rangeEnd.value = new Date(centerMs + span / 2)
}

function setMode(mode: ViewMode) {
  viewMode.value = mode
  const now = Date.now()
  if (mode === '5h') {
    setHorizon(now, 15 * 3600_000) // 5h ± around
  } else if (mode === 'month') {
    setHorizon(now, 60 * 86_400_000)
  } else {
    setHorizon(now, 42 * 86_400_000)
  }
  notifyPeriodChange(true)
  scrollToNow(true)
}

/** Shift visible window forward/backward (month / 2-week / 5h step). */
function shiftPeriod(dir: -1 | 1) {
  const stepMs = viewMode.value === '5h'
    ? 5 * 3600_000
    : viewMode.value === 'week'
      ? 14 * 86_400_000
      : 30 * 86_400_000
  setHorizon(horizonCenter() + dir * stepMs)
  notifyPeriodChange()
  scrollToCenter()
}

function scrollToCenter() {
  requestAnimationFrame(() => {
    const el = scrollEl.value
    if (!el) return
    const target = Math.max(0, (el.scrollWidth - el.clientWidth) / 2)
    scrollToX(target, true)
  })
}

function scrollToNow(smooth = true) {
  requestAnimationFrame(() => {
    const el = scrollEl.value
    if (!el) return
    const { start, end } = viewRange.value
    const span = Math.max(1, end.getTime() - start.getTime())
    const ratio = (Date.now() - start.getTime()) / span
    const target = Math.max(0, ratio * el.scrollWidth - el.clientWidth / 2)
    scrollToX(target, smooth)
  })
}

function scrollToX(left: number, smooth: boolean) {
  const el = scrollEl.value
  if (!el) return
  try {
    if (typeof el.scrollTo === 'function') {
      el.scrollTo({ left, behavior: smooth ? 'smooth' : 'auto' })
    } else {
      el.scrollLeft = left
    }
  } catch {
    el.scrollLeft = left
  }
}

function jumpToCurrent() {
  const now = new Date()
  if (viewMode.value === 'month') {
    // Back to today = current calendar month (1st → month end).
    const start = startOfLocalMonth(now)
    rangeStart.value = start
    rangeEnd.value = addMonths(start, 1)
    notifyPeriodChange(true)
    scrollToX(0, true)
    return
  }
  const nowMs = now.getTime()
  if (viewMode.value === '5h') setHorizon(nowMs, 15 * 3600_000)
  else setHorizon(nowMs, 42 * 86_400_000)
  notifyPeriodChange(true)
  scrollToNow(true)
}

function notifyPeriodChange(_force = false) {
  // No toast; month markers are the only cross-month signal.
}

function onGanttScroll() {
  const el = scrollEl.value
  if (!el) return
  const pxPerMs = el.scrollWidth / Math.max(1, viewRange.value.end.getTime() - viewRange.value.start.getTime())
  const bufferPx = Math.max(240, el.clientWidth * 0.6)
  if (el.scrollLeft + el.clientWidth > el.scrollWidth - bufferPx) {
    // extend right by ~30d (or 15h in 5h mode)
    const addMs = viewMode.value === '5h' ? 15 * 3600_000 : 30 * 86_400_000
    const newEnd = new Date(rangeEnd.value.getTime() + addMs)
    rangeEnd.value = newEnd
    detectMonthCross()
  } else if (el.scrollLeft < bufferPx) {
    const addMs = viewMode.value === '5h' ? 15 * 3600_000 : 30 * 86_400_000
    const addedPx = addMs * pxPerMs
    const oldLeft = el.scrollLeft
    const newStart = new Date(rangeStart.value.getTime() - addMs)
    rangeStart.value = newStart
    requestAnimationFrame(() => {
      el.scrollLeft = oldLeft + addedPx
    })
    detectMonthCross()
  }
}

let lastVisibleMonth = ''
function visibleCenterMs(): number {
  const el = scrollEl.value
  if (!el) return horizonCenter()
  const { start, end } = viewRange.value
  const span = Math.max(1, end.getTime() - start.getTime())
  const ratio = (el.scrollLeft + el.clientWidth / 2) / Math.max(1, el.scrollWidth)
  return start.getTime() + ratio * span
}

function detectMonthCross() {
  const ms = visibleCenterMs()
  const d = new Date(ms)
  const key = `${d.getFullYear()}-${d.getMonth()}`
  if (!lastVisibleMonth) {
    lastVisibleMonth = key
    return
  }
  if (key === lastVisibleMonth) return
  lastVisibleMonth = key
}



function pickLedgerSeries(account: AccountProfitSummary, preferred: ProfitQuotaWindow): Array<ProfitQuotaWindow & { startMs: number; endMs: number }> {
  const kind = preferred.kind
  const rows = (account.quota_windows || []).filter((w) => w.kind === kind)
  if (rows.length <= 1) return []
  const out: Array<ProfitQuotaWindow & { startMs: number; endMs: number }> = []
  for (const w of rows) {
    const startMs = parseTime(w.start_at)
    const endMs = parseTime(w.end_at)
    if (startMs == null || endMs == null || endMs <= startMs) continue
    out.push({ ...w, startMs, endMs })
  }
  // Need multiple concrete rows to treat as ledger.
  if (out.length <= 1) return []
  out.sort((a, b) => a.startMs - b.startMs)
  return out
}

function pickPreferredWindow(account: AccountProfitSummary): ProfitQuotaWindow | null {
  const windows = [...(account.quota_windows || [])]
  if (!windows.length) {
    if (account.seven_day_utilization != null) {
      return synthesizeFromUtil('7d', account.seven_day_utilization, 10080)
    }
    if (account.five_hour_utilization != null) {
      return synthesizeFromUtil('5h', account.five_hour_utilization, 300)
    }
    return null
  }
  const rank = (kind?: string) => {
    if (kind === '7d' || kind === '30d') return 0
    if (kind === '5h') return 1
    if (kind === '24h') return 2
    if (kind === 'session') return 3
    return 4
  }
  windows.sort((a, b) => rank(a.kind) - rank(b.kind))
  return windows[0] || null
}

function synthesizeFromUtil(kind: string, used: number, minutes: number): ProfitQuotaWindow {
  const end = new Date(Date.now() + minutes * 60_000)
  const start = new Date(end.getTime() - minutes * 60_000)
  return {
    id: kind,
    label: kind,
    kind,
    used_percent: used,
    start_at: start.toISOString(),
    end_at: end.toISOString(),
    window_minutes: minutes
  }
}

function expandWindowOccurrences(
  window: ProfitQuotaWindow,
  viewStart: Date,
  viewEnd: Date
): Array<ProfitQuotaWindow & { startMs: number; endMs: number }> {
  const minutes = window.window_minutes && window.window_minutes > 0
    ? window.window_minutes
    : inferMinutes(window.kind)
  const durationMs = Math.max(60_000, minutes * 60_000)
  let endMs = parseTime(window.end_at)
  let startMs = parseTime(window.start_at)
  const recurringUntilMs = parseTime(window.recurring_until_at)
  const recurringFromMs = parseTime(window.recurring_from_at)
  if (endMs == null && startMs != null) endMs = startMs + durationMs
  if (startMs == null && endMs != null) startMs = endMs - durationMs
  if (startMs == null || endMs == null) return []

  // When subscription already ended (live bar starts at/after recurring_until),
  // drop the whole series — no dashed future and no leftover live bar past cutoff.
  if (recurringUntilMs != null && startMs >= recurringUntilMs) return []

  // Clip the live/current bar when bookkeeping cycle ends mid-window (expired no-renew).
  // Future projections are also capped; history still rolls back within recurring_from.
  if (recurringUntilMs != null && endMs > recurringUntilMs) {
    endMs = recurringUntilMs
  }
  if (endMs <= startMs) return []

  const out: Array<ProfitQuotaWindow & { startMs: number; endMs: number }> = []
  const earliest = viewStart.getTime() - durationMs
  const latest = viewEnd.getTime() + durationMs
  let cursorEnd = endMs
  while (cursorEnd >= earliest) {
    const cursorStart = cursorEnd - durationMs
    // Don't project back past the active cycle start (gap before cycle stays empty).
    if (recurringFromMs != null && cursorStart < recurringFromMs) break
    if (cursorEnd > viewStart.getTime() && cursorStart < viewEnd.getTime()) {
      out.push({ ...window, startMs: cursorStart, endMs: cursorEnd })
    }
    cursorEnd -= durationMs
    if (out.length > 40) break
  }
  cursorEnd = endMs + durationMs
  while (cursorEnd <= latest) {
    const cursorStart = cursorEnd - durationMs
    if (recurringUntilMs != null && cursorStart >= recurringUntilMs) break
    const cappedEnd = recurringUntilMs != null ? Math.min(cursorEnd, recurringUntilMs) : cursorEnd
    if (cappedEnd <= cursorStart) break
    if (cappedEnd > viewStart.getTime() && cursorStart < viewEnd.getTime()) {
      out.push({ ...window, startMs: cursorStart, endMs: cappedEnd })
    }
    cursorEnd += durationMs
    if (out.length > 40) break
  }
  out.sort((a, b) => a.startMs - b.startMs)
  return out
}

/** Compact lane label for the sticky column; CSS truncate is the final clip. */
function shortenAccountName(name: string, max = 12): string {
  const raw = (name || '').trim()
  if (!raw) return '—'
  if ([...raw].length <= max) return raw
  // Prefer first two hyphen segments when that already fits (GPT-自有-4 → GPT-自有).
  const parts = raw.split(/[-_·]+/).filter(Boolean)
  if (parts.length >= 2) {
    const two = `${parts[0]}-${parts[1]}`
    if ([...two].length <= max) return two
  }
  return `${[...raw].slice(0, Math.max(1, max - 1)).join('')}…`
}

function buildBar(
  win: ProfitQuotaWindow & { startMs: number; endMs: number },
  viewStart: Date,
  viewEnd: Date,
  now: number,
  paletteIndex: number,
  winIndex: number
) {
  const viewStartMs = viewStart.getTime()
  const viewEndMs = viewEnd.getTime()
  const clippedStart = Math.max(win.startMs, viewStartMs)
  const clippedEnd = Math.min(win.endMs, viewEndMs)
  if (clippedEnd <= clippedStart) return null

  const left = ((clippedStart - viewStartMs) / (viewEndMs - viewStartMs)) * 100
  const width = ((clippedEnd - clippedStart) / (viewEndMs - viewStartMs)) * 100
  if (width <= 0.2) return null

  let status: 'current' | 'upcoming' | 'ended' = 'current'
  if (win.endMs <= now) status = 'ended'
  else if (win.startMs > now) status = 'upcoming'

  const colors = palette[paletteIndex % palette.length]
  const className =
    status === 'current' ? colors.solid
      : status === 'ended' ? colors.soft
        : colors.upcoming

  // Only the live (current) snapshot has a trustworthy used%; projected history/future bars show time only.
  const used = win.used_percent
  const percentLabel = status === 'current' && used != null && !Number.isNaN(Number(used))
    ? `${Math.round(Number(used))}%`
    : ''

  const title = [
    win.label || win.kind,
    percentLabel,
    `${formatDateTime(new Date(win.startMs))} → ${formatDateTime(new Date(win.endMs))}`,
    status
  ].filter(Boolean).join(' · ')

  const label = percentLabel
    || (status === 'upcoming'
      ? formatDateTime(new Date(win.startMs))
      : formatDateTime(new Date(win.endMs)))

  const clippedLeft = win.startMs < viewStartMs
  const clippedRight = win.endMs > viewEndMs
  const radiusClass = clippedLeft && clippedRight
    ? 'rounded-none'
    : clippedLeft
      ? 'rounded-l-none'
      : clippedRight
        ? 'rounded-r-none'
        : ''
  return {
    key: `${win.id}-${win.startMs}-${winIndex}`,
    className: `${className} ${radiusClass}`.trim(),
    style: {
      left: `${left}%`,
      width: `${Math.max(width, 1.2)}%`
    },
    label,
    title,
    startMs: win.startMs,
    endMs: win.endMs,
    status,
    kind: win.kind,
    labelRaw: win.label || win.kind
  }
}

function inferMinutes(kind?: string) {
  if (kind === '5h') return 300
  if (kind === '7d') return 10080
  if (kind === '30d') return 30 * 24 * 60
  if (kind === '24h') return 1440
  if (kind === 'session') return 300
  return 300
}

/** Prefer server label; otherwise derive from window_minutes so free 30d is not stuck as 7d. */
function formatWindowKindLabel(window?: { label?: string; kind?: string; window_minutes?: number | null } | null): string {
  if (!window) return 'window'
  if (window.label && window.label !== '7d' && window.label !== 'window') return window.label
  const minutes = window.window_minutes && window.window_minutes > 0
    ? window.window_minutes
    : inferMinutes(window.kind)
  if (minutes >= 20 * 24 * 60) {
    const days = Math.round(minutes / (24 * 60))
    return `${days}d`
  }
  if (minutes >= 36 * 60) {
    const days = minutes / (24 * 60)
    return Number.isInteger(days) ? `${days}d` : `${days.toFixed(1)}d`
  }
  if (minutes >= 90) {
    const hours = minutes / 60
    return Number.isInteger(hours) ? `${hours}h` : `${hours.toFixed(1)}h`
  }
  if (minutes > 0) return `${minutes}m`
  return window.label || window.kind || 'window'
}

function parseTime(value?: string) {
  if (!value) return null
  const ms = Date.parse(value)
  return Number.isNaN(ms) ? null : ms
}

function startOfLocalDay(date: Date) {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate())
}

function startOfLocalMonth(date: Date) {
  return new Date(date.getFullYear(), date.getMonth(), 1)
}

function addMonths(date: Date, delta: number) {
  return new Date(date.getFullYear(), date.getMonth() + delta, 1)
}

function formatMonthYearShort(date: Date) {
  return new Intl.DateTimeFormat(locale.value || undefined, { month: 'short', year: 'numeric' }).format(date)
}

function formatShortDate(date: Date) {
  return new Intl.DateTimeFormat(locale.value || undefined, { month: '2-digit', day: '2-digit' }).format(date)
}

function formatDay(date: Date) {
  return new Intl.DateTimeFormat(locale.value || undefined, { month: '2-digit', day: '2-digit' }).format(date)
}

function formatMonthDay(date: Date) {
  return String(date.getDate())
}

function formatWeekday(date: Date) {
  return new Intl.DateTimeFormat(locale.value || undefined, { weekday: 'short' }).format(date)
}

function formatHour(date: Date) {
  return new Intl.DateTimeFormat(locale.value || undefined, { hour: '2-digit', minute: '2-digit' }).format(date)
}

function formatDateTime(date: Date) {
  return new Intl.DateTimeFormat(locale.value || undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }).format(date)
}
</script>

<style scoped>
</style>
