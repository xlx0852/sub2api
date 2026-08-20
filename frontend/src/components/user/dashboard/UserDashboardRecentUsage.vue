<template>
  <div class="card">
    <div class="flex items-center justify-between border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('dashboard.recentUsage') }}</h2>
      <span class="badge badge-gray">{{ t('dashboard.last7Days') }}</span>
    </div>
    <div class="p-6">
      <div v-if="loading" class="flex items-center justify-center py-12">
        <LoadingSpinner size="lg" />
      </div>
      <div v-else-if="data.length === 0" class="py-8">
        <EmptyState :title="t('dashboard.noUsageRecords')" :description="t('dashboard.startUsingApi')" />
      </div>
      <div v-else class="space-y-3">
        <div class="hidden grid-cols-12 gap-4 px-4 text-xs font-medium uppercase tracking-wide text-gray-400 dark:text-dark-500 sm:grid">
          <span class="col-span-5">{{ t('dashboard.model') }}</span>
          <span class="col-span-3">{{ t('dashboard.time') }}</span>
          <span class="col-span-2 text-right">Tokens</span>
          <span class="col-span-2 text-right">{{ t('dashboard.actual') }} / {{ t('dashboard.standard') }}</span>
        </div>
        <div v-for="log in data" :key="log.id" class="grid grid-cols-12 items-center gap-2 rounded-xl bg-gray-50 p-4 transition-colors hover:bg-gray-100 dark:bg-dark-800/50 dark:hover:bg-dark-800 sm:gap-4">
          <div class="col-span-12 flex items-center gap-3 sm:col-span-5">
            <div class="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg bg-primary-100 dark:bg-primary-900/30">
              <Icon name="beaker" size="sm" class="text-primary-600 dark:text-primary-400" />
            </div>
            <p class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ log.model }}</p>
          </div>
          <p class="col-span-6 text-xs text-gray-500 dark:text-dark-400 sm:col-span-3">{{ formatDateTime(log.created_at) }}</p>
          <p class="col-span-3 text-right text-xs text-gray-500 dark:text-dark-400 sm:col-span-2">{{ (log.input_tokens + log.output_tokens).toLocaleString() }} tokens</p>
          <p class="col-span-3 text-right text-sm font-semibold sm:col-span-2">
            <span class="text-green-600 dark:text-green-400" :title="t('dashboard.actual')">{{ formatCost(log.actual_cost) }}</span>
            <span class="font-normal text-gray-400 dark:text-gray-500" :title="t('dashboard.standard')"> / {{ formatCost(log.total_cost) }}</span>
          </p>
        </div>

        <router-link to="/usage" class="flex items-center justify-center gap-2 py-3 text-sm font-medium text-primary-600 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300">
          {{ t('dashboard.viewAllUsage') }}
          <Icon name="arrowRight" size="sm" />
        </router-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/ui/LoadingSpinner.vue'
import EmptyState from '@/ui/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime } from '@/utils/format'
import type { UsageLog } from '@/types'

defineProps<{
  data: UsageLog[]
  loading: boolean
}>()
const { t } = useI18n()
const formatCost = (c: number) => c.toFixed(4)
</script>
