<template>
  <BaseDialog
    :show="show"
    :title="account?.account_name || t('admin.profit.accountDetails')"
    variant="drawer"
    width="wide"
    @close="emit('close')"
  >
    <div v-if="!account" class="py-10 text-center text-sm text-gray-500">
      {{ t('admin.profit.empty') }}
    </div>

    <div v-else class="space-y-4">
      <!-- Identity -->
      <div class="space-y-1">
        <div class="flex flex-wrap items-center gap-2">
          <span class="truncate text-base font-semibold text-gray-900 dark:text-white">{{ account.account_name }}</span>
          <span
            class="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium"
            :class="account.cost_type === 'subscription'
              ? 'bg-violet-50 text-violet-700 dark:bg-violet-950/40 dark:text-violet-300'
              : 'bg-sky-50 text-sky-700 dark:bg-sky-950/40 dark:text-sky-300'"
          >{{ account.cost_type === 'subscription' ? t('admin.profit.subscription') : t('admin.profit.metered') }}</span>
          <span
            v-if="account.deleted"
            class="shrink-0 rounded bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-600 dark:bg-dark-600 dark:text-dark-200"
          >{{ t('admin.profit.deletedAccount') }}</span>
          <span
            v-if="activeStatus"
            class="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium"
            :class="statusClass(activeStatus)"
          >{{ statusLabel(activeStatus) }}</span>
        </div>
        <p class="text-xs text-gray-400">
          #{{ account.account_id }} · {{ account.platform }} / {{ account.account_type }}
          · {{ fmtNumber(requestCount) }} {{ t('admin.profit.requestsUnit') }}
        </p>
        <p v-if="windowRangeLabel" class="text-xs text-gray-500 dark:text-dark-400">
          {{ windowKindLabel }}: {{ windowRangeLabel }}
        </p>
        <div
          v-if="account.break_even_rate != null"
          class="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[11px]"
          :title="breakEvenTitle"
        >
          <span class="rounded bg-violet-50 px-1.5 py-0.5 font-semibold tabular-nums text-violet-700 dark:bg-violet-950/40 dark:text-violet-300">
            {{ t('admin.profit.breakEvenRate') }} ×{{ fmtRate(account.break_even_rate) }}
          </span>
          <span v-if="account.break_even_current_rate != null" class="text-gray-400 dark:text-dark-400">
            {{ t('admin.profit.breakEvenCurrentRate') }} ×{{ fmtRate(account.break_even_current_rate) }}
          </span>
        </div>
      </div>

      <!-- Selected window economics -->
      <div class="space-y-2.5 rounded-lg border border-gray-100 p-3 dark:border-dark-700">
        <div class="grid grid-cols-[3.25rem_minmax(80px,1fr)_5.5rem] items-center gap-2 text-xs">
          <span class="text-gray-500 dark:text-dark-400">{{ t('admin.profit.revenue') }}</span>
          <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-600">
            <div class="h-full rounded-full bg-emerald-500 transition-all" :style="{ width: barWidth(revenue) }" />
          </div>
          <span class="text-right font-medium tabular-nums text-gray-800 dark:text-dark-100">${{ fmt(revenue) }}</span>
        </div>
        <div class="grid grid-cols-[3.25rem_minmax(80px,1fr)_5.5rem] items-center gap-2 text-xs">
          <span class="text-gray-500 dark:text-dark-400">{{ t('admin.profit.amortizedCost') }}</span>
          <div class="h-2 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-600">
            <div class="h-full rounded-full bg-amber-400 transition-all" :style="{ width: barWidth(cost) }" />
          </div>
          <span class="text-right font-medium tabular-nums text-gray-600 dark:text-dark-300">${{ fmt(cost) }}</span>
        </div>
      </div>

      <div class="rounded-lg bg-gray-50 px-3 py-2.5 dark:bg-dark-700/70">
        <div class="text-[11px] text-gray-400">{{ t('admin.profit.profit') }}</div>
        <div
          class="mt-0.5 text-lg font-bold tabular-nums"
          :class="profit >= 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'"
        >
          {{ profit >= 0 ? '+' : '-' }}${{ fmt(Math.abs(profit)) }}
        </div>
        <div class="mt-0.5 text-xs font-medium tabular-nums" :class="profit < 0 ? 'text-red-500' : 'text-gray-500 dark:text-dark-300'">
          {{ marginLabel }}
        </div>
      </div>

      <!-- History list -->
      <div class="space-y-2">
        <div class="flex items-center justify-between gap-2">
          <h4 class="text-xs font-semibold text-gray-700 dark:text-dark-200">{{ t('admin.profit.windowHistoryTitle') }}</h4>
          <LoadingSpinner v-if="historyLoading" size="sm" />
        </div>
        <p class="text-[11px] leading-4 text-gray-400">{{ t('admin.profit.windowHistoryHint') }}</p>

        <div v-if="!historyLoading && !historyRows.length" class="rounded-lg border border-dashed border-gray-200 px-3 py-4 text-center text-xs text-gray-400 dark:border-dark-600">
          {{ t('admin.profit.windowHistoryEmpty') }}
        </div>

        <div v-else class="max-h-72 space-y-1.5 overflow-y-auto pr-0.5">
          <button
            v-for="item in historyRows"
            :key="`${item.start_at}-${item.end_at}`"
            type="button"
            class="w-full rounded-lg border px-2.5 py-2 text-left transition"
            :class="isActive(item)
              ? 'border-primary-300 bg-primary-50/70 dark:border-primary-700 dark:bg-primary-950/30'
              : 'border-gray-100 hover:border-gray-200 hover:bg-gray-50 dark:border-dark-700 dark:hover:border-dark-600 dark:hover:bg-dark-700/50'"
            @click="emit('select-window', item)"
          >
            <div class="flex items-center justify-between gap-2">
              <div class="min-w-0">
                <div class="truncate text-[11px] font-semibold text-gray-800 dark:text-dark-100">
                  {{ formatRange(item.start_at, item.end_at) }}
                </div>
                <div class="mt-0.5 flex flex-wrap items-center gap-1.5 text-[10px] text-gray-400">
                  <span class="rounded px-1 py-0.5 font-medium" :class="statusClass(item.status)">{{ statusLabel(item.status) }}</span>
                  <span v-if="item.kind">{{ item.kind }}</span>
                  <span>{{ fmtNumber(item.requests) }} {{ t('admin.profit.requestsUnit') }}</span>
                </div>
              </div>
              <div class="shrink-0 text-right">
                <div
                  class="text-xs font-bold tabular-nums"
                  :class="item.profit >= 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'"
                >
                  {{ item.profit >= 0 ? '+' : '-' }}${{ fmt(Math.abs(item.profit)) }}
                </div>
                <div class="text-[10px] tabular-nums text-gray-400">
                  ${{ fmt(item.revenue) }} / ${{ fmt(item.cost) }}
                </div>
              </div>
            </div>
          </button>
        </div>
      </div>

      <p class="text-[11px] leading-4 text-gray-400">
        {{ drawerHint }}
      </p>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AccountProfitSummary, ProfitWindowEconomicsItem } from '@/api/admin/profit'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'

const props = defineProps<{
  show: boolean
  account: AccountProfitSummary | null
  selectedWindow?: ProfitWindowEconomicsItem | null
  windowHistory?: ProfitWindowEconomicsItem[]
  historyLoading?: boolean
}>()

const emit = defineEmits<{
  close: []
  'select-window': [item: ProfitWindowEconomicsItem]
}>()
const { t } = useI18n()

const active = computed(() => props.selectedWindow || null)
const historyRows = computed(() => props.windowHistory || [])

const revenue = computed(() => {
  if (active.value) return Number(active.value.revenue) || 0
  // Prefer drawer quota window (7d/5h); never the pool cycle billing window.
  const a = props.account
  if (a?.drawer_quota_revenue != null) return a.drawer_quota_revenue
  if (a?.billing_window_revenue != null && a.billing_window_source === 'quota_window') return a.billing_window_revenue
  return a?.revenue ?? 0
})
const cost = computed(() => {
  if (active.value) return Number(active.value.cost) || 0
  const a = props.account
  if (a?.drawer_quota_cost != null) return a.drawer_quota_cost
  if (a?.billing_window_cost != null && a.billing_window_source === 'quota_window') return a.billing_window_cost
  return a?.cost ?? 0
})
const profit = computed(() => {
  if (active.value) return Number(active.value.profit) || 0
  const a = props.account
  if (a?.drawer_quota_profit != null) return a.drawer_quota_profit
  if (a?.billing_window_profit != null && a.billing_window_source === 'quota_window') return a.billing_window_profit
  return a?.profit ?? 0
})
const requestCount = computed(() => {
  if (active.value) return Number(active.value.requests) || 0
  const a = props.account
  if (a?.drawer_quota_requests != null) return a.drawer_quota_requests
  if (a?.billing_window_requests != null && a.billing_window_source === 'quota_window') return a.billing_window_requests
  return a?.requests ?? 0
})
const activeStatus = computed(() => active.value?.status || '')

const marginLabel = computed(() => {
  if (revenue.value <= 0) return '—'
  return `${((profit.value / revenue.value) * 100).toFixed(1)}%`
})

const windowRangeLabel = computed(() => {
  if (active.value) return formatRange(active.value.start_at, active.value.end_at)
  const a = props.account
  if (a?.drawer_quota_start) return formatRange(a.drawer_quota_start, a.drawer_quota_end)
  if (!a?.billing_window_start) return ''
  return formatRange(a.billing_window_start, a.billing_window_end)
})

const windowKindLabel = computed(() => {
  const kind = active.value?.kind || props.account?.drawer_quota_kind || ''
  if (kind) return t('admin.profit.drawerQuotaWindow', { kind })
  if (props.account?.billing_window_source === 'cycle') return t('admin.profit.drawerBillingCycle')
  if (props.account?.billing_window_kind) {
    return t('admin.profit.drawerQuotaWindow', { kind: props.account.billing_window_kind })
  }
  return t('admin.profit.quotaWindowTitle')
})

const drawerHint = computed(() => {
  if (active.value) return t('admin.profit.drawerSelectedWindowHint')
  return t('admin.profit.drawerQuotaHint')
})

function isActive(item: ProfitWindowEconomicsItem) {
  if (!active.value) return false
  return item.start_at === active.value.start_at && item.end_at === active.value.end_at
}

function statusLabel(status?: string) {
  if (status === 'ended') return t('admin.profit.quotaWindowEnded')
  if (status === 'upcoming') return t('admin.profit.quotaWindowUpcoming')
  if (status === 'current') return t('admin.profit.quotaWindowCurrent')
  return status || ''
}

function statusClass(status?: string) {
  if (status === 'ended') return 'bg-gray-100 text-gray-600 dark:bg-dark-600 dark:text-dark-200'
  if (status === 'upcoming') return 'bg-sky-50 text-sky-700 dark:bg-sky-950/40 dark:text-sky-300'
  if (status === 'current') return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300'
  return 'bg-gray-100 text-gray-500'
}

function formatRange(start?: string, end?: string) {
  const fmtD = (v?: string) => {
    if (!v) return ''
    const d = new Date(v)
    if (Number.isNaN(d.getTime())) return v
    return new Intl.DateTimeFormat(undefined, {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit'
    }).format(d)
  }
  return `${fmtD(start)} → ${fmtD(end)}`
}

const breakEvenTitle = computed(() => {
  const a = props.account
  if (!a) return ''
  const parts = [t('admin.profit.breakEvenHint')]
  if (a.break_even_period_fee != null && a.break_even_period_days != null) {
    parts.push(t('admin.profit.breakEvenPeriod', { fee: fmt(a.break_even_period_fee), days: a.break_even_period_days }))
  }
  if (a.break_even_full_window_revenue != null && a.break_even_windows_per_period != null) {
    parts.push(t('admin.profit.breakEvenDetail', {
      kind: a.break_even_window_kind || '—',
      used: Math.round(a.break_even_used_percent ?? 0),
      full: fmt(a.break_even_full_window_revenue),
      windows: (a.break_even_windows_per_period ?? 0).toFixed(1),
      capacity: fmt(a.break_even_capacity_revenue)
    }))
  }
  return parts.join('\n')
})

function barWidth(value: number) {
  const maximum = Math.max(Math.abs(revenue.value), Math.abs(cost.value))
  if (maximum <= 0 || value === 0) return '0%'
  return `${Math.max(2, Math.min(100, (Math.abs(value) / maximum) * 100))}%`
}

const fmt = (value?: number) => (value ?? 0).toFixed(2)
const fmtNumber = (value?: number) => new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(value ?? 0)
const fmtRate = (value?: number) => (value ?? 0).toFixed(2)
</script>
