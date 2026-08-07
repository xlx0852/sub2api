<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <div class="flex items-center justify-between gap-2">
        <div>
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ t('dashboard.groupAvailability.title') }}
          </h2>
          <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
            {{ t('dashboard.groupAvailability.subtitle') }}
          </p>
        </div>
        <button
          type="button"
          class="text-gray-400 transition-colors hover:text-primary-500 disabled:opacity-50"
          :disabled="loading"
          @click="reload"
        >
          <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
        </button>
      </div>
    </div>

    <div class="p-4 sm:p-6">
      <div v-if="loading && !items.length" class="flex items-center justify-center py-8 text-sm text-gray-500">
        <Icon name="refresh" size="md" class="mr-2 animate-spin" />
        {{ t('common.loading') }}
      </div>

      <div
        v-else-if="!items.length"
        class="py-6 text-center text-sm text-gray-500"
      >
        {{ t('dashboard.groupAvailability.empty') }}
      </div>

      <div v-else class="space-y-3">
        <div
          v-for="g in items"
          :key="g.group_id"
          class="rounded-lg border border-gray-200 p-3 dark:border-dark-600"
        >
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div class="flex min-w-0 items-center gap-2">
              <PlatformIcon :platform="(g.platform as any)" size="xs" />
              <span class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ g.group_name }}</span>
            </div>
            <div class="flex items-center gap-2 text-xs">
              <span
                class="rounded px-1.5 py-0.5 font-semibold"
                :class="badgeClass(g.day?.status)"
                :title="t('dashboard.groupAvailability.last24h')"
              >24h {{ fmtRate(g.day?.success_rate) }}</span>
              <span
                class="rounded px-1.5 py-0.5 font-semibold"
                :class="badgeClass(g.week?.status)"
                :title="t('dashboard.groupAvailability.last7d')"
              >7d {{ fmtRate(g.week?.success_rate) }}</span>
            </div>
          </div>

          <div v-if="g.day?.buckets?.length" class="mt-2">
            <div class="flex h-4 w-full items-end gap-[2px]">
              <div
                v-for="(b, i) in g.day.buckets"
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
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getDashboardGroupAvailability, type TrafficAvailabilityBucket, type UserGroupAvailabilityItem } from '@/api/usage'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'

const { t } = useI18n()

const items = ref<UserGroupAvailabilityItem[]>([])
const loading = ref(false)

function fmtRate(v?: number | null) {
  if (v == null) return t('dashboard.groupAvailability.noTraffic')
  return `${v.toFixed(1)}%`
}

function badgeClass(status?: string) {
  switch (status) {
    case 'healthy': return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300'
    case 'degraded': return 'bg-amber-100 text-amber-800 dark:bg-amber-950/50 dark:text-amber-200'
    case 'attention': return 'bg-red-100 text-red-700 dark:bg-red-950/40 dark:text-red-300'
    default: return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
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

function bucketTitle(b: TrafficAvailabilityBucket) {
  const time = new Date(b.start_at).toLocaleTimeString()
  const rate = b.success_rate == null ? '—' : `${b.success_rate.toFixed(1)}%`
  return `${time} · ${rate} · ok ${b.success_count}/fail ${b.failure_count}`
}

async function reload() {
  loading.value = true
  try {
    const res = await getDashboardGroupAvailability()
    items.value = res.items || []
  } catch (e) {
    items.value = []
  } finally {
    loading.value = false
  }
}

onMounted(reload)
</script>
