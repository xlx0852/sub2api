<template>
  <div
    data-testid="usage-window-row"
    :class="[
      'grid min-w-0 w-full max-w-full items-stretch gap-1.5 py-0.5',
      hasStatsRow
        ? 'grid-cols-1 sm:grid-cols-[142px_minmax(208px,1fr)]'
        : 'grid-cols-[minmax(0,142px)]'
    ]"
  >
    <div class="flex min-h-11 flex-col justify-center rounded-md border border-gray-100 bg-gray-50/80 px-2 py-1.5 dark:border-white/5 dark:bg-white/[0.03]">
      <div class="mb-1 flex items-center gap-1.5 whitespace-nowrap text-[10px] leading-3">
        <span :class="['max-w-[48px] truncate rounded px-1.5 py-0.5 font-semibold', colorClasses.badge]" :title="label">
          {{ label }}
        </span>
        <span class="ml-auto font-semibold text-gray-700 tabular-nums dark:text-gray-200">
          {{ displayPercent }}
        </span>
        <span v-if="shouldShowResetTime" class="text-gray-400 tabular-nums dark:text-gray-500">
          {{ formatResetTime }}
        </span>
      </div>
      <div class="h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700">
        <div
          data-testid="usage-progress-fill"
          :class="['h-full rounded-full transition-all duration-300', progressToneClass]"
          :style="{ width: barWidth }"
        ></div>
      </div>
    </div>

    <div v-if="hasStatsRow" class="min-w-0">
      <UsageStatLine
        v-if="showWindowStats"
        :requests="formatRequests"
        :tokens="formatTokens"
        :account-cost="formatAccountCost"
        :user-cost="windowStats?.user_cost != null ? formatUserCost : null"
        stacked
      />
      <div
        v-if="visibleExtraStats.length > 0"
        class="flex min-h-11 items-center rounded-md border border-amber-100 bg-amber-50/60 px-2 text-[10px] font-medium leading-4 text-amber-700 dark:border-amber-900/50 dark:bg-amber-950/20 dark:text-amber-300"
      >
        <span v-for="stat in visibleExtraStats" :key="stat.label" :title="stat.title">
          {{ stat.label }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useIntervalFn } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import type { WindowStats } from '@/types'
import { formatCompactNumber } from '@/utils/format'
import UsageStatLine from './UsageStatLine.vue'

interface ExtraStat {
  label: string
  title?: string
}

const props = defineProps<{
  label: string
  utilization: number // Percentage (0-100+)
  resetsAt?: string | null
  color: 'indigo' | 'emerald' | 'purple' | 'amber'
  windowStats?: WindowStats | null
  showNowWhenIdle?: boolean
  displayPercent?: string | null
  extraStats?: ExtraStat[]
}>()

const { t } = useI18n()

// Reactive clock for countdown — only runs when a reset time is shown,
// to avoid creating many idle timers across large account lists.
const now = ref(new Date())
const { pause: pauseClock, resume: resumeClock } = useIntervalFn(
  () => {
    now.value = new Date()
  },
  60_000,
  { immediate: false },
)
if (props.resetsAt) resumeClock()
watch(
  () => props.resetsAt,
  (val) => {
    if (val) {
      now.value = new Date()
      resumeClock()
    } else {
      pauseClock()
    }
  },
)

// Bar width (capped at 100%)
const barWidth = computed(() => {
  return `${Math.min(props.utilization, 100)}%`
})

// Display percentage (cap at 999% for readability)
const displayPercent = computed(() => {
  if (props.displayPercent != null) return props.displayPercent
  const percent = Math.round(props.utilization)
  return percent > 999 ? '>999%' : `${percent}%`
})

const progressToneClass = computed(() => {
  if (props.utilization >= 100) return 'bg-red-500 dark:bg-red-400'
  if (props.utilization >= 80) return 'bg-amber-500 dark:bg-amber-400'
  return 'bg-emerald-500 dark:bg-emerald-400'
})

const showWindowStats = computed(() => {
  return !!props.windowStats && (props.windowStats.requests > 0 || props.windowStats.tokens > 0)
})

const visibleExtraStats = computed(() => props.extraStats ?? [])

const hasStatsRow = computed(() => {
  return showWindowStats.value || visibleExtraStats.value.length > 0
})

const colorClasses = computed(() => ({
  indigo: {
    badge: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-950/70 dark:text-indigo-300'
  },
  emerald: {
    badge: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/70 dark:text-emerald-300'
  },
  purple: {
    badge: 'bg-purple-100 text-purple-700 dark:bg-purple-950/70 dark:text-purple-300'
  },
  amber: {
    badge: 'bg-amber-100 text-amber-700 dark:bg-amber-950/70 dark:text-amber-300'
  }
}[props.color]))

const shouldShowResetTime = computed(() => {
  if (props.resetsAt) return true
  return Boolean(props.showNowWhenIdle && props.utilization <= 0)
})

// Format reset time
const formatResetTime = computed(() => {
  // For rolling windows, when utilization is 0%, treat as immediately available.
  if (props.showNowWhenIdle && props.utilization <= 0) {
    return t('usage.resetNow')
  }

  if (!props.resetsAt) return '-'

  const date = new Date(props.resetsAt)
  const diffMs = date.getTime() - now.value.getTime()

  // resetsAt 已过期：utilization>0 说明后端窗口数据还没刷新（active poll 没回写），
  // 显示「待刷新」以区别于真正可用的「现在」。
  if (diffMs <= 0) {
    return props.utilization > 0 ? t('usage.resetPending') : t('usage.resetNow')
  }

  const diffHours = Math.floor(diffMs / (1000 * 60 * 60))
  const diffMins = Math.floor((diffMs % (1000 * 60 * 60)) / (1000 * 60))

  if (diffHours >= 24) {
    const days = Math.floor(diffHours / 24)
    return `${days}d ${diffHours % 24}h`
  } else if (diffHours > 0) {
    return `${diffHours}h ${diffMins}m`
  } else {
    return `${diffMins}m`
  }
})

// Window stats formatters
const formatRequests = computed(() => {
  if (!props.windowStats) return ''
  return formatCompactNumber(props.windowStats.requests, { allowBillions: false })
})

const formatTokens = computed(() => {
  if (!props.windowStats) return ''
  return formatCompactNumber(props.windowStats.tokens)
})

const formatAccountCost = computed(() => {
  if (!props.windowStats) return '0.00'
  return props.windowStats.cost.toFixed(2)
})

const formatUserCost = computed(() => {
  if (!props.windowStats || props.windowStats.user_cost == null) return '0.00'
  return props.windowStats.user_cost.toFixed(2)
})

</script>
