<template>
  <div class="card flex h-full flex-col p-4">
    <!-- 标题行 + 时间筛选控件 -->
    <div class="flex flex-wrap items-center gap-x-4 gap-y-3">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.dashboard.tokenUsageTrend') }}</h3>
      <div class="flex flex-wrap items-center gap-3 sm:ml-auto">
        <div class="flex items-center gap-2">
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('dashboard.timeRange') }}</span>
          <DateRangePicker :start-date="startDate" :end-date="endDate" @update:startDate="$emit('update:startDate', $event)" @update:endDate="$emit('update:endDate', $event)" @change="$emit('dateRangeChange', $event)" />
        </div>
        <div class="flex items-center gap-2">
          <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('dashboard.granularity') }}</span>
          <div class="w-24">
            <Select :model-value="granularity" :options="[{value:'day', label:t('dashboard.day')}, {value:'hour', label:t('dashboard.hour')}]" @update:model-value="$emit('update:granularity', $event)" @change="$emit('granularityChange')" />
          </div>
        </div>
        <button @click="$emit('refresh')" :disabled="loading" class="btn btn-secondary !px-3 !py-1.5 text-xs">
          {{ t('common.refresh') }}
        </button>
      </div>
    </div>

    <!-- 趋势图：放大占满卡片主体 -->
    <div class="relative mt-4 min-h-0 flex-1">
      <TokenUsageTrend :trend-data="trend" :loading="loading" embedded />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/ui/Select.vue'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import type { TrendDataPoint } from '@/types'

defineProps<{ loading: boolean, startDate: string, endDate: string, granularity: string, trend: TrendDataPoint[] }>()
defineEmits(['update:startDate', 'update:endDate', 'update:granularity', 'dateRangeChange', 'granularityChange', 'refresh'])
const { t } = useI18n()
</script>
