<script setup lang="ts">
import Pagination from '@/components/common/Pagination.vue'
import SemanticBadge from '@/components/common/SemanticBadge.vue'
import Icon from '@/components/icons/Icon.vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

interface GroupUserOverrideRow {
  user_id: number
  user_email?: string
  user_name?: string
  user_notes?: string
  user_status?: string
  rpm_override?: number | null
  rate_multiplier?: number | null
}

defineProps<{
  entries: GroupUserOverrideRow[]
  valueLabel: string
  valueHint?: string
  page: number
  pageSize: number
  total: number
  showFinalValue?: boolean
  finalValueLabel?: string
}>()

const emit = defineEmits<{
  remove: [userId: number]
  'update:page': [page: number]
  'update:pageSize': [pageSize: number]
}>()
</script>

<template>
  <div>
    <div class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
      <div class="max-h-[420px] overflow-y-auto">
        <table class="w-full text-sm">
          <thead class="sticky top-0 z-[1]">
            <tr class="border-b border-gray-200 bg-gray-50 dark:border-dark-600 dark:bg-dark-700">
              <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.columns.userEmail') }}</th>
              <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">ID</th>
              <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.columns.userName') }}</th>
              <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.columns.userNotes') }}</th>
              <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.columns.userStatus') }}</th>
              <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400" :title="valueHint">{{ valueLabel }}</th>
              <th v-if="showFinalValue" class="px-3 py-2 text-left text-xs font-medium text-primary-600 dark:text-primary-400">{{ finalValueLabel }}</th>
              <th class="w-10 px-2 py-2" />
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-100 dark:divide-dark-600">
            <tr v-for="entry in entries" :key="entry.user_id" class="hover:bg-gray-50 dark:hover:bg-dark-700/50">
              <td class="px-3 py-2 text-gray-600 dark:text-gray-400">{{ entry.user_email }}</td>
              <td class="whitespace-nowrap px-3 py-2 text-gray-400 dark:text-gray-500">{{ entry.user_id }}</td>
              <td class="whitespace-nowrap px-3 py-2 text-gray-900 dark:text-white">{{ entry.user_name || '-' }}</td>
              <td class="max-w-[160px] truncate px-3 py-2 text-gray-500 dark:text-gray-400" :title="entry.user_notes">{{ entry.user_notes || '-' }}</td>
              <td class="whitespace-nowrap px-3 py-2">
                <SemanticBadge :tone="entry.user_status === 'active' ? 'success' : 'neutral'">{{ entry.user_status }}</SemanticBadge>
              </td>
              <td class="whitespace-nowrap px-3 py-2"><slot name="value" :entry="entry" /></td>
              <td v-if="showFinalValue" class="whitespace-nowrap px-3 py-2 font-medium text-primary-600 dark:text-primary-400"><slot name="final-value" :entry="entry" /></td>
              <td class="px-2 py-2">
                <button type="button" class="rounded p-1 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400" @click="emit('remove', entry.user_id)">
                  <Icon name="trash" size="sm" />
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
    <Pagination :total="total" :page="page" :page-size="pageSize" @update:page="emit('update:page', $event)" @update:page-size="emit('update:pageSize', $event)" />
  </div>
</template>
