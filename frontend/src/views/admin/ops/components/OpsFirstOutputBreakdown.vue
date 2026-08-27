<template>
  <section class="rounded-2xl bg-white p-4 ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700 sm:rounded-3xl sm:p-6">
    <div class="mb-3 flex min-w-0 items-center justify-between gap-3">
      <div class="min-w-0">
        <div class="flex items-center gap-2">
          <h2 class="truncate text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.ops.firstOutputBreakdown.title') }}
          </h2>
          <HelpTooltip :content="t('admin.ops.firstOutputBreakdown.tooltip')" />
        </div>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.ops.firstOutputBreakdown.samples', { count: breakdown?.eligible_streaming_requests ?? 0 }) }}
        </p>
      </div>
      <div class="shrink-0 text-right">
        <div class="text-[11px] text-gray-500 dark:text-gray-400">{{ t('admin.ops.firstOutputBreakdown.total') }}</div>
        <div class="font-mono text-lg font-semibold tabular-nums text-gray-950 dark:text-white">
          {{ formatMs(totalAverage) }}
        </div>
      </div>
    </div>

    <div class="grid grid-cols-2 gap-x-4 gap-y-3 sm:grid-cols-3 xl:grid-cols-5">
      <div v-for="stage in stages" :key="stage.key" class="min-w-0">
        <div class="mb-1.5 flex items-center justify-between gap-2 text-[11px]">
          <span class="truncate font-medium text-gray-600 dark:text-gray-300">{{ stage.label }}</span>
          <span class="shrink-0 font-mono tabular-nums text-gray-900 dark:text-white">{{ formatMs(stage.value.avg_ms) }}</span>
        </div>
        <div class="h-1.5 overflow-hidden bg-gray-200 dark:bg-white/10">
          <div class="h-full bg-emerald-500 transition-[width]" :style="{ width: `${stage.share ?? 0}%` }" />
        </div>
        <div class="mt-1 flex items-center justify-between gap-2 text-[10px] text-gray-400 dark:text-gray-500">
          <span>{{ stage.share == null ? '--' : formatPercent(stage.share / 100) }}</span>
          <span>{{ t('admin.ops.firstOutputBreakdown.coverage', { value: formatPercent(stage.value.coverage) }) }}</span>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpsFirstOutputBreakdown, OpsFirstOutputStageAverage } from '@/api/admin/ops'
import HelpTooltip from '@/components/common/HelpTooltip.vue'

const props = defineProps<{ breakdown?: OpsFirstOutputBreakdown | null }>()
const { t } = useI18n()

const emptyStage = (): OpsFirstOutputStageAverage => ({ avg_ms: null, sample_count: 0, coverage: 0 })
const totalAverage = computed(() => props.breakdown?.gateway_first_output_total?.avg_ms ?? null)

const stages = computed(() => {
  const total = totalAverage.value ?? 0
  const items = [
    ['local', t('admin.ops.firstOutputBreakdown.local'), props.breakdown?.gateway_pre_upstream ?? emptyStage()],
    ['connect', t('admin.ops.firstOutputBreakdown.connect'), props.breakdown?.upstream_connect ?? emptyStage()],
    ['upstream', t('admin.ops.firstOutputBreakdown.upstream'), props.breakdown?.upstream_first_output ?? emptyStage()],
    ['retry', t('admin.ops.firstOutputBreakdown.retry'), props.breakdown?.retry_failover ?? emptyStage()],
    ['write', t('admin.ops.firstOutputBreakdown.write'), props.breakdown?.gateway_first_output_write ?? emptyStage()]
  ] as const
  return items.map(([key, label, value]) => ({
    key,
    label,
    value,
    share: total > 0 && value.avg_ms != null ? Math.min(100, Math.max(0, (value.avg_ms / total) * 100)) : null
  }))
})

function formatMs(value?: number | null): string {
  if (value == null || !Number.isFinite(value)) return '--'
  if (value >= 1000) return `${(value / 1000).toFixed(value >= 10000 ? 1 : 2)}s`
  return `${Math.round(value)}ms`
}

function formatPercent(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '0%'
  return `${Math.min(100, value * 100).toFixed(value >= 0.1 ? 0 : 1)}%`
}
</script>
