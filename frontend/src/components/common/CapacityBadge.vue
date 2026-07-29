<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  current: string | number
  max: string | number
  colorClass?: string
  tooltip?: string
  suffix?: string
}>()

const automaticColorClass = computed(() => {
  if (props.colorClass) return props.colorClass

  const current = typeof props.current === 'number' ? props.current : Number.NaN
  const max = typeof props.max === 'number' ? props.max : Number.NaN
  if (Number.isFinite(current) && Number.isFinite(max) && max > 0 && current >= max) {
    return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
  }
  if (Number.isFinite(current) && current > 0) {
    return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
  }
  return 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400'
})
</script>

<template>
  <span
    :class="[
      'inline-flex min-h-7 items-center gap-1.5 whitespace-nowrap rounded-lg px-2 py-1 text-xs font-medium leading-none',
      automaticColorClass
    ]"
    :title="tooltip"
  >
    <span class="inline-flex h-3 w-3 shrink-0 items-center justify-center [&>svg]:h-3 [&>svg]:w-3">
      <slot />
    </span>
    <span class="font-mono tabular-nums">{{ current }}</span>
    <span class="opacity-45">/</span>
    <span class="font-mono tabular-nums">{{ max }}</span>
    <span v-if="suffix" class="text-[10px] opacity-60">{{ suffix }}</span>
  </span>
</template>
