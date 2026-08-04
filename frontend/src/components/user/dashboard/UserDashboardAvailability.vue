<template>
  <section class="rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
    <div class="mb-5 flex items-center justify-between gap-3">
      <div>
        <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('channelStatus.availabilityTitle') }}</h2>
        <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('channelStatus.availabilitySubtitle') }}</p>
      </div>
      <div class="flex items-center gap-2">
        <button
          type="button"
          class="inline-flex h-8 w-8 items-center justify-center rounded-lg text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 disabled:cursor-not-allowed disabled:opacity-50 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white"
          :disabled="loading"
          :title="t('common.refresh')"
          :aria-label="t('common.refresh')"
          @click="emit('refresh')"
        >
          <Icon name="refresh" size="xs" :class="loading ? 'animate-spin' : ''" />
        </button>
        <RouterLink to="/monitor" class="text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400">{{ t('channelStatus.viewAll') }}</RouterLink>
      </div>
    </div>

    <div class="mb-4 flex flex-wrap gap-2">
      <button v-for="option in platformOptions" :key="option.value" type="button" class="rounded-full border px-3 py-1.5 text-xs font-medium transition-colors" :class="platform === option.value ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300' : 'border-gray-200 text-gray-600 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700'" @click="emit('update:platform', option.value)">{{ option.label }}</button>
    </div>

    <div v-if="loading && !data" class="h-40 animate-pulse rounded-lg bg-gray-100 dark:bg-dark-700" />
    <template v-else>
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <div v-for="metric in metrics" :key="metric.label" class="rounded-lg border border-gray-100 bg-gray-50/70 px-4 py-3 dark:border-dark-700 dark:bg-dark-900/40">
          <div class="text-xs text-gray-500 dark:text-dark-400">{{ metric.label }}</div>
          <div class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ metric.value }}</div>
        </div>
      </div>

      <div class="mt-5 grid grid-cols-[repeat(auto-fill,minmax(10px,1fr))] gap-1" data-testid="availability-grid">
        <span v-for="point in displayPoints" :key="point.key" class="aspect-square min-h-2.5 rounded-[3px]" :class="pointClass(point.status)" :title="point.title" />
      </div>

      <div class="mt-4 flex flex-wrap items-center gap-x-5 gap-y-2 text-xs text-gray-500 dark:text-dark-400">
        <span v-for="legend in legends" :key="legend.label" class="inline-flex items-center gap-1.5"><i class="h-2.5 w-2.5 rounded-[3px]" :class="legend.class" />{{ legend.label }}</span>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'
import type { TrafficAvailability, TrafficAvailabilityBucket } from '@/api/usage'

const props = defineProps<{ data: TrafficAvailability | null; loading: boolean; platform: string }>()
const emit = defineEmits<{ refresh: []; 'update:platform': [value: string] }>()
const { t } = useI18n()
const platformOptions = computed(() => [
  { value: '', label: t('channelStatus.allPlatforms') },
  { value: 'openai', label: 'OpenAI' },
  { value: 'anthropic', label: 'Claude' },
  { value: 'grok', label: 'Grok' },
  { value: 'kimi', label: 'Kimi' }
])

const displayPoints = computed(() => {
  const actual = props.data?.buckets || []
  if (actual.length) return actual.map((bucket) => ({ ...bucket, key: bucket.start_at, title: bucketTitle(bucket) }))
  return Array.from({ length: 120 }, (_, index) => ({ key: `empty-${index}`, status: null, title: t('channelStatus.noSamples') }))
})
const metrics = computed(() => [
  { label: t('channelStatus.successRate'), value: props.data?.success_rate == null ? t('channelStatus.noSamples') : `${props.data.success_rate.toFixed(2)}%` },
  { label: t('channelStatus.avgLatency'), value: props.data?.average_latency_ms == null ? '-' : `${Math.round(props.data.average_latency_ms).toLocaleString()} ms` },
  { label: t('channelStatus.samples'), value: (props.data?.sample_count || 0).toLocaleString() }
])
const legends = computed(() => [
  { label: t('channelStatus.healthy'), class: 'bg-emerald-500' },
  { label: t('channelStatus.degraded'), class: 'bg-amber-400' },
  { label: t('channelStatus.attention'), class: 'bg-orange-500' }
])
const pointClass = (status: TrafficAvailabilityBucket['status'] | null) => status == null || status === 'no_traffic' ? 'bg-gray-200 dark:bg-dark-600' : status === 'healthy' ? 'bg-emerald-500' : status === 'degraded' ? 'bg-amber-400' : 'bg-orange-500'
const bucketTitle = (bucket: TrafficAvailabilityBucket) => `${new Date(bucket.start_at).toLocaleString()} · ${bucket.sample_count} · ${bucket.success_rate == null ? t('channelStatus.noSamples') : `${bucket.success_rate.toFixed(2)}%`}`
</script>
