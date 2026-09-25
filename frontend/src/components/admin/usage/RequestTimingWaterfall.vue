<template>
  <section class="rounded-xl border border-gray-200 p-5 dark:border-dark-600" data-testid="request-timing-waterfall">
    <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('usage.timingDetail.waterfall') }}</h3>
      <div class="flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
        <span v-if="bodySize" class="rounded bg-gray-100 px-2 py-0.5 font-medium dark:bg-dark-700">
          {{ bodySize }}
        </span>
        <span>{{ t('usage.timingDetail.axisHint') }}</span>
      </div>
    </div>

    <div v-if="!lanes.length" class="py-8 text-center text-sm text-gray-400 dark:text-dark-400">
      {{ t('usage.timingDetail.notCollected') }}
    </div>

    <template v-else>
      <!-- 时间轴：0 → 总耗时 -->
      <div class="space-y-3">
        <div v-for="lane in lanes" :key="lane.key" class="flex items-center gap-3">
          <div class="w-24 shrink-0 text-xs text-gray-500 dark:text-gray-400">{{ lane.label }}</div>
          <div class="relative h-7 flex-1 overflow-hidden rounded bg-gray-100 dark:bg-dark-700">
            <div
              v-for="(segment, index) in lane.segments"
              :key="`${lane.key}-${index}`"
              class="absolute top-0 flex h-full items-center overflow-hidden px-1 text-[10px] font-medium text-white"
              :style="segmentStyle(segment)"
              :title="segmentTitle(segment)"
              :data-segment="segment.key"
            >
              <span v-if="segmentWidth(segment) >= 12" class="truncate">{{ segmentLabel(segment) }}</span>
              <span v-else-if="segmentWidth(segment) >= 5" class="truncate">{{ formatMs(segmentDuration(segment)) }}</span>
            </div>
            <div
              v-if="firstTokenPercent != null"
              class="absolute top-0 h-full w-px bg-rose-500"
              :style="{ left: firstTokenPercent + '%' }"
              :title="t('usage.timingDetail.firstToken')"
            />
          </div>
          <div class="w-16 shrink-0 text-right text-xs tabular-nums text-gray-500 dark:text-gray-400">
            {{ formatMs(lane.total) }}
          </div>
        </div>
      </div>

      <!-- 时间刻度 -->
      <div class="mt-1 flex items-center gap-3">
        <div class="w-24 shrink-0" />
        <div class="relative flex-1 text-[10px] tabular-nums text-gray-400 dark:text-dark-400">
          <span class="absolute left-0">0</span>
          <span class="absolute left-1/2 -translate-x-1/2">{{ formatMs(total / 2) }}</span>
          <span class="absolute right-0">{{ formatMs(total) }}</span>
        </div>
        <div class="w-16 shrink-0" />
      </div>

      <!-- 图例 -->
      <div class="mt-4 flex flex-wrap gap-x-3 gap-y-1.5 border-t border-gray-100 pt-3 dark:border-dark-600">
        <span v-for="item in legend" :key="item.key" class="inline-flex items-center gap-1.5 text-[11px] text-gray-500 dark:text-gray-400">
          <span class="inline-block h-2.5 w-2.5 rounded-sm" :style="{ backgroundColor: item.color }" />
          {{ item.label }}
          <span class="tabular-nums text-gray-400">{{ formatMs(item.duration) }}</span>
        </span>
        <span v-if="firstTokenPercent != null" class="inline-flex items-center gap-1.5 text-[11px] text-gray-500 dark:text-gray-400">
          <span class="inline-block h-2.5 w-0.5 bg-rose-500" />
          {{ t('usage.timingDetail.firstToken') }}
          <span class="tabular-nums text-gray-400">{{ formatMs(firstTokenMs) }}</span>
        </span>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { TimingSegment } from '@/types'

const props = defineProps<{
  segments: TimingSegment[] | null | undefined
  totalMs: number | null | undefined
  firstTokenMs?: number | null
  bodyBytes?: number | null
}>()

const { t } = useI18n()

defineOptions({ name: 'RequestTimingWaterfall' })

const SEGMENT_COLORS: Record<string, string> = {
  body_wait: '#94a3b8',
  body_receive: '#0ea5e9',
  api_key_auth: '#6366f1',
  security_audit: '#f59e0b',
  account_select: '#10b981',
  user_queue: '#a855f7',
  account_queue: '#ec4899',
  upstream_connect: '#14b8a6',
  upstream_write: '#3b82f6',
  upstream_first_byte: '#8b5cf6',
  upstream_response_headers: '#6366f1',
  upstream_stream: '#f97316'
}

const LANE_LABELS: Record<string, string> = {
  client: 'usage.timingDetail.clientToGateway',
  gateway: 'usage.timingDetail.gatewayInternal',
  upstream: 'usage.timingDetail.gatewayToUpstream'
}

const total = computed(() => {
  if (props.totalMs != null && props.totalMs > 0) return props.totalMs
  const max = Math.max(0, ...(props.segments || []).map((s) => s.end_ms ?? 0))
  return max > 0 ? max : 1
})

const lanes = computed(() => {
  const list = props.segments || []
  if (!list.length) return []
  const grouped = new Map<string, TimingSegment[]>()
  for (const segment of list) {
    const key = segment.lane === 'upstream' && segment.attempt ? `upstream#${segment.attempt}` : segment.lane
    const bucket = grouped.get(key) || []
    bucket.push(segment)
    grouped.set(key, bucket)
  }
  const order = ['client', 'gateway']
  const keys = [...grouped.keys()].sort((a, b) => {
    const ai = order.indexOf(a)
    const bi = order.indexOf(b)
    if (ai >= 0 || bi >= 0) return (ai < 0 ? 99 : ai) - (bi < 0 ? 99 : bi)
    return a.localeCompare(b)
  })
  return keys.map((key) => {
    const items = grouped.get(key) || []
    const base = key.split('#')[0]
    const attempt = key.includes('#') ? key.split('#')[1] : ''
    const label = t(LANE_LABELS[base] || base) + (attempt ? ` #${attempt}` : '')
    return { key, label, segments: items, total: Math.max(...items.map((s) => s.end_ms ?? 0), 0) }
  })
})

const legend = computed(() => {
  const seen = new Map<string, TimingSegment>()
  for (const segment of props.segments || []) {
    const existing = seen.get(segment.key)
    if (!existing) {
      seen.set(segment.key, { ...segment })
      continue
    }
    existing.end_ms = Math.max(existing.end_ms ?? 0, segment.end_ms ?? 0) + (existing.end_ms ?? 0)
  }
  return [...seen.values()].map((segment) => ({
    key: segment.key,
    label: segmentLabel(segment),
    color: SEGMENT_COLORS[segment.key] || '#64748b',
    duration: segmentDuration(segment)
  }))
})

const firstTokenPercent = computed(() => {
  if (props.firstTokenMs == null || props.firstTokenMs < 0) return null
  return Math.min(100, (props.firstTokenMs / total.value) * 100)
})

const bodySize = computed(() => {
  const bytes = props.bodyBytes
  if (bytes == null) return ''
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`
  return `${(bytes / 1024 / 1024).toFixed(2)} MiB`
})

function segmentDuration(segment: TimingSegment): number {
  return Math.max(0, (segment.end_ms ?? 0) - (segment.start_ms ?? 0))
}

function segmentLabel(segment: TimingSegment): string {
  const key = `usage.timingDetail.segment.${segment.key}`
  const translated = t(key)
  return translated === key ? segment.key : translated
}

function segmentWidth(segment: TimingSegment): number {
  return (segmentDuration(segment) / total.value) * 100
}

function segmentStyle(segment: TimingSegment) {
  return {
    left: `${((segment.start_ms ?? 0) / total.value) * 100}%`,
    width: `${Math.max(segmentWidth(segment), 0.4)}%`,
    backgroundColor: SEGMENT_COLORS[segment.key] || '#64748b'
  }
}

function segmentTitle(segment: TimingSegment): string {
  return `${segmentLabel(segment)} · ${formatMs(segment.start_ms)} → ${formatMs(segment.end_ms)}（${formatMs(segmentDuration(segment))}）`
}

function formatMs(ms: number | null | undefined): string {
  if (ms == null) return '-'
  if (ms < 1000) return `${Math.round(ms)}ms`
  if (ms >= 60_000) return `${Math.floor(ms / 60_000)}m ${((ms % 60_000) / 1000).toFixed(1)}s`
  return `${(ms / 1000).toFixed(2)}s`
}
</script>
