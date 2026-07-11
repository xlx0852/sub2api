<template>
  <div
    :class="[
      'grid min-w-0 gap-1 tabular-nums',
      stacked ? 'grid-cols-2' : 'grid-cols-4'
    ]"
  >
    <div data-testid="usage-stat-volume" class="contents">
      <span class="usage-metric usage-metric-indigo">
        <span class="usage-metric-label">{{ t('usage.totalRequests') }}</span>
        <strong>{{ requests }} req</strong>
      </span>
      <span class="usage-metric usage-metric-cyan">
        <span class="usage-metric-label">{{ t('usage.totalTokens') }}</span>
        <strong>{{ tokens }}</strong>
      </span>
    </div>
    <div data-testid="usage-stat-billing" class="contents">
      <span class="usage-metric usage-metric-amber" :title="t('usage.accountBilled')">
        <span class="usage-metric-label">{{ t('usage.accountCost') }}</span>
        <strong>A ${{ accountCost }}</strong>
      </span>
      <span
        v-if="userCost != null"
        class="usage-metric usage-metric-emerald"
        :title="t('usage.userBilled')"
      >
        <span class="usage-metric-label">{{ t('usage.userBilled') }}</span>
        <strong>U ${{ userCost }}</strong>
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

withDefaults(defineProps<{
  requests: string
  tokens: string
  accountCost: string
  userCost?: string | null
  stacked?: boolean
}>(), {
  userCost: null,
  stacked: false
})

const { t } = useI18n()
</script>

<style scoped>
.usage-metric {
  @apply flex min-w-[66px] flex-col rounded-md border border-gray-100 bg-gray-50 px-1.5 py-1 text-[10px] leading-3 text-gray-700 dark:border-white/5 dark:bg-white/[0.04] dark:text-gray-200;
}

.usage-metric-label {
  @apply mb-0.5 text-[8px] font-medium leading-3 text-gray-400 dark:text-gray-500;
}

.usage-metric strong {
  @apply whitespace-nowrap font-semibold;
}

.usage-metric-indigo strong { @apply text-indigo-600 dark:text-indigo-300; }
.usage-metric-cyan strong { @apply text-cyan-600 dark:text-cyan-300; }
.usage-metric-amber strong { @apply text-amber-600 dark:text-amber-300; }
.usage-metric-emerald strong { @apply text-emerald-600 dark:text-emerald-300; }
</style>
