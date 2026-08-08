<template>
  <AppLayout>
    <TablePageLayout transparent compact>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <div class="flex items-center gap-2">
            <div class="inline-flex rounded-lg bg-gray-100 p-1 dark:bg-dark-700">
              <button
                data-testid="profit-review-tab"
                type="button"
                class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
                :class="activeView === 'review' ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white' : 'text-gray-500 hover:text-gray-800 dark:text-dark-300 dark:hover:text-white'"
                @click="activeView = 'review'"
              >
                {{ t('admin.profit.reviewTab') }}
              </button>
              <button
                data-testid="profit-forecast-tab"
                type="button"
                class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
                :class="activeView === 'forecast' ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white' : 'text-gray-500 hover:text-gray-800 dark:text-dark-300 dark:hover:text-white'"
                @click="activeView = 'forecast'"
              >
                {{ t('admin.profit.supplyForecastTab') }}
              </button>
            </div>
            <HelpTooltip
              v-if="activeView === 'forecast'"
              :content="t('admin.profit.supplyForecastHint')"
              width-class="w-72"
            >
              <Icon name="infoCircle" size="xs" class="text-gray-400 dark:text-dark-400" />
            </HelpTooltip>
          </div>

          <div v-if="activeView === 'review'" class="ml-auto flex min-w-0 flex-wrap items-center justify-end gap-3">
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
        <SupplyForecastPanel v-if="activeView === 'forecast'" />
        <div v-else class="flex flex-col space-y-5">
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

          <QuotaWindowPanel :accounts="accountRows" @select="openAccountDrawer" />

        </div>
      </template>
    </TablePageLayout>

    <AccountProfitDrawer
      :show="showAccountDrawer"
      :account="drawerAccount"
      @close="showAccountDrawer = false"
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
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Icon from '@/components/icons/Icon.vue'
import SupplyForecastPanel from '@/components/admin/profit/SupplyForecastPanel.vue'
import QuotaWindowPanel from '@/components/admin/profit/QuotaWindowPanel.vue'
import AccountProfitDrawer from '@/components/admin/profit/AccountProfitDrawer.vue'
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
const activeView = ref<'review' | 'forecast'>('review')
const data = ref<ProfitSummaryResponse | null>(null)
const trendPoints = ref<ProfitTrendPoint[]>([])
const snapshotGeneratedAt = ref('')
const accountRows = computed(() => [...(data.value?.accounts || [])].sort((a, b) => b.profit - a.profit))
const showAccountDrawer = ref(false)
const drawerAccountId = ref<number | null>(null)
const drawerAccount = computed(() => accountRows.value.find((a) => a.account_id === drawerAccountId.value) || null)
function openAccountDrawer(accountId: number) {
  drawerAccountId.value = accountId
  showAccountDrawer.value = true
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
