<template>
  <AppLayout>
    <TablePageLayout transparent>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
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
            <input v-model="startDate" type="date" class="input-sm" @change="loadAll" />
            <span class="text-gray-400">-</span>
            <input v-model="endDate" type="date" class="input-sm" @change="loadAll" />
          </div>
          <div class="ml-auto flex items-center gap-1.5 text-xs text-gray-400 dark:text-dark-400">
            <Icon name="infoCircle" size="xs" />
            <span>{{ t('admin.profit.globalOnlyHint') }}</span>
          </div>
        </div>
      </template>

      <template #table>
        <div class="space-y-5">
          <section class="grid grid-cols-1 gap-4 sm:grid-cols-3" :class="loading ? 'opacity-60' : ''">
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
          </section>

          <section class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
            <div class="mb-1 flex flex-wrap items-center justify-between gap-3">
              <div>
                <h3 class="text-sm font-semibold text-gray-800 dark:text-dark-100">{{ t('admin.profit.trendTitle') }}</h3>
                <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.profit.globalTrendHint') }}</p>
              </div>
              <LoadingSpinner v-if="trendLoading" size="sm" />
            </div>
            <div class="mt-4 h-72">
              <Line v-if="chartData" :data="chartData" :options="chartOptions" />
              <div v-else-if="!trendLoading" class="flex h-full items-center justify-center text-sm text-gray-400">{{ t('admin.profit.empty') }}</div>
            </div>
          </section>

          <section class="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
            <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700">
              <div>
                <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.profit.accountDetails') }}</h3>
                <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.profit.accountDetailsHint') }}</p>
              </div>
              <div class="flex flex-wrap items-center gap-2 text-xs font-medium">
                <span class="rounded-full bg-emerald-50 px-2.5 py-1 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300">{{ t('admin.profit.profitableAccounts', { count: profitableCount }) }}</span>
                <span v-if="lossCount" class="rounded-full bg-red-50 px-2.5 py-1 text-red-700 dark:bg-red-950/40 dark:text-red-300">{{ t('admin.profit.lossAccounts', { count: lossCount }) }}</span>
                <span class="rounded-full bg-gray-100 px-2.5 py-1 text-gray-600 dark:bg-dark-700 dark:text-dark-300">{{ t('admin.profit.accountCount', { count: accountRows.length }) }}</span>
              </div>
            </div>

            <div v-if="accountRows.length" class="max-h-[44rem] divide-y divide-gray-100 overflow-y-auto dark:divide-dark-700">
              <article
                v-for="account in accountRows"
                :key="account.account_id"
                data-testid="account-profit-item"
                class="grid gap-4 px-4 py-4 transition-colors hover:bg-gray-50/70 sm:px-5 md:grid-cols-[minmax(180px,0.9fr)_minmax(300px,1.8fr)_minmax(110px,0.55fr)] md:items-center dark:hover:bg-dark-700/40"
              >
                <div class="min-w-0">
                  <div class="flex min-w-0 items-center gap-2">
                    <div class="truncate font-semibold text-gray-900 dark:text-white">{{ account.account_name }}</div>
                    <span class="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium" :class="account.cost_type === 'subscription' ? 'bg-violet-50 text-violet-700 dark:bg-violet-950/40 dark:text-violet-300' : 'bg-sky-50 text-sky-700 dark:bg-sky-950/40 dark:text-sky-300'">
                      {{ account.cost_type === 'subscription' ? t('admin.profit.subscription') : t('admin.profit.metered') }}
                    </span>
                  </div>
                  <div class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-gray-400">
                    <span>#{{ account.account_id }} · {{ account.platform }} / {{ account.account_type }}</span>
                    <span>{{ fmtNumber(account.requests) }} {{ t('admin.profit.requestsUnit') }}</span>
                    <span v-if="account.cost_type === 'subscription' && !account.configured" class="text-amber-600 dark:text-amber-400">{{ t('admin.profit.unconfigured') }}</span>
                  </div>
                </div>

                <div class="space-y-2.5">
                  <div class="grid grid-cols-[3.25rem_minmax(80px,1fr)_5.5rem] items-center gap-2 text-xs">
                    <span class="text-gray-500 dark:text-dark-400">{{ t('admin.profit.revenue') }}</span>
                    <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-600">
                      <div data-testid="revenue-bar" class="h-full rounded-full bg-emerald-500 transition-all" :style="{ width: comparisonBarWidth(account.revenue, account) }" />
                    </div>
                    <span class="text-right font-medium tabular-nums text-gray-800 dark:text-dark-100">${{ fmt(account.revenue) }}</span>
                  </div>
                  <div class="grid grid-cols-[3.25rem_minmax(80px,1fr)_5.5rem] items-center gap-2 text-xs">
                    <span class="text-gray-500 dark:text-dark-400">{{ t('admin.profit.amortizedCost') }}</span>
                    <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-600">
                      <div data-testid="cost-bar" class="h-full rounded-full bg-amber-400 transition-all" :style="{ width: comparisonBarWidth(account.cost, account) }" />
                    </div>
                    <span class="text-right font-medium tabular-nums text-gray-600 dark:text-dark-300">${{ fmt(account.cost) }}</span>
                  </div>
                </div>

                <div class="rounded-lg bg-gray-50 px-3 py-2.5 md:text-right dark:bg-dark-700/70">
                  <div class="text-[11px] text-gray-400">{{ t('admin.profit.profit') }}</div>
                  <div class="mt-0.5 text-lg font-bold tabular-nums" :class="account.profit >= 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'">
                    {{ account.profit >= 0 ? '+' : '-' }}${{ fmt(Math.abs(account.profit)) }}
                  </div>
                  <div class="mt-0.5 text-xs font-medium tabular-nums" :class="account.profit < 0 ? 'text-red-500' : 'text-gray-500 dark:text-dark-300'">
                    {{ account.revenue > 0 ? fmtPercent(account.margin) : '—' }}
                  </div>
                </div>
              </article>
            </div>
            <div v-else-if="!loading" class="flex min-h-40 items-center justify-center px-5 py-10 text-sm text-gray-400">
              {{ t('admin.profit.empty') }}
            </div>
          </section>
        </div>
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type { AccountProfitSummary, ProfitSummaryResponse, ProfitTrendPoint } from '@/api/admin/profit'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'
import { Line } from 'vue-chartjs'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend, Filler)

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const trendLoading = ref(false)
const data = ref<ProfitSummaryResponse | null>(null)
const trendPoints = ref<ProfitTrendPoint[]>([])
const accountRows = computed(() => [...(data.value?.accounts || [])].sort((a, b) => b.profit - a.profit))
const profitableCount = computed(() => accountRows.value.filter((account) => account.profit > 0).length)
const lossCount = computed(() => accountRows.value.filter((account) => account.profit < 0).length)

const today = new Date()
const fmtDate = (date: Date) => date.toISOString().slice(0, 10)
const startDate = ref(fmtDate(new Date(today.getTime() - 6 * 86_400_000)))
const endDate = ref(fmtDate(today))
const activePreset = ref('7d')

const presets = computed(() => [
  { key: '7d', label: t('admin.profit.last7Days') },
  { key: '30d', label: t('admin.profit.last30Days') },
  { key: 'month', label: t('admin.profit.currentMonth') }
])

function applyPreset(key: string) {
  activePreset.value = key
  const now = new Date()
  if (key === '7d') startDate.value = fmtDate(new Date(now.getTime() - 6 * 86_400_000))
  if (key === '30d') startDate.value = fmtDate(new Date(now.getTime() - 29 * 86_400_000))
  if (key === 'month') startDate.value = fmtDate(new Date(now.getFullYear(), now.getMonth(), 1))
  endDate.value = fmtDate(now)
  void loadAll()
}

const isDark = computed(() => document.documentElement.classList.contains('dark'))
const chartData = computed(() => {
  if (!trendPoints.value.length) return null
  return {
    labels: trendPoints.value.map((point) => point.date.slice(5)),
    datasets: [
      { label: t('admin.profit.revenue'), data: trendPoints.value.map((point) => point.revenue), borderColor: '#10b981', backgroundColor: '#10b98120', fill: true, tension: 0.3 },
      { label: t('admin.profit.amortizedCost'), data: trendPoints.value.map((point) => point.cost), borderColor: '#f59e0b', backgroundColor: '#f59e0b20', fill: true, tension: 0.3 },
      { label: t('admin.profit.profit'), data: trendPoints.value.map((point) => point.profit), borderColor: '#3b82f6', backgroundColor: 'transparent', fill: false, tension: 0.3 }
    ]
  }
})

const chartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  plugins: { legend: { labels: { color: isDark.value ? '#e5e7eb' : '#374151' } } },
  scales: {
    x: { ticks: { color: isDark.value ? '#9ca3af' : '#6b7280' }, grid: { color: isDark.value ? '#374151' : '#e5e7eb' } },
    y: { ticks: { color: isDark.value ? '#9ca3af' : '#6b7280' }, grid: { color: isDark.value ? '#374151' : '#e5e7eb' } }
  }
}))

const fmt = (value?: number) => (value ?? 0).toFixed(2)
const fmtNumber = (value?: number) => new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(value ?? 0)
const fmtPercent = (value?: number) => `${(value ?? 0).toFixed(1)}%`
const comparisonBarWidth = (value: number, account: AccountProfitSummary) => {
  const maximum = Math.max(Math.abs(account.revenue), Math.abs(account.cost))
  if (maximum <= 0 || value === 0) return '0%'
  return `${Math.max(2, Math.min(100, Math.abs(value) / maximum * 100))}%`
}

async function loadAll() {
  loading.value = true
  trendLoading.value = true
  try {
    const overview = await adminAPI.profit.overview(startDate.value, endDate.value)
    data.value = overview.summary
    trendPoints.value = overview.points || []
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
