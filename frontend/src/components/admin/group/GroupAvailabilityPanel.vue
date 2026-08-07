<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between gap-2">
      <div class="flex items-center gap-2">
        <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.groups.availability.title') }}
        </h4>
        <span
          class="rounded px-1.5 py-0.5 text-[10px] font-medium text-gray-500 bg-gray-100 dark:bg-dark-700 dark:text-gray-400"
          :title="t('admin.groups.availability.sourceHint')"
        >{{ t('admin.groups.availability.sourceTag') }}</span>
      </div>
      <button
        type="button"
        class="btn btn-secondary btn-sm"
        :disabled="loading"
        @click="reload"
      >
        <Icon name="refresh" size="sm" class="mr-1" :class="loading ? 'animate-spin' : ''" />
        {{ t('common.refresh') }}
      </button>
    </div>

    <div v-if="loading && !summary" class="flex items-center justify-center py-8 text-sm text-gray-500">
      <Icon name="refresh" size="md" class="mr-2 animate-spin" />
      {{ t('common.loading') }}
    </div>

    <template v-else-if="summary">
      <!-- Summary cards -->
      <div class="grid gap-3 sm:grid-cols-2">
        <div
          v-for="win in windowCards"
          :key="win.key"
          class="rounded-lg border border-gray-200 p-3 dark:border-dark-600"
        >
          <div class="flex items-center justify-between gap-2">
            <p class="text-xs font-medium uppercase tracking-wide text-gray-400">{{ win.label }}</p>
            <span
              class="rounded px-1.5 py-0.5 text-[11px] font-semibold"
              :class="statusBadgeClass(win.data?.status)"
            >{{ statusLabel(win.data?.status) }}</span>
          </div>
          <p class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">
            {{ formatRate(win.data?.success_rate) }}
          </p>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.groups.availability.samples', { ok: win.data?.success_count ?? 0, fail: win.data?.failure_count ?? 0 }) }}
            · {{ formatLatency(win.data?.average_latency_ms) }}
          </p>
        </div>
      </div>

      <!-- 24h timeline -->
      <div v-if="dayBuckets.length">
        <div class="mb-1.5 flex items-center justify-between text-[11px] text-gray-400">
          <span>{{ t('admin.groups.availability.timeline24h') }}</span>
          <span>{{ t('admin.groups.availability.timelineHint') }}</span>
        </div>
        <div class="flex h-5 w-full items-end gap-[2px]">
          <div
            v-for="(b, i) in dayBuckets"
            :key="i"
            class="min-w-[2px] flex-1 rounded-sm"
            :class="bucketClass(b.status)"
            :style="{ height: bucketHeight(b.status) + '%' }"
            :title="bucketTitle(b)"
          />
        </div>
        <div class="mt-1 flex justify-between text-[9px] uppercase tracking-widest text-gray-400">
          <span>{{ t('monitorCommon.past') }}</span>
          <span>{{ t('monitorCommon.now') }}</span>
        </div>
      </div>
    </template>

    <div v-else class="py-6 text-center text-sm text-gray-500">
      {{ t('admin.groups.availability.loadError') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type { GroupAvailabilityPoint, GroupAvailabilitySummary } from '@/api/admin/groups'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  groupId: number | null
  active?: boolean
}>()

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const summary = ref<GroupAvailabilitySummary | null>(null)

const windowCards = computed(() => [
  { key: 'day', label: t('admin.groups.availability.last24h'), data: summary.value?.day },
  { key: 'week', label: t('admin.groups.availability.last7d'), data: summary.value?.week },
])

const dayBuckets = computed<GroupAvailabilityPoint[]>(() => summary.value?.day?.buckets || [])

function statusLabel(status?: string) {
  switch (status) {
    case 'healthy': return t('admin.groups.availability.status.healthy')
    case 'degraded': return t('admin.groups.availability.status.degraded')
    case 'attention': return t('admin.groups.availability.status.attention')
    default: return t('admin.groups.availability.status.noTraffic')
  }
}

function statusBadgeClass(status?: string) {
  switch (status) {
    case 'healthy':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300'
    case 'degraded':
      return 'bg-amber-100 text-amber-800 dark:bg-amber-950/50 dark:text-amber-200'
    case 'attention':
      return 'bg-red-100 text-red-700 dark:bg-red-950/40 dark:text-red-300'
    default:
      return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
  }
}

function bucketClass(status?: string) {
  switch (status) {
    case 'healthy': return 'bg-emerald-500'
    case 'degraded': return 'bg-amber-500'
    case 'attention': return 'bg-red-500'
    default: return 'bg-gray-300 dark:bg-dark-600'
  }
}

function bucketHeight(status?: string) {
  switch (status) {
    case 'healthy': return 100
    case 'degraded': return 65
    case 'attention': return 35
    default: return 15
  }
}

function bucketTitle(b: GroupAvailabilityPoint) {
  const time = new Date(b.start_at).toLocaleTimeString()
  const rate = b.success_rate == null ? '—' : `${b.success_rate.toFixed(1)}%`
  return `${time} · ${statusLabel(b.status)} · ${rate} · ok ${b.success_count}/fail ${b.failure_count}`
}

function formatRate(v?: number | null) {
  if (v == null) return t('admin.groups.availability.noTraffic')
  return `${v.toFixed(1)}%`
}

function formatLatency(v?: number | null) {
  if (v == null) return '—'
  if (v >= 1000) return `${(v / 1000).toFixed(2)}s`
  return `${Math.round(v)}ms`
}

async function reload() {
  if (!props.groupId) return
  loading.value = true
  try {
    summary.value = await adminAPI.groups.getAvailability(props.groupId)
  } catch (e: any) {
    summary.value = null
    appStore.showError(extractApiErrorMessage(e, t('admin.groups.availability.loadError')))
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.active !== false, props.groupId] as const,
  ([active, id]) => {
    if (active && id) reload()
    else if (!active) summary.value = null
  },
  { immediate: true },
)

defineExpose({ reload })
</script>
