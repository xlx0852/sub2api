<template>
  <div class="flex flex-col gap-0.5">
    <!-- 并发槽位 -->
    <CapacityBadge compact :color-class="concurrencyClass" :current="currentConcurrency" :max="account.concurrency">
      <svg class="h-2.5 w-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25a2.25 2.25 0 01-2.25 2.25H6a2.25 2.25 0 01-2.25-2.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25A2.25 2.25 0 0118 10.5h-2.25a2.25 2.25 0 01-2.25-2.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 0118 20.25h-2.25A2.25 2.25 0 0113.5 18v-2.25z" />
      </svg>
    </CapacityBadge>

    <!-- API Key 账号配额限制 -->
    <QuotaBadge v-if="showDailyQuota" :used="account.quota_daily_used ?? 0" :limit="account.quota_daily_limit!" label="D" />
    <QuotaBadge v-if="showWeeklyQuota" :used="account.quota_weekly_used ?? 0" :limit="account.quota_weekly_limit!" label="W" />
    <QuotaBadge v-if="showTotalQuota" :used="account.quota_used ?? 0" :limit="account.quota_limit!" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Account } from '@/types'
import CapacityBadge from '@/components/common/CapacityBadge.vue'
import QuotaBadge from '@/components/account/QuotaBadge.vue'

const props = defineProps<{
  account: Account
}>()

// ====== 并发 ======
const currentConcurrency = computed(() => props.account.current_concurrency || 0)

const concurrencyClass = computed(() => {
  const current = currentConcurrency.value
  const max = props.account.concurrency
  if (current >= max) return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
  if (current > 0) return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
  return 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400'
})

// ====== 配额 ======
const isQuotaEligible = computed(() => props.account.type === 'apikey')

const showDailyQuota = computed(() =>
  isQuotaEligible.value && props.account.quota_daily_limit != null && props.account.quota_daily_limit > 0
)
const showWeeklyQuota = computed(() =>
  isQuotaEligible.value && props.account.quota_weekly_limit != null && props.account.quota_weekly_limit > 0
)
const showTotalQuota = computed(() =>
  isQuotaEligible.value && props.account.quota_limit != null && props.account.quota_limit > 0
)
</script>
