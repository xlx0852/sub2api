<script setup lang="ts">
import { computed } from 'vue'

type BadgeTone = 'neutral' | 'info' | 'success' | 'warning' | 'danger'

const props = withDefaults(defineProps<{
  tone?: BadgeTone
  size?: 'xs' | 'sm'
  pill?: boolean
  dot?: boolean
}>(), {
  tone: 'neutral',
  size: 'sm',
  pill: true,
  dot: false
})

const toneClass = computed(() => ({
  neutral: 'bg-gray-100 text-gray-600 dark:bg-dark-600 dark:text-gray-400',
  info: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300',
  success: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
  warning: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
  danger: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
}[props.tone]))

const sizeClass = computed(() => props.size === 'xs'
  ? 'min-h-5 gap-1 px-1.5 py-0.5 text-[10px]'
  : 'min-h-6 gap-1.5 px-2 py-0.5 text-xs'
)
</script>

<template>
  <span :class="['inline-flex items-center whitespace-nowrap font-medium leading-none', pill ? 'rounded-full' : 'rounded-md', sizeClass, toneClass]">
    <span v-if="dot" class="h-1.5 w-1.5 shrink-0 rounded-full bg-current opacity-70" />
    <slot />
  </span>
</template>
