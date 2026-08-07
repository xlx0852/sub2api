<template>
  <BaseDialog
    :show="show"
    :title="t('admin.groups.pricing.title')"
    width="wide"
    @close="emit('close')"
  >
    <div v-if="loading" class="flex items-center justify-center py-16 text-sm text-gray-500">
      <Icon name="refresh" size="md" class="mr-2 animate-spin" />
      {{ t('common.loading') }}
    </div>

    <div v-else-if="summary" class="space-y-5">
      <!-- Billing flow -->
      <div class="rounded-lg border border-sky-200 bg-sky-50/80 p-3 text-sm dark:border-sky-900/50 dark:bg-sky-950/30">
        <p class="font-medium text-sky-900 dark:text-sky-100">
          {{ t('admin.groups.pricing.billingFlowTitle') }}
        </p>
        <p class="mt-1 text-sky-800/90 dark:text-sky-200/90">
          {{ t('admin.groups.pricing.billingFlowDesc') }}
        </p>
        <p class="mt-1 text-xs text-sky-700/80 dark:text-sky-300/70">
          {{ t('admin.groups.pricing.billingFlowCostNote') }}
        </p>
        <p class="mt-1 text-xs opacity-80">
          {{ t('admin.groups.pricing.architectureSplit') }}
        </p>
      </div>

      <!-- Current source -->
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0 space-y-1">
          <p class="text-xs font-medium uppercase tracking-wide text-gray-400">
            {{ t('admin.groups.pricing.currentSource') }}
          </p>
          <div class="flex flex-wrap items-center gap-2">
            <span
              class="inline-flex items-center rounded-md px-2 py-1 text-xs font-semibold"
              :class="sourceBadgeClass"
            >
              {{ sourceLabel }}
            </span>
            <span v-if="summary.policy_name" class="text-sm font-medium text-gray-900 dark:text-white">
              {{ summary.policy_name }}
            </span>
            <span
              v-if="summary.inactive_policy"
              class="rounded bg-amber-100 px-1.5 py-0.5 text-[11px] font-medium text-amber-800 dark:bg-amber-950/50 dark:text-amber-200"
            >
              {{ t('admin.groups.pricing.inactivePolicy') }}
            </span>
          </div>
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.groups.pricing.groupRate', { rate: formatRate(summary.rate_multiplier) }) }}
            · {{ group?.name }}
          </p>
        </div>
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="loading"
          @click="reload"
        >
          <Icon name="refresh" size="sm" class="mr-1" :class="loading ? 'animate-spin' : ''" />
          {{ t('common.refresh') }}
        </button>
      </div>

      <!-- Official-only warning -->
      <div
        v-if="!summary.effective"
        class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900 dark:border-amber-900/40 dark:bg-amber-950/30 dark:text-amber-100"
      >
        <p class="font-medium">{{ t('admin.groups.pricing.officialOnlyTitle') }}</p>
        <p class="mt-1 text-xs opacity-90">{{ t('admin.groups.pricing.officialOnlyDesc') }}</p>
      </div>

      <!-- Bind selector -->
      <div class="space-y-2">
        <label class="input-label">{{ t('admin.groups.pricing.selectPolicy') }}</label>
        <Select
          v-model="selectedPolicyKey"
          :options="policyOptions"
          :placeholder="t('admin.groups.pricing.selectPolicyPlaceholder')"
          class="w-full"
        />
        <p v-if="selectedSharedHint" class="text-xs text-gray-500 dark:text-gray-400">
          {{ selectedSharedHint }}
        </p>
        <div class="flex flex-wrap items-center gap-2 pt-1">
          <button
            type="button"
            class="btn btn-primary btn-sm"
            :disabled="saving || !selectionChanged"
            @click="saveBinding"
          >
            <Icon v-if="saving" name="refresh" size="sm" class="mr-1 animate-spin" />
            {{ t('admin.groups.pricing.applyBinding') }}
          </button>
          <button
            v-if="summary.policy_id"
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="saving"
            @click="unbind"
          >
            {{ t('admin.groups.pricing.followOfficial') }}
          </button>
          <a
            :href="workspaceHref"
            class="btn btn-secondary btn-sm"
            @click="emit('close')"
          >
            <Icon name="externalLink" size="sm" class="mr-1" />
            {{ t('admin.groups.pricing.openPolicyLibrary') }}
          </a>
        </div>
      </div>

      <!-- Model preview table -->
      <div>
        <div class="mb-2 flex items-center justify-between gap-2">
          <p class="text-sm font-medium text-gray-900 dark:text-white">
            {{ t('admin.groups.pricing.modelPreview') }}
          </p>
          <p class="text-[11px] text-gray-400">
            {{ t('admin.groups.pricing.unitHint') }}
          </p>
        </div>

        <div
          v-if="!summary.models?.length"
          class="rounded-lg border border-dashed border-gray-200 px-4 py-8 text-center text-sm text-gray-500 dark:border-dark-600"
        >
          {{
            summary.effective
              ? t('admin.groups.pricing.noModelOverrides')
              : t('admin.groups.pricing.noPolicyModels')
          }}
        </div>

        <div v-else class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-600">
          <table class="min-w-full divide-y divide-gray-200 text-xs dark:divide-dark-600">
            <thead class="bg-gray-50 dark:bg-dark-800">
              <tr class="text-left text-[11px] font-medium uppercase tracking-wide text-gray-500">
                <th class="px-3 py-2">{{ t('admin.groups.pricing.colModel') }}</th>
                <th class="px-3 py-2">{{ t('admin.groups.pricing.colOfficial') }}</th>
                <th class="px-3 py-2">{{ t('admin.groups.pricing.colSell') }}</th>
                <th class="px-3 py-2">{{ t('admin.groups.pricing.colSource') }}</th>
                <th class="px-3 py-2">{{ t('admin.groups.pricing.colEffective') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 bg-white dark:divide-dark-700 dark:bg-dark-900">
              <tr v-for="row in summary.models" :key="row.model">
                <td class="max-w-[12rem] truncate px-3 py-2 font-mono text-gray-900 dark:text-white" :title="row.model">
                  {{ row.model }}
                  <span
                    v-if="row.markup_n != null"
                    class="ml-1 rounded bg-violet-100 px-1 py-0.5 text-[10px] font-semibold text-violet-800 dark:bg-violet-950/40 dark:text-violet-200"
                  >×{{ row.markup_n }}</span>
                </td>
                <td class="whitespace-nowrap px-3 py-2 text-gray-600 dark:text-gray-300">
                  {{ formatPair(row.official_input, row.official_output) }}
                </td>
                <td class="whitespace-nowrap px-3 py-2 text-gray-600 dark:text-gray-300">
                  {{ formatPair(row.sell_input, row.sell_output) }}
                </td>
                <td class="px-3 py-2">
                  <span
                    class="rounded px-1.5 py-0.5 text-[11px] font-medium"
                    :class="row.source === 'policy'
                      ? 'bg-violet-100 text-violet-800 dark:bg-violet-950/40 dark:text-violet-200'
                      : 'bg-gray-100 text-gray-700 dark:bg-dark-600 dark:text-gray-300'"
                  >
                    {{ row.source === 'policy'
                      ? t('admin.groups.pricing.sourcePolicy')
                      : t('admin.groups.pricing.sourceOfficial') }}
                  </span>
                </td>
                <td class="whitespace-nowrap px-3 py-2 font-medium text-emerald-700 dark:text-emerald-300">
                  {{ formatPair(row.effective_input, row.effective_output) }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <div v-else class="py-12 text-center text-sm text-gray-500">
      {{ t('admin.groups.pricing.loadError') }}
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button type="button" class="btn btn-secondary" @click="emit('close')">
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { GroupPricingSummary } from '@/api/admin/groups'
import type { AdminGroup } from '@/types'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  show: boolean
  group: AdminGroup | null
}>()

const emit = defineEmits<{
  close: []
  success: []
}>()

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const saving = ref(false)
const summary = ref<GroupPricingSummary | null>(null)
/** '' = official; otherwise policy id as string for Select */
const selectedPolicyKey = ref<string>('')

const workspaceHref = computed(() => {
  const id = props.group?.id
  return id
    ? `/admin/sell-price-policies/pricing?group=${id}`
    : '/admin/sell-price-policies/pricing'
})

const sourceLabel = computed(() => {
  if (!summary.value) return ''
  if (summary.value.effective) return t('admin.groups.pricing.sourcePolicy')
  if (summary.value.policy_id) return t('admin.groups.pricing.sourcePolicyInactive')
  return t('admin.groups.pricing.sourceOfficial')
})

const sourceBadgeClass = computed(() => {
  if (!summary.value) return 'bg-gray-100 text-gray-700'
  if (summary.value.effective) {
    return 'bg-violet-100 text-violet-800 dark:bg-violet-950/40 dark:text-violet-200'
  }
  if (summary.value.policy_id) {
    return 'bg-amber-100 text-amber-800 dark:bg-amber-950/40 dark:text-amber-200'
  }
  return 'bg-gray-100 text-gray-700 dark:bg-dark-600 dark:text-gray-300'
})

const policyOptions = computed(() => {
  const opts: { value: string; label: string }[] = [
    { value: '', label: t('admin.groups.pricing.optionOfficial') }
  ]
  for (const p of summary.value?.available_policies || []) {
    const status =
      p.status === 'active'
        ? ''
        : ` (${t('admin.groups.pricing.statusDisabled')})`
    const models = p.model_count > 0 ? ` · ${p.model_count}` : ''
    opts.push({
      value: String(p.id),
      label: `${p.name}${status}${models}`
    })
  }
  return opts
})

const selectedSharedHint = computed(() => {
  const key = selectedPolicyKey.value
  if (!key || !summary.value) return ''
  const id = Number(key)
  const p = summary.value.available_policies?.find((x) => x.id === id)
  if (!p?.bound_other_group_names?.length) return ''
  return t('admin.groups.pricing.sharedWith', {
    names: p.bound_other_group_names.slice(0, 5).join(', ')
  })
})

const selectionChanged = computed(() => {
  if (!summary.value) return false
  const current = summary.value.policy_id ? String(summary.value.policy_id) : ''
  return selectedPolicyKey.value !== current
})

function formatRate(n: number) {
  if (n == null || Number.isNaN(n)) return '1'
  return Number.isInteger(n) ? String(n) : n.toFixed(2).replace(/\.?0+$/, '')
}

function formatPrice(v: number | null | undefined) {
  if (v == null || Number.isNaN(v)) return '—'
  if (v === 0) return '$0'
  if (v >= 1) return `$${v.toFixed(2)}`
  if (v >= 0.01) return `$${v.toFixed(4)}`
  return `$${v.toFixed(6)}`
}

function formatPair(input: number | null | undefined, output: number | null | undefined) {
  return `${formatPrice(input)} / ${formatPrice(output)}`
}

async function reload() {
  if (!props.group) return
  loading.value = true
  try {
    const data = await adminAPI.groups.getPricingSummary(props.group.id)
    summary.value = data
    selectedPolicyKey.value = data.policy_id ? String(data.policy_id) : ''
  } catch (e: any) {
    summary.value = null
    appStore.showError(e?.message || t('admin.groups.pricing.loadError'))
  } finally {
    loading.value = false
  }
}

async function saveBinding() {
  if (!props.group) return
  saving.value = true
  try {
    const policyId = selectedPolicyKey.value ? Number(selectedPolicyKey.value) : null
    const data = await adminAPI.groups.bindSellPricePolicy(props.group.id, policyId)
    summary.value = data
    selectedPolicyKey.value = data.policy_id ? String(data.policy_id) : ''
    appStore.showSuccess(t('admin.groups.pricing.bindSuccess'))
    emit('success')
  } catch (e: any) {
    appStore.showError(e?.message || t('admin.groups.pricing.bindError'))
  } finally {
    saving.value = false
  }
}

async function unbind() {
  selectedPolicyKey.value = ''
  await saveBinding()
}

watch(
  () => [props.show, props.group?.id] as const,
  ([show]) => {
    if (show && props.group) {
      reload()
    } else if (!show) {
      summary.value = null
      selectedPolicyKey.value = ''
    }
  }
)
</script>
