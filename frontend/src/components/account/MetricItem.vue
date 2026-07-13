<template>
  <div class="min-w-0">
    <div class="mb-1 truncate text-[11px] text-gray-500 dark:text-gray-400" :title="label">{{ label }}</div>
    <div :class="['truncate text-sm font-semibold tabular-nums', valueClass]" :title="value">{{ value }}</div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  label: string
  value: string
  tone?: 'neutral' | 'amber' | 'emerald'
}>(), {
  tone: 'neutral'
})

const valueClass = computed(() => ({
  neutral: 'text-gray-900 dark:text-white',
  amber: 'text-amber-600 dark:text-amber-300',
  emerald: 'text-emerald-600 dark:text-emerald-300'
}[props.tone]))
</script>
