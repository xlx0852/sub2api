<template>
  <div class="space-y-5">
    <section class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800 sm:p-5">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <div class="flex items-center gap-2">
            <span class="flex h-9 w-9 items-center justify-center rounded-lg bg-gray-900 text-white dark:bg-white dark:text-gray-900">
              <Icon name="sparkles" size="sm" />
            </span>
            <div>
              <h2 class="font-semibold text-gray-900 dark:text-white">{{ t('admin.profit.forecastTitle') }}</h2>
              <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.profit.forecastDescription') }}</p>
            </div>
          </div>
        </div>

        <div class="flex flex-wrap items-end gap-3">
          <label class="space-y-1">
            <span class="block text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('admin.profit.horizon') }}</span>
            <select v-model.number="horizonDays" data-testid="forecast-horizon" class="forecast-select" @change="loadForecast()">
              <option v-for="days in [7, 30, 60, 90]" :key="days" :value="days">{{ days }} {{ t('admin.profit.daysUnit').trim() }}</option>
            </select>
          </label>
          <label class="space-y-1">
            <span class="block text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('admin.profit.safetyMargin') }}</span>
            <select v-model.number="safetyMargin" data-testid="forecast-safety" class="forecast-select" @change="loadForecast()">
              <option v-for="margin in [0, 0.1, 0.2, 0.3]" :key="margin" :value="margin">{{ percent(margin) }}</option>
            </select>
          </label>
          <button
            data-testid="forecast-refresh"
            type="button"
            class="inline-flex h-9 items-center gap-1.5 rounded-lg border border-gray-200 bg-white px-3 text-sm font-medium text-gray-700 transition hover:bg-gray-50 disabled:opacity-60 dark:border-dark-600 dark:bg-dark-700 dark:text-dark-100 dark:hover:bg-dark-600"
            :disabled="loading"
            @click="loadForecast(true)"
          >
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
            {{ t('admin.profit.refreshForecast') }}
          </button>
        </div>
      </div>
    </section>

    <div v-if="loading && !forecast" class="flex min-h-72 items-center justify-center rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
      <LoadingSpinner size="lg" />
    </div>

    <section v-else-if="loadError && !forecast" class="rounded-xl border border-red-200 bg-red-50 p-5 text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200">
      <div class="flex items-start gap-3">
        <Icon name="exclamationCircle" size="lg" />
        <div>
          <h3 class="font-semibold">{{ t('admin.profit.loadFailed') }}</h3>
          <p class="mt-1 text-sm">{{ loadError }}</p>
        </div>
      </div>
    </section>

    <template v-else-if="forecast">
      <section v-if="!forecast.available" class="rounded-xl border border-amber-200 bg-amber-50 p-5 text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/30 dark:text-amber-200">
        <div class="flex items-start gap-3">
          <Icon name="exclamationTriangle" size="lg" />
          <div>
            <h3 class="font-semibold">{{ t('admin.profit.forecastUnavailable') }}</h3>
            <p class="mt-1 text-sm">{{ unavailableText(forecast.unavailable_reason) }}</p>
          </div>
        </div>
      </section>

      <section class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4" :class="loading ? 'opacity-60' : ''">
        <article class="forecast-metric">
          <div class="forecast-icon"><Icon name="dollar" /></div>
          <div>
            <p class="forecast-label">{{ t('admin.profit.storedValue') }}</p>
            <p class="forecast-value">${{ money(forecast.spendable_balance) }}</p>
            <p class="forecast-note">{{ t('admin.profit.eligibleUsers', { count: formatNumber(forecast.eligible_users) }) }} · {{ t('admin.profit.frozenBalance') }} ${{ money(forecast.frozen_balance) }}</p>
          </div>
        </article>
        <article class="forecast-metric">
          <div class="forecast-icon"><Icon name="fire" /></div>
          <div>
            <p class="forecast-label">{{ t('admin.profit.dailyBurn') }}</p>
            <p class="forecast-value">${{ money(forecast.base_daily_demand) }}</p>
            <p class="forecast-note">{{ t('admin.profit.dailyBurn7', { value: money(forecast.daily_burn_7) }) }} · {{ t('admin.profit.dailyBurn30', { value: money(forecast.daily_burn_30) }) }}</p>
          </div>
        </article>
        <article class="forecast-metric">
          <div class="forecast-icon"><Icon name="clock" /></div>
          <div>
            <p class="forecast-label">{{ t('admin.profit.runway') }}</p>
            <p class="forecast-value">{{ forecast.runway_days == null ? '—' : t('admin.profit.runwayDays', { days: decimal(forecast.runway_days) }) }}</p>
            <p class="forecast-note">{{ t('admin.profit.balanceCappedHint') }}</p>
          </div>
        </article>
        <article class="forecast-metric">
          <div class="forecast-icon"><Icon name="server" /></div>
          <div>
            <p class="forecast-label">{{ t('admin.profit.planningConsumption') }}</p>
            <p class="forecast-value">${{ money(forecast.planning_consumption) }}</p>
            <p class="forecast-note">{{ t('admin.profit.projectedConsumption', { days: forecast.horizon_days }) }} ${{ money(forecast.projected_consumption) }} · +{{ percent(forecast.safety_margin) }}</p>
          </div>
        </article>
      </section>

      <section class="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
        <header class="flex flex-wrap items-end justify-between gap-3 border-b border-gray-100 px-4 py-4 dark:border-dark-700 sm:px-5">
          <div>
            <h3 class="font-semibold text-gray-900 dark:text-white">{{ t('admin.profit.platformSupply') }}</h3>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.profit.platformSupplyHint') }}</p>
          </div>
          <p v-if="forecast.generated_at" class="text-xs text-gray-400">{{ t('admin.profit.forecastGeneratedAt', { time: generatedAt }) }}</p>
        </header>

        <div v-if="forecast.platforms.length" class="grid gap-4 p-4 lg:grid-cols-2 sm:p-5">
          <article v-for="platform in forecast.platforms" :key="platform.platform" data-testid="platform-forecast" class="rounded-xl border border-gray-200 p-4 dark:border-dark-600">
            <div class="flex items-center justify-between gap-3">
              <div class="flex min-w-0 items-center gap-2">
                <span class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-dark-200"><Icon name="cloud" size="sm" /></span>
                <div class="min-w-0">
                  <h4 class="truncate font-semibold capitalize text-gray-900 dark:text-white">{{ platform.platform }}</h4>
                  <p class="text-xs text-gray-400">{{ t('admin.profit.demandShare', { value: percent(platform.demand_share) }) }} · ${{ money(platform.planning_consumption) }}</p>
                </div>
              </div>
              <span class="rounded-full px-2 py-1 text-[11px] font-medium" :class="confidenceClass(platform.confidence)">
                {{ t('admin.profit.confidence') }} {{ confidenceText(platform.confidence) }}
              </span>
            </div>

            <div class="mt-4 grid gap-3 sm:grid-cols-2">
              <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700/70">
                <div class="flex items-center gap-1.5 text-xs font-medium text-gray-600 dark:text-dark-200"><Icon name="server" size="xs" />{{ t('admin.profit.subscriptionSupply') }}</div>
                <template v-if="platform.subscription_share > 0 && platform.required_subscription_accounts != null">
                  <div class="mt-3 grid grid-cols-2 gap-2">
                    <div><p class="text-[11px] text-gray-400">{{ t('admin.profit.requiredAccounts') }}</p><p class="mt-0.5 text-xl font-bold text-gray-900 dark:text-white">{{ platform.required_subscription_accounts }}</p></div>
                    <div><p class="text-[11px] text-gray-400">{{ t('admin.profit.currentAccounts') }}</p><p class="mt-0.5 text-xl font-bold text-gray-900 dark:text-white">{{ platform.current_subscription_accounts }}</p></div>
                  </div>
                  <div class="mt-3 flex flex-wrap items-center gap-2 text-xs">
                    <span v-if="(platform.subscription_account_gap ?? 0) > 0" class="rounded-full bg-red-50 px-2 py-1 font-medium text-red-700 dark:bg-red-950/40 dark:text-red-300">{{ t('admin.profit.accountGap', { count: platform.subscription_account_gap }) }}</span>
                    <span v-else class="rounded-full bg-emerald-50 px-2 py-1 font-medium text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300">{{ t('admin.profit.accountSurplus', { count: platform.subscription_account_surplus ?? 0 }) }}</span>
                    <span v-if="platform.quota_exhausted" class="rounded-full bg-amber-50 px-2 py-1 font-medium text-amber-700 dark:bg-amber-950/40 dark:text-amber-300">{{ t('admin.profit.quotaExhausted') }}</span>
                    <span v-if="platform.quota_snapshot_stale" class="rounded-full bg-gray-100 px-2 py-1 font-medium text-gray-500 dark:bg-dark-600 dark:text-dark-200">{{ t('admin.profit.quotaSnapshotStale') }}</span>
                  </div>
                  <p class="mt-3 text-[11px] text-gray-400">{{ t('admin.profit.quotaCapacity') }} ${{ money(platform.account_daily_capacity_quota ?? platform.account_daily_capacity_p75) }} · {{ t('admin.profit.quotaRemaining', { pct: Math.round(platform.quota_remaining_pct ?? 0) }) }} · {{ t('admin.profit.sampleCoverage', { accounts: platform.sample_accounts, days: platform.sample_account_days }) }}</p>
                </template>
                <p v-else class="mt-3 text-xs leading-5 text-gray-400">{{ platform.subscription_share <= 0 ? t('admin.profit.noSubscriptionDemand') : unavailableText(platform.subscription_unavailable_reason) }}</p>
              </div>

              <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700/70">
                <div class="flex items-center gap-1.5 text-xs font-medium text-gray-600 dark:text-dark-200"><Icon name="creditCard" size="xs" />{{ t('admin.profit.meteredSupply') }}</div>
                <template v-if="platform.metered_share > 0 && platform.metered_procurement_budget != null">
                  <p class="mt-3 text-[11px] text-gray-400">{{ t('admin.profit.meteredBudget') }}</p>
                  <p class="mt-0.5 text-xl font-bold text-gray-900 dark:text-white">${{ money(platform.metered_procurement_budget) }}</p>
                  <p class="mt-3 text-[11px] text-gray-400">{{ t('admin.profit.meteredCostRatio', { value: percent(platform.metered_cost_ratio ?? 0) }) }}</p>
                </template>
                <p v-else class="mt-3 text-xs leading-5 text-gray-400">{{ platform.metered_share <= 0 ? t('admin.profit.noMeteredDemand') : unavailableText(platform.metered_unavailable_reason) }}</p>
              </div>
            </div>
          </article>
        </div>
        <p class="border-t border-gray-100 px-5 py-3 text-xs text-gray-400 dark:border-dark-700">{{ t('admin.profit.forecastDisclaimer') }}</p>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { SupplyForecastConfidence, SupplyForecastResponse } from '@/api/admin/profit'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const forecast = ref<SupplyForecastResponse | null>(null)
const loadError = ref('')
const horizonDays = ref(30)
const safetyMargin = ref(0.2)

const numberFormatter = new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 })
const moneyFormatter = new Intl.NumberFormat(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })
const generatedAt = computed(() => {
  if (!forecast.value?.generated_at) return ''
  const date = new Date(forecast.value.generated_at)
  return Number.isNaN(date.getTime()) ? forecast.value.generated_at : new Intl.DateTimeFormat(undefined, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(date)
})

const money = (value?: number) => moneyFormatter.format(value ?? 0)
const decimal = (value: number) => new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 }).format(value)
const formatNumber = (value: number) => numberFormatter.format(value)
const percent = (value: number) => `${(value * 100).toFixed(value > 0 && value < 0.1 ? 1 : 0)}%`
const confidenceText = (confidence: SupplyForecastConfidence) => t(`admin.profit.confidence${confidence.charAt(0).toUpperCase()}${confidence.slice(1)}`)
const confidenceClass = (confidence: SupplyForecastConfidence) => ({
  high: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300',
  medium: 'bg-amber-50 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300',
  low: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300'
}[confidence])
const unavailableText = (reason?: string) => ({
  no_recent_balance_usage: t('admin.profit.forecastNoRecentUsage'),
  no_platform_mix: t('admin.profit.forecastNoPlatformMix'),
  no_subscription_capacity_sample: t('admin.profit.forecastNoSubscriptionSample'),
  no_metered_cost_sample: t('admin.profit.forecastNoMeteredSample')
}[reason || ''] || t('admin.profit.forecastUnavailable'))

async function loadForecast(forceRefresh = false) {
  loading.value = true
  try {
    forecast.value = await adminAPI.profit.supplyForecast(horizonDays.value, safetyMargin.value, forceRefresh)
    loadError.value = ''
  } catch (error) {
    loadError.value = extractApiErrorMessage(error, t('admin.profit.loadFailed'))
    appStore.showError(loadError.value)
  } finally {
    loading.value = false
  }
}

onMounted(() => { void loadForecast() })
</script>

<style scoped>
.forecast-select {
  @apply h-9 rounded-lg border border-gray-200 bg-white px-2.5 text-sm text-gray-800 focus:border-primary-500 focus:outline-none dark:border-dark-600 dark:bg-dark-700 dark:text-white;
}
.forecast-metric {
  @apply flex min-w-0 items-start gap-3 rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800;
}
.forecast-icon {
  @apply flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-dark-200;
}
.forecast-label {
  @apply text-xs font-medium text-gray-500 dark:text-dark-400;
}
.forecast-value {
  @apply mt-1 text-2xl font-bold tabular-nums text-gray-900 dark:text-white;
}
.forecast-note {
  @apply mt-1 text-[11px] leading-4 text-gray-400;
}
</style>
