<template>
  <div
    data-testid="usage-window-row"
    :class="[
      'grid items-center gap-x-2 py-px',
      hasStatsRow ? 'grid-cols-[auto_auto]' : 'grid-cols-[auto]'
    ]"
  >
    <div class="flex items-center gap-1.5 whitespace-nowrap text-[10px] leading-4">
      <span class="w-[32px] shrink-0 border-l-2 border-gray-300 pl-1 font-semibold text-gray-700 dark:border-gray-600 dark:text-gray-200">
        {{ label }}
      </span>

      <div class="h-1 w-7 shrink-0 overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700">
        <div
          class="h-full bg-gray-600 transition-all duration-300 dark:bg-gray-300"
          :style="{ width: barWidth }"
        ></div>
      </div>

      <span class="w-[32px] shrink-0 text-right font-medium text-gray-600 tabular-nums dark:text-gray-400">
        {{ displayPercent }}
      </span>

      <span v-if="shouldShowResetTime" class="shrink-0 text-gray-400 tabular-nums dark:text-gray-500">
        {{ formatResetTime }}
      </span>
    </div>

    <div v-if="hasStatsRow" class="border-l border-gray-200 pl-2 dark:border-gray-700">
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
        class="flex items-center gap-1.5 whitespace-nowrap text-[9px] leading-[13px] text-gray-500 dark:text-gray-400"
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

const showWindowStats = computed(() => {
  return !!props.windowStats && (props.windowStats.requests > 0 || props.windowStats.tokens > 0)
})

const visibleExtraStats = computed(() => props.extraStats ?? [])

const hasStatsRow = computed(() => {
  return showWindowStats.value || visibleExtraStats.value.length > 0
})

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
