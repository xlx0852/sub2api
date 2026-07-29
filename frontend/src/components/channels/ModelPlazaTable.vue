<template>
  <div class="card overflow-hidden">
    <div class="overflow-x-auto">
      <table class="w-full min-w-[980px] border-collapse text-sm">
        <thead>
          <tr class="border-b border-gray-100 bg-gray-50/60 text-xs font-medium uppercase tracking-wide text-gray-500 dark:border-dark-700 dark:bg-dark-800/60 dark:text-gray-400">
            <th class="px-4 py-3 text-left">{{ t('availableChannels.columns.model') }}</th>
            <th class="w-[110px] px-3 py-3 text-left">{{ t('availableChannels.columns.provider') }}</th>
            <th class="w-[100px] px-3 py-3 text-left">{{ t('availableChannels.columns.billingMode') }}</th>
            <th class="w-[120px] px-3 py-3 text-right">{{ t('availableChannels.pricing.inputPrice') }}</th>
            <th class="w-[120px] px-3 py-3 text-right">{{ t('availableChannels.pricing.outputPrice') }}</th>
            <th class="w-[120px] px-3 py-3 text-right">{{ t('availableChannels.pricing.cacheReadPrice') }}</th>
            <th class="w-[120px] px-3 py-3 text-right">{{ t('availableChannels.pricing.cacheWritePrice') }}</th>
            <th class="w-[90px] px-3 py-3 text-center">{{ t('availableChannels.columns.discount') }}</th>
            <th class="min-w-[180px] px-4 py-3 text-left">{{ t('availableChannels.columns.channels') }}</th>
          </tr>
        </thead>

        <tbody v-if="loading || rows.length === 0">
          <TableStateRow :colspan="9" :loading="loading" :empty-label="emptyLabel" />
        </tbody>

        <tbody v-else>
          <tr
            v-for="row in rows"
            :key="row.key"
            class="border-b border-gray-100/80 transition-colors hover:bg-gray-50/50 dark:border-dark-700/60 dark:hover:bg-dark-800/40"
          >
            <td class="px-4 py-3 align-middle">
              <div class="flex items-start gap-3">
                <div class="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl bg-gray-100 dark:bg-dark-700">
                  <ModelIcon :model="row.name" size="20px" />
                </div>
                <div class="min-w-0">
                  <div class="truncate font-medium text-gray-900 dark:text-white">
                    {{ row.display_name }}
                  </div>
                  <div class="mt-0.5 truncate font-mono text-[11px] text-gray-500 dark:text-gray-400">
                    {{ row.name }}
                  </div>
                  <div v-if="row.tags.length" class="mt-1.5 flex flex-wrap gap-1">
                    <span
                      v-for="tag in row.tags"
                      :key="tag"
                      class="rounded-md bg-emerald-50 px-1.5 py-0.5 text-[10px] font-medium text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300"
                    >
                      {{ tagLabel(tag) }}
                    </span>
                  </div>
                </div>
              </div>
            </td>

            <td class="px-3 py-3 align-middle">
              <span
                :class="[
                  'inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-[11px] font-medium uppercase',
                  platformBadgeClass(row.platform),
                ]"
              >
                <PlatformIcon :platform="row.platform as GroupPlatform" size="xs" />
                {{ row.platform }}
              </span>
            </td>

            <td class="px-3 py-3 align-middle text-xs text-gray-600 dark:text-gray-300">
              {{ billingModeLabel(row) }}
            </td>

            <td class="px-3 py-3 align-middle text-right font-mono text-xs text-gray-800 dark:text-gray-200">
              {{ priceCell(row, 'input') }}
            </td>
            <td class="px-3 py-3 align-middle text-right font-mono text-xs text-gray-800 dark:text-gray-200">
              {{ priceCell(row, 'output') }}
            </td>
            <td class="px-3 py-3 align-middle text-right font-mono text-xs text-gray-800 dark:text-gray-200">
              {{ priceCell(row, 'cache_read') }}
            </td>
            <td class="px-3 py-3 align-middle text-right font-mono text-xs text-gray-800 dark:text-gray-200">
              {{ priceCell(row, 'cache_write') }}
            </td>

            <td class="px-3 py-3 align-middle text-center">
              <span
                v-if="discountLabel(row)"
                class="inline-flex rounded-full bg-emerald-500/15 px-2 py-0.5 text-[11px] font-semibold text-emerald-600 dark:text-emerald-300"
              >
                {{ discountLabel(row) }}
              </span>
              <span v-else class="text-xs text-gray-400">-</span>
            </td>

            <td class="px-4 py-3 align-middle">
              <div class="flex flex-wrap gap-1.5">
                <span
                  v-for="offer in visibleOffers(row)"
                  :key="`${row.key}-${offer.id}-${offer.channel_name}`"
                  class="inline-flex max-w-full items-center gap-1 rounded-md border border-gray-200 bg-white px-2 py-0.5 text-[11px] text-gray-700 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300"
                  :title="offerTitle(offer)"
                >
                  <span class="truncate">{{ offer.channel_name }}</span>
                  <span class="text-gray-400">·</span>
                  <span class="truncate text-gray-500 dark:text-gray-400">{{ offer.name }}</span>
                </span>
                <span
                  v-if="row.offers.length > maxOffers"
                  class="inline-flex items-center rounded-md bg-gray-100 px-2 py-0.5 text-[11px] text-gray-500 dark:bg-dark-700 dark:text-gray-400"
                >
                  +{{ row.offers.length - maxOffers }}
                </span>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import TableStateRow from '@/components/common/TableStateRow.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { GroupPlatform } from '@/types'
import { platformBadgeClass } from '@/utils/platformColors'
import { formatScaled } from '@/utils/pricing'
import {
  BILLING_MODE_TOKEN,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_IMAGE,
} from '@/constants/channel'
import type { ModelPlazaGroupOffer, ModelPlazaRow } from '@/utils/modelPlaza'
import { billingModeOf } from '@/utils/modelPlaza'

const props = withDefaults(
  defineProps<{
    rows: ModelPlazaRow[]
    loading: boolean
    emptyLabel: string
    maxOffers?: number
  }>(),
  { maxOffers: 3 },
)

const { t } = useI18n()
const perMillion = 1_000_000

function tagLabel(tag: string): string {
  const key = `availableChannels.tags.${tag}`
  const translated = t(key)
  return translated === key ? tag : translated
}

function billingModeLabel(row: ModelPlazaRow): string {
  const mode = billingModeOf(row)
  if (!mode) return '-'
  if (mode === BILLING_MODE_TOKEN) return t('availableChannels.pricing.billingModeToken')
  if (mode === BILLING_MODE_PER_REQUEST) return t('availableChannels.pricing.billingModePerRequest')
  if (mode === BILLING_MODE_IMAGE) return t('availableChannels.pricing.billingModeImage')
  return mode
}

function unitSuffix(mode: string | null | undefined): string {
  if (mode === BILLING_MODE_TOKEN) return t('availableChannels.pricing.unitPerMillion')
  return t('availableChannels.pricing.unitPerRequest')
}

function priceCell(row: ModelPlazaRow, kind: 'input' | 'output' | 'cache_read' | 'cache_write'): string {
  const pricing = row.best_offer?.effective_pricing
  if (!pricing) return '-'

  if (pricing.billing_mode !== BILLING_MODE_TOKEN) {
    if (kind === 'input') {
      const value =
        pricing.billing_mode === BILLING_MODE_IMAGE
          ? pricing.image_output_price ?? pricing.per_request_price
          : pricing.per_request_price
      if (value == null) return '-'
      return `${formatScaled(value, 1)} ${unitSuffix(pricing.billing_mode)}`
    }
    return '-'
  }

  const scale = perMillion
  const map = {
    input: pricing.input_price,
    output: pricing.output_price,
    cache_read: pricing.cache_read_price,
    cache_write: pricing.cache_write_price,
  } as const
  const value = map[kind]
  if (value == null) return '-'
  return `${formatScaled(value, scale)} ${unitSuffix(BILLING_MODE_TOKEN)}`
}

function discountLabel(row: ModelPlazaRow): string {
  const offer = row.best_offer
  if (!offer) return ''
  let multiplier = offer.rate_multiplier
  if (
    offer.base_pricing?.input_price != null &&
    offer.effective_pricing?.input_price != null &&
    offer.base_pricing.input_price > 0
  ) {
    multiplier = offer.effective_pricing.input_price / offer.base_pricing.input_price
  } else if (
    offer.base_pricing?.per_request_price != null &&
    offer.effective_pricing?.per_request_price != null &&
    offer.base_pricing.per_request_price > 0
  ) {
    multiplier = offer.effective_pricing.per_request_price / offer.base_pricing.per_request_price
  }
  if (!(multiplier > 0) || multiplier >= 0.999) return ''
  const pct = Math.round(multiplier * 1000) / 10
  // 0.12 → 12%
  if (pct < 100) return `${pct}%`
  return ''
}

function visibleOffers(row: ModelPlazaRow): ModelPlazaGroupOffer[] {
  return row.offers.slice(0, props.maxOffers)
}

function offerTitle(offer: ModelPlazaGroupOffer): string {
  const bits = [offer.channel_name, offer.name]
  if (offer.is_exclusive) bits.push(t('availableChannels.exclusive'))
  bits.push(`${offer.rate_multiplier}x`)
  return bits.filter(Boolean).join(' · ')
}
</script>
