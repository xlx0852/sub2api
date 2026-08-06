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
        v-if="showFullUtilizationEstimate"
        data-testid="usage-full-estimate"
        class="mt-1 rounded-md border border-dashed border-gray-200 bg-white/70 p-1.5 dark:border-dark-600 dark:bg-dark-800/40"
      >
        <div class="mb-1 text-[9px] font-medium text-gray-400 dark:text-gray-500">
          {{ t('usage.fullUtilizationEstimate') }}
        </div>
        <UsageStatLine
          :requests="formatEstimatedRequests"
          :tokens="formatEstimatedTokens"
          :account-cost="formatEstimatedAccountCost"
          :user-cost="displayedFullEstimate?.user_cost != null ? formatEstimatedUserCost : null"
          stacked
        />
      </div>
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
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useIntervalFn } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import type { WindowStats } from '@/types'
import { formatCompactNumber } from '@/utils/format'
import UsageStatLine from './UsageStatLine.vue'

interface ExtraStat {
  label: string
  title?: string
}

// 满额预估展示稳定：上游 utilization / window_stats 轮询会抖，
// factor=100/util 对抖动极敏感。展示层做短防抖 + 相对变化阈值。
const FULL_ESTIMATE_DEBOUNCE_MS = 1200
const FULL_ESTIMATE_MIN_REL_CHANGE = 0.04 // 预估总值相对变化 <4% 不更新展示
const FULL_ESTIMATE_MIN_UTIL_DELTA = 0.6 // 利用率绝对变化 <0.6pp 视为噪声

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

// Linear projection: current local window usage / upstream utilized share.
// Values above 100% are excluded because they no longer describe a useful
// "full quota" projection and would produce an estimate below actual usage.
function computeRawFullEstimate(
  utilization: number,
  stats: WindowStats | null | undefined
): WindowStats | null {
  if (!stats) return null
  if (!(stats.requests > 0 || stats.tokens > 0)) return null
  if (utilization <= 0 || utilization > 100) return null
  const factor = 100 / utilization
  return {
    requests: stats.requests * factor,
    tokens: stats.tokens * factor,
    cost: stats.cost * factor,
    user_cost: stats.user_cost == null ? undefined : stats.user_cost * factor
  }
}

function estimateSignature(stats: WindowStats | null): number {
  if (!stats) return 0
  // 用用户扣费优先；没有则用账号成本/tokens 作量级锚点
  if (stats.user_cost != null && stats.user_cost > 0) return stats.user_cost
  if (stats.cost > 0) return stats.cost
  return stats.tokens
}

const displayedFullEstimate = ref<WindowStats | null>(
  computeRawFullEstimate(props.utilization, props.windowStats)
)
const lastCommittedUtil = ref(props.utilization)
let fullEstimateTimer: ReturnType<typeof setTimeout> | null = null

function commitFullEstimate(next: WindowStats | null, utilization: number) {
  displayedFullEstimate.value = next
  lastCommittedUtil.value = utilization
}

function shouldCommitFullEstimate(
  prev: WindowStats | null,
  next: WindowStats | null,
  prevUtil: number,
  nextUtil: number
): boolean {
  if (next == null) return prev != null
  if (prev == null) return true
  // 利用率从可预估区间掉出/进入已在 next==null 分支处理
  const utilDelta = Math.abs(nextUtil - prevUtil)
  const prevSig = estimateSignature(prev)
  const nextSig = estimateSignature(next)
  if (prevSig <= 0) return true
  const rel = Math.abs(nextSig - prevSig) / prevSig
  // 小抖动：利用率与预估量级都几乎不变 → 保持旧展示
  if (utilDelta < FULL_ESTIMATE_MIN_UTIL_DELTA && rel < FULL_ESTIMATE_MIN_REL_CHANGE) {
    return false
  }
  return true
}

function scheduleFullEstimateUpdate() {
  const next = computeRawFullEstimate(props.utilization, props.windowStats)
  // 立刻隐藏：利用率归零/超额时预估无意义，不应防抖拖着旧数
  if (next == null) {
    if (fullEstimateTimer != null) {
      clearTimeout(fullEstimateTimer)
      fullEstimateTimer = null
    }
    commitFullEstimate(null, props.utilization)
    return
  }
  // 首次有值：立即展示，避免空白等 1.2s
  if (displayedFullEstimate.value == null) {
    commitFullEstimate(next, props.utilization)
    return
  }
  if (fullEstimateTimer != null) clearTimeout(fullEstimateTimer)
  fullEstimateTimer = setTimeout(() => {
    fullEstimateTimer = null
    const candidate = computeRawFullEstimate(props.utilization, props.windowStats)
    if (
      shouldCommitFullEstimate(
        displayedFullEstimate.value,
        candidate,
        lastCommittedUtil.value,
        props.utilization
      )
    ) {
      commitFullEstimate(candidate, props.utilization)
    }
  }, FULL_ESTIMATE_DEBOUNCE_MS)
}

watch(
  () => [props.utilization, props.windowStats] as const,
  () => {
    scheduleFullEstimateUpdate()
  },
  { deep: true }
)

onBeforeUnmount(() => {
  if (fullEstimateTimer != null) {
    clearTimeout(fullEstimateTimer)
    fullEstimateTimer = null
  }
})

const showFullUtilizationEstimate = computed(() => displayedFullEstimate.value != null)

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

const formatEstimatedRequests = computed(() =>
  formatCompactNumber(Math.round(displayedFullEstimate.value?.requests ?? 0), { allowBillions: false })
)
const formatEstimatedTokens = computed(() => formatCompactNumber(displayedFullEstimate.value?.tokens ?? 0))
const formatEstimatedAccountCost = computed(() => (displayedFullEstimate.value?.cost ?? 0).toFixed(2))
const formatEstimatedUserCost = computed(() => (displayedFullEstimate.value?.user_cost ?? 0).toFixed(2))

</script>
