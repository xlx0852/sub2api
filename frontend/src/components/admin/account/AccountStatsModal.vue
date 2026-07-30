<template>
  <div class="account-stats-panel space-y-4">
    <div v-if="loading" class="flex items-center justify-center py-16">
      <LoadingSpinner />
    </div>

    <template v-else-if="stats">
      <section class="overflow-hidden rounded-2xl border border-gray-200 bg-gray-200 dark:border-dark-600 dark:bg-dark-600">
        <div class="bg-white p-4 dark:bg-dark-800">
          <div class="mb-4 flex items-center justify-between gap-3">
            <div class="flex min-w-0 items-center gap-2">
              <span class="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-emerald-50 text-emerald-600 dark:bg-emerald-950/50 dark:text-emerald-300">
                <Icon name="dollar" size="sm" />
              </span>
              <div class="min-w-0">
                <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.accounts.usageDetails.financialSummary') }}</h3>
                <p class="text-[11px] text-gray-400 dark:text-dark-400">{{ financialScopeLabel }}</p>
              </div>
            </div>
            <span v-if="profitSummary" class="shrink-0 rounded px-2 py-1 text-[11px] font-medium" :class="profitSummary.cost_type === 'subscription' ? 'bg-violet-100 text-violet-700 dark:bg-violet-900/40 dark:text-violet-300' : 'bg-sky-100 text-sky-700 dark:bg-sky-900/40 dark:text-sky-300'">
              {{ profitSummary.cost_type === 'subscription' ? t('admin.profit.subscription') : t('admin.profit.metered') }}
            </span>
          </div>

          <div v-if="profitLoading" class="grid grid-cols-3 gap-3" aria-busy="true">
            <div v-for="index in 3" :key="index" class="h-16 animate-pulse rounded-lg bg-gray-100 dark:bg-dark-700" />
          </div>
          <template v-else-if="financialMetrics">
            <div v-if="financialMetrics.missingCycle" class="mb-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-[11px] text-amber-700 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-300">
              {{ t('admin.accounts.usageDetails.missingCycleHint') }}
            </div>
            <div v-if="financialMetrics.terminated" class="mb-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-[11px] text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300">
              <div class="font-semibold">{{ t('admin.profit.banSettled') }}</div>
              <div class="mt-0.5">{{ t('admin.profit.terminatedCycleScope', { time: new Date(financialMetrics.terminatedAt).toLocaleString() }) }}</div>
              <div class="mt-1 flex flex-wrap gap-x-4 gap-y-1">
                <span>{{ t('admin.profit.receivedRefund') }} <strong>${{ formatCost(financialMetrics.refundTotal) }}</strong></span>
                <span>{{ t('admin.profit.recoveryProgress') }} <strong>{{ financialMetrics.recoveryProgress.toFixed(1) }}%</strong></span>
                <span>{{ t('admin.profit.confirmedLoss') }} <strong>${{ formatCost(financialMetrics.loss) }}</strong></span>
              </div>
            </div>
            <div class="grid grid-cols-3 gap-3">
              <div class="min-w-0">
                <p class="text-[11px] text-gray-400 dark:text-dark-400">{{ financialMetrics.revenueLabel }}</p>
                <p class="mt-1 truncate text-lg font-semibold text-gray-950 dark:text-white">${{ formatCost(financialMetrics.revenue) }}</p>
              </div>
              <div class="min-w-0">
                <p class="text-[11px] text-gray-400 dark:text-dark-400">{{ financialMetrics.costLabel }}</p>
                <p class="mt-1 truncate text-lg font-semibold text-gray-950 dark:text-white">
                  {{ financialMetrics.cost == null ? '-' : `$${formatCost(financialMetrics.cost)}` }}
                </p>
              </div>
              <div class="min-w-0">
                <p class="text-[11px] text-gray-400 dark:text-dark-400">{{ financialMetrics.profitLabel }}</p>
                <p v-if="financialMetrics.profit != null" class="mt-1 truncate text-lg font-semibold" :class="financialMetrics.profit >= 0 ? 'text-emerald-600 dark:text-emerald-300' : 'text-red-600 dark:text-red-300'">
                  {{ financialMetrics.profit >= 0 ? '+' : '' }}${{ formatCost(financialMetrics.profit) }}
                </p>
                <p v-else class="mt-1 text-lg font-semibold text-gray-400 dark:text-dark-400">-</p>
              </div>
            </div>
            <div class="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 border-t border-gray-100 pt-3 text-[11px] text-gray-500 dark:border-dark-600 dark:text-dark-400">
              <span>{{ t('admin.accounts.usageDetails.profitMargin') }} <strong class="ml-1 text-gray-800 dark:text-dark-100">{{ financialMetrics.margin == null ? '-' : `${financialMetrics.margin.toFixed(1)}%` }}</strong></span>
              <span v-if="!financialMetrics.cycle">{{ t('admin.accounts.usageDetails.profitRequests') }} <strong class="ml-1 text-gray-800 dark:text-dark-100">{{ formatNumber(financialMetrics.requests) }}</strong></span>
              <span v-if="financialMetrics.costType === 'metered'">{{ t('admin.profit.meteredCostSnapshotHint') }}</span>
            </div>
          </template>
          <p v-else class="text-sm text-gray-400 dark:text-dark-400">{{ t('admin.profit.empty') }}</p>
        </div>
      </section>

      <section class="overflow-hidden rounded-2xl border border-gray-200 bg-gray-200 dark:border-dark-600 dark:bg-dark-600">
        <div class="summary-grid grid grid-cols-2 gap-px">
          <div class="min-w-0 bg-white p-4 dark:bg-dark-800">
            <div class="flex items-center justify-between gap-3">
              <span class="text-xs font-medium text-gray-500 dark:text-dark-300">{{ t('admin.accounts.stats.totalCost') }}</span>
              <span class="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-gray-200 text-gray-600 dark:border-dark-500 dark:text-dark-200">
                <Icon name="dollar" size="sm" />
              </span>
            </div>
            <p class="mt-4 truncate text-2xl font-semibold tracking-tight text-gray-950 dark:text-white">
              ${{ formatCost(stats.summary.total_cost) }}
            </p>
            <p class="mt-1 truncate text-[11px] text-gray-400 dark:text-dark-400">
              {{ t('usage.userBilled') }} ${{ formatCost(stats.summary.total_user_cost) }}
            </p>
          </div>

          <div class="min-w-0 bg-white p-4 dark:bg-dark-800">
            <div class="flex items-center justify-between gap-3">
              <span class="text-xs font-medium text-gray-500 dark:text-dark-300">{{ t('admin.accounts.stats.totalRequests') }}</span>
              <span class="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-gray-200 text-gray-600 dark:border-dark-500 dark:text-dark-200">
                <Icon name="bolt" size="sm" />
              </span>
            </div>
            <p class="mt-4 truncate text-2xl font-semibold tracking-tight text-gray-950 dark:text-white">
              {{ formatNumber(stats.summary.total_requests) }}
            </p>
            <p class="mt-1 text-[11px] text-gray-400 dark:text-dark-400">{{ t('admin.accounts.stats.totalCalls') }}</p>
          </div>

          <div class="min-w-0 bg-white p-4 dark:bg-dark-800">
            <div class="flex items-center justify-between gap-3">
              <span class="text-xs font-medium text-gray-500 dark:text-dark-300">{{ t('admin.accounts.stats.avgDailyCost') }}</span>
              <span class="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-gray-200 text-gray-600 dark:border-dark-500 dark:text-dark-200">
                <Icon name="calculator" size="sm" />
              </span>
            </div>
            <p class="mt-4 truncate text-2xl font-semibold tracking-tight text-gray-950 dark:text-white">
              ${{ formatCost(stats.summary.avg_daily_cost) }}
            </p>
            <p class="mt-1 truncate text-[11px] text-gray-400 dark:text-dark-400">
              {{ t('usage.userBilled') }} ${{ formatCost(stats.summary.avg_daily_user_cost) }}
            </p>
          </div>

          <div class="min-w-0 bg-white p-4 dark:bg-dark-800">
            <div class="flex items-center justify-between gap-3">
              <span class="text-xs font-medium text-gray-500 dark:text-dark-300">{{ t('admin.accounts.stats.avgDailyRequests') }}</span>
              <span class="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-gray-200 text-gray-600 dark:border-dark-500 dark:text-dark-200">
                <Icon name="trendingUp" size="sm" />
              </span>
            </div>
            <p class="mt-4 truncate text-2xl font-semibold tracking-tight text-gray-950 dark:text-white">
              {{ formatNumber(Math.round(stats.summary.avg_daily_requests)) }}
            </p>
            <p class="mt-1 text-[11px] text-gray-400 dark:text-dark-400">{{ t('admin.accounts.stats.avgDailyUsage') }}</p>
          </div>
        </div>
      </section>

      <div class="highlights-grid grid grid-cols-1 gap-4">
        <section class="today-card rounded-2xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800">
          <div class="mb-4 flex items-center gap-2">
            <span class="inline-flex h-8 w-8 items-center justify-center rounded-lg bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-dark-200">
              <Icon name="clock" size="sm" />
            </span>
            <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.accounts.stats.todayOverview') }}</h3>
          </div>
          <dl class="grid grid-cols-2 gap-x-4 gap-y-3">
            <div>
              <dt class="text-[11px] text-gray-400 dark:text-dark-400">{{ t('usage.accountBilled') }}</dt>
              <dd class="mt-1 text-sm font-semibold text-gray-950 dark:text-white">${{ formatCost(stats.summary.today?.cost || 0) }}</dd>
            </div>
            <div>
              <dt class="text-[11px] text-gray-400 dark:text-dark-400">{{ t('usage.userBilled') }}</dt>
              <dd class="mt-1 text-sm font-semibold text-gray-950 dark:text-white">${{ formatCost(stats.summary.today?.user_cost || 0) }}</dd>
            </div>
            <div>
              <dt class="text-[11px] text-gray-400 dark:text-dark-400">{{ t('admin.accounts.stats.requests') }}</dt>
              <dd class="mt-1 text-sm font-semibold text-gray-950 dark:text-white">{{ formatNumber(stats.summary.today?.requests || 0) }}</dd>
            </div>
            <div>
              <dt class="text-[11px] text-gray-400 dark:text-dark-400">{{ t('admin.accounts.stats.tokens') }}</dt>
              <dd class="mt-1 text-sm font-semibold text-gray-950 dark:text-white">{{ formatTokens(stats.summary.today?.tokens || 0) }}</dd>
            </div>
          </dl>
        </section>

        <section class="rounded-2xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800">
          <div class="mb-4 flex items-center gap-2">
            <span class="inline-flex h-8 w-8 items-center justify-center rounded-lg bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-dark-200">
              <Icon name="fire" size="sm" />
            </span>
            <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.accounts.stats.highestCostDay') }}</h3>
          </div>
          <dl class="space-y-2.5">
            <div class="flex items-center justify-between gap-3">
              <dt class="text-xs text-gray-400 dark:text-dark-400">{{ t('admin.accounts.stats.date') }}</dt>
              <dd class="text-sm font-medium text-gray-950 dark:text-white">{{ stats.summary.highest_cost_day?.label || '-' }}</dd>
            </div>
            <div class="flex items-center justify-between gap-3">
              <dt class="text-xs text-gray-400 dark:text-dark-400">{{ t('usage.accountBilled') }}</dt>
              <dd class="text-sm font-semibold text-gray-950 dark:text-white">${{ formatCost(stats.summary.highest_cost_day?.cost || 0) }}</dd>
            </div>
            <div class="flex items-center justify-between gap-3">
              <dt class="text-xs text-gray-400 dark:text-dark-400">{{ t('admin.accounts.stats.requests') }}</dt>
              <dd class="text-sm font-medium text-gray-950 dark:text-white">{{ formatNumber(stats.summary.highest_cost_day?.requests || 0) }}</dd>
            </div>
          </dl>
        </section>

        <section class="rounded-2xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800">
          <div class="mb-4 flex items-center gap-2">
            <span class="inline-flex h-8 w-8 items-center justify-center rounded-lg bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-dark-200">
              <Icon name="trendingUp" size="sm" />
            </span>
            <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.accounts.stats.highestRequestDay') }}</h3>
          </div>
          <dl class="space-y-2.5">
            <div class="flex items-center justify-between gap-3">
              <dt class="text-xs text-gray-400 dark:text-dark-400">{{ t('admin.accounts.stats.date') }}</dt>
              <dd class="text-sm font-medium text-gray-950 dark:text-white">{{ stats.summary.highest_request_day?.label || '-' }}</dd>
            </div>
            <div class="flex items-center justify-between gap-3">
              <dt class="text-xs text-gray-400 dark:text-dark-400">{{ t('admin.accounts.stats.requests') }}</dt>
              <dd class="text-sm font-semibold text-gray-950 dark:text-white">{{ formatNumber(stats.summary.highest_request_day?.requests || 0) }}</dd>
            </div>
            <div class="flex items-center justify-between gap-3">
              <dt class="text-xs text-gray-400 dark:text-dark-400">{{ t('usage.accountBilled') }}</dt>
              <dd class="text-sm font-medium text-gray-950 dark:text-white">${{ formatCost(stats.summary.highest_request_day?.cost || 0) }}</dd>
            </div>
          </dl>
        </section>
      </div>

      <section class="overflow-hidden rounded-2xl border border-gray-200 bg-gray-200 dark:border-dark-600 dark:bg-dark-600">
        <div class="efficiency-grid grid grid-cols-2 gap-px">
          <div class="bg-white p-4 dark:bg-dark-800">
            <p class="text-[11px] text-gray-400 dark:text-dark-400">{{ t('admin.accounts.stats.totalTokens') }}</p>
            <p class="mt-1.5 text-lg font-semibold text-gray-950 dark:text-white">{{ formatTokens(stats.summary.total_tokens) }}</p>
          </div>
          <div class="bg-white p-4 dark:bg-dark-800">
            <p class="text-[11px] text-gray-400 dark:text-dark-400">{{ t('admin.accounts.stats.dailyAvgTokens') }}</p>
            <p class="mt-1.5 text-lg font-semibold text-gray-950 dark:text-white">{{ formatTokens(Math.round(stats.summary.avg_daily_tokens)) }}</p>
          </div>
          <div class="bg-white p-4 dark:bg-dark-800">
            <p class="text-[11px] text-gray-400 dark:text-dark-400">{{ t('admin.accounts.stats.avgResponseTime') }}</p>
            <p class="mt-1.5 text-lg font-semibold text-gray-950 dark:text-white">{{ formatDuration(stats.summary.avg_duration_ms) }}</p>
          </div>
          <div class="bg-white p-4 dark:bg-dark-800">
            <p class="text-[11px] text-gray-400 dark:text-dark-400">{{ t('admin.accounts.stats.daysActive') }}</p>
            <p class="mt-1.5 text-lg font-semibold text-gray-950 dark:text-white">{{ stats.summary.actual_days_used }} / {{ stats.summary.days }}</p>
          </div>
        </div>
      </section>

      <section class="rounded-2xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800">
        <div class="mb-4 flex items-center justify-between gap-3">
          <h3 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('admin.accounts.stats.usageTrend') }}</h3>
          <span class="text-[11px] text-gray-400 dark:text-dark-400">30D</span>
        </div>
        <div class="h-64 min-w-0">
          <Line v-if="trendChartData" :data="trendChartData" :options="lineChartOptions" />
          <div v-else class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.dashboard.noDataAvailable') }}
          </div>
        </div>
      </section>

      <div class="space-y-4">
        <ModelDistributionChart :model-stats="stats.models" :loading="false" />
        <EndpointDistributionChart
          :endpoint-stats="stats.endpoints || []"
          :loading="false"
          :title="t('usage.inboundEndpoint')"
        />
        <EndpointDistributionChart
          :endpoint-stats="stats.upstream_endpoints || []"
          :loading="false"
          :title="t('usage.upstreamEndpoint')"
        />
      </div>
    </template>

    <div v-else-if="!loading" class="flex flex-col items-center justify-center py-16 text-gray-500 dark:text-gray-400">
      <span class="mb-4 inline-flex h-12 w-12 items-center justify-center rounded-full border border-gray-200 dark:border-dark-600">
        <Icon name="chartBar" size="lg" />
      </span>
      <p class="text-sm">{{ t('admin.accounts.stats.noData') }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'
import { Line } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import ModelDistributionChart from '@/components/charts/ModelDistributionChart.vue'
import EndpointDistributionChart from '@/components/charts/EndpointDistributionChart.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import type { Account, AccountUsageStatsResponse } from '@/types'
import type { AccountProfitSummary } from '@/api/admin/profit'

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
)

const { t } = useI18n()

const props = defineProps<{
  account: Account | null
}>()

const loading = ref(false)
const stats = ref<AccountUsageStatsResponse | null>(null)
const profitLoading = ref(false)
const profitSummary = ref<AccountProfitSummary | null>(null)
let loadSequence = 0
let profitLoadSequence = 0

const financialMetrics = computed(() => {
  const summary = profitSummary.value
  if (!summary) return null

  if (summary.cost_type === 'subscription') {
    const hasActiveCycle =
      summary.billing_window_start &&
      summary.billing_window_end &&
      summary.billing_window_revenue != null &&
      summary.billing_window_cost != null &&
      summary.billing_window_profit != null

    if (hasActiveCycle) {
      const revenue = summary.billing_window_revenue!
      const profit = summary.billing_window_profit!
      return {
        cycle: true,
        missingCycle: false,
        terminated: Boolean(summary.billing_window_terminated_at),
        terminatedAt: summary.billing_window_terminated_at || '',
        refundTotal: summary.billing_window_refund_total || 0,
        recoveryProgress: summary.billing_window_recovery_progress || 0,
        loss: summary.billing_window_loss || 0,
        costType: summary.cost_type,
        requests: summary.requests,
        scopeLabel: t('admin.accounts.usageDetails.currentCycleRange', {
          start: summary.billing_window_start!.slice(0, 10),
          end: summary.billing_window_end!.slice(0, 10)
        }),
        revenueLabel: t('admin.profit.currentCycleRevenue'),
        costLabel: summary.billing_window_terminated_at || (summary.billing_window_refund_total || 0) > 0 ? t('admin.profit.netPurchaseCost') : t('admin.profit.currentCyclePurchaseCost'),
        profitLabel: summary.billing_window_terminated_at ? t('admin.profit.terminatedCycleProfit') : t('admin.profit.currentCycleProfit'),
        revenue,
        cost: summary.billing_window_cost!,
        profit,
        margin: revenue > 0 ? profit / revenue * 100 : 0
      }
    }

    return {
      cycle: false,
      missingCycle: true,
      terminated: false,
      terminatedAt: '',
      refundTotal: 0,
      recoveryProgress: 0,
      loss: 0,
      costType: summary.cost_type,
      requests: summary.requests,
      scopeLabel: t('admin.accounts.usageDetails.noActiveCycle'),
      revenueLabel: t('admin.profit.periodRevenue'),
      costLabel: t('admin.profit.currentCyclePurchaseCost'),
      profitLabel: t('admin.profit.currentCycleProfit'),
      revenue: summary.revenue,
      cost: null,
      profit: null,
      margin: null
    }
  }

  return {
    cycle: false,
    missingCycle: false,
    terminated: false,
    terminatedAt: '',
    refundTotal: 0,
    recoveryProgress: 0,
    loss: 0,
    costType: summary.cost_type,
    requests: summary.requests,
    scopeLabel: t('admin.accounts.usageDetails.last30Days'),
    revenueLabel: t('admin.profit.periodRevenue'),
    costLabel: t('admin.profit.periodUpstreamCost'),
    profitLabel: t('admin.profit.periodProfit'),
    revenue: summary.revenue,
    cost: summary.cost,
    profit: summary.profit,
    margin: summary.margin
  }
})

const financialScopeLabel = computed(() => financialMetrics.value?.scopeLabel || t('admin.accounts.usageDetails.last30Days'))

// Dark mode detection
const isDarkMode = computed(() => {
  return document.documentElement.classList.contains('dark')
})

// Chart colors
const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb'
}))

// Line chart data
const trendChartData = computed(() => {
  if (!stats.value?.history?.length) return null

  return {
    labels: stats.value.history.map((h) => h.label),
    datasets: [
      {
        label: t('usage.accountBilled') + ' (USD)',
        data: stats.value.history.map((h) => h.actual_cost),
        borderColor: '#3b82f6',
        backgroundColor: 'rgba(59, 130, 246, 0.1)',
        fill: true,
        tension: 0.3,
        yAxisID: 'y'
      },
      {
        label: t('usage.userBilled') + ' (USD)',
        data: stats.value.history.map((h) => h.user_cost),
        borderColor: '#10b981',
        backgroundColor: 'rgba(16, 185, 129, 0.08)',
        fill: false,
        tension: 0.3,
        borderDash: [5, 5],
        yAxisID: 'y'
      },
      {
        label: t('admin.accounts.stats.requests'),
        data: stats.value.history.map((h) => h.requests),
        borderColor: '#f97316',
        backgroundColor: 'rgba(249, 115, 22, 0.1)',
        fill: false,
        tension: 0.3,
        yAxisID: 'y1'
      }
    ]
  }
})

// Line chart options with dual Y-axis
const lineChartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: {
    intersect: false,
    mode: 'index' as const
  },
  plugins: {
    legend: {
      position: 'top' as const,
      labels: {
        color: chartColors.value.text,
        usePointStyle: true,
        pointStyle: 'circle',
        padding: 15,
        font: {
          size: 11
        }
      }
    },
    tooltip: {
      callbacks: {
        label: (context: any) => {
          const label = context.dataset.label || ''
          const value = context.raw
          if (label.includes('USD')) {
            return `${label}: $${formatCost(value)}`
          }
          return `${label}: ${formatNumber(value)}`
        }
      }
    }
  },
  scales: {
    x: {
      grid: {
        color: chartColors.value.grid
      },
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10
        },
        maxRotation: 45,
        minRotation: 0
      }
    },
    y: {
      type: 'linear' as const,
      display: true,
      position: 'left' as const,
      grid: {
        color: chartColors.value.grid
      },
      ticks: {
        color: '#3b82f6',
        font: {
          size: 10
        },
        callback: (value: string | number) => '$' + formatCost(Number(value))
      },
      title: {
        display: true,
        text: t('usage.accountBilled') + ' (USD)',
        color: '#3b82f6',
        font: {
          size: 11
        }
      }
    },
    y1: {
      type: 'linear' as const,
      display: true,
      position: 'right' as const,
      grid: {
        drawOnChartArea: false
      },
      ticks: {
        color: '#f97316',
        font: {
          size: 10
        },
        callback: (value: string | number) => formatNumber(Number(value))
      },
      title: {
        display: true,
        text: t('admin.accounts.stats.requests'),
        color: '#f97316',
        font: {
          size: 11
        }
      }
    }
  }
}))

const loadStats = async () => {
  if (!props.account) return

  const accountID = props.account.id
  const sequence = ++loadSequence
  loading.value = true
  try {
    const response = await adminAPI.accounts.getStats(accountID, 30)
    if (sequence === loadSequence && props.account?.id === accountID) {
      stats.value = response
    }
  } catch (error) {
    if (sequence === loadSequence && props.account?.id === accountID) {
      console.error('Failed to load account stats:', error)
      stats.value = null
    }
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

const loadProfitSummary = async () => {
  if (!props.account) return

  const accountID = props.account.id
  const sequence = ++profitLoadSequence
  const end = new Date()
  const start = new Date(end.getTime() - 29 * 86_400_000)
  profitLoading.value = true
  try {
    const response = await adminAPI.profit.summary(
      start.toISOString().slice(0, 10),
      end.toISOString().slice(0, 10),
      accountID
    )
    if (sequence === profitLoadSequence && props.account?.id === accountID) {
      profitSummary.value = response.accounts[0] || null
    }
  } catch (error) {
    if (sequence === profitLoadSequence && props.account?.id === accountID) {
      console.error('Failed to load account profit summary:', error)
      profitSummary.value = null
    }
  } finally {
    if (sequence === profitLoadSequence) profitLoading.value = false
  }
}

watch(
  () => props.account?.id,
  async (accountID) => {
    if (accountID && props.account) {
      await Promise.all([loadStats(), loadProfitSummary()])
    } else {
      loadSequence += 1
      profitLoadSequence += 1
      stats.value = null
      profitSummary.value = null
      loading.value = false
      profitLoading.value = false
    }
  },
  { immediate: true }
)

// Format helpers
const formatCost = (value: number): string => {
  const absolute = Math.abs(value)
  if (absolute >= 1000) {
    return (value / 1000).toFixed(2) + 'K'
  } else if (absolute >= 1) {
    return value.toFixed(2)
  } else if (absolute >= 0.01) {
    return value.toFixed(3)
  }
  return value.toFixed(4)
}

const formatNumber = (value: number): string => {
  if (value >= 1_000_000) {
    return (value / 1_000_000).toFixed(2) + 'M'
  } else if (value >= 1_000) {
    return (value / 1_000).toFixed(2) + 'K'
  }
  return value.toLocaleString()
}

const formatTokens = (value: number): string => {
  if (value >= 1_000_000_000) {
    return `${(value / 1_000_000_000).toFixed(2)}B`
  } else if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(2)}M`
  } else if (value >= 1_000) {
    return `${(value / 1_000).toFixed(2)}K`
  }
  return value.toLocaleString()
}

const formatDuration = (ms: number): string => {
  if (ms >= 1000) {
    return `${(ms / 1000).toFixed(2)}s`
  }
  return `${Math.round(ms)}ms`
}
</script>

<style scoped>
.account-stats-panel {
  container-type: inline-size;
}

@container (min-width: 440px) {
  .highlights-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .today-card {
    grid-column: 1 / -1;
  }
}

@container (min-width: 760px) {
  .summary-grid,
  .efficiency-grid {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }

  .highlights-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .today-card {
    grid-column: auto;
  }
}
</style>
