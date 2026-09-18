<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { Chart, LineController, LineElement, PointElement, LinearScale, Tooltip, Legend } from 'chart.js'
import { useI18n } from 'vue-i18n'
import { opsAPI, type OpsPerformanceResponse, type OpsPerformanceMetric, type OpsRequestType } from '@/api/admin/ops'
import EmptyState from '@/ui/EmptyState.vue'

Chart.register(LineController, LineElement, PointElement, LinearScale, Tooltip, Legend)

type MetricKey = 'ttft' | 'tps' | 'duration'

const props = withDefaults(defineProps<{
  timeRange: string
  platform?: string
  groupId?: number | null
  requestType?: OpsRequestType | ''
  refreshToken: number
}>(), { platform: '', groupId: null, requestType: '' })

const { t } = useI18n()
const canvas = ref<HTMLCanvasElement | null>(null)
const chart = ref<Chart | null>(null)
const loading = ref(false)
const errorMessage = ref('')
const response = ref<OpsPerformanceResponse | null>(null)
const metric = ref<MetricKey>('tps')
const compare = ref(false)
const model = ref('')

const summary = computed(() => response.value?.window?.summary)
const metricInfo = computed(() => {
  const current = summary.value
  if (!current) return null
  const map: Record<MetricKey, OpsPerformanceMetric> = {
    ttft: current.ttft,
    tps: current.generation_tps,
    duration: current.duration
  }
  return map[metric.value]
})

const metricLabel = computed(() => t(`admin.ops.performance.metrics.${metric.value}`))
const metricUnit = computed(() => metric.value === 'tps' ? 'tok/s' : 'ms')

function formatNumber(value?: number | null, digits = 0): string {
  if (value == null || !Number.isFinite(value)) return '--'
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: digits }).format(value)
}

function formatPercent(value?: number | null): string {
  if (value == null || !Number.isFinite(value)) return '--'
  return `${(value * 100).toFixed(1)}%`
}

function formatMetric(value?: number | null): string {
  if (value == null || !Number.isFinite(value)) return '--'
  if (metric.value === 'tps') return formatNumber(value, 1)
  return formatNumber(value, 0)
}

function formatDelta(value?: number | null): string {
  if (value == null || !Number.isFinite(value)) return '--'
  return `${value >= 0 ? '+' : ''}${(value * 100).toFixed(0)}%`
}

function metricValue(point: { ttft_ms?: number | null; generation_tps?: number | null; duration_ms?: number | null }): number | null {
  const value = metric.value === 'ttft' ? point.ttft_ms : metric.value === 'tps' ? point.generation_tps : point.duration_ms
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

async function loadData() {
  loading.value = true
  errorMessage.value = ''
  try {
    response.value = await opsAPI.getDashboardPerformance({
      time_range: props.timeRange as any,
      platform: props.platform || undefined,
      group_id: props.groupId || undefined,
      request_type: props.requestType || undefined,
      model: model.value || undefined,
      point_limit: 500,
      compare: compare.value,
      timezone: Intl.DateTimeFormat().resolvedOptions().timeZone
    })
    await nextTick()
    renderChart()
  } catch (error: any) {
    response.value = null
    errorMessage.value = error?.message || t('admin.ops.performance.failedToLoad')
    destroyChart()
  } finally {
    loading.value = false
  }
}

function destroyChart() {
  chart.value?.destroy()
  chart.value = null
}

function renderChart() {
  destroyChart()
  if (!canvas.value || !response.value?.window) return
  const points = response.value.window.points
  const success = points.flatMap((point) => {
    const value = metricValue(point)
    return point.success && value != null ? [{ x: new Date(point.at).getTime(), y: value }] : []
  })
  const failures = points.flatMap((point) => !point.success
    ? [{ x: new Date(point.at).getTime(), y: 0, statusCode: point.status_code, failureType: point.failure_type }]
    : [])
  chart.value = new Chart(canvas.value, {
    type: 'line',
    data: {
      datasets: [
        { label: t('admin.ops.performance.success'), data: success, borderColor: 'rgba(20, 184, 166, 0.35)', backgroundColor: '#14b8a6', pointRadius: 4, pointHoverRadius: 6, showLine: false },
        { label: t('admin.ops.performance.failures'), data: failures, borderColor: 'rgba(234, 88, 12, 0.35)', backgroundColor: '#ea580c', pointRadius: 5, pointHoverRadius: 7, showLine: false }
      ]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      animation: false,
      parsing: false,
      scales: {
        x: { type: 'linear', ticks: { maxTicksLimit: 7, callback: (value) => new Date(Number(value)).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) }, grid: { display: false } },
        y: { beginAtZero: true, title: { display: true, text: `${metricLabel.value} (${metricUnit.value})` }, grid: { color: 'rgba(148, 163, 184, 0.18)' } }
      },
      plugins: {
        legend: { display: true, position: 'bottom', labels: { usePointStyle: true, boxWidth: 8 } },
        tooltip: { callbacks: { title: (items) => items[0] ? new Date(Number(items[0].parsed.x)).toLocaleString() : '', label: (item) => `${item.dataset.label}: ${formatMetric(Number(item.parsed.y))} ${metricUnit.value}` } }
      }
    }
  })
}

watch(() => [props.timeRange, props.platform, props.groupId, props.requestType, props.refreshToken, compare.value, model.value], () => void loadData(), { immediate: true })
watch(metric, () => renderChart())
onBeforeUnmount(destroyChart)
</script>

<template>
  <section class="rounded-2xl bg-white p-4 ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700 sm:p-6">
    <div class="mb-5 flex flex-wrap items-start justify-between gap-3">
      <div>
        <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('admin.ops.performance.title') }}</h2>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.performance.description') }}</p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <label class="sr-only" for="ops-performance-model">{{ t('admin.ops.performance.model') }}</label>
        <input id="ops-performance-model" v-model="model" type="text" class="h-9 w-44 rounded-lg border border-gray-200 bg-white px-3 text-xs text-gray-900 outline-none ring-primary-500 placeholder:text-gray-400 focus:ring-2 dark:border-dark-700 dark:bg-dark-900 dark:text-white" :placeholder="t('admin.ops.performance.modelPlaceholder')" />
        <label class="flex items-center gap-2 text-xs text-gray-600 dark:text-gray-300">
          <input v-model="compare" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600" />
          {{ t('admin.ops.performance.compare') }}
        </label>
      </div>
    </div>

    <div v-if="errorMessage" class="mb-4 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-600 dark:bg-red-900/20 dark:text-red-400">{{ errorMessage }}</div>
    <div v-if="loading" class="py-10 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('admin.ops.loadingText') }}</div>
    <EmptyState v-else-if="!response?.window || response.window.summary.request_count === 0" :title="t('common.noData')" :description="t('admin.ops.performance.empty')" />

    <template v-else>
      <div class="grid grid-cols-2 gap-3 lg:grid-cols-5">
        <div v-for="card in [
          { key: 'request', label: t('admin.ops.performance.totalRequests'), value: formatNumber(summary?.request_count) },
          { key: 'success', label: t('admin.ops.performance.successRate'), value: formatPercent(summary?.success_rate) },
          { key: 'ttft', label: t('admin.ops.performance.ttft'), value: `${formatNumber(summary?.ttft.p50, 1)} / ${formatNumber(summary?.ttft.p90, 1)} ms` },
          { key: 'tps', label: t('admin.ops.performance.tps'), value: `${formatNumber(summary?.generation_tps.p50, 1)} / ${formatNumber(summary?.generation_tps.p90, 1)} tok/s` },
          { key: 'duration', label: t('admin.ops.performance.duration'), value: `${formatNumber(summary?.duration.p50, 0)} / ${formatNumber(summary?.duration.p90, 0)} ms` }
        ]" :key="card.key" class="min-w-0 rounded-xl bg-gray-50 px-3 py-3 dark:bg-dark-900">
          <div class="truncate text-[11px] font-medium text-gray-500 dark:text-gray-400">{{ card.label }}</div>
          <div class="mt-1 truncate text-lg font-semibold tabular-nums text-gray-950 dark:text-white">{{ card.value }}</div>
        </div>
      </div>

      <div class="mt-6 flex flex-wrap items-center justify-between gap-3">
        <div class="flex flex-wrap gap-1 rounded-lg bg-gray-100 p-1 dark:bg-dark-900" role="tablist">
          <button v-for="item in (['ttft', 'tps', 'duration'] as MetricKey[])" :key="item" type="button" class="rounded-md px-3 py-1.5 text-xs font-semibold" :class="metric === item ? 'bg-gray-950 text-white dark:bg-white dark:text-gray-950' : 'text-gray-500 dark:text-gray-400'" @click="metric = item">{{ t(`admin.ops.performance.metrics.${item}`) }}</button>
        </div>
        <div class="text-right text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.performance.coverage', { value: formatPercent(metricInfo?.coverage) }) }}
          <span v-if="response.window.points_sampled"> · {{ t('admin.ops.performance.sampled') }}</span>
        </div>
      </div>
      <div class="mt-3 h-[320px] w-full"><canvas ref="canvas" /></div>

      <div class="mt-4 grid gap-3 sm:grid-cols-3">
        <div class="border-l-2 border-teal-500 pl-3 text-xs text-gray-600 dark:text-gray-300">
          <div class="font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.performance.context') }}</div>
          <div class="mt-1">P50 {{ formatNumber(summary?.input_tokens.p50) }} · P90 {{ formatNumber(summary?.input_tokens.p90) }} tok</div>
          <div class="mt-1 text-gray-400">{{ t('admin.ops.performance.coverage', { value: formatPercent(summary?.input_tokens.coverage) }) }}</div>
        </div>
        <div class="border-l-2 border-amber-500 pl-3 text-xs text-gray-600 dark:text-gray-300">
          <div class="font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.performance.failures') }}</div>
          <div class="mt-1">{{ formatNumber(summary?.failure_count) }} / {{ formatNumber(summary?.request_count) }}</div>
        </div>
        <div v-if="response.comparison" class="border-l-2 border-sky-500 pl-3 text-xs text-gray-600 dark:text-gray-300">
          <div class="font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.performance.comparison') }}</div>
          <div class="mt-1">{{ formatNumber(response.comparison.summary.request_count) }} {{ t('admin.ops.performance.totalRequests').toLowerCase() }}</div>
          <div v-if="response.comparison_deltas" class="mt-1 text-sky-600 dark:text-sky-400">
            {{ t('admin.ops.performance.delta') }} {{ formatDelta(response.comparison_deltas.success_rate) }}
          </div>
        </div>
      </div>
    </template>
  </section>
</template>
