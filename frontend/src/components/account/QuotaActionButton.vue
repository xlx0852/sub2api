<template>
  <button
    type="button"
    :class="[
      'inline-flex h-6 items-center gap-1 rounded-md border px-2 text-[10px] font-medium transition-colors',
      'disabled:cursor-not-allowed disabled:opacity-50',
      toneClasses
    ]"
    :disabled="disabled || loading"
    :title="title"
  >
    <Icon name="refresh" size="xs" :class="{ 'animate-spin': loading }" />
    <slot />
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Icon from '@/components/icons/Icon.vue'

const props = withDefaults(defineProps<{
  loading?: boolean
  disabled?: boolean
  title?: string
  tone?: 'primary' | 'neutral'
}>(), {
  loading: false,
  disabled: false,
  title: undefined,
  tone: 'primary'
})

const toneClasses = computed(() => (
  props.tone === 'primary'
    ? 'border-primary-200 bg-primary-50 text-primary-700 hover:border-primary-300 hover:bg-primary-100 dark:border-primary-800 dark:bg-primary-950/40 dark:text-primary-300 dark:hover:bg-primary-900/50'
    : 'border-gray-200 bg-gray-50 text-gray-600 hover:border-gray-300 hover:bg-gray-100 dark:border-gray-700 dark:bg-gray-800/70 dark:text-gray-300 dark:hover:bg-gray-700'
))
</script>
