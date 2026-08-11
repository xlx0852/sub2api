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
      <div class="flex min-w-0 items-stretch gap-2">
        <!-- Left labels column -->
        <div class="w-[6.75rem] shrink-0 sm:w-[7.5rem]">
          <div class="mb-2 h-12" />
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

        <!-- Virtualized continuous timeline -->
        <div
          ref="scrollEl"
          class="relative min-w-0 flex-1 overflow-x-auto overscroll-x-contain"
          data-testid="profit-quota-window-scroll"
          @scroll.passive="onGanttScroll"
        >
          <div
            data-testid="profit-quota-window-content"
            :style="{ width: `${contentWidth}px` }"
          >
            <!-- Day / hour ticks -->
            <div data-testid="quota-window-axis-header" class="relative mb-2 h-12">
              <div
                v-for="tick in visibleTicks"
                :key="tick.key"
                class="absolute top-0 flex -translate-x-1/2 flex-col items-center"
                :style="{ left: `${tick.left}px` }"
                data-testid="quota-window-axis-tick"
                :data-minor="tick.isMinor ? 'true' : 'false'"
              >
                <span class="text-[10px] font-medium uppercase tracking-wide text-gray-400 dark:text-dark-400">{{ tick.weekday }}</span>
                <span
                  class="tabular-nums"
                  :class="tick.isMinor
                    ? 'text-[8px] font-medium text-gray-400 dark:text-dark-500'
                    : 'text-[11px] font-semibold text-gray-600 dark:text-dark-200'"
                >{{ tick.day }}</span>
              </div>
              <div
                v-for="mb in visibleMonthMarkers"
                :key="`month-${mb.key}`"
                class="pointer-events-none absolute top-7 z-10 flex -translate-x-1/2 flex-col items-center"
                :style="{ left: `${mb.left}px` }"
                data-testid="quota-window-month-marker"
              >
                <span class="rounded-full border border-violet-300 bg-violet-50 px-1.5 py-px text-[9px] font-semibold text-violet-700 dark:border-violet-700 dark:bg-violet-950/60 dark:text-violet-300">
                  {{ mb.label }}
                </span>
              </div>
              <div
                v-if="nowLeftPx != null"
                class="pointer-events-none absolute bottom-0 top-0 w-px bg-rose-400/80"
                :style="{ left: `${nowLeftPx}px` }"
              />
            </div>

            <!-- Lanes with one shared grid instead of one grid per account. -->
            <div class="relative">
              <div class="pointer-events-none absolute inset-0 z-10" aria-hidden="true">
                <div
                  v-for="seg in visibleTicks"
                  :key="`grid-${seg.key}`"
                  class="pointer-events-none absolute bottom-0 top-0 w-px bg-gray-200/70 dark:bg-dark-600/80"
                  :style="{ left: `${seg.left}px` }"
                  data-testid="quota-window-grid-line"
                />
                <div
                  v-for="mb in visibleMonthMarkers"
                  :key="`grid-month-${mb.key}`"
                  class="pointer-events-none absolute bottom-0 top-0 z-10 w-px bg-violet-400/70"
                  :style="{ left: `${mb.left}px` }"
                />
                <div
                  v-if="nowLeftPx != null"
                  class="pointer-events-none absolute bottom-0 top-0 w-px bg-rose-400/50"
                  :style="{ left: `${nowLeftPx}px` }"
                />
              </div>

              <div class="relative space-y-2.5">
                <div
                  v-for="lane in lanes"
                  :key="`track-${lane.accountId}`"
                  class="relative h-9 rounded-lg bg-gray-50/75 ring-1 ring-inset ring-gray-100 dark:bg-dark-700/40 dark:ring-dark-600"
                >
                  <button
                    v-for="bar in lane.bars"
                    :key="bar.key"
                    type="button"
                    class="absolute top-1.5 z-20 h-6 cursor-pointer overflow-hidden rounded-md border text-[10px] font-semibold tabular-nums shadow-sm transition hover:brightness-95"
                    :class="bar.className"
                    :style="{ left: `${bar.left}px`, width: `${bar.width}px` }"
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
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AccountProfitSummary } from '@/api/admin/profit'
import {
  inferWindowMinutes,
  prepareQuotaAccounts,
  windowsForRange,
  type TimelineWindow
} from './quotaTimeline'

export type QuotaWindowSelectPayload = {
  accountId: number
  startMs: number
  endMs: number
  status: WindowBarStatus
  kind?: string
  label?: string
  windows: Array<{ startMs: number; endMs: number; status: WindowBarStatus; kind?: string; label?: string }>
}

type WindowBarStatus = 'current' | 'upcoming' | 'ended' | 'drifting' | 'waiting_activation'

const emit = defineEmits<{ select: [payload: QuotaWindowSelectPayload] }>()

type ViewMode = 'week' | 'month' | '5h'

const props = defineProps<{
  accounts: AccountProfitSummary[]
}>()

const { t, locale } = useI18n()

const viewMode = ref<ViewMode>('month')
const scrollEl = ref<HTMLElement | null>(null)

// ---- Virtual continuous timeline ----
// All times are absolute ms since epoch; content width is fixed per day/hour.
const PX_PER_DAY = 36
const PX_PER_HOUR = 140

const DAY_MS = 86_400_000
const HOUR_MS = 3_600_000
const initialSpanMs = 180 * DAY_MS
const rangeStart = ref(Date.now() - initialSpanMs / 2)
const rangeEnd = ref(Date.now() + initialSpanMs / 2)
const viewportWidth = ref(1024)
const viewLeft = ref(Math.max(0, (180 * PX_PER_DAY - viewportWidth.value) / 2))

const viewDurationMs = computed(() => Math.max(1, rangeEnd.value - rangeStart.value))

const contentWidth = computed(() => {
  if (viewMode.value === '5h') {
    const hours = viewDurationMs.value / 3600_000
    return Math.max(640, Math.ceil(hours * PX_PER_HOUR))
  }
  const days = viewDurationMs.value / 86_400_000
  return Math.max(860, Math.ceil(days * PX_PER_DAY))
})

function msToPx(ms: number): number {
  const span = viewDurationMs.value
  if (contentWidth.value <= 0 || span <= 0) return 0
  return ((ms - rangeStart.value) / span) * contentWidth.value
}

function resetHorizon(mode: ViewMode) {
  const now = Date.now()
  const span = mode === 'month'
    ? 180 * DAY_MS
    : mode === 'week'
      ? 84 * DAY_MS
      : 48 * HOUR_MS
  rangeStart.value = now - span / 2
  rangeEnd.value = now + span / 2
  const width = mode === '5h'
    ? (span / HOUR_MS) * PX_PER_HOUR
    : (span / DAY_MS) * PX_PER_DAY
  viewLeft.value = Math.max(0, (width - viewportWidth.value) / 2)
}

const rangeLabel = computed(() => {
  const scope = viewMode.value === '5h'
    ? t('admin.profit.quotaWindowBy5h')
    : viewMode.value === 'month'
      ? t('admin.profit.quotaWindowByMonth')
      : t('admin.profit.quotaWindowByWeek')
  return t('admin.profit.quotaWindowRange', {
    start: formatShortDate(new Date(rangeStart.value)),
    end: formatShortDate(new Date(rangeEnd.value - 1)),
    scope
  })
})

const nowLeftPx = computed(() => {
  const now = Date.now()
  if (now < rangeStart.value || now > rangeEnd.value) return null
  return msToPx(now)
})

// Visible window in ms (viewport + overscan)
const visibleMs = computed(() => {
  const span = viewDurationMs.value
  if (viewportWidth.value <= 0 || span <= 0 || contentWidth.value <= 0) {
    return { start: rangeStart.value, end: rangeEnd.value }
  }
  const ratioStart = viewLeft.value / contentWidth.value
  const ratioEnd = (viewLeft.value + viewportWidth.value) / contentWidth.value
  const overscanPx = Math.min(320, Math.max(120, viewportWidth.value / 3))
  const overscanMs = (overscanPx / contentWidth.value) * span
  return {
    start: Math.max(rangeStart.value, rangeStart.value + ratioStart * span - overscanMs),
    end: Math.min(rangeEnd.value, rangeStart.value + ratioEnd * span + overscanMs)
  }
})

type Tick = {
  key: string
  left: number
  weekday: string
  day: string
  isMonthStart: boolean
  isMinor: boolean
}

const visibleTicks = computed<Tick[]>(() => {
  const { start, end } = visibleMs.value
  const ticks: Tick[] = []
  if (viewMode.value === '5h') {
    // hourly ticks
    let cursor = Math.ceil(start / 3600_000) * 3600_000
    while (cursor < end) {
      const d = new Date(cursor)
      ticks.push({
        key: `h-${cursor}`,
        left: msToPx(cursor),
        weekday: '',
        day: formatHour(d),
        isMonthStart: false,
        isMinor: false
      })
      cursor += 3600_000
    }
    return ticks
  }
  let cursor = startOfLocalDay(new Date(start)).getTime()
  if (cursor < start) cursor += 86_400_000
  while (cursor < end) {
    const d = new Date(cursor)
    const isMonthStart = d.getDate() === 1
    const showLabel = viewMode.value !== 'month' || isMonthStart || d.getDate() % 2 === 1
    const isMinor = viewMode.value === 'month' && !showLabel
    ticks.push({
      key: `d-${cursor}`,
      left: msToPx(cursor),
      weekday: viewMode.value === 'month' ? '' : formatWeekday(d),
      day: viewMode.value === 'month' ? formatMonthDay(d) : formatDay(d),
      isMonthStart,
      isMinor
    })
    cursor += 86_400_000
  }
  return ticks
})

const visibleMonthMarkers = computed(() => {
  const { start, end } = visibleMs.value
  const out: Array<{ key: string; left: number; label: string }> = []
  let cursor = startOfLocalMonth(new Date(start)).getTime()
  if (cursor < start) cursor = addMonths(new Date(cursor), 1).getTime()
  while (cursor < end) {
    out.push({
      key: String(cursor),
      left: msToPx(cursor),
      label: formatMonthYearShort(new Date(cursor))
    })
    cursor = addMonths(new Date(cursor), 1).getTime()
  }
  // ensure first visible month label even when mid-month
  if (!out.length) {
    out.push({ key: `m-${start}`, left: msToPx(start), label: formatMonthYearShort(new Date(start)) })
  }
  return out
})

// ---- Lane windows (ledger + projection, clipped per visible range) ----
type LaneWindow = TimelineWindow
type Bar = {
  key: string
  className: string
  label: string
  title: string
  left: number
  width: number
  startMs: number
  endMs: number
  status: WindowBarStatus
  kind?: string
  labelRaw?: string
}

const palette = [
  {
    solid: 'bg-slate-600/85 border-slate-700 text-white dark:bg-slate-500/80 dark:border-slate-400',
    soft: 'bg-slate-200/80 border-slate-300 text-slate-700 dark:bg-dark-600 dark:border-dark-500 dark:text-dark-100',
    upcoming: 'border-dashed border-slate-400 bg-white/40 text-slate-500 dark:bg-transparent dark:text-dark-300',
    drifting: 'border border-dashed border-slate-400 bg-slate-100/60 text-slate-600 dark:border-dark-400 dark:bg-dark-700/50 dark:text-dark-200',
    dot: 'bg-slate-500'
  },
  {
    solid: 'bg-sky-600/85 border-sky-700 text-white dark:bg-sky-500/80 dark:border-sky-400',
    soft: 'bg-sky-100 border-sky-200 text-sky-800 dark:bg-sky-950/50 dark:border-sky-800 dark:text-sky-200',
    upcoming: 'border-dashed border-sky-400 bg-white/40 text-sky-600 dark:bg-transparent dark:text-sky-300',
    drifting: 'border border-dashed border-sky-400 bg-sky-50/60 text-sky-600 dark:border-sky-700 dark:bg-sky-950/50 dark:text-sky-200',
    dot: 'bg-sky-500'
  },
  {
    solid: 'bg-violet-600/85 border-violet-700 text-white dark:bg-violet-500/80 dark:border-violet-400',
    soft: 'bg-violet-100 border-violet-200 text-violet-800 dark:bg-violet-950/50 dark:border-violet-800 dark:text-violet-200',
    upcoming: 'border-dashed border-violet-400 bg-white/40 text-violet-600 dark:bg-transparent dark:text-violet-300',
    drifting: 'border border-dashed border-violet-400 bg-violet-50/60 text-violet-600 dark:border-violet-700 dark:bg-violet-950/50 dark:text-violet-200',
    dot: 'bg-violet-500'
  },
  {
    solid: 'bg-emerald-600/85 border-emerald-700 text-white dark:bg-emerald-500/80 dark:border-emerald-400',
    soft: 'bg-emerald-100 border-emerald-200 text-emerald-800 dark:bg-emerald-950/50 dark:border-emerald-800 dark:text-emerald-200',
    upcoming: 'border-dashed border-emerald-400 bg-white/40 text-emerald-600 dark:bg-transparent dark:text-emerald-300',
    drifting: 'border border-dashed border-emerald-400 bg-emerald-50/60 text-emerald-600 dark:border-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-200',
    dot: 'bg-emerald-500'
  },
  {
    solid: 'bg-amber-500/90 border-amber-600 text-white dark:bg-amber-500/80 dark:border-amber-400',
    soft: 'bg-amber-100 border-amber-200 text-amber-900 dark:bg-amber-950/40 dark:border-amber-800 dark:text-amber-200',
    upcoming: 'border-dashed border-amber-400 bg-white/40 text-amber-700 dark:bg-transparent dark:text-amber-300',
    drifting: 'border border-dashed border-amber-400 bg-amber-50/60 text-amber-700 dark:border-amber-700 dark:bg-amber-950/50 dark:text-amber-200',
    dot: 'bg-amber-500'
  }
]

const preparedAccounts = computed(() => prepareQuotaAccounts(props.accounts || [], Date.now()))

// Only materialize ledger rows or projections intersecting the viewport overscan.
const laneSeries = computed(() => {
  const now = Date.now()
  const { start, end } = visibleMs.value
  return preparedAccounts.value
    .map((prepared, index) => {
      const { account, preferred } = prepared
      const windows = windowsForRange(prepared, start, end)
      const used = preferred.used_percent
      const usedLabel = used == null || Number.isNaN(Number(used)) ? '—' : `${Math.round(Number(used))}%`
      const colors = palette[index % palette.length]
      const accountName = account.account_name || `#${account.account_id}`
      return {
        accountId: account.account_id,
        accountName,
        shortName: shortenAccountName(accountName),
        kindLabel: formatWindowKindLabel(preferred),
        usedLabel,
        fullTitle: `${accountName} · ${account.platform} · ${usedLabel}`,
        meta: `${account.platform} · ${usedLabel}`,
        dotClass: colors.dot,
        windows,
        paletteIndex: index,
        now
      }
    })
    .filter((row): row is NonNullable<typeof row> => row != null)
})

const lanes = computed(() => {
  return laneSeries.value.map((lane) => {
    const colors = palette[lane.paletteIndex % palette.length]
    const bars: Bar[] = lane.windows
      .map((win, winIndex) => buildBar(win, lane.now, colors, winIndex))
      .filter((bar): bar is Bar => bar != null)
    const windowPayloads = bars.map((bar) => ({
      startMs: bar.startMs,
      endMs: bar.endMs,
      status: bar.status,
      kind: bar.kind,
      label: bar.labelRaw
    }))
    return { ...lane, bars, windowPayloads }
  })
})

function buildBar(
  win: LaneWindow,
  now: number,
  colors: (typeof palette)[number],
  winIndex: number
): Bar | null {
  const clippedStart = Math.max(win.startMs, rangeStart.value)
  const clippedEnd = Math.min(win.endMs, rangeEnd.value)
  if (clippedEnd <= clippedStart) return null
  const left = msToPx(clippedStart)
  const widthPx = Math.max(8, msToPx(clippedEnd) - left)

  let status: WindowBarStatus = win.status === 'waiting_activation' ? 'waiting_activation' : 'current'
  if (status !== 'waiting_activation' && win.startMs > now) {
    status = 'upcoming'
  } else if (status !== 'waiting_activation' && win.endMs <= now) {
    // Drifting only for ledger rows that are still open but past end (vacuum).
    const isOpen = win.is_open === true || String(win.id || '').endsWith('-open')
    status = isOpen ? 'drifting' : 'ended'
  }

  const className =
    status === 'current' ? colors.solid
      : status === 'ended' ? colors.soft
        : status === 'drifting' || status === 'waiting_activation' ? colors.drifting
          : colors.upcoming

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
      : status === 'waiting_activation'
        ? t('admin.profit.quotaWindowWaitingActivation')
        : status === 'drifting'
          ? t('admin.profit.quotaWindowDrifting')
          : formatDateTime(new Date(win.endMs)))

  const clippedLeft = win.startMs < rangeStart.value
  const clippedRight = win.endMs > rangeEnd.value
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
    label,
    title,
    left,
    width: widthPx,
    startMs: win.startMs,
    endMs: win.endMs,
    status,
    kind: win.kind,
    labelRaw: win.label || win.kind
  }
}

// ---- Interactions ----
const modes = computed(() => [
  { key: 'week' as const, label: t('admin.profit.quotaWindowByWeek') },
  { key: 'month' as const, label: t('admin.profit.quotaWindowByMonth') },
  { key: '5h' as const, label: t('admin.profit.quotaWindowBy5h') }
])

function setMode(mode: ViewMode) {
  viewMode.value = mode
  resetHorizon(mode)
  requestAnimationFrame(() => scrollToNow(false))
}

function shiftPeriod(dir: -1 | 1) {
  const stepMs = viewMode.value === '5h'
    ? 5 * 3600_000
    : viewMode.value === 'week'
      ? 14 * 86_400_000
      : 30 * 86_400_000
  const center = (rangeStart.value + rangeEnd.value) / 2 + dir * stepMs
  const span = viewDurationMs.value
  rangeStart.value = center - span / 2
  rangeEnd.value = center + span / 2
  requestAnimationFrame(() => scrollToCenter())
}

function jumpToCurrent() {
  resetHorizon(viewMode.value)
  requestAnimationFrame(() => scrollToNow(false))
}

function scrollToCenter() {
  const el = scrollEl.value
  if (!el) return
  scrollToX(Math.max(0, (el.scrollWidth - el.clientWidth) / 2), true)
}

function scrollToNow(smooth: boolean) {
  const el = scrollEl.value
  if (!el) return
  const width = el.clientWidth > 0 ? el.clientWidth : viewportWidth.value
  const target = Math.max(0, msToPx(Date.now()) - width / 2)
  scrollToX(target, smooth)
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
  viewLeft.value = left
}

let scrollRaf: number | null = null
let initialPositionRaf: number | null = null
let resizeObserver: ResizeObserver | null = null
let rebasing = false
let initialTodayPositioned = false

function onGanttScroll() {
  const el = scrollEl.value
  if (!el) return
  if (scrollRaf != null) return
  scrollRaf = requestAnimationFrame(() => {
    scrollRaf = null
    viewLeft.value = el.scrollLeft
    if (el.clientWidth > 0) viewportWidth.value = el.clientWidth
    if (rebasing) return
    const bufferPx = Math.max(320, el.clientWidth * 0.7)
    let direction: -1 | 1 | 0 = 0
    if (el.scrollLeft + el.clientWidth > el.scrollWidth - bufferPx) {
      direction = 1
    } else if (el.scrollLeft < bufferPx) {
      direction = -1
    }
    if (direction !== 0) rebaseHorizon(direction, el)
  })
}

function rebaseHorizon(direction: -1 | 1, el: HTMLElement) {
  const shiftMs = viewMode.value === '5h'
    ? 12 * HOUR_MS
    : viewMode.value === 'week'
      ? 14 * DAY_MS
      : 30 * DAY_MS
  const shiftPx = shiftMs * (contentWidth.value / viewDurationMs.value)
  const previousLeft = el.scrollLeft
  rebasing = true
  rangeStart.value += direction * shiftMs
  rangeEnd.value += direction * shiftMs
  requestAnimationFrame(() => {
    const nextLeft = previousLeft - direction * shiftPx
    el.scrollLeft = Math.max(0, Math.min(nextLeft, el.scrollWidth - el.clientWidth))
    viewLeft.value = el.scrollLeft
    if (el.clientWidth > 0) viewportWidth.value = el.clientWidth
    rebasing = false
  })
}

watch(scrollEl, (el) => {
  resizeObserver?.disconnect()
  resizeObserver = null
  if (initialPositionRaf != null) {
    cancelAnimationFrame(initialPositionRaf)
    initialPositionRaf = null
  }
  if (!el) return

  if (el.clientWidth > 0) viewportWidth.value = el.clientWidth
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver((entries) => {
      const width = entries[0]?.contentRect.width || scrollEl.value?.clientWidth || 0
      if (width > 0) viewportWidth.value = width
    })
    resizeObserver.observe(el)
  }
  if (initialTodayPositioned) return

  initialPositionRaf = requestAnimationFrame(() => {
    initialPositionRaf = null
    if (scrollEl.value !== el) return
    resetHorizon(viewMode.value)
    scrollToNow(false)
    initialTodayPositioned = true
  })
}, { flush: 'post' })

onBeforeUnmount(() => {
  if (scrollRaf != null) cancelAnimationFrame(scrollRaf)
  if (initialPositionRaf != null) cancelAnimationFrame(initialPositionRaf)
  resizeObserver?.disconnect()
})

function formatWindowKindLabel(window?: { label?: string; kind?: string; window_minutes?: number | null } | null): string {
  if (!window) return 'window'
  if (window.label && window.label !== '7d' && window.label !== 'window') return window.label
  const minutes = window.window_minutes && window.window_minutes > 0
    ? window.window_minutes
    : inferWindowMinutes(window.kind)
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

function startOfLocalDay(date: Date) {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate())
}

function startOfLocalMonth(date: Date) {
  return new Date(date.getFullYear(), date.getMonth(), 1)
}

function addMonths(date: Date, delta: number) {
  return new Date(date.getFullYear(), date.getMonth() + delta, 1)
}

const dateFormatters = computed(() => {
  const language = locale.value || undefined
  return {
    shortDate: new Intl.DateTimeFormat(language, { month: '2-digit', day: '2-digit' }),
    weekday: new Intl.DateTimeFormat(language, { weekday: 'short' }),
    monthYear: new Intl.DateTimeFormat(language, { month: 'short', year: 'numeric' }),
    hour: new Intl.DateTimeFormat(language, { hour: '2-digit', minute: '2-digit' }),
    dateTime: new Intl.DateTimeFormat(language, {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    })
  }
})

function formatShortDate(date: Date) {
  return dateFormatters.value.shortDate.format(date)
}

function formatDay(date: Date) {
  return dateFormatters.value.shortDate.format(date)
}

function formatMonthDay(date: Date) {
  return String(date.getDate())
}

function formatWeekday(date: Date) {
  return dateFormatters.value.weekday.format(date)
}

function formatMonthYearShort(date: Date) {
  return dateFormatters.value.monthYear.format(date)
}

function formatHour(date: Date) {
  return dateFormatters.value.hour.format(date)
}

function formatDateTime(date: Date) {
  return dateFormatters.value.dateTime.format(date)
}

function shortenAccountName(name: string, max = 12): string {
  const raw = (name || '').trim()
  if (!raw) return '—'
  if ([...raw].length <= max) return raw
  const parts = raw.split(/[-_·]+/).filter(Boolean)
  if (parts.length >= 2) {
    const two = `${parts[0]}-${parts[1]}`
    if ([...two].length <= max) return two
  }
  return `${[...raw].slice(0, Math.max(1, max - 1)).join('')}…`
}
</script>
