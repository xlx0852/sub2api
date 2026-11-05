<template>
  <div class="space-y-6">
    <!-- 定价管线开关 -->
    <section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
        {{ t('admin.settings.pricing.pipelineTitle') }}
      </h3>
      <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.pricing.pipelineHint') }}
      </p>

      <div class="mt-3 flex flex-wrap items-center gap-3">
        <Select
          :modelValue="pipelineMode"
          :options="pipelineOptions"
          class="w-56"
          @update:modelValue="onSelectMode"
        />
        <button
          type="button"
          class="btn btn-primary btn-sm"
          :disabled="pipelineSaving || pipelineMode === pipelineSaved"
          @click="savePipeline"
        >
          {{ pipelineSaving ? t('common.saving') : t('common.save') }}
        </button>
        <span class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.pricing.pipelineCurrent', { mode: pipelineSaved || 'legacy' }) }}
        </span>
      </div>

      <p class="mt-2 rounded bg-amber-50 px-2 py-1.5 text-[11px] leading-5 text-amber-800 dark:bg-amber-950/20 dark:text-amber-200">
        {{ t('admin.settings.pricing.pipelineWarning') }}
      </p>

      <div class="mt-4 border-t border-gray-100 pt-3 dark:border-dark-700">
        <h4 class="text-xs font-semibold text-gray-900 dark:text-white">
          {{ t('admin.settings.pricing.priceSourceTitle') }}
        </h4>
        <p class="mt-1 text-[11px] leading-5 text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.pricing.priceSourceHint') }}
        </p>
        <div class="mt-2 flex flex-wrap items-center gap-3">
          <Select
            :modelValue="priceSource"
            :options="priceSourceOptions"
            class="w-64"
            @update:modelValue="onSelectPriceSource"
          />
          <button
            type="button"
            class="btn btn-primary btn-sm"
            :disabled="priceSourceSaving || priceSource === priceSourceSaved"
            @click="savePriceSource"
          >
            {{ priceSourceSaving ? t('common.saving') : t('common.save') }}
          </button>
          <span class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.pricing.priceSourceCurrent', { source: priceSourceSaved }) }}
          </span>
        </div>

        <h4 class="mt-4 text-xs font-semibold text-gray-900 dark:text-white">
          {{ t('admin.settings.pricing.registrySourceTitle') }}
        </h4>
        <p class="mt-1 text-[11px] leading-5 text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.pricing.registrySourceHint') }}
        </p>
        <div class="mt-2 flex flex-wrap items-center gap-3">
          <Select
            :modelValue="registrySource"
            :options="registryOptions"
            class="w-64"
            @update:modelValue="onSelectRegistrySource"
          />
          <button
            type="button"
            class="btn btn-primary btn-sm"
            :disabled="registrySaving || registrySource === registrySourceSaved"
            @click="saveRegistrySource"
          >
            {{ registrySaving ? t('common.saving') : t('common.saving') }}
          </button>
          <span class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.pricing.registrySourceCurrent', { source: registrySourceSaved }) }}
          </span>
          <span class="text-[11px] text-amber-600 dark:text-amber-400">
            {{ t('admin.settings.pricing.registrySourceRestart') }}
          </span>
        </div>
      </div>
    </section>

    <!-- 成本基 -->
    <section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.settings.pricing.costTitle') }}
          </h3>
          <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.pricing.costHint') }}
          </p>
        </div>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="costLoading" @click="loadCosts">
          {{ t('common.refresh') }}
        </button>
      </div>

      <div class="mt-3 overflow-x-auto">
        <table class="min-w-full text-xs">
          <thead class="text-left text-gray-500 dark:text-gray-400">
            <tr>
              <th class="py-1 pr-3">ID</th>
              <th class="py-1 pr-3">{{ t('admin.settings.pricing.colVendor') }}</th>
              <th class="py-1 pr-3">{{ t('admin.settings.pricing.colModel') }}</th>
              <th class="py-1 pr-3">{{ t('admin.settings.pricing.colCurrency') }}</th>
              <th class="py-1 pr-3 text-right">{{ t('admin.settings.pricing.colInput') }}</th>
              <th class="py-1 pr-3 text-right">{{ t('admin.settings.pricing.colCached') }}</th>
              <th class="py-1 pr-3 text-right">{{ t('admin.settings.pricing.colOutput') }}</th>
              <th class="py-1 pr-3 text-right">{{ t('admin.settings.pricing.colDiscount') }}</th>
              <th class="py-1 pr-3" />
            </tr>
          </thead>
          <tbody class="text-gray-700 dark:text-gray-200">
            <tr v-for="row in costs" :key="row.id" class="border-t border-gray-100 dark:border-dark-700">
              <td class="py-1 pr-3 text-gray-400">{{ row.id }}</td>
              <td class="py-1 pr-3">{{ row.vendor }}</td>
              <td class="py-1 pr-3 font-mono">{{ row.model_pattern }}</td>
              <td class="py-1 pr-3">{{ row.currency }}</td>
              <td class="py-1 pr-3 text-right">{{ row.input_per_mtok }}</td>
              <td class="py-1 pr-3 text-right">{{ row.cached_per_mtok }}</td>
              <td class="py-1 pr-3 text-right">{{ row.output_per_mtok }}</td>
              <td class="py-1 pr-3 text-right">{{ row.discount_pct }}%</td>
              <td class="py-1 pr-3 text-right">
                <button type="button" class="text-red-500 hover:text-red-600" @click="removeCost(row.id)">
                  {{ t('common.delete') }}
                </button>
              </td>
            </tr>
            <tr v-if="!costs.length">
              <td colspan="9" class="py-2 text-center text-gray-400">{{ t('admin.settings.pricing.costEmpty') }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 新增/更新（vendor + model 唯一） -->
      <div class="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-8">
        <input v-model="draft.vendor" class="input text-xs" :placeholder="t('admin.settings.pricing.colVendor')" />
        <input v-model="draft.model_pattern" class="input text-xs sm:col-span-2" :placeholder="t('admin.settings.pricing.colModel')" />
        <Select v-model="draft.currency" :options="currencyOptions" class="text-xs" />
        <input v-model.number="draft.input_per_mtok" type="number" step="any" class="input text-xs" :placeholder="t('admin.settings.pricing.colInput')" />
        <input v-model.number="draft.cached_per_mtok" type="number" step="any" class="input text-xs" :placeholder="t('admin.settings.pricing.colCached')" />
        <input v-model.number="draft.output_per_mtok" type="number" step="any" class="input text-xs" :placeholder="t('admin.settings.pricing.colOutput')" />
        <input v-model.number="draft.discount_pct" type="number" step="any" min="0" max="100" class="input text-xs" :placeholder="t('admin.settings.pricing.colDiscount')" />
      </div>
      <div class="mt-2 flex items-center gap-2">
        <button type="button" class="btn btn-primary btn-sm" :disabled="costSaving" @click="saveCost">
          {{ t('admin.settings.pricing.upsert') }}
        </button>
        <span class="text-[11px] text-gray-400">{{ t('admin.settings.pricing.upsertHint') }}</span>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/ui/Select.vue'
import adminAPI from '@/api/admin'
import { useAppStore } from '@/stores/app'
import { apiErrorMessage } from '@/api/client'
import type { PriceCost } from '@/api/admin/settings'

const { t } = useI18n()
const appStore = useAppStore()

const pipelineMode = ref<'legacy' | 'shadow' | 'unified'>('legacy')
const pipelineSaved = ref<'legacy' | 'shadow' | 'unified'>('legacy')
const pipelineSaving = ref(false)
const pipelineOptions = [
  { value: 'legacy', label: 'legacy' },
  { value: 'shadow', label: 'shadow' },
  { value: 'unified', label: 'unified' }
]

const priceSource = ref<'legacy' | 'dual' | 'model_prices'>('legacy')
const priceSourceSaved = ref<'legacy' | 'dual' | 'model_prices'>('legacy')
const priceSourceSaving = ref(false)
const priceSourceOptions = [
  { value: 'legacy', label: 'legacy（现有链路）' },
  { value: 'dual', label: 'dual（双算打点，按 legacy 扣费）' },
  { value: 'model_prices', label: 'model_prices（注册表价格）' }
]

function onSelectPriceSource(value: unknown) {
  priceSource.value = value as typeof priceSource.value
}

async function savePriceSource() {
  priceSourceSaving.value = true
  try {
    const data = await adminAPI.settings.updatePricingPipeline({ pricing_source: priceSource.value })
    priceSourceSaved.value = data.pricing_source
    appStore.showSuccess(t('admin.settings.pricing.priceSourceSaved', { source: data.pricing_source }))
  } catch (error) {
    appStore.showError(apiErrorMessage(error))
  } finally {
    priceSourceSaving.value = false
  }
}

const registrySource = ref<'file' | 'dual' | 'db'>('file')
const registrySourceSaved = ref<'file' | 'dual' | 'db'>('file')
const registrySaving = ref(false)
const registryOptions = [
  { value: 'file', label: 'file（编译进二进制的 catalog）' },
  { value: 'dual', label: 'dual（比对差异，只打日志）' },
  { value: 'db', label: 'db（用注册表覆盖，含别名/默认价）' }
]

function onSelectRegistrySource(value: unknown) {
  registrySource.value = value as typeof registrySource.value
}

async function saveRegistrySource() {
  registrySaving.value = true
  try {
    const data = await adminAPI.settings.updatePricingPipeline({ registry_source: registrySource.value })
    registrySourceSaved.value = data.registry_source
    appStore.showSuccess(t('admin.settings.pricing.registrySourceSaved', { source: data.registry_source }))
  } catch (error) {
    appStore.showError(apiErrorMessage(error))
  } finally {
    registrySaving.value = false
  }
}

const costs = ref<PriceCost[]>([])
const costLoading = ref(false)
const costSaving = ref(false)
const currencyOptions = [
  { value: 'USD', label: 'USD' },
  { value: 'CNY', label: 'CNY' }
]
const draft = reactive({
  vendor: '',
  model_pattern: '',
  currency: 'USD' as 'USD' | 'CNY',
  input_per_mtok: 0,
  cached_per_mtok: 0,
  output_per_mtok: 0,
  discount_pct: 0
})

function onSelectMode(value: unknown) {
  pipelineMode.value = value as typeof pipelineMode.value
}

async function loadPipeline() {
  try {
    const data = await adminAPI.settings.getPricingPipeline()
    pipelineMode.value = data.mode
    pipelineSaved.value = data.mode
    priceSource.value = data.pricing_source || 'legacy'
    priceSourceSaved.value = data.pricing_source || 'legacy'
    registrySource.value = data.registry_source || 'file'
    registrySourceSaved.value = data.registry_source || 'file'
  } catch (error) {
    appStore.showError(apiErrorMessage(error))
  }
}

async function savePipeline() {
  pipelineSaving.value = true
  try {
    const data = await adminAPI.settings.updatePricingPipeline({ mode: pipelineMode.value })
    pipelineSaved.value = data.mode
    appStore.showSuccess(t('admin.settings.pricing.pipelineSaved', { mode: data.mode }))
  } catch (error) {
    appStore.showError(apiErrorMessage(error))
  } finally {
    pipelineSaving.value = false
  }
}

async function loadCosts() {
  costLoading.value = true
  try {
    costs.value = await adminAPI.settings.listPriceCosts()
  } catch (error) {
    appStore.showError(apiErrorMessage(error))
  } finally {
    costLoading.value = false
  }
}

async function saveCost() {
  if (!draft.vendor || !draft.model_pattern) {
    appStore.showError(t('admin.settings.pricing.vendorModelRequired'))
    return
  }
  costSaving.value = true
  try {
    await adminAPI.settings.upsertPriceCost({ ...draft, source_url: '', notes: '' })
    appStore.showSuccess(t('admin.settings.pricing.costSaved'))
    await loadCosts()
  } catch (error) {
    appStore.showError(apiErrorMessage(error))
  } finally {
    costSaving.value = false
  }
}

async function removeCost(id: number) {
  try {
    await adminAPI.settings.deletePriceCost(id)
    await loadCosts()
  } catch (error) {
    appStore.showError(apiErrorMessage(error))
  }
}

onMounted(() => {
  void loadPipeline()
  void loadCosts()
})
</script>
