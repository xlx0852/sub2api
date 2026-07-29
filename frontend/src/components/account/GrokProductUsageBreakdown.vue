<template>
  <section
    v-if="rows.length > 0"
    data-testid="grok-product-usage-breakdown"
    class="mt-2 rounded-md border border-gray-100 bg-gray-50/70 px-2.5 py-2 dark:border-white/5 dark:bg-white/[0.03]"
  >
    <div class="mb-2 text-[10px] font-semibold text-gray-600 dark:text-gray-300">
      {{ t('admin.accounts.usageWindow.grokProductBreakdown') }}
    </div>
    <div class="space-y-2">
      <div v-for="row in rows" :key="row.key" class="min-w-0" :data-product="row.key">
        <div class="mb-1 flex items-center justify-between gap-3 text-[10px] leading-3">
          <span class="min-w-0 truncate font-medium text-gray-600 dark:text-gray-300" :title="row.label">
            {{ row.label }}
          </span>
          <span class="shrink-0 font-semibold tabular-nums text-gray-700 dark:text-gray-200">
            {{ t('admin.accounts.usageWindow.grokProductUsed', { value: row.displayPercent }) }}
          </span>
        </div>
        <div class="h-1.5 overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700">
          <div
            :class="['h-full rounded-full transition-all duration-300', row.toneClass]"
            :style="{ width: `${row.utilization}%` }"
          />
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { GrokBillingProductUsage } from '@/api/admin/grok'

const props = defineProps<{
  products?: GrokBillingProductUsage[] | null
}>()

const { t } = useI18n()

const productLabel = (product: string): string => {
  switch (product.toLowerCase().replace(/[^a-z0-9]/g, '')) {
    case 'api':
      return t('admin.accounts.usageWindow.grokProductApi')
    case 'grokbuild':
      return t('admin.accounts.usageWindow.grokProductBuild')
    case 'grokchat':
      return t('admin.accounts.usageWindow.grokProductChat')
    default:
      return product
  }
}

const rows = computed(() => (props.products || [])
  .filter(product => typeof product?.product === 'string' && product.product.trim().length > 0)
  .map((product, index) => {
    const rawPercent = product.usage_percent
    const hasPercent = typeof rawPercent === 'number' && Number.isFinite(rawPercent)
    const utilization = hasPercent ? Math.max(0, Math.min(100, rawPercent)) : 0
    return {
      key: `${product.product.toLowerCase()}:${index}`,
      label: productLabel(product.product.trim()),
      utilization,
      displayPercent: hasPercent ? `${Math.round(Math.max(0, rawPercent))}%` : '--',
      toneClass: utilization >= 100
        ? 'bg-red-500 dark:bg-red-400'
        : utilization >= 80
          ? 'bg-amber-500 dark:bg-amber-400'
          : 'bg-emerald-500 dark:bg-emerald-400'
    }
  }))
</script>
