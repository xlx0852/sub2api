<template>
  <div class="min-w-0 rounded bg-white/70 px-2 py-1.5 dark:bg-dark-800/60">
    <div class="truncate text-[10px] text-gray-500 dark:text-dark-400">{{ label }}</div>
    <div class="mt-0.5 truncate font-semibold" :class="danger ? 'text-red-600 dark:text-red-300' : 'text-gray-800 dark:text-dark-100'">{{ value }}</div>
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ label: string; value: string; danger?: boolean }>(), { danger: false })
</script>
