<template>
  <div v-if="loading" class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
    <div v-for="index in 8" :key="index" class="h-48 animate-pulse rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800" />
  </div>
  <div v-else-if="rows.length === 0" class="rounded-2xl border border-dashed border-gray-300 bg-white py-16 text-center text-sm text-gray-500 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400">
    {{ emptyLabel }}
  </div>
  <div v-else class="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4" data-testid="model-plaza-grid">
    <article v-for="row in rows" :key="row.key" class="group flex min-h-[198px] flex-col rounded-lg border border-gray-200 bg-white p-3 shadow-sm transition-all hover:-translate-y-0.5 hover:border-primary-200 hover:shadow-md dark:border-dark-700 dark:bg-dark-800 dark:hover:border-primary-800">
      <header class="flex items-start justify-between gap-2">
        <div class="flex min-w-0 items-center gap-2.5">
          <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-gray-100 bg-gray-50 dark:border-dark-600 dark:bg-dark-700"><ModelIcon :model="row.name" size="21px" /></div>
          <div class="min-w-0"><h3 class="truncate text-sm font-semibold text-gray-950 dark:text-white" :title="row.display_name">{{ row.display_name }}</h3><p class="mt-0.5 truncate font-mono text-[11px] text-gray-500 dark:text-gray-400" :title="row.name">{{ row.name }}</p></div>
        </div>
        <span v-if="discountLabel(row)" class="shrink-0 rounded-full bg-emerald-100 px-2 py-1 text-[10px] font-semibold text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-300">{{ t('availableChannels.columns.discount') }} {{ discountLabel(row) }}</span>
      </header>

      <div class="mt-2 flex flex-wrap items-center gap-1">
        <span :class="['inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-[10px] font-medium uppercase', platformBadgeClass(row.platform)]"><PlatformIcon :platform="row.platform as GroupPlatform" size="xs" />{{ row.platform }}</span>
        <span v-for="tag in row.tags" :key="tag" class="rounded-md bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300">{{ tagLabel(tag) }}</span>
      </div>

      <div class="mt-2.5 grid grid-cols-2 gap-1.5 text-xs">
        <div v-for="price in prices(row)" :key="price.label" class="rounded-md border border-gray-100 bg-gray-50/70 px-2 py-1.5 dark:border-dark-700 dark:bg-dark-900/40"><div class="text-[10px] text-gray-500 dark:text-gray-400">{{ price.label }}</div><div class="mt-0.5 truncate font-mono font-semibold text-gray-900 dark:text-white" :title="price.value">{{ price.value }}</div></div>
      </div>

      <footer class="mt-auto flex items-end justify-between gap-2 border-t border-gray-100 pt-2 dark:border-dark-700">
        <div v-if="showOffers" class="min-w-0"><div class="text-[10px] text-gray-400">{{ t('availableChannels.columns.channels') }}</div><div class="mt-0.5 flex flex-wrap gap-1"><span v-for="offer in visibleOffers(row)" :key="`${row.key}-${offer.id}`" class="max-w-[120px] truncate rounded bg-gray-100 px-1.5 py-0.5 text-[10px] text-gray-600 dark:bg-dark-700 dark:text-gray-300" :title="offerTitle(offer)">{{ offer.name }}</span><span v-if="row.offers.length > maxOffers" class="text-[10px] text-gray-400">+{{ row.offers.length - maxOffers }}</span></div></div>
        <span v-else class="text-[10px] text-gray-400">{{ t('availableChannels.publicPricing.publicRate') }}</span>
        <button type="button" class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white" :title="t('common.copy')" :aria-label="t('common.copy')" @click="copyToClipboard(row.name)"><Icon name="copy" size="xs" /></button>
      </footer>
    </article>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { GroupPlatform } from '@/types'
import { platformBadgeClass } from '@/utils/platformColors'
import { formatScaled } from '@/utils/pricing'
import { BILLING_MODE_TOKEN, BILLING_MODE_IMAGE, BILLING_MODE_VIDEO } from '@/constants/channel'
import type { ModelPlazaGroupOffer, ModelPlazaRow } from '@/utils/modelPlaza'
import { useClipboard } from '@/composables/useClipboard'

const props = withDefaults(defineProps<{ rows: ModelPlazaRow[]; loading: boolean; emptyLabel: string; maxOffers?: number; showOffers?: boolean }>(), { maxOffers: 2, showOffers: true })
const { t } = useI18n()
const { copyToClipboard } = useClipboard()

function tagLabel(tag: string) { const key = `availableChannels.tags.${tag}`; const translated = t(key); return translated === key ? tag : translated }
function displayPrice(value: number | null | undefined, scale = 1_000_000) { return value == null ? '-' : `${formatScaled(value, scale)} ${scale === 1 ? t('availableChannels.pricing.unitPerRequest') : t('availableChannels.pricing.unitPerMillion')}` }
function prices(row: ModelPlazaRow) {
  const pricing = row.best_offer?.effective_pricing
  if (!pricing) return [{ label: t('availableChannels.pricing.inputPrice'), value: '-' }, { label: t('availableChannels.pricing.outputPrice'), value: '-' }]
  if (pricing.billing_mode !== BILLING_MODE_TOKEN) {
    const isVideo = pricing.billing_mode === BILLING_MODE_VIDEO
    return [{
      label: isVideo ? t('availableChannels.pricing.videoOutputPrice') : pricing.billing_mode === BILLING_MODE_IMAGE ? t('availableChannels.pricing.imageOutputPrice') : t('availableChannels.pricing.perRequestPrice'),
      value: pricing.image_output_price == null && pricing.per_request_price == null
        ? '-'
        : `${formatScaled(pricing.image_output_price ?? pricing.per_request_price ?? 0, 1)} ${isVideo ? t('availableChannels.pricing.unitPerSecond') : t('availableChannels.pricing.unitPerRequest')}`,
    }]
  }
  return [
    { label: t('availableChannels.pricing.inputPrice'), value: displayPrice(pricing.input_price) },
    { label: t('availableChannels.pricing.outputPrice'), value: displayPrice(pricing.output_price) },
    { label: t('availableChannels.pricing.cacheReadPrice'), value: displayPrice(pricing.cache_read_price) },
    { label: t('availableChannels.pricing.cacheWritePrice'), value: displayPrice(pricing.cache_write_price) },
  ]
}
function discountLabel(row: ModelPlazaRow) {
  const offer = row.best_offer
  if (!offer) return ''
  let paidRatio = offer.rate_multiplier
  if (offer.base_pricing?.input_price != null && offer.effective_pricing?.input_price != null && offer.base_pricing.input_price > 0) {
    paidRatio = offer.effective_pricing.input_price / offer.base_pricing.input_price
  } else if (offer.base_pricing?.per_request_price != null && offer.effective_pricing?.per_request_price != null && offer.base_pricing.per_request_price > 0) {
    paidRatio = offer.effective_pricing.per_request_price / offer.base_pricing.per_request_price
  }
  if (!(paidRatio > 0) || paidRatio >= 0.999) return ''
  return `${Math.round((1 - paidRatio) * 1000) / 10}%`
}
function visibleOffers(row: ModelPlazaRow): ModelPlazaGroupOffer[] { return row.offers.slice(0, props.maxOffers) }
function offerTitle(offer: ModelPlazaGroupOffer) { return [offer.channel_name, offer.name, `${offer.rate_multiplier}x`].join(' · ') }
</script>
