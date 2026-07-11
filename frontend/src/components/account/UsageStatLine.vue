<template>
  <div
    :class="[
      'text-[9px] leading-[13px] text-gray-500 tabular-nums dark:text-gray-400',
      stacked ? 'grid gap-y-0' : 'flex items-center gap-2 whitespace-nowrap'
    ]"
  >
    <span data-testid="usage-stat-volume" class="whitespace-nowrap">
      {{ requests }} req <span aria-hidden="true" class="text-gray-300 dark:text-gray-600">·</span> {{ tokens }}
    </span>
    <span
      data-testid="usage-stat-billing"
      class="whitespace-nowrap"
      :class="stacked ? '' : 'border-l border-gray-200 pl-2 dark:border-gray-700'"
    >
      <span :title="t('usage.accountBilled')">A ${{ accountCost }}</span>
      <template v-if="userCost != null">
        <span aria-hidden="true" class="text-gray-300 dark:text-gray-600"> · </span>
        <span :title="t('usage.userBilled')">U ${{ userCost }}</span>
      </template>
    </span>
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
