<template>
  <button
    type="button"
    class="group w-full max-w-full rounded-md border border-transparent px-2 py-1.5 text-left transition-colors hover:border-gray-200 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-primary-500/40 dark:hover:border-dark-600 dark:hover:bg-white/[0.03]"
    :aria-label="t('admin.accounts.usageDetails.viewDetails')"
    @click="emit('open')"
  >
    <div v-if="loading" class="grid min-h-[58px] content-center gap-2" aria-busy="true">
      <div class="h-3 w-32 animate-pulse rounded bg-gray-200 dark:bg-gray-700" />
      <div class="h-2 w-full animate-pulse rounded bg-gray-200 dark:bg-gray-700" />
      <div class="h-2 w-4/5 animate-pulse rounded bg-gray-200 dark:bg-gray-700" />
    </div>

    <div v-else class="grid min-h-[58px] gap-1.5">
      <div class="flex min-w-0 items-center gap-2">
        <span v-if="showStatus" :class="['inline-flex h-5 shrink-0 items-center rounded px-1.5 text-[10px] font-semibold', statusClasses]">
          {{ presentation.statusLabel }}
        </span>
        <span v-if="presentation.plan" class="min-w-0 truncate text-[10px] text-gray-500 dark:text-gray-400" :title="presentation.plan">
          {{ presentation.plan }}
        </span>
        <span class="ml-auto inline-flex shrink-0 items-center gap-1 text-[10px] font-medium text-primary-600 opacity-70 transition-opacity group-hover:opacity-100 dark:text-primary-400">
          {{ t('admin.accounts.usageDetails.viewDetails') }}
          <Icon name="chevronRight" size="xs" />
        </span>
      </div>

      <div v-if="primaryWindows.length" class="grid gap-1">
        <div
          v-for="window in primaryWindows"
          :key="window.key"
          class="grid grid-cols-[42px_minmax(56px,1fr)_34px_48px] items-center gap-1.5 text-[10px] leading-4 tabular-nums"
        >
          <span class="truncate font-medium text-gray-600 dark:text-gray-300" :title="window.label">{{ window.label }}</span>
          <span class="h-1.5 overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700">
            <span :class="['block h-full rounded-full', windowBarClass(window.utilization)]" :style="{ width: `${Math.min(window.utilization, 100)}%` }" />
          </span>
          <span :class="windowValueClass(window.utilization)">{{ Math.round(window.utilization) }}%</span>
          <span class="truncate text-gray-400 dark:text-gray-500" :title="formatResetTime(window.resetsAt)">{{ formatResetTime(window.resetsAt) }}</span>
        </div>
      </div>

      <div v-else-if="presentation.today" class="flex min-w-0 items-center gap-2 text-[10px] tabular-nums">
        <span class="rounded bg-indigo-50 px-1.5 py-0.5 font-medium text-indigo-700 dark:bg-indigo-950/50 dark:text-indigo-300">
          {{ formatCompactNumber(presentation.today.requests, { allowBillions: false }) }} req
        </span>
        <span class="rounded bg-cyan-50 px-1.5 py-0.5 font-medium text-cyan-700 dark:bg-cyan-950/50 dark:text-cyan-300">
          {{ formatCompactNumber(presentation.today.tokens) }}
        </span>
        <span class="rounded bg-amber-50 px-1.5 py-0.5 font-medium text-amber-700 dark:bg-amber-950/50 dark:text-amber-300">
          A ${{ presentation.today.cost.toFixed(2) }}
        </span>
      </div>

      <div v-else class="flex items-center gap-1.5 text-[10px] text-gray-400 dark:text-gray-500">
        <Icon name="infoCircle" size="xs" />
        <span>{{ error || t('admin.accounts.usageDetails.noUsageData') }}</span>
      </div>

      <div v-if="presentation.today && primaryWindows.length" class="flex items-center gap-2 text-[9px] leading-3 text-gray-400 dark:text-gray-500 tabular-nums">
        <span>{{ formatCompactNumber(presentation.today.requests, { allowBillions: false }) }} req</span>
        <span>{{ formatCompactNumber(presentation.today.tokens) }}</span>
        <span>A ${{ presentation.today.cost.toFixed(2) }}</span>
      </div>
    </div>
  </button>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useIntervalFn } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { formatCompactNumber } from '@/utils/format'
import type { AccountUsagePresentation } from '@/utils/accountUsagePresentation'

const props = withDefaults(defineProps<{
  presentation: AccountUsagePresentation
  loading?: boolean
  error?: string | null
  showStatus?: boolean
}>(), {
  loading: false,
  error: null,
  showStatus: true
})

const emit = defineEmits<{
  open: []
}>()

const { t } = useI18n()
const now = ref(Date.now())
useIntervalFn(() => { now.value = Date.now() }, 60_000)

const primaryWindows = computed(() => props.presentation.windows.slice(0, 2))

const statusClasses = computed(() => ({
  neutral: 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-300',
  success: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/60 dark:text-emerald-300',
  warning: 'bg-amber-100 text-amber-700 dark:bg-amber-950/60 dark:text-amber-300',
  danger: 'bg-red-100 text-red-700 dark:bg-red-950/60 dark:text-red-300'
}[props.presentation.statusTone]))

const windowBarClass = (utilization: number) => {
  if (utilization >= 100) return 'bg-red-500 dark:bg-red-400'
  if (utilization >= 80) return 'bg-amber-500 dark:bg-amber-400'
  return 'bg-emerald-500 dark:bg-emerald-400'
}

const windowValueClass = (utilization: number) => {
  if (utilization >= 100) return 'font-semibold text-red-600 dark:text-red-300'
  if (utilization >= 80) return 'font-semibold text-amber-600 dark:text-amber-300'
  return 'text-gray-600 dark:text-gray-300'
}

const formatResetTime = (value: string | null): string => {
  if (!value) return '-'
  const resetAt = new Date(value).getTime()
  if (!Number.isFinite(resetAt)) return '-'
  const diffMinutes = Math.max(0, Math.floor((resetAt - now.value) / 60_000))
  if (diffMinutes <= 0) return t('usage.resetNow')
  const hours = Math.floor(diffMinutes / 60)
  if (hours >= 24) return `${Math.floor(hours / 24)}d ${hours % 24}h`
  if (hours > 0) return `${hours}h ${diffMinutes % 60}m`
  return `${diffMinutes}m`
}
</script>
