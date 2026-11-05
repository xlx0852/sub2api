<template>
  <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-800">
    <!-- Collapsed summary header (clickable) -->
    <div
      class="flex cursor-pointer select-none items-center gap-2"
      @click="collapsed = !collapsed"
    >
      <Icon
        :name="collapsed ? 'chevronRight' : 'chevronDown'"
        size="sm"
        :stroke-width="2"
        class="flex-shrink-0 text-gray-400 transition-transform duration-200"
      />

      <!-- Summary: model tags + price + billing badge -->
      <div v-if="collapsed" class="flex min-w-0 flex-1 items-center gap-2 overflow-hidden">
        <!-- Compact model tags (show first 3) -->
        <div class="flex min-w-0 flex-1 flex-wrap items-center gap-1">
          <span
            v-for="(m, i) in entry.models.slice(0, 3)"
            :key="i"
            class="inline-flex shrink-0 rounded px-1.5 py-0.5 text-xs"
            :class="getPlatformTagClass(props.platform || '')"
          >
            {{ m }}
          </span>
          <span
            v-if="entry.models.length > 3"
            class="whitespace-nowrap text-xs text-gray-400"
          >
            +{{ entry.models.length - 3 }}
          </span>
          <span
            v-if="entry.models.length === 0"
            class="text-xs italic text-gray-400"
          >
            {{ t('admin.channels.form.noModels') }}
          </span>
        </div>

        <span
          class="hidden max-w-[14rem] truncate text-[11px] sm:inline"
          :class="priceEmpty ? 'text-amber-600 dark:text-amber-400' : 'text-gray-500 dark:text-gray-400'"
          :title="priceSummary"
        >
          {{ priceSummary }}
        </span>

        <!-- Billing mode badge -->
        <span
          class="flex-shrink-0 rounded-full bg-primary-100 px-2 py-0.5 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
        >
          {{ billingModeLabel }}
        </span>
      </div>

      <!-- Expanded: show the label "Pricing Entry" or similar -->
      <div v-else class="flex-1 text-xs font-medium text-gray-500 dark:text-gray-400">
        {{ t('admin.channels.form.pricingEntry') }}
      </div>

      <!-- Remove button (always visible, stop propagation) -->
      <button
        type="button"
        @click.stop="emit('remove')"
        class="flex-shrink-0 rounded p-1 text-gray-400 hover:text-red-500"
      >
        <Icon name="trash" size="sm" />
      </button>
    </div>

    <!-- Expandable content with transition -->
    <div
      class="collapsible-content"
      :class="{ 'collapsible-content--collapsed': collapsed }"
    >
      <div class="collapsible-inner">
        <!-- Header: Models + Billing Mode + Currency -->
        <div class="mt-3 flex items-start gap-2">
          <div class="flex-1">
            <label class="text-xs font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.channels.form.models') }} <span class="text-red-500">*</span>
            </label>
            <ModelTagInput
              :models="entry.models"
              :platform="props.platform"
              @update:models="onModelsUpdate($event)"
              :placeholder="t('admin.channels.form.modelsPlaceholder')"
              class="mt-1"
            />
          </div>
          <div class="w-40">
            <label class="text-xs font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.channels.form.billingMode') }}
            </label>
            <Select
              :modelValue="entry.billing_mode"
              @update:modelValue="emit('update', { ...entry, billing_mode: $event as BillingMode, intervals: [] })"
              :options="billingModeOptions"
              class="mt-1"
            />
          </div>
			<div class="w-28">
				<label class="text-xs font-medium text-gray-500 dark:text-gray-400">
					{{ t('admin.channels.form.currency') }}
				</label>
				<Select
					:modelValue="entry.currency || 'USD'"
					@update:modelValue="emit('update', { ...entry, currency: $event as 'USD' | 'CNY' })"
					:options="currencyOptions"
					class="mt-1"
				/>
			</div>
        </div>

        <!-- Token mode -->
        <div v-if="entry.billing_mode === 'token'">
          <!-- Quick sell-price tools -->
          <div class="mt-3 flex flex-wrap items-center gap-2 rounded-md border border-dashed border-gray-300 bg-white/70 px-2 py-2 dark:border-dark-500 dark:bg-dark-900/40">
            <button
              type="button"
              class="rounded border border-gray-200 px-2 py-1 text-[11px] font-medium text-gray-700 hover:border-primary-300 hover:text-primary-700 disabled:opacity-50 dark:border-dark-600 dark:text-gray-200"
              :disabled="quickBusy || entry.models.length === 0 || isFxAuto"
              @click="fillOfficialPrices"
            >
              {{ quickBusy ? t('admin.channels.form.quickPricingLoading') : t('admin.channels.form.fillOfficialPrice') }}
            </button>
            <div class="flex items-center gap-1">
              <span class="text-[11px] text-gray-500">{{ t('admin.channels.form.scaleOfficialPrice') }}</span>
              <button
                v-for="n in [2, 4]"
                :key="n"
                type="button"
                class="rounded border border-amber-200 bg-amber-50 px-1.5 py-0.5 text-[11px] font-semibold text-amber-800 hover:bg-amber-100 disabled:opacity-50 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-200"
                :disabled="quickBusy || entry.models.length === 0 || isFxAuto"
                @click="scaleFromOfficial(n)"
              >
                ×{{ n }}
              </button>
              <input
                v-model.number="customScale"
                type="number"
                step="0.1"
                class="input h-7 w-16 px-1 text-[11px]"
                :placeholder="t('admin.channels.form.customScalePlaceholder')"
              />
              <button
                type="button"
                class="rounded border border-amber-200 px-1.5 py-0.5 text-[11px] text-amber-800 hover:bg-amber-50 disabled:opacity-50 dark:border-amber-900/50 dark:text-amber-200"
                :disabled="quickBusy || entry.models.length === 0 || isFxAuto || !(Number(customScale) > 0)"
                @click="scaleFromOfficial(Number(customScale))"
              >
                {{ t('admin.channels.form.applyScale') }}
              </button>
            </div>
            <button
              type="button"
              class="rounded border border-gray-200 px-2 py-1 text-[11px] text-gray-600 hover:text-gray-900 disabled:opacity-50 dark:border-dark-600 dark:text-gray-300"
              :disabled="isFxAuto"
              @click="clearOverrides"
            >
              {{ t('admin.channels.form.clearPriceOverride') }}
            </button>
            <span v-if="isFxAuto" class="text-[11px] text-sky-700 dark:text-sky-300">
              {{ t('admin.channels.form.quickToolsDisabledByFxAuto') }}
            </span>
          </div>
          <p class="mt-1 text-[11px] leading-4 text-gray-400">
            {{ t('admin.channels.form.tokenPriceHint') }}
          </p>

          <!-- Default prices (fallback when no interval matches) -->
          <label class="mt-3 block text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('admin.channels.form.defaultPrices') }}
            <span class="ml-1 font-normal text-gray-400">{{ currencySymbol }} / 1M tokens</span>
            <span
              v-if="isFxAuto"
              class="ml-1 rounded bg-sky-100 px-1 py-0.5 text-[10px] font-normal text-sky-700 dark:bg-sky-900/40 dark:text-sky-300"
            >
              {{ t('admin.channels.form.lockedByFxAuto') }}
            </span>
          </label>
          <!-- fx_auto：这些格子的值由成本派生，手填不生效，因此只读展示 -->
          <div v-if="isFxAuto" class="mt-1 rounded-md bg-sky-50/60 px-2 py-1.5 text-[11px] leading-5 text-sky-800 dark:bg-sky-950/20 dark:text-sky-200">
            {{ fxFormulaText }}
          </div>
          <div class="mt-1 grid grid-cols-2 gap-2 sm:grid-cols-5" :class="isFxAuto ? 'opacity-60' : ''">
            <div>
              <label class="text-xs text-gray-400">{{ t('admin.channels.form.inputPrice') }}</label>
              <input :value="entry.input_price" @input="emitField('input_price', ($event.target as HTMLInputElement).value)"
                :disabled="isFxAuto" type="number" step="any" class="input mt-0.5 text-sm"
                :placeholder="t('admin.channels.form.pricePlaceholder')" />
            </div>
            <div>
              <label class="text-xs text-gray-400">{{ t('admin.channels.form.outputPrice') }}</label>
              <input :value="entry.output_price" @input="emitField('output_price', ($event.target as HTMLInputElement).value)"
                :disabled="isFxAuto" type="number" step="any" class="input mt-0.5 text-sm"
                :placeholder="t('admin.channels.form.pricePlaceholder')" />
            </div>
            <div>
              <label class="text-xs text-gray-400">{{ t('admin.channels.form.cacheWritePrice') }}</label>
              <input :value="entry.cache_write_price" @input="emitField('cache_write_price', ($event.target as HTMLInputElement).value)"
                :disabled="isFxAuto" type="number" step="any" class="input mt-0.5 text-sm"
                :placeholder="t('admin.channels.form.pricePlaceholder')" />
            </div>
            <div>
              <label class="text-xs text-gray-400">{{ t('admin.channels.form.cacheReadPrice') }}</label>
              <input :value="entry.cache_read_price" @input="emitField('cache_read_price', ($event.target as HTMLInputElement).value)"
                :disabled="isFxAuto" type="number" step="any" class="input mt-0.5 text-sm"
                :placeholder="t('admin.channels.form.pricePlaceholder')" />
            </div>
            <div>
              <label class="text-xs text-gray-400">{{ t('admin.channels.form.imageTokenPrice') }}</label>
              <input :value="entry.image_output_price" @input="emitField('image_output_price', ($event.target as HTMLInputElement).value)"
                type="number" step="any" class="input mt-0.5 text-sm" :placeholder="t('admin.channels.form.pricePlaceholder')" />
              <p class="mt-0.5 text-[10px] leading-3 text-amber-600/90 dark:text-amber-400/90">{{ t('admin.channels.form.imageOutputNullHint') }}</p>
            </div>
          </div>

          <!-- 售价派生（L2）：fx_auto 由成本基 × 汇率 × (1+加价) 派生，其余两种不换汇 -->
          <div class="mt-3 rounded-md border border-dashed border-sky-300 bg-sky-50/40 px-2 py-2 dark:border-sky-900/50 dark:bg-sky-950/20">
            <div class="flex flex-wrap items-center gap-2">
              <span class="text-[11px] font-medium text-sky-800 dark:text-sky-200">
                {{ t('admin.channels.form.fxMode') }}
              </span>
              <Select
                :modelValue="entry.fx_mode || 'manual'"
                @update:modelValue="emit('update', { ...entry, fx_mode: $event as 'manual' | 'cny_native' | 'fx_auto' })"
                :options="fxModeOptions"
                class="w-44"
              />
              <template v-if="(entry.fx_mode || 'manual') === 'fx_auto'">
                <label class="text-[11px] text-gray-500">{{ t('admin.channels.form.costId') }}</label>
                <input
                  :value="entry.cost_id"
                  @input="emitField('cost_id', ($event.target as HTMLInputElement).value)"
                  type="number" step="1" min="1" class="input h-7 w-20 px-1 text-[11px]"
                  :placeholder="t('admin.channels.form.costIdPlaceholder')"
                />
                <label class="text-[11px] text-gray-500">{{ t('admin.channels.form.fxRate') }}</label>
                <input
                  :value="entry.fx_rate"
                  @input="emitField('fx_rate', ($event.target as HTMLInputElement).value)"
                  type="number" step="any" class="input h-7 w-20 px-1 text-[11px]"
                  placeholder="7.0"
                />
                <label class="text-[11px] text-gray-500">{{ t('admin.channels.form.markupPct') }}</label>
                <input
                  :value="entry.markup_pct"
                  @input="emitField('markup_pct', ($event.target as HTMLInputElement).value)"
                  type="number" step="any" class="input h-7 w-16 px-1 text-[11px]"
                  placeholder="0"
                />
                <span class="text-[11px] text-sky-700 dark:text-sky-300">
                  {{ t('admin.channels.form.fxMultiplierPreview', { factor: fxFactorPreview }) }}
                </span>
              </template>
              <span v-else class="text-[11px] text-gray-400">
                {{ t('admin.channels.form.fxModeManualHint') }}
              </span>
            </div>
          </div>

          <!-- Token intervals -->
          <div class="mt-3">
            <div class="flex items-center justify-between">
              <label class="text-xs font-medium text-gray-500 dark:text-gray-400">
                {{ t('admin.channels.form.intervals') }}
                <span class="ml-1 font-normal text-gray-400">(min, max]</span>
              </label>
              <button type="button" @click="addInterval" class="text-xs text-primary-600 hover:text-primary-700">
                + {{ t('admin.channels.form.addInterval') }}
              </button>
            </div>
            <div v-if="entry.intervals && entry.intervals.length > 0" class="mt-2 space-y-2">
              <IntervalRow
                v-for="(iv, idx) in entry.intervals"
                :key="idx"
                :interval="iv"
                :mode="entry.billing_mode"
                @update="updateInterval(idx, $event)"
                @remove="removeInterval(idx)"
              />
            </div>
          </div>
        </div>

        <!-- Per-request mode -->
        <div v-else-if="entry.billing_mode === 'per_request'">
          <!-- Default per-request price -->
          <label class="mt-3 block text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('admin.channels.form.defaultPerRequestPrice') }}
            <span class="ml-1 font-normal text-gray-400">{{ currencySymbol }}</span>
          </label>
          <div class="mt-1 w-48">
            <input :value="entry.per_request_price" @input="emitField('per_request_price', ($event.target as HTMLInputElement).value)"
              type="number" step="any" class="input text-sm" :placeholder="t('admin.channels.form.pricePlaceholder')" />
          </div>

          <!-- Tiers -->
          <div class="mt-3 flex items-center justify-between">
            <label class="text-xs font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.channels.form.requestTiers') }}
            </label>
            <button type="button" @click="addInterval" class="text-xs text-primary-600 hover:text-primary-700">
              + {{ t('admin.channels.form.addTier') }}
            </button>
          </div>
          <div v-if="entry.intervals && entry.intervals.length > 0" class="mt-2 space-y-2">
            <IntervalRow
              v-for="(iv, idx) in entry.intervals"
              :key="idx"
              :interval="iv"
              :mode="entry.billing_mode"
              @update="updateInterval(idx, $event)"
              @remove="removeInterval(idx)"
            />
          </div>
          <div v-else class="mt-2 rounded border border-dashed border-gray-300 p-3 text-center text-xs text-gray-400 dark:border-dark-500">
            {{ t('admin.channels.form.noTiersYet') }}
          </div>
        </div>

        <!-- Image mode -->
        <div v-else-if="entry.billing_mode === 'image'">
          <!-- Default image price (per-request, same as per_request mode) -->
          <label class="mt-3 block text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('admin.channels.form.defaultImagePrice') }}
            <span class="ml-1 font-normal text-gray-400">{{ currencySymbol }}</span>
          </label>
          <div class="mt-1 w-48">
            <input :value="entry.per_request_price" @input="emitField('per_request_price', ($event.target as HTMLInputElement).value)"
              type="number" step="any" class="input text-sm" :placeholder="t('admin.channels.form.pricePlaceholder')" />
          </div>

          <!-- Image tiers -->
          <div class="mt-3 flex items-center justify-between">
            <label class="text-xs font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.channels.form.imageTiers') }}
            </label>
            <button type="button" @click="addImageTier" class="text-xs text-primary-600 hover:text-primary-700">
              + {{ t('admin.channels.form.addTier') }}
            </button>
          </div>
          <div v-if="entry.intervals && entry.intervals.length > 0" class="mt-2 space-y-2">
            <IntervalRow
              v-for="(iv, idx) in entry.intervals"
              :key="idx"
              :interval="iv"
              :mode="entry.billing_mode"
              @update="updateInterval(idx, $event)"
              @remove="removeInterval(idx)"
            />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/ui/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import IntervalRow from './IntervalRow.vue'
import ModelTagInput from './ModelTagInput.vue'
import type { PricingFormEntry, IntervalFormEntry } from './types'
import { perTokenToMTok, getPlatformTagClass } from './types'
import type { BillingMode } from '@/api/admin/channels'
import channelsAPI from '@/api/admin/channels'
import {
  clearTokenPriceOverrides,
  formatTokenPriceSummary,
  isTokenPricingEmpty,
  officialPricingToFormMTok,
  scaleOfficialPricingMTok,
  type OfficialTokenPricing,
} from './channelPricingTools'

const { t } = useI18n()

const props = defineProps<{
  entry: PricingFormEntry
  platform?: string
  /** 成本基列表（父组件传入，用于 fx_auto 行的公式与只读展示） */
  priceCosts?: { id: number; currency: string; input_per_mtok: number; output_per_mtok: number; cached_per_mtok: number }[]
}>()

const emit = defineEmits<{
  update: [entry: PricingFormEntry]
  remove: []
}>()

// Collapse state: entries with existing models default to collapsed
const collapsed = ref(props.entry.models.length > 0)
const quickBusy = ref(false)
const customScale = ref(4)

const billingModeOptions = computed(() => [
  { value: 'token', label: t('admin.channels.billingMode.token') },
  { value: 'per_request', label: t('admin.channels.billingMode.perRequest') },
  { value: 'image', label: t('admin.channels.billingMode.image') }
])

const isFxAuto = computed(() => (props.entry.fx_mode || 'manual') === 'fx_auto')

// 成本基（由父组件一次性传入，避免每张卡片各查一次）
const cost = computed(() => {
  const id = Number(props.entry.cost_id)
  if (!id) return null
  return (props.priceCosts || []).find((c) => c.id === id) || null
})

// 公式文案：成本 $x × 汇率 = ¥y → 用户实付（再乘分组倍率）
const fxFormulaText = computed(() => {
  const rate = Number(props.entry.fx_rate)
  const markup = Number(props.entry.markup_pct || 0)
  const c = cost.value
  if (!c) {
    return t('admin.channels.form.fxFormulaNoCost', { rate: Number.isFinite(rate) && rate > 0 ? rate : '-' })
  }
  const factor = (Number.isFinite(rate) && rate > 0 ? rate : 0) * (1 + markup / 100)
  const sellIn = (c.input_per_mtok * factor).toFixed(4).replace(/\.?0+$/, '')
  const sellOut = (c.output_per_mtok * factor).toFixed(4).replace(/\.?0+$/, '')
  return t('admin.channels.form.fxFormula', {
    currency: c.currency,
    in: c.input_per_mtok,
    out: c.output_per_mtok,
    rate: Number.isFinite(rate) && rate > 0 ? rate : '-',
    factor: factor.toFixed(4).replace(/\.?0+$/, ''),
    sellIn,
    sellOut,
  })
})

const fxModeOptions = computed(() => [
  { value: 'manual', label: t('admin.channels.form.fxModeManual') },
  { value: 'cny_native', label: t('admin.channels.form.fxModeCnyNative') },
  { value: 'fx_auto', label: t('admin.channels.form.fxModeAuto') },
])

// 汇率 × (1+加价%)，即成本基到售价的换算乘数，供录入时核对。
const fxFactorPreview = computed(() => {
  const rate = Number(props.entry.fx_rate)
  const markup = Number(props.entry.markup_pct || 0)
  if (!(rate > 0)) return '-'
  const factor = rate * (1 + markup / 100)
  return factor.toFixed(4).replace(/\.?0+$/, '')
})

const currencyOptions = [
	{ value: 'USD', label: 'USD ($)' },
	{ value: 'CNY', label: 'CNY (¥)' },
]

const currencySymbol = computed(() => props.entry.currency === 'CNY' ? '¥' : '$')

const billingModeLabel = computed(() => {
  const opt = billingModeOptions.value.find(o => o.value === props.entry.billing_mode)
  return opt ? opt.label : props.entry.billing_mode
})

const priceEmpty = computed(() => isTokenPricingEmpty(props.entry))
const priceSummary = computed(() =>
  formatTokenPriceSummary(props.entry, t('admin.channels.form.fallbackOfficialShort')),
)

function emitField(field: keyof PricingFormEntry, value: string) {
  emit('update', { ...props.entry, [field]: value === '' ? null : value })
}

function pickLookupModel(models: string[]): string | null {
  const exact = models.find((m) => m && !m.includes('*'))
  return exact || models.find((m) => !!m) || null
}

async function fetchOfficialForEntry(): Promise<OfficialTokenPricing | null> {
  const model = pickLookupModel(props.entry.models)
  if (!model) return null
  const batch = await channelsAPI.batchGetModelDefaultPricing([model])
  return batch.items?.[model] || null
}

async function fillOfficialPrices() {
  if (quickBusy.value) return
  quickBusy.value = true
  try {
    const official = await fetchOfficialForEntry()
    const fields = official ? officialPricingToFormMTok(official) : null
    if (!fields) return
    emit('update', { ...props.entry, ...fields })
  } catch {
    // ignore
  } finally {
    quickBusy.value = false
  }
}

async function scaleFromOfficial(multiplier: number) {
  if (quickBusy.value) return
  if (!(multiplier > 0)) return
  quickBusy.value = true
  try {
    const official = await fetchOfficialForEntry()
    const fields = official ? scaleOfficialPricingMTok(official, multiplier) : null
    if (!fields) return
    emit('update', { ...props.entry, ...fields })
  } catch {
    // ignore
  } finally {
    quickBusy.value = false
  }
}

function clearOverrides() {
  emit('update', {
    ...props.entry,
    ...clearTokenPriceOverrides(),
  })
}

function addInterval() {
  const intervals = [...(props.entry.intervals || [])]
  intervals.push({
    min_tokens: 0, max_tokens: null, tier_label: '',
    input_price: null, output_price: null, cache_write_price: null,
    cache_read_price: null, per_request_price: null,
    sort_order: intervals.length
  })
  emit('update', { ...props.entry, intervals })
}

function addImageTier() {
  const intervals = [...(props.entry.intervals || [])]
  const labels = ['1K', '2K', '4K', 'HD']
  intervals.push({
    min_tokens: 0, max_tokens: null, tier_label: labels[intervals.length] || '',
    input_price: null, output_price: null, cache_write_price: null,
    cache_read_price: null, per_request_price: null,
    sort_order: intervals.length
  })
  emit('update', { ...props.entry, intervals })
}

function updateInterval(idx: number, updated: IntervalFormEntry) {
  const intervals = [...(props.entry.intervals || [])]
  intervals[idx] = updated
  emit('update', { ...props.entry, intervals })
}

function removeInterval(idx: number) {
  const intervals = [...(props.entry.intervals || [])]
  intervals.splice(idx, 1)
  emit('update', { ...props.entry, intervals })
}

async function onModelsUpdate(newModels: string[]) {
  const oldModels = props.entry.models
  emit('update', { ...props.entry, models: newModels })

  // 只在新增模型且当前无价格时自动填充
  const addedModels = newModels.filter(m => !oldModels.includes(m))
  if (addedModels.length === 0) return

  // 检查是否所有价格字段都为空
  const e = props.entry
  const hasPrice = e.input_price != null || e.output_price != null ||
                   e.cache_write_price != null || e.cache_read_price != null
  if (hasPrice) return

  // 查询第一个新增模型的默认价格
  try {
    const result = await channelsAPI.getModelDefaultPricing(addedModels[0])
    if (result.found) {
      emit('update', {
        ...props.entry,
        models: newModels,
			currency: result.currency === 'CNY' ? 'CNY' : 'USD',
        input_price: perTokenToMTok(result.input_price ?? null),
        output_price: perTokenToMTok(result.output_price ?? null),
        cache_write_price: perTokenToMTok(result.cache_write_price ?? null),
        cache_read_price: perTokenToMTok(result.cache_read_price ?? null),
        image_output_price: perTokenToMTok(result.image_output_price ?? null),
      })
    }
  } catch {
    // 查询失败不影响用户操作
  }
}
</script>

<style scoped>
.collapsible-content {
  display: grid;
  grid-template-rows: 1fr;
  transition: grid-template-rows 0.25s ease;
}

.collapsible-content--collapsed {
  grid-template-rows: 0fr;
}

.collapsible-inner {
  overflow: hidden;
}
</style>
