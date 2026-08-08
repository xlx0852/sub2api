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
        </div>
        <p class="text-xs text-gray-400">
          #{{ account.account_id }} · {{ account.platform }} / {{ account.account_type }}
          · {{ fmtNumber(account.requests) }} {{ t('admin.profit.requestsUnit') }}
        </p>
        <p v-if="cycleRangeLabel" class="text-xs text-gray-500 dark:text-dark-400">
          {{ t('admin.profit.quotaWindowTitle') }}: {{ cycleRangeLabel }}
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

      <!-- Revenue / amortized cost bars -->
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

      <!-- Profit -->
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

      <p class="text-[11px] leading-4 text-gray-400">
        {{ t('admin.profit.drawerCycleHint') }}
      </p>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AccountProfitSummary } from '@/api/admin/profit'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{
  show: boolean
  account: AccountProfitSummary | null
}>()

const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()

const isCycle = computed(() => {
  const a = props.account
  return !!a && a.cost_type === 'subscription' && a.billing_window_revenue != null
})

const revenue = computed(() =>
  isCycle.value ? (props.account!.billing_window_revenue ?? 0) : (props.account?.revenue ?? 0),
)
const cost = computed(() =>
  isCycle.value ? (props.account!.billing_window_cost ?? 0) : (props.account?.cost ?? 0),
)
const profit = computed(() =>
  isCycle.value ? (props.account!.billing_window_profit ?? 0) : (props.account?.profit ?? 0),
)

const marginLabel = computed(() => {
  if (revenue.value <= 0) return '—'
  return `${((profit.value / revenue.value) * 100).toFixed(1)}%`
})

const cycleRangeLabel = computed(() => {
  const a = props.account
  if (!isCycle.value || !a) return ''
  const fmtD = (v?: string) => (v ? new Date(v).toLocaleString() : '')
  return `${fmtD(a.billing_window_start)} → ${fmtD(a.billing_window_end)}`
})

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
      capacity: fmt(a.break_even_capacity_revenue),
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
