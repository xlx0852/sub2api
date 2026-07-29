<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'

withDefaults(defineProps<{
  colspan: number
  loading?: boolean
  emptyLabel?: string
}>(), {
  loading: false,
  emptyLabel: ''
})
</script>

<template>
  <tr>
    <td :colspan="colspan" class="py-12 text-center">
      <Icon v-if="loading" name="refresh" size="lg" class="inline-block animate-spin text-gray-400" />
      <div v-else>
        <Icon name="inbox" size="xl" class="mx-auto mb-3 h-12 w-12 text-gray-300 dark:text-gray-500" />
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ emptyLabel }}</p>
      </div>
    </td>
  </tr>
</template>
