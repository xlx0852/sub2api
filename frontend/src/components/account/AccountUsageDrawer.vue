<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition-opacity duration-200"
      enter-from-class="opacity-0"
      leave-active-class="transition-opacity duration-150"
      leave-to-class="opacity-0"
    >
      <div v-if="show && account" class="pointer-events-none fixed inset-0 z-[90]" role="presentation">
        <Transition
          appear
          enter-active-class="transition-transform duration-200 ease-out"
          enter-from-class="translate-x-full"
          leave-active-class="transition-transform duration-150 ease-in"
          leave-to-class="translate-x-full"
        >
          <aside
            v-if="show && account"
            ref="drawerRef"
            class="pointer-events-auto absolute inset-y-0 right-0 flex min-w-0 w-full flex-col overflow-hidden border-l border-gray-200 bg-white sm:w-[560px] dark:border-dark-600 dark:bg-dark-900"
            role="dialog"
            aria-modal="false"
            :aria-labelledby="drawerTitleId"
          >
            <header class="flex min-w-0 shrink-0 items-start justify-between gap-4 border-b border-gray-200 px-4 py-4 sm:px-5 dark:border-dark-600">
              <div class="min-w-0 flex-1">
                <div class="flex flex-wrap items-center gap-2">
                  <h2 :id="drawerTitleId" class="text-base font-semibold text-gray-900 dark:text-white">
                    {{ t('admin.accounts.usageDetails.title') }}
                  </h2>
                  <span :class="['inline-flex h-6 items-center rounded px-2 text-xs font-medium', statusClasses]">
                    {{ usageState.presentation.statusLabel }}
                  </span>
                </div>
                <div class="mt-3 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1.5">
                  <span class="min-w-0 truncate text-sm font-medium text-gray-800 dark:text-gray-100" :title="account.name">
                    {{ account.name }}
                  </span>
                  <span class="shrink-0 font-mono text-xs text-gray-400">#{{ account.id }}</span>
                  <div class="min-w-0 overflow-hidden">
                    <PlatformTypeBadge
                      :platform="account.platform"
                      :type="account.type"
                      :plan-type="usageState.presentation.plan || ''"
                      :privacy-mode="accountPrivacyMode"
                      :subscription-expires-at="accountSubscriptionExpiresAt"
                    />
                  </div>
                </div>
              </div>
              <button
                ref="closeButtonRef"
                type="button"
                class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 focus:outline-none focus:ring-2 focus:ring-primary-500/40 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white"
                :aria-label="t('common.close')"
                @click="emit('close')"
              >
                <Icon name="x" size="sm" />
              </button>
            </header>

            <nav class="grid shrink-0 grid-cols-4 gap-1 border-b border-gray-200 px-4 py-2 dark:border-dark-600" role="tablist">
              <button
                v-for="tab in tabs"
                :key="tab.key"
                type="button"
                role="tab"
                :aria-selected="activeTab === tab.key"
                :class="[
                  'inline-flex h-9 min-w-0 items-center justify-center gap-1.5 rounded-md px-2 text-xs font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-primary-500/40',
                  activeTab === tab.key
                    ? 'bg-primary-600 text-white dark:bg-primary-500'
                    : 'text-gray-500 hover:bg-gray-100 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white'
                ]"
                @click="activeTab = tab.key"
              >
                <Icon :name="tab.icon" size="xs" />
                <span class="truncate">{{ tab.label }}</span>
              </button>
            </nav>

            <div class="min-h-0 min-w-0 flex-1 overflow-y-auto overflow-x-hidden px-4 py-5 sm:px-5">
              <section v-show="activeTab === 'quota'" role="tabpanel" class="space-y-5">
                <div>
                  <div class="mb-4 flex items-start justify-between gap-4">
                    <div class="min-w-0">
                      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accounts.usageDetails.officialQuota') }}</h3>
                      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.usageDetails.officialQuotaHint') }}</p>
                    </div>
                    <span v-if="usageState.presentation.updatedAt" class="inline-flex shrink-0 items-center gap-1 text-xs text-gray-400 dark:text-gray-500">
                      <Icon name="clock" size="xs" />
                      {{ t('admin.accounts.usageDetails.updatedAtLabel', { time: formatRelativeTime(usageState.presentation.updatedAt, translateRelativeTime) }) }}
                    </span>
                  </div>
                  <AccountUsageCell
                    :key="`detail-${account.id}`"
                    :account="account"
                    :today-stats="todayStats"
                    :today-stats-loading="todayStatsLoading"
                    :manual-refresh-token="manualRefreshToken"
                    variant="detail"
                    @state-change="handleUsageState"
                  />
                </div>
              </section>

              <section v-if="activeTab === 'statistics'" role="tabpanel">
                <AccountStatsPanel :account="account" />
              </section>

              <section v-show="activeTab === 'performance'" role="tabpanel" class="space-y-5">
                <div>
                  <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accounts.usageDetails.performance24h') }}</h3>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.usageDetails.performanceHint') }}</p>
                </div>
                <div v-if="performanceLoading" class="grid grid-cols-2 gap-3" aria-busy="true">
                  <div v-for="index in 6" :key="index" class="h-16 animate-pulse rounded-md bg-gray-100 dark:bg-dark-700" />
                </div>
                <div v-else-if="performanceStats?.stats?.length" class="space-y-5">
                  <div v-for="stat in performanceStats.stats" :key="stat.request_type">
                    <div class="mb-3 flex items-center justify-between gap-3 border-b border-gray-200 pb-2 dark:border-dark-600">
                      <div>
                        <span class="text-sm font-semibold text-gray-900 dark:text-white">{{ requestTypeLabel(stat.request_type) }}</span>
                        <span class="ml-2 text-xs text-gray-400 dark:text-gray-500">{{ stat.request_count }} {{ t('admin.accounts.usageDetails.requestsUnit') }}</span>
                      </div>
                      <span v-if="(stat.ws_preflight_fail_count || 0) > 0" class="rounded bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-950/60 dark:text-amber-300">
                        {{ t('admin.accounts.usageDetails.preflightFailures', { count: stat.ws_preflight_fail_count || 0 }) }}
                      </span>
                    </div>
                    <div class="grid grid-cols-2 gap-x-5 gap-y-4 sm:grid-cols-3">
                      <MetricItem :label="t('admin.accounts.usageDetails.avgLatency')" :value="formatDuration(stat.avg_duration_ms)" />
                      <MetricItem :label="t('admin.accounts.usageDetails.p90Latency')" :value="formatDuration(stat.p90_duration_ms)" />
                      <MetricItem :label="t('admin.accounts.usageDetails.avgFirstToken')" :value="formatDuration(stat.avg_first_token_ms)" />
                      <MetricItem :label="t('admin.accounts.usageDetails.connectionReuse')" :value="formatPercent(stat.ws_conn_reused_rate)" />
                      <MetricItem :label="t('admin.accounts.usageDetails.queueWait')" :value="formatDuration(stat.avg_ws_queue_wait_ms)" />
                      <MetricItem :label="t('admin.accounts.usageDetails.payload')" :value="formatBytes(stat.avg_ws_payload_bytes)" />
                    </div>
                  </div>
                </div>
                <div v-else class="rounded-md border border-dashed border-gray-200 py-10 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
                  {{ t('admin.accounts.usageDetails.noPerformanceData') }}
                </div>
              </section>

              <section v-show="activeTab === 'diagnostics'" role="tabpanel" class="space-y-5">
                <div>
                  <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.accounts.usageDetails.diagnostics') }}</h3>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.usageDetails.diagnosticsHint') }}</p>
                </div>
                <dl class="divide-y divide-gray-200 border-y border-gray-200 dark:divide-dark-600 dark:border-dark-600">
                  <div class="grid grid-cols-[130px_minmax(0,1fr)] gap-4 py-3">
                    <dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.usageDetails.accountStatus') }}</dt>
                    <dd class="text-right text-xs font-medium text-gray-800 dark:text-gray-100">{{ account.status }}</dd>
                  </div>
                  <div
                    v-for="item in usageState.presentation.diagnostics"
                    :key="item.label"
                    class="grid grid-cols-[130px_minmax(0,1fr)] gap-4 py-3"
                  >
                    <dt class="text-xs text-gray-500 dark:text-gray-400">{{ item.label }}</dt>
                    <dd :class="['break-words text-right text-xs font-medium', diagnosticValueClass(item.tone)]">
                      {{ formatDiagnosticValue(item.value) }}
                    </dd>
                  </div>
                </dl>
                <div v-if="usageState.error" class="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-300">
                  {{ usageState.error }}
                </div>
                <div v-if="usageState.presentation.needsReauth" class="flex justify-end">
                  <button
                    type="button"
                    class="inline-flex h-9 items-center gap-1.5 rounded-md bg-primary-600 px-3 text-xs font-medium text-white transition-colors hover:bg-primary-700 dark:bg-primary-500 dark:hover:bg-primary-600"
                    @click="emit('reauthorize', account)"
                  >
                    <Icon name="link" size="xs" />
                    {{ t('admin.accounts.reAuthorize') }}
                  </button>
                </div>
              </section>
            </div>

          </aside>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account, AccountUsageInfo, WindowStats } from '@/types'
import type { AccountPerformanceStats } from '@/api/admin/accounts'
import type { AccountUsagePresentation, AccountUsageTone } from '@/utils/accountUsagePresentation'
import { buildAccountUsagePresentation } from '@/utils/accountUsagePresentation'
import { formatRelativeTime } from '@/utils/format'
import Icon from '@/components/icons/Icon.vue'
import PlatformTypeBadge from '@/components/common/PlatformTypeBadge.vue'
import AccountUsageCell from './AccountUsageCell.vue'
import MetricItem from './MetricItem.vue'
import AccountStatsPanel from '@/components/admin/account/AccountStatsModal.vue'

type UsageTab = 'quota' | 'statistics' | 'performance' | 'diagnostics'
type IconName = InstanceType<typeof Icon>['$props']['name']

const props = withDefaults(defineProps<{
  show: boolean
  account: Account | null
  todayStats?: WindowStats | null
  todayStatsLoading?: boolean
  performanceStats?: AccountPerformanceStats | null
  performanceLoading?: boolean
  manualRefreshToken?: number
  initialTab?: 'quota' | 'statistics'
}>(), {
  todayStats: null,
  todayStatsLoading: false,
  performanceStats: null,
  performanceLoading: false,
  manualRefreshToken: 0,
  initialTab: 'quota'
})

const emit = defineEmits<{
  close: []
  reauthorize: [account: Account]
}>()

const { t } = useI18n()
const translateRelativeTime = (key: string, params?: Record<string, number>) => t(key, params || {})
const drawerRef = ref<HTMLElement | null>(null)
const closeButtonRef = ref<HTMLButtonElement | null>(null)
const activeTab = ref<UsageTab>('quota')
const previousActiveElement = ref<HTMLElement | null>(null)
const drawerTitleId = `account-usage-drawer-title-${Math.random().toString(36).slice(2)}`

const emptyPresentation = computed(() => buildAccountUsagePresentation({
  account: props.account || ({ id: 0, platform: 'openai', type: 'apikey' } as Account),
  usageInfo: null,
  todayStats: props.todayStats,
  t
}))

const usageState = ref<{
  usageInfo: AccountUsageInfo | null
  loading: boolean
  error: string | null
  presentation: AccountUsagePresentation
}>({
  usageInfo: null,
  loading: false,
  error: null,
  presentation: emptyPresentation.value
})

const tabs = computed<Array<{ key: UsageTab; label: string; icon: IconName }>>(() => [
  { key: 'quota', label: t('admin.accounts.usageDetails.tabs.quota'), icon: 'chart' },
  { key: 'statistics', label: t('admin.accounts.usageDetails.tabs.statistics'), icon: 'bolt' },
  { key: 'performance', label: t('admin.accounts.usageDetails.tabs.performance'), icon: 'clock' },
  { key: 'diagnostics', label: t('admin.accounts.usageDetails.tabs.diagnostics'), icon: 'infoCircle' }
])

const statusClasses = computed(() => ({
  neutral: 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-300',
  success: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/60 dark:text-emerald-300',
  warning: 'bg-amber-100 text-amber-700 dark:bg-amber-950/60 dark:text-amber-300',
  danger: 'bg-red-100 text-red-700 dark:bg-red-950/60 dark:text-red-300'
}[usageState.value.presentation.statusTone]))

const accountPrivacyMode = computed(() => {
  const value = props.account?.extra?.privacy_mode
  return typeof value === 'string' ? value : props.account?.parent_privacy_mode
})

const accountSubscriptionExpiresAt = computed(() => {
  const value = props.account?.credentials?.subscription_expires_at
  return typeof value === 'string' ? value : props.account?.parent_subscription_expires_at
})

const handleUsageState = (state: typeof usageState.value) => {
  usageState.value = state
}

const handleKeydown = (event: KeyboardEvent) => {
  if (!props.show) return
  if (event.key === 'Escape') emit('close')
}

watch(
  () => props.show,
  async (show) => {
    if (show) {
      previousActiveElement.value = document.activeElement as HTMLElement | null
      await nextTick()
      closeButtonRef.value?.focus()
      window.addEventListener('keydown', handleKeydown)
      return
    }
    window.removeEventListener('keydown', handleKeydown)
    previousActiveElement.value?.focus()
  },
  { immediate: true }
)

watch(
  () => props.account?.id,
  () => {
    activeTab.value = props.initialTab
    usageState.value = {
      usageInfo: null,
      loading: false,
      error: null,
      presentation: emptyPresentation.value
    }
  }
)

watch(
  () => props.initialTab,
  (tab) => {
    if (props.show) activeTab.value = tab
  }
)

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleKeydown)
})

const formatDuration = (value?: number | null) => {
  if (value == null || !Number.isFinite(value)) return '-'
  if (value >= 1000) return `${(value / 1000).toFixed(1)}s`
  return `${Math.round(value)}ms`
}

const formatPercent = (value?: number | null) => {
  if (value == null || !Number.isFinite(value)) return '-'
  return `${Math.round(value * 100)}%`
}

const formatBytes = (value?: number | null) => {
  if (value == null || !Number.isFinite(value)) return '-'
  if (value >= 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)}MB`
  if (value >= 1024) return `${(value / 1024).toFixed(1)}KB`
  return `${Math.round(value)}B`
}

const requestTypeLabel = (requestType: string) => {
  if (requestType === 'ws_v2') return 'WS'
  if (requestType === 'stream') return t('admin.accounts.usageDetails.requestTypeStream')
  if (requestType === 'compact') return 'Compact'
  return requestType || '-'
}

const diagnosticValueClass = (tone?: AccountUsageTone) => ({
  success: 'text-emerald-600 dark:text-emerald-300',
  warning: 'text-amber-600 dark:text-amber-300',
  danger: 'text-red-600 dark:text-red-300',
  neutral: 'text-gray-800 dark:text-gray-100'
}[tone || 'neutral'])

const formatDiagnosticValue = (value: string) => {
  if (!value || value === '-') return '-'
  const timestamp = new Date(value).getTime()
  if (!Number.isFinite(timestamp) || !value.includes('T')) return value
  return formatRelativeTime(value)
}
</script>
