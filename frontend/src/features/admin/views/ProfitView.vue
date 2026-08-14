<template>
  <AppLayout>
    <TablePageLayout transparent compact>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="ml-auto flex min-w-0 flex-wrap items-center justify-end gap-3">
            <div class="flex flex-wrap items-center gap-2">
              <button
                v-for="preset in presets"
                :key="preset.key"
                class="rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
                :class="activePreset === preset.key
                  ? 'bg-primary-600 text-white'
                  : 'bg-gray-100 text-gray-700 hover:bg-gray-200 dark:bg-dark-700 dark:text-dark-200 dark:hover:bg-dark-600'"
                @click="applyPreset(preset.key)"
              >
                {{ preset.label }}
              </button>
            </div>
            <div class="flex items-center gap-2">
              <input v-model="startDate" type="date" class="input-sm" @change="loadAll()" />
              <span class="text-gray-400">-</span>
              <input v-model="endDate" type="date" class="input-sm" @change="loadAll()" />
            </div>
            <div class="flex flex-wrap items-center justify-end gap-2 text-xs text-gray-400 dark:text-dark-400">
              <button
                data-testid="profit-refresh"
                type="button"
                class="inline-flex h-8 items-center gap-1.5 rounded-lg border border-gray-200 bg-white px-2.5 font-medium text-gray-600 transition-colors hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-600 dark:bg-dark-700 dark:text-dark-200 dark:hover:bg-dark-600"
                :disabled="loading"
                :title="snapshotGeneratedAt ? t('admin.profit.snapshotGeneratedAt', { time: snapshotTimeLabel }) : t('admin.profit.refreshSnapshot')"
                @click="loadAll(true)"
              >
                <Icon name="refresh" size="xs" :class="loading ? 'animate-spin' : ''" />
                <span data-testid="profit-snapshot-time">
                  {{ snapshotGeneratedAt ? snapshotTimeLabel : t('admin.profit.refreshSnapshot') }}
                </span>
              </button>
              <span
                class="inline-flex items-center"
                :title="t('admin.profit.globalOnlyHint')"
                :aria-label="t('admin.profit.globalOnlyHint')"
              >
                <Icon name="infoCircle" size="xs" />
              </span>
            </div>
          </div>
        </div>
      </template>

      <template #table>
        <div class="flex flex-col space-y-5">
          <section class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4" :class="loading ? 'opacity-60' : ''">
            <div class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
              <div class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.profit.totalRevenue') }}</div>
              <div class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">${{ fmt(data?.total_revenue) }}</div>
            </div>
            <div class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
              <div class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.profit.amortizedCost') }}</div>
              <div class="mt-1 text-2xl font-bold text-gray-900 dark:text-white">${{ fmt(data?.total_cost) }}</div>
            </div>
            <div class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
              <div class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.profit.totalProfit') }}</div>
              <div class="mt-1 text-2xl font-bold" :class="(data?.total_profit ?? 0) >= 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'">
                ${{ fmt(data?.total_profit) }}
              </div>
            </div>
            <div
              class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800"
              :title="t('admin.profit.currentUserBalanceHint')"
              data-testid="profit-current-user-balance"
            >
              <div class="text-sm text-gray-500 dark:text-dark-400">{{ t('admin.profit.currentUserBalance') }}</div>
              <div class="mt-1 text-2xl font-bold text-sky-600 dark:text-sky-400">${{ fmt(currentUserBalance) }}</div>
            </div>
          </section>

          <section class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
            <div class="mb-1 flex flex-wrap items-center justify-between gap-3">
              <div>
                <h3 class="text-sm font-semibold text-gray-800 dark:text-dark-100">{{ t('admin.profit.trendTitle') }}</h3>
                <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.profit.globalTrendHint') }}</p>
              </div>
              <div class="flex items-center gap-2">
                <div class="inline-flex rounded-lg bg-gray-100 p-1 dark:bg-dark-700">
                  <button
                    v-for="metric in trendMetrics"
                    :key="metric.key"
                    type="button"
                    class="rounded-md px-2.5 py-1 text-xs font-medium transition"
                    :class="trendMetric === metric.key
                      ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white'
                      : 'text-gray-500 hover:text-gray-800 dark:text-dark-300 dark:hover:text-white'"
                    @click="trendMetric = metric.key"
                  >
                    {{ metric.label }}
                  </button>
                </div>
                <LoadingSpinner v-if="trendLoading" size="sm" />
              </div>
            </div>
            <div class="mt-4 h-80" data-testid="profit-trend-chart">
              <Bar v-if="chartData" :data="chartData" :options="chartOptions" />
              <div v-else-if="!trendLoading" class="flex h-full items-center justify-center text-sm text-gray-400">{{ t('admin.profit.empty') }}</div>
            </div>
          </section>

          <QuotaWindowPanel :accounts="accountRows" @select="openAccountDrawer" />

        </div>
      </template>
    </TablePageLayout>

    <AccountProfitDrawer
      :show="showAccountDrawer"
      :account="drawerAccount"
      :selected-window="drawerSelectedWindow"
      :window-history="drawerWindowHistory"
      :history-loading="drawerHistoryLoading"
      @close="closeAccountDrawer"
      @select-window="onDrawerSelectWindow"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type { ProfitSummaryResponse, ProfitTrendPoint } from '@/api/admin/profit'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import QuotaWindowPanel from '@/components/admin/profit/QuotaWindowPanel.vue'
import type { QuotaWindowSelectPayload } from '@/components/admin/profit/QuotaWindowPanel.vue'
import { windowsForDrawerSelection } from '@/components/admin/profit/quotaTimeline'
import AccountProfitDrawer from '@/components/admin/profit/AccountProfitDrawer.vue'
import type { AccountProfitSummary, ProfitWindowEconomicsItem } from '@/api/admin/profit'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  BarElement,
  Tooltip,
  Legend
} from 'chart.js'
import type { ChartOptions, TooltipItem } from 'chart.js'
import { Bar } from 'vue-chartjs'
import type { ProfitTrendAccountSlice } from '@/api/admin/profit'

ChartJS.register(CategoryScale, LinearScale, BarElement, Tooltip, Legend)

type TrendMetric = 'revenue' | 'cost' | 'profit'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const trendLoading = ref(false)
const data = ref<ProfitSummaryResponse | null>(null)
const trendPoints = ref<ProfitTrendPoint[]>([])
const snapshotGeneratedAt = ref('')
const currentUserBalance = ref(0)
const trendMetric = ref<TrendMetric>('revenue')
const accountRows = computed(() => [...(data.value?.accounts || [])].sort((a, b) => b.profit - a.profit))
const showAccountDrawer = ref(false)
const drawerAccountId = ref<number | null>(null)
const drawerAccountDetail = ref<AccountProfitSummary | null>(null)
const drawerAccount = computed(() => {
  if (drawerAccountDetail.value?.account_id === drawerAccountId.value) return drawerAccountDetail.value
  return accountRows.value.find((a) => a.account_id === drawerAccountId.value) || null
})
const drawerSelectedWindow = ref<ProfitWindowEconomicsItem | null>(null)
const drawerWindowHistory = ref<ProfitWindowEconomicsItem[]>([])
const drawerHistoryLoading = ref(false)

function closeAccountDrawer() {
  showAccountDrawer.value = false
}

async function openAccountDrawer(payload: QuotaWindowSelectPayload) {
  drawerAccountId.value = payload.accountId
  drawerAccountDetail.value = null
  showAccountDrawer.value = true
  drawerSelectedWindow.value = {
    start_at: new Date(payload.startMs).toISOString(),
    end_at: new Date(payload.endMs).toISOString(),
    kind: payload.kind,
    label: payload.label,
    status: payload.status,
    requests: 0,
    revenue: 0,
    cost: 0,
    profit: 0
  }
  drawerWindowHistory.value = []
  await Promise.all([
    loadDrawerAccountSummary(payload.accountId),
    loadDrawerWindowEconomics(payload.accountId, payload.startMs, payload.endMs, payload.windows, payload.kind, payload.label)
  ])
}

async function loadDrawerAccountSummary(accountId: number) {
  try {
    const response = await adminAPI.profit.summary(startDate.value, endDate.value, accountId)
    if (drawerAccountId.value === accountId) {
      drawerAccountDetail.value = response.accounts?.[0] || null
    }
  } catch (error) {
    if (drawerAccountId.value === accountId) {
      appStore.showError(extractApiErrorMessage(error, t('admin.profit.loadFailed')))
    }
  }
}

function onDrawerSelectWindow(item: ProfitWindowEconomicsItem) {
  drawerSelectedWindow.value = item
}

function expandLedgerWindows(
  accountId: number,
  windows: QuotaWindowSelectPayload['windows'],
  selectedStartMs: number,
  selectedEndMs: number,
  kind?: string,
  label?: string
) {
  const account = accountRows.value.find((row) => row.account_id === accountId)
  if (!account) {
    return selectedStartMs && selectedEndMs
      ? [{ startMs: selectedStartMs, endMs: selectedEndMs, status: 'current' as const, kind, label }]
      : [...(windows || [])]
  }

  // The drawer and gantt must consume the same ledger/projection engine. The
  // clicked bar only selects the range; it must never become a second recurrence
  // anchor because observed ledger boundaries can drift by a few seconds.
  return windowsForDrawerSelection(account, Date.now(), selectedStartMs, selectedEndMs, kind)
}

async function loadDrawerWindowEconomics(
  accountId: number,
  selectedStartMs: number,
  selectedEndMs: number,
  windows: QuotaWindowSelectPayload['windows'],
  kind?: string,
  label?: string
) {
  drawerHistoryLoading.value = true
  try {
    const ledger = expandLedgerWindows(accountId, windows, selectedStartMs, selectedEndMs, kind, label)
    const queries = ledger.map((w) => ({
      start_at: new Date(w.startMs).toISOString(),
      end_at: new Date(w.endMs).toISOString(),
      kind: w.kind || kind,
      label: w.label || label
    }))
    const resp = await adminAPI.profit.windowEconomics(accountId, queries)
    drawerWindowHistory.value = resp.windows || []
    const hit = drawerWindowHistory.value.find((w) => {
      const ms = Date.parse(w.start_at)
      return !Number.isNaN(ms) && Math.abs(ms - selectedStartMs) < 2000
    })
    if (hit) drawerSelectedWindow.value = hit
    else if (drawerWindowHistory.value[0]) {
      // keep click selection skeleton if exact miss
    }
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.profit.loadFailed')))
  } finally {
    drawerHistoryLoading.value = false
  }
}

const today = new Date()
const fmtDate = (date: Date) => date.toISOString().slice(0, 10)
const startDate = ref(fmtDate(new Date(today.getTime() - 6 * 86_400_000)))
const endDate = ref(fmtDate(today))
const activePreset = ref('7d')

const presets = computed(() => [
  { key: 'today', label: t('admin.profit.today') },
  { key: '7d', label: t('admin.profit.last7Days') },
  { key: '30d', label: t('admin.profit.last30Days') },
  { key: 'month', label: t('admin.profit.currentMonth') }
])

const trendMetrics = computed(() => [
  { key: 'revenue' as const, label: t('admin.profit.trendMetricRevenue') },
  { key: 'cost' as const, label: t('admin.profit.trendMetricCost') },
  { key: 'profit' as const, label: t('admin.profit.trendMetricProfit') }
])

/** Stable palette for stacked account segments (readable on white/dark). */
const accountPalette = [
  '#3b82f6', '#10b981', '#f59e0b', '#8b5cf6', '#ef4444',
  '#06b6d4', '#ec4899', '#84cc16', '#6366f1', '#14b8a6',
  '#a855f7', '#f97316'
]

function applyPreset(key: string) {
  activePreset.value = key
  const now = new Date()
  if (key === 'today') startDate.value = fmtDate(now)
  if (key === '7d') startDate.value = fmtDate(new Date(now.getTime() - 6 * 86_400_000))
  if (key === '30d') startDate.value = fmtDate(new Date(now.getTime() - 29 * 86_400_000))
  if (key === 'month') startDate.value = fmtDate(new Date(now.getFullYear(), now.getMonth(), 1))
  endDate.value = fmtDate(now)
  void loadAll()
}

const isDark = computed(() => document.documentElement.classList.contains('dark'))

const TOP_ACCOUNT_SERIES = 8

function metricValue(slice: ProfitTrendAccountSlice | ProfitTrendPoint, metric: TrendMetric): number {
  if (metric === 'cost') return Number(slice.cost) || 0
  if (metric === 'profit') return Number(slice.profit) || 0
  return Number(slice.revenue) || 0
}

const stackedSeries = computed(() => {
  const points = trendPoints.value
  if (!points.length) return null

  // Rank accounts by absolute contribution on the active metric across the range.
  const totals = new Map<number, { id: number; name: string; total: number }>()
  for (const point of points) {
    for (const acc of point.accounts || []) {
      const prev = totals.get(acc.account_id)
      const add = Math.abs(metricValue(acc, trendMetric.value))
      if (prev) {
        prev.total += add
      } else {
        totals.set(acc.account_id, {
          id: acc.account_id,
          name: acc.account_name || `#${acc.account_id}`,
          total: add
        })
      }
    }
  }
  const ranked = [...totals.values()].sort((a, b) => b.total - a.total)
  const top = ranked.slice(0, TOP_ACCOUNT_SERIES)
  const topIds = new Set(top.map((a) => a.id))
  const hasOthers = ranked.length > top.length

  const labels = points.map((p) => p.date.slice(5))
  const seriesDefs = top.map((a, idx) => ({
    key: `acc-${a.id}`,
    id: a.id,
    label: a.name,
    color: accountPalette[idx % accountPalette.length]
  }))
  if (hasOthers) {
    seriesDefs.push({
      key: 'others',
      id: -1,
      label: t('admin.profit.trendOthers'),
      color: isDark.value ? '#6b7280' : '#9ca3af'
    })
  }

  // Fallback: no per-account slices from older API → single total bar.
  if (!seriesDefs.length) {
    return {
      labels,
      datasets: [{
        label: trendMetrics.value.find((m) => m.key === trendMetric.value)?.label || trendMetric.value,
        data: points.map((p) => metricValue(p, trendMetric.value)),
        backgroundColor: '#3b82f6',
        borderRadius: 4,
        stack: 'mix',
        maxBarThickness: 48
      }]
    }
  }

  const datasets = seriesDefs.map((def) => ({
    label: def.label,
    data: points.map((point) => {
      const accounts = point.accounts || []
      if (def.id === -1) {
        return accounts
          .filter((a) => !topIds.has(a.account_id))
          .reduce((sum, a) => sum + metricValue(a, trendMetric.value), 0)
      }
      const hit = accounts.find((a) => a.account_id === def.id)
      return hit ? metricValue(hit, trendMetric.value) : 0
    }),
    backgroundColor: def.color,
    borderColor: def.color,
    borderWidth: 0,
    borderSkipped: false,
    borderRadius: 2,
    stack: 'mix',
    maxBarThickness: 48
  }))

  return { labels, datasets }
})

const chartData = computed(() => stackedSeries.value)

const chartOptions = computed<ChartOptions<'bar'>>(() => {
  const tick = isDark.value ? '#9ca3af' : '#6b7280'
  const grid = isDark.value ? '#374151' : '#e5e7eb'
  const labelColor = isDark.value ? '#e5e7eb' : '#374151'
  return {
    responsive: true,
    maintainAspectRatio: false,
    interaction: { mode: 'index', intersect: false },
    plugins: {
      legend: {
        position: 'bottom',
        labels: {
          color: labelColor,
          boxWidth: 10,
          boxHeight: 10,
          usePointStyle: true,
          pointStyle: 'rectRounded',
          padding: 14
        }
      },
      tooltip: {
        callbacks: {
          label(ctx: TooltipItem<'bar'>) {
            const raw = Number(ctx.raw) || 0
            const stacked = ctx.chart.data.datasets
              .filter((ds) => !ds.hidden)
              .reduce((sum, ds) => sum + (Number(ds.data[ctx.dataIndex]) || 0), 0)
            const pct = stacked !== 0 ? Math.round((raw / stacked) * 1000) / 10 : 0
            const money = `$${raw.toFixed(2)}`
            return `${ctx.dataset.label}: ${money} (${t('admin.profit.trendShare', { pct })})`
          },
          footer(items: TooltipItem<'bar'>[]) {
            if (!items.length) return ''
            const total = items.reduce((sum, it) => sum + (Number(it.raw) || 0), 0)
            return `Σ $${total.toFixed(2)}`
          }
        }
      }
    },
    scales: {
      x: {
        stacked: true,
        ticks: { color: tick },
        grid: { display: false }
      },
      y: {
        stacked: true,
        ticks: {
          color: tick,
          callback: (value) => {
            const n = Number(value)
            if (Number.isNaN(n)) return value
            return n >= 1000 || n <= -1000 ? `$${(n / 1000).toFixed(1)}k` : `$${n}`
          }
        },
        grid: { color: grid }
      }
    }
  }
})

const fmt = (value?: number) => (value ?? 0).toFixed(2)
const snapshotTimeLabel = computed(() => {
  if (!snapshotGeneratedAt.value) return ''
  const generatedAt = new Date(snapshotGeneratedAt.value)
  if (Number.isNaN(generatedAt.getTime())) return snapshotGeneratedAt.value
  return new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(generatedAt)
})

async function loadAll(forceRefresh = false) {
  loading.value = true
  trendLoading.value = true
  try {
    const overview = await adminAPI.profit.overview(startDate.value, endDate.value, forceRefresh)
    data.value = overview.summary
    currentUserBalance.value = overview.current_user_balance ?? 0
    trendPoints.value = overview.points || []
    snapshotGeneratedAt.value = overview.generated_at || ''
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.profit.loadFailed')))
  } finally {
    loading.value = false
    trendLoading.value = false
  }
}

onMounted(() => { void loadAll() })
</script>

<style scoped>
.input-sm {
  @apply rounded-lg border border-gray-200 bg-white px-2 py-1.5 text-sm text-gray-900 focus:border-primary-500 focus:outline-none dark:border-dark-600 dark:bg-dark-700 dark:text-white;
}
</style>
