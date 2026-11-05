<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-3 lg:flex-row lg:items-start">
          <div class="flex flex-wrap items-center gap-2">
            <input v-model="search" class="input w-64" :placeholder="t('admin.models.searchPlaceholder')" @keyup.enter="load" />
            <Select v-model="scope" :options="scopeOptions" class="w-52" />
            <Select v-model="platform" :options="platformOptions" class="w-40" @update:modelValue="load" />
            <button type="button" class="btn btn-secondary" :disabled="loading" @click="load">
              {{ loading ? t('common.loading') : t('common.refresh') }}
            </button>
          </div>
          <div class="flex flex-shrink-0 flex-wrap items-center gap-2 lg:justify-end">
            <button type="button" class="btn btn-primary" @click="openCreate">
              + {{ t('admin.models.create') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>

      <div class="overflow-x-auto">
        <table class="min-w-full text-sm">
          <thead class="table-header bg-[#f0f0ec] text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:bg-dark-800 dark:text-dark-400">
            <tr>
              <th class="px-3 py-3">{{ t('admin.models.colId') }}</th>
              <th class="px-3 py-3">{{ t('admin.models.colPlatform') }}</th>
              <th class="px-3 py-3">{{ t('admin.models.colDisplay') }}</th>
              <th class="px-3 py-2 text-right">{{ t('admin.models.colContext') }}</th>
              <th class="px-3 py-2 text-right">{{ t('admin.models.colPrices') }}</th>
              <th class="px-3 py-3">{{ t('admin.models.colStatus') }}</th>
              <th class="px-3 py-2" />
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 text-gray-700 dark:divide-dark-700 dark:text-gray-200">
            <tr v-for="m in visibleModels" :key="m.id" class="transition-colors hover:bg-black/[0.02] dark:hover:bg-white/[0.03]">
              <td class="px-3 py-2.5 font-mono">{{ m.id }}</td>
              <td class="px-3 py-2.5">
                <span v-if="m.platform">{{ m.platform }}</span>
                <span v-else class="rounded bg-gray-100 px-1.5 py-0.5 text-[11px] text-gray-500 dark:bg-dark-700 dark:text-gray-400" :title="t('admin.models.legacyTagHint')">{{ t('admin.models.legacyTag') }}</span>
              </td>
              <td class="px-3 py-2.5">{{ m.display_name || '-' }}</td>
              <td class="px-3 py-2.5 text-right">{{ m.context_window ?? '-' }}</td>
              <td class="px-3 py-2.5 text-right">
                <span v-if="m.price_row_count" class="text-gray-700 dark:text-gray-200">{{ m.price_row_count }}</span>
                <span v-else-if="!m.platform" class="text-gray-400" :title="t('admin.models.priceViaAliasHint')">{{ t('admin.models.priceViaAlias') }}</span>
                <span v-else class="text-amber-600 dark:text-amber-400" :title="t('admin.models.noPrices')">0</span>
              </td>
              <td class="px-3 py-2.5">
                <span v-if="m.retired" class="rounded bg-gray-100 px-1.5 py-0.5 text-[11px] dark:bg-dark-700">{{ t('admin.models.retired') }}</span>
                <span v-else-if="!m.enabled" class="rounded bg-gray-100 px-1.5 py-0.5 text-[11px] dark:bg-dark-700">{{ t('admin.models.disabled') }}</span>
                <span v-else class="rounded bg-emerald-100 px-1.5 py-0.5 text-[11px] text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300">{{ t('admin.models.enabled') }}</span>
              </td>
              <td class="px-3 py-2.5 text-right">
                <button type="button" class="text-primary-600 hover:text-primary-700" @click="openEdit(m)">
                  {{ t('admin.models.edit') }}
                </button>
              </td>
            </tr>
            <tr v-if="!visibleModels.length">
              <td colspan="7" class="px-3 py-6 text-center text-gray-400">{{ t('admin.models.empty') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      </template>
    </TablePageLayout>

    <BaseDialog :show="editorOpen" :title="editing?.id || t('admin.models.create')" width="extra-wide" @close="editorOpen = false">
      <div v-if="editing" class="space-y-4">
        <!-- 身份与能力 -->
        <section class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
          <h3 class="text-sm font-medium">{{ t('admin.models.identity') }}</h3>
          <div class="mt-2 grid grid-cols-2 gap-3 sm:grid-cols-4">
            <label class="text-xs">
              <span class="text-gray-500">{{ t('admin.models.colId') }}</span>
              <input v-model="editing.id" :disabled="isExisting" class="input mt-1 text-sm font-mono disabled:opacity-60" />
            </label>
            <label class="text-xs">
              <span class="text-gray-500">{{ t('admin.models.colPlatform') }}</span>
              <input v-model="editing.platform" class="input mt-1 text-sm" :placeholder="t('admin.models.platformPlaceholder')" />
            </label>
            <label class="text-xs">
              <span class="text-gray-500">{{ t('admin.models.colDisplay') }}</span>
              <input v-model="editing.display_name" class="input mt-1 text-sm" />
            </label>
            <label class="text-xs">
              <span class="text-gray-500">{{ t('admin.models.colContext') }}</span>
              <input v-model.number="editing.context_window" type="number" class="input mt-1 text-sm" />
            </label>
            <label class="text-xs sm:col-span-2">
              <span class="text-gray-500">{{ t('admin.models.upstreamId') }}</span>
              <input v-model="editing.upstream_id" class="input mt-1 text-sm font-mono" :placeholder="t('admin.models.upstreamHint')" />
            </label>
            <div class="flex items-end pb-1 sm:col-span-2">
              <div class="flex flex-wrap items-center gap-4 text-xs">
                <label class="inline-flex items-center gap-1.5">
                  <input v-model="editing.enabled" type="checkbox" />
                  <span>{{ t('admin.models.enabled') }}</span>
                </label>
                <label class="inline-flex items-center gap-1.5">
                  <input v-model="editing.retired" type="checkbox" />
                  <span>{{ t('admin.models.retired') }}</span>
                </label>
                <label class="inline-flex items-center gap-1.5">
                  <input v-model="editing.is_reasoning" type="checkbox" />
                  <span>{{ t('admin.models.reasoning') }}</span>
                </label>
              </div>
            </div>
          </div>
        </section>

        <!-- 别名 -->
        <section class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
          <div class="flex items-center justify-between">
            <h3 class="text-sm font-medium">{{ t('admin.models.aliases') }}</h3>
            <button type="button" class="text-xs text-primary-600" @click="addAlias">+ {{ t('admin.models.addAlias') }}</button>
          </div>
          <div class="mt-2 space-y-2">
            <div v-for="(a, i) in aliases" :key="i" class="flex gap-2">
              <input v-model="a.alias" class="input flex-1 text-xs font-mono" :placeholder="t('admin.models.aliasPlaceholder')" />
              <input v-model="a.platform" class="input w-32 text-xs" :placeholder="t('admin.models.platformPlaceholder')" />
              <Select v-model="a.kind" :options="aliasKindOptions" class="w-40" />
              <button type="button" class="text-xs text-red-500" @click="aliases.splice(i, 1)">{{ t('common.delete') }}</button>
            </div>
            <p v-if="!aliases.length" class="text-xs text-gray-400">{{ t('admin.models.noAliases') }}</p>
          </div>
        </section>

        <!-- 各组价格 -->
        <section class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
          <h3 class="text-sm font-medium">{{ t('admin.models.prices') }}</h3>
          <p class="mt-1 text-[11px] text-gray-400">{{ t('admin.models.pricesHint') }}</p>
          <div class="mt-2 overflow-x-auto">
            <table class="min-w-full text-xs">
              <thead class="text-left text-gray-500">
                <tr class="border-b border-gray-100 dark:border-dark-700">
                  <th class="w-[22%] py-1.5 pr-2 text-left font-medium">{{ t('admin.models.colGroup') }}</th>
                  <th class="w-[14%] py-1.5 pr-2 text-right font-medium">{{ t('admin.models.priceInput') }}</th>
                  <th class="w-[14%] py-1.5 pr-2 text-right font-medium">{{ t('admin.models.priceOutput') }}</th>
                  <th class="w-[16%] py-1.5 pr-2 text-right font-medium">{{ t('admin.models.priceCacheRead') }}</th>
                  <th class="w-[14%] py-1.5 pr-2 text-left font-medium">{{ t('admin.models.fxMode') }}</th>
                  <th class="w-[10%] py-1.5 pr-2 text-right font-medium">{{ t('admin.models.fxRate') }}</th>
                  <th class="w-[10%] py-1.5 pr-2 text-right font-medium" />
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in prices" :key="row.id" class="border-t border-gray-100 dark:border-dark-700">
                  <td class="py-1 pr-2">{{ row.group_id == null ? t('admin.models.defaultPrice') : groupLabel(row.group_id) }}</td>
                  <td class="py-1 pr-2 text-right">{{ perMillion(row.input_price) }}</td>
                  <td class="py-1 pr-2 text-right">{{ perMillion(row.output_price) }}</td>
                  <td class="py-1 pr-2 text-right">{{ perMillion(row.cache_read_price) }}</td>
                  <td class="py-1 pr-2">{{ fxModeLabel(row.fx_mode) }}</td>
                  <td class="py-1 pr-2 text-right">{{ row.fx_rate ?? '-' }}</td>
                  <td class="py-1 pr-2 text-right">
                    <button type="button" class="text-red-500 hover:text-red-600" @click="removePrice(row)">{{ t('common.delete') }}</button>
                  </td>
                </tr>
                <tr v-if="!prices.length">
                  <td colspan="7" class="py-2 text-center text-amber-600 dark:text-amber-400">{{ t('admin.models.noPrices') }}</td>
                </tr>
              </tbody>
            </table>
          </div>

          <!-- 新增/更新一行价格 -->
          <div class="mt-3 rounded-lg border border-dashed border-gray-300 p-3 dark:border-dark-500">
            <p class="text-xs font-medium text-gray-700 dark:text-gray-200">{{ t('admin.models.newPriceRow') }}</p>
            <div class="mt-2 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6">
              <label class="text-xs">
                <span class="text-gray-500">{{ t('admin.models.colGroup') }}</span>
                <Select v-model="draft.group_id" :options="groupOptions" class="mt-1 text-xs" />
              </label>
              <label class="text-xs">
                <span class="text-gray-500">{{ t('admin.models.priceInput') }}</span>
                <input v-model.number="draft.input" type="number" step="any" class="input mt-1 text-xs" :placeholder="t('admin.models.pricePlaceholder')" />
              </label>
              <label class="text-xs">
                <span class="text-gray-500">{{ t('admin.models.priceOutput') }}</span>
                <input v-model.number="draft.output" type="number" step="any" class="input mt-1 text-xs" :placeholder="t('admin.models.pricePlaceholder')" />
              </label>
              <label class="text-xs">
                <span class="text-gray-500">{{ t('admin.models.priceCacheRead') }}</span>
                <input v-model.number="draft.cache_read" type="number" step="any" class="input mt-1 text-xs" :placeholder="t('admin.models.pricePlaceholder')" />
              </label>
              <label class="text-xs">
                <span class="text-gray-500">{{ t('admin.models.fxMode') }}</span>
                <Select v-model="draft.fx_mode" :options="fxModeOptions" class="mt-1 text-xs" />
              </label>
              <label v-if="draft.fx_mode === 'fx_auto'" class="text-xs">
                <span class="text-gray-500">{{ t('admin.models.fxRate') }}</span>
                <input v-model.number="draft.fx_rate" type="number" step="any" class="input mt-1 text-xs" placeholder="7" />
              </label>
              <label v-if="draft.fx_mode === 'fx_auto'" class="text-xs">
                <span class="text-gray-500">{{ t('admin.models.costId') }}</span>
                <input v-model.number="draft.cost_id" type="number" step="1" class="input mt-1 text-xs" :placeholder="t('admin.models.costIdPlaceholder')" />
              </label>
            </div>
            <div class="mt-3 flex justify-end">
              <button type="button" class="btn btn-primary btn-sm" :disabled="saving" @click="savePrice">
                {{ t('admin.models.savePrice') }}
              </button>
            </div>
          </div>
        </section>
      </div>

      <template #footer>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn btn-secondary" @click="editorOpen = false">{{ t('common.cancel') }}</button>
          <button type="button" class="btn btn-primary" :disabled="saving" @click="saveModel">{{ t('common.save') }}</button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import BaseDialog from '@/ui/BaseDialog.vue'
import Select from '@/ui/Select.vue'
import adminAPI from '@/api/admin'
import { useAppStore } from '@/stores/app'
import { apiErrorMessage } from '@/api/client'
import type { ModelPriceRow, ModelRegistryEntry } from '@/api/admin/modelRegistry'
import type { AdminGroup } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

const models = ref<ModelRegistryEntry[]>([])
const loading = ref(false)
const search = ref('')
const platform = ref('')
// 范围：model = 有平台的真实模型（默认）；legacy = 只有兜底价的兼容 id；all = 全部
const scope = ref<'model' | 'legacy' | 'all'>('model')
const scopeOptions = computed(() => [
  { value: 'model', label: t('admin.models.scopeModel') },
  { value: 'legacy', label: t('admin.models.scopeLegacy') },
  { value: 'all', label: t('admin.models.scopeAll') }
])
const visibleModels = computed(() => {
  const list = models.value
  if (scope.value === 'all') return list
  if (scope.value === 'legacy') return list.filter((m) => !m.platform)
  return list.filter((m) => !!m.platform)
})
const platformOptions = computed(() => [
  { value: '', label: t('admin.models.allPlatforms') },
  { value: 'openai', label: 'openai' },
  { value: 'anthropic', label: 'anthropic' },
  { value: 'grok', label: 'grok' },
  { value: 'kimi', label: 'kimi' },
  { value: 'deepseek', label: 'deepseek' },
  { value: 'gemini', label: 'gemini' },
  { value: 'antigravity', label: 'antigravity' }
])

const editorOpen = ref(false)
const isExisting = ref(false)
const editing = ref<MutableModel | null>(null)
const aliases = ref<{ alias: string; platform: string; kind: string }[]>([])
const prices = ref<ModelPriceRow[]>([])
const groups = ref<AdminGroup[]>([])
const saving = ref(false)

type MutableModel = Omit<ModelRegistryEntry, 'media' | 'price_row_count'> & { media?: Record<string, boolean> }

const groupOptions = computed(() => [
  { value: null as number | null, label: t('admin.models.defaultPrice') },
  ...groups.value.map((g) => ({ value: g.id as number | null, label: g.name }))
])
const aliasKindOptions = computed(() =>
  ['alias', 'id_override', 'id_reverse', 'retired'].map((kind) => ({
    value: kind,
    label: t('admin.models.aliasKinds.' + kind)
  }))
)
const fxModeOptions = computed(() =>
  ['manual', 'cny_native', 'fx_auto'].map((mode) => ({
    value: mode,
    label: t('admin.models.fxModes.' + mode)
  }))
)
const fxModeLabel = (mode: string) =>
  ['manual', 'cny_native', 'fx_auto'].includes(mode)
    ? t('admin.models.fxModes.' + mode)
    : mode

const draft = reactive<{
  group_id: number | null
  input: number | null
  output: number | null
  cache_read: number | null
  fx_mode: string
  fx_rate: number | null
  cost_id: number | null
}>({ group_id: null, input: null, output: null, cache_read: null, fx_mode: 'manual', fx_rate: 7, cost_id: null })

function perMillion(v: number | null | undefined) {
  return v == null ? '-' : (v * 1_000_000).toFixed(4).replace(/\.?0+$/, '')
}

function groupLabel(id: number) {
  return groups.value.find((g) => g.id === id)?.name || `#${id}`
}

async function load() {
  loading.value = true
  try {
    models.value = await adminAPI.models.listModels({ platform: platform.value || undefined, search: search.value || undefined })
  } catch (error) {
    appStore.showError(apiErrorMessage(error))
  } finally {
    loading.value = false
  }
}

async function loadGroups() {
  try {
    const res = await adminAPI.groups.list(1, 200)
    groups.value = res.items as AdminGroup[]
  } catch {
    groups.value = []
  }
}

function openCreate() {
  isExisting.value = false
  editing.value = {
    id: '', platform: 'openai', display_name: '', family: '', owned_by: '',
    context_window: null, max_context_window: null, media: {},
    is_reasoning: false, enabled: true, retired: false,
    is_default_chat: false, is_default_test: false, upstream_id: '', version: 0, notes: ''
  } as MutableModel
  aliases.value = []
  prices.value = []
  editorOpen.value = true
}

async function openEdit(m: ModelRegistryEntry) {
  isExisting.value = true
  editing.value = { ...m }
  editorOpen.value = true
  try {
    aliases.value = (await adminAPI.models.listModelAliases(m.id)).map((a) => ({
      alias: a.alias, platform: a.platform, kind: a.kind
    }))
    prices.value = await adminAPI.models.listModelPrices(m.id)
  } catch (error) {
    appStore.showError(apiErrorMessage(error))
  }
}

function addAlias() {
  aliases.value.push({ alias: '', platform: editing.value?.platform || '', kind: 'alias' })
}

async function saveModel() {
  if (!editing.value) return
  saving.value = true
  try {
    await adminAPI.models.upsertModel(editing.value)
    if (isExisting.value) {
      await adminAPI.models.replaceModelAliases(editing.value.id, aliases.value.filter((a) => a.alias.trim()))
    }
    appStore.showSuccess(t('admin.models.saved'))
    await load()
    if (isExisting.value) {
      prices.value = await adminAPI.models.listModelPrices(editing.value.id)
    }
  } catch (error) {
    appStore.showError(apiErrorMessage(error))
  } finally {
    saving.value = false
  }
}

async function savePrice() {
  if (!editing.value?.id || !isExisting.value) {
    appStore.showError(t('admin.models.saveModelFirst'))
    return
  }
  saving.value = true
  try {
    await adminAPI.models.upsertModelPrice(editing.value.id, {
      group_id: draft.group_id,
      currency: 'CNY',
      fx_mode: draft.fx_mode,
      fx_rate: draft.fx_mode === 'fx_auto' ? draft.fx_rate : null,
      cost_id: draft.fx_mode === 'fx_auto' ? draft.cost_id : null,
      input_price: draft.input == null ? null : draft.input / 1_000_000,
      output_price: draft.output == null ? null : draft.output / 1_000_000,
      cache_read_price: draft.cache_read == null ? null : draft.cache_read / 1_000_000
    })
    prices.value = await adminAPI.models.listModelPrices(editing.value.id)
    await load()
  } catch (error) {
    appStore.showError(apiErrorMessage(error))
  } finally {
    saving.value = false
  }
}

async function removePrice(row: ModelPriceRow) {
  if (!editing.value) return
  try {
    await adminAPI.models.deleteModelPrice(editing.value.id, row.group_id)
    prices.value = await adminAPI.models.listModelPrices(editing.value.id)
    await load()
  } catch (error) {
    appStore.showError(apiErrorMessage(error))
  }
}

onMounted(() => {
  void load()
  void loadGroups()
})
</script>
