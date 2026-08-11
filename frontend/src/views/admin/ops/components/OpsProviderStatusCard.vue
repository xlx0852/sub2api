<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ProviderStatusCurrent, ProviderStatusSnapshot } from '@/api/admin/ops'

const props = defineProps<{
  current: ProviderStatusCurrent | null
  history: ProviderStatusSnapshot[]
  loading: boolean
}>()

const { t } = useI18n()

const importantComponents = computed(() => {
  const components = props.current?.snapshot?.components ?? []
  const names = ['responses', 'conversations', 'codex in chatgpt desktop']
  return components.filter((component) => names.includes(component.name.toLowerCase()))
})

const activeIncidents = computed(() => props.current?.snapshot?.incidents?.filter((incident) => incident.status !== 'resolved') ?? [])
const indicator = computed(() => props.current?.snapshot?.overall_indicator ?? 'unknown')
const operational = computed(() => indicator.value === 'none')
const statusTone = computed(() => {
  if (!props.current || props.current.freshness === 'unavailable') return 'bg-gray-100 text-gray-600 dark:bg-gray-700/50 dark:text-gray-300'
  if (props.current.freshness === 'stale') return 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300'
  return operational.value
    ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-300'
    : 'bg-rose-100 text-rose-800 dark:bg-rose-900/30 dark:text-rose-300'
})

const formatTime = (value?: string) => value ? new Date(value).toLocaleString() : t('admin.ops.providerStatus.noData')
</script>

<template>
  <section class="rounded-2xl bg-white p-4 ring-1 ring-gray-900/5 sm:rounded-3xl sm:p-6 dark:bg-dark-800 dark:ring-dark-700">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
      <div>
        <div class="flex flex-wrap items-center gap-2">
          <h3 class="text-sm font-bold text-gray-900 dark:text-white">{{ t('admin.ops.providerStatus.title') }}</h3>
          <span class="rounded-full px-2.5 py-1 text-[11px] font-bold" :class="statusTone">
            <template v-if="loading">{{ t('common.loading') }}</template>
            <template v-else-if="current?.freshness === 'unavailable'">{{ t('admin.ops.providerStatus.unavailable') }}</template>
            <template v-else-if="current?.freshness === 'stale'">{{ t('admin.ops.providerStatus.stale') }}</template>
            <template v-else>{{ operational ? t('admin.ops.providerStatus.operational') : current?.snapshot?.overall_description }}</template>
          </span>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.providerStatus.description') }}</p>
      </div>
      <a
        v-if="current?.snapshot?.source_url"
        :href="current.snapshot.source_url.replace('/api/v2/summary.json', '/')"
        target="_blank"
        rel="noopener noreferrer"
        class="text-xs font-semibold text-primary-600 hover:underline dark:text-primary-400"
      >
        {{ t('admin.ops.providerStatus.viewSource') }}
      </a>
    </div>

    <div v-if="current?.snapshot" class="mt-4 grid gap-3 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)]">
      <div class="grid grid-cols-1 gap-2 sm:grid-cols-3 lg:grid-cols-1 xl:grid-cols-3">
        <div v-for="component in importantComponents" :key="component.id" class="rounded-xl bg-gray-50 px-3 py-2 dark:bg-dark-900/70">
          <div class="truncate text-xs font-semibold text-gray-700 dark:text-gray-200">{{ component.name }}</div>
          <div class="mt-1 text-[11px]" :class="component.status === 'operational' ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-600 dark:text-rose-400'">
            {{ component.status }}
          </div>
        </div>
      </div>

      <div class="rounded-xl border border-gray-100 px-3 py-2 dark:border-dark-700">
        <div class="text-[11px] font-semibold uppercase tracking-wide text-gray-400">{{ t('admin.ops.providerStatus.activeIncidents') }}</div>
        <div v-if="activeIncidents.length" class="mt-2 space-y-2">
          <div v-for="incident in activeIncidents" :key="incident.id" class="text-xs text-gray-700 dark:text-gray-200">
            <span class="font-semibold">{{ incident.name }}</span>
            <span class="ml-2 text-gray-400">{{ incident.status }} · {{ formatTime(incident.updated_at) }}</span>
          </div>
        </div>
        <div v-else class="mt-2 text-xs text-gray-400">{{ t('admin.ops.providerStatus.noActiveIncidents') }}</div>
      </div>
    </div>

    <div class="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-gray-400">
      <span>{{ t('admin.ops.providerStatus.sourceUpdated') }} {{ formatTime(current?.snapshot?.source_updated_at) }}</span>
      <span>{{ t('admin.ops.providerStatus.fetchedAt') }} {{ formatTime(current?.snapshot?.fetched_at) }}</span>
      <span v-if="history.length">{{ t('admin.ops.providerStatus.changesInRange', { count: history.length }) }}</span>
      <span v-if="current?.last_error" class="text-amber-600 dark:text-amber-400">{{ current.last_error }}</span>
    </div>
  </section>
</template>
