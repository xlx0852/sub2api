<template>
  <AppLayout>
    <div class="flex h-[calc(100dvh-3.5rem)] min-h-[32rem] flex-col gap-0 lg:flex-row">
      <!-- Left: group picker -->
      <aside
        class="flex w-full shrink-0 flex-col border-b border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900 lg:w-80 lg:border-b-0 lg:border-r"
      >
        <div class="space-y-3 border-b border-gray-100 p-4 dark:border-dark-700">
          <div>
            <h1 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('admin.channels.title') }}
            </h1>
            <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
              {{ t('admin.channels.workspaceHint') }}
            </p>
          </div>
          <div class="relative">
            <Icon
              name="search"
              size="md"
              class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
            />
            <input
              v-model="groupSearch"
              type="text"
              class="input pl-10"
              :placeholder="t('admin.channels.searchGroups')"
            />
          </div>
          <div class="flex items-center gap-2">
            <Select
              v-model="platformFilter"
              :options="platformFilterOptions"
              class="min-w-0 flex-1"
            />
            <button
              type="button"
              class="btn btn-secondary btn-sm shrink-0"
              :disabled="groupsLoading"
              :title="t('common.refresh')"
              @click="reloadGroups"
            >
              <Icon name="refresh" size="sm" :class="groupsLoading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>

        <div class="flex-1 overflow-y-auto p-2">
          <div v-if="groupsLoading && !groups.length" class="py-10 text-center text-sm text-gray-500">
            {{ t('common.loading') }}
          </div>
          <div
            v-else-if="filteredGroups.length === 0"
            class="px-3 py-10 text-center text-sm text-gray-500"
          >
            {{ t('admin.channels.noGroupsMatch') }}
          </div>
          <button
            v-for="g in filteredGroups"
            :key="g.id"
            type="button"
            class="mb-1 flex w-full items-start gap-2 rounded-lg px-3 py-2.5 text-left transition-colors"
            :class="
              selectedGroupId === g.id
                ? 'bg-primary-50 ring-1 ring-primary-200 dark:bg-primary-950/30 dark:ring-primary-800'
                : 'hover:bg-gray-50 dark:hover:bg-dark-800'
            "
            @click="selectGroup(g.id)"
          >
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-1.5">
                <PlatformIcon
                  :platform="g.platform"
                  size="xs"
                  :class="platformTextClass(g.platform)"
                />
                <span
                  class="truncate text-sm font-medium"
                  :class="
                    selectedGroupId === g.id
                      ? 'text-primary-800 dark:text-primary-200'
                      : 'text-gray-900 dark:text-white'
                  "
                >{{ g.name }}</span>
              </div>
              <div class="mt-1 flex flex-wrap items-center gap-1.5 text-[11px]">
                <span class="rounded bg-gray-100 px-1.5 py-0.5 text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                  {{ formatRate(g.rate_multiplier) }}x
                </span>
                <span
                  class="rounded px-1.5 py-0.5 font-medium"
                  :class="sourceChipClass(g.sell_price_source)"
                >
                  {{ sourceChipLabel(g.sell_price_source) }}
                </span>
              </div>
            </div>
          </button>
        </div>
      </aside>

      <!-- Right: in-page editor -->
      <section class="flex min-w-0 flex-1 flex-col bg-gray-50/60 dark:bg-dark-950">
        <div
          v-if="!selectedGroup"
          class="flex flex-1 flex-col items-center justify-center gap-2 px-6 text-center"
        >
          <Icon name="dollar" size="lg" class="text-gray-300 dark:text-gray-600" />
          <p class="text-sm font-medium text-gray-700 dark:text-gray-200">
            {{ t('admin.channels.selectGroupPrompt') }}
          </p>
          <p class="max-w-md text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.channels.selectGroupHint') }}
          </p>
        </div>

        <template v-else>
          <header
            class="flex flex-wrap items-start justify-between gap-3 border-b border-gray-200 bg-white px-4 py-3 dark:border-dark-700 dark:bg-dark-900 sm:px-6"
          >
            <div class="min-w-0 space-y-1">
              <div class="flex flex-wrap items-center gap-2">
                <PlatformIcon
                  :platform="selectedGroup.platform"
                  size="sm"
                  :class="platformTextClass(selectedGroup.platform)"
                />
                <h2 class="truncate text-lg font-semibold text-gray-900 dark:text-white">
                  {{ selectedGroup.name }}
                </h2>
                <span
                  class="rounded-md px-2 py-0.5 text-xs font-semibold"
                  :class="sourceBadgeClass"
                >
                  {{ sourceLabel }}
                </span>
                <span
                  v-if="summary?.inactive_policy"
                  class="rounded bg-amber-100 px-1.5 py-0.5 text-[11px] font-medium text-amber-800 dark:bg-amber-950/50 dark:text-amber-200"
                >
                  {{ t('admin.groups.pricing.inactivePolicy') }}
                </span>
              </div>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.channels.groupRateLine', { rate: formatRate(selectedGroup.rate_multiplier) }) }}
                <template v-if="summary?.policy_name">
                  · {{ t('admin.channels.boundPolicy', { name: summary.policy_name }) }}
                </template>
              </p>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="panelLoading"
                @click="reloadSelected"
              >
                <Icon name="refresh" size="sm" class="mr-1" :class="panelLoading ? 'animate-spin' : ''" />
                {{ t('common.refresh') }}
              </button>
              <button
                v-if="summary?.policy_id"
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="saving || panelLoading"
                @click="unbindOfficial"
              >
                {{ t('admin.channels.followOfficial') }}
              </button>
              <button
                type="button"
                class="btn btn-primary btn-sm"
                :disabled="saving || panelLoading || !canSave"
                @click="handleSave"
              >
                <Icon v-if="saving" name="refresh" size="sm" class="mr-1 animate-spin" />
                {{ saving ? t('common.submitting') : t('admin.channels.savePricing') }}
              </button>
            </div>
          </header>

          <div class="flex-1 overflow-y-auto px-4 py-4 sm:px-6">
            <div v-if="panelLoading" class="flex items-center justify-center py-20 text-sm text-gray-500">
              <Icon name="refresh" size="md" class="mr-2 animate-spin" />
              {{ t('common.loading') }}
            </div>

            <div v-else class="mx-auto max-w-4xl space-y-5">
              <!-- Billing flow -->
              <div
                class="rounded-lg border border-sky-200 bg-sky-50/80 p-3 text-sm dark:border-sky-900/50 dark:bg-sky-950/30"
              >
                <p class="font-medium text-sky-900 dark:text-sky-100">
                  {{ t('admin.channels.form.billingFlowTitle') }}
                </p>
                <p class="mt-1 text-sky-800/90 dark:text-sky-200/90">
                  {{ t('admin.channels.form.billingFlowDesc') }}
                </p>
                <p class="mt-1 text-xs text-sky-700/80 dark:text-sky-300/70">
                  {{ t('admin.channels.form.billingFlowCostNote') }}
                </p>
              </div>

              <!-- Official-only callout -->
              <div
                v-if="!summary?.effective"
                class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-900 dark:border-amber-900/40 dark:bg-amber-950/30 dark:text-amber-100"
              >
                <p class="font-medium">{{ t('admin.groups.pricing.officialOnlyTitle') }}</p>
                <p class="mt-1 text-xs opacity-90">{{ t('admin.channels.officialOnlyWorkspace') }}</p>
              </div>

              <!-- Shared policy warning -->
              <div
                v-if="sharedGroupNames.length"
                class="rounded-lg border border-violet-200 bg-violet-50 p-3 text-sm text-violet-900 dark:border-violet-900/40 dark:bg-violet-950/30 dark:text-violet-100"
              >
                <p class="font-medium">{{ t('admin.channels.sharedPolicyTitle') }}</p>
                <p class="mt-1 text-xs opacity-90">
                  {{ t('admin.channels.sharedPolicyDesc', { names: sharedGroupNames.join(', ') }) }}
                </p>
              </div>

              <!-- Policy meta -->
              <div class="grid gap-3 sm:grid-cols-2">
                <div>
                  <label class="input-label">{{ t('admin.channels.form.name') }}</label>
                  <input
                    v-model="form.name"
                    type="text"
                    class="input"
                    :placeholder="defaultPolicyName"
                  />
                </div>
                <div>
                  <label class="input-label">{{ t('admin.channels.form.status') }}</label>
                  <Select v-model="form.status" :options="statusEditOptions" />
                </div>
                <div class="sm:col-span-2">
                  <label class="input-label">{{ t('admin.channels.form.description') }}</label>
                  <input
                    v-model="form.description"
                    type="text"
                    class="input"
                    :placeholder="t('admin.channels.form.descriptionPlaceholder')"
                  />
                </div>
              </div>

              <!-- Sell pricing entries -->
              <div class="space-y-3 rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-600 dark:bg-dark-900">
                <div class="flex flex-wrap items-center justify-between gap-2">
                  <div class="flex items-center gap-2 text-sm font-medium text-gray-900 dark:text-white">
                    <PlatformIcon
                      :platform="selectedGroup.platform"
                      size="xs"
                      :class="platformTextClass(selectedGroup.platform)"
                    />
                    <span>{{ t('admin.channels.form.modelPricing') }}</span>
                    <span class="text-xs font-normal text-gray-400">
                      {{ t('admin.groups.platforms.' + selectedGroup.platform, selectedGroup.platform) }}
                    </span>
                  </div>
                  <div class="flex items-center gap-2">
                    <button
                      type="button"
                      class="text-xs text-gray-500 hover:text-primary-600 disabled:opacity-50"
                      :disabled="syncing"
                      @click="syncLatestModels"
                    >
                      {{ syncing ? t('admin.channels.form.syncingModels') : t('admin.channels.form.syncLatestModels') }}
                    </button>
                    <button
                      type="button"
                      class="text-xs text-primary-600 hover:text-primary-700"
                      @click="addPricingEntry"
                    >
                      + {{ t('common.add') }}
                    </button>
                  </div>
                </div>

                <div
                  v-if="form.model_pricing.length === 0"
                  class="rounded border border-dashed border-gray-300 p-4 text-center text-xs text-gray-400 dark:border-dark-500"
                >
                  {{ t('admin.channels.form.noPricingRules') }}
                </div>
                <div v-else class="space-y-2">
                  <PricingEntryCard
                    v-for="(entry, idx) in form.model_pricing"
                    :key="idx"
                    :entry="entry"
                    :platform="selectedGroup.platform"
                    @update="updatePricingEntry(idx, $event)"
                    @remove="removePricingEntry(idx)"
                  />
                </div>
              </div>

              <!-- Live preview -->
              <div v-if="summary?.models?.length" class="space-y-2">
                <div class="flex items-center justify-between gap-2">
                  <p class="text-sm font-medium text-gray-900 dark:text-white">
                    {{ t('admin.groups.pricing.modelPreview') }}
                  </p>
                  <p class="text-[11px] text-gray-400">
                    {{ t('admin.groups.pricing.unitHint') }}
                  </p>
                </div>
                <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-600">
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
                        <td
                          class="max-w-[12rem] truncate px-3 py-2 font-mono text-gray-900 dark:text-white"
                          :title="row.model"
                        >
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
                            :class="
                              row.source === 'policy'
                                ? 'bg-violet-100 text-violet-800 dark:bg-violet-950/40 dark:text-violet-200'
                                : 'bg-gray-100 text-gray-700 dark:bg-dark-600 dark:text-gray-300'
                            "
                          >
                            {{
                              row.source === 'policy'
                                ? t('admin.groups.pricing.sourcePolicy')
                                : t('admin.groups.pricing.sourceOfficial')
                            }}
                          </span>
                        </td>
                        <td class="whitespace-nowrap px-3 py-2 font-medium text-emerald-700 dark:text-emerald-300">
                          {{ formatPair(row.effective_input, row.effective_output) }}
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
                <p class="text-[11px] text-gray-400">
                  {{ t('admin.channels.previewNote') }}
                </p>
              </div>
            </div>
          </div>
        </template>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type {
  Channel,
  ChannelModelPricing,
  CreateChannelRequest,
  UpdateChannelRequest,
} from '@/api/admin/channels'
import type { GroupPricingSummary } from '@/api/admin/groups'
import type { PricingFormEntry } from '@/components/admin/channel/types'
import {
  findModelConflict,
  formIntervalsToAPI,
  mTokToPerToken,
  perTokenToMTok,
  validateIntervals,
} from '@/components/admin/channel/types'
import type { AdminGroup, GroupPlatform, GroupSellPriceSource } from '@/types'
import { platformTextClass } from '@/utils/platformColors'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import PricingEntryCard from '@/components/admin/channel/PricingEntryCard.vue'

const { t } = useI18n()
const appStore = useAppStore()
const route = useRoute()
const router = useRouter()

const groups = ref<AdminGroup[]>([])
const groupsLoading = ref(false)
const groupSearch = ref('')
const platformFilter = ref('')
const selectedGroupId = ref<number | null>(null)

const panelLoading = ref(false)
const saving = ref(false)
const syncing = ref(false)
const summary = ref<GroupPricingSummary | null>(null)
const boundPolicy = ref<Channel | null>(null)

const form = reactive({
  name: '',
  description: '',
  status: 'active',
  model_pricing: [] as PricingFormEntry[],
})

const platformOrder: GroupPlatform[] = ['anthropic', 'openai', 'antigravity', 'grok']

const platformFilterOptions = computed(() => [
  { value: '', label: t('admin.channels.allPlatforms') },
  ...platformOrder.map((p) => ({
    value: p,
    label: t('admin.groups.platforms.' + p, p),
  })),
])

const statusEditOptions = computed(() => [
  { value: 'active', label: t('admin.channels.statusActive') },
  { value: 'disabled', label: t('admin.channels.statusDisabled') },
])

const selectedGroup = computed(
  () => groups.value.find((g) => g.id === selectedGroupId.value) || null,
)

const filteredGroups = computed(() => {
  const q = groupSearch.value.trim().toLowerCase()
  return groups.value.filter((g) => {
    if (platformFilter.value && g.platform !== platformFilter.value) return false
    if (!q) return true
    const hay = `${g.name} ${g.platform} ${g.sell_price_source?.policy_name || ''}`.toLowerCase()
    return hay.includes(q)
  })
})

const defaultPolicyName = computed(() => {
  const g = selectedGroup.value
  if (!g) return t('admin.channels.form.namePlaceholder')
  return t('admin.channels.defaultPolicyName', { group: g.name })
})

const sourceLabel = computed(() => {
  if (!summary.value) return t('admin.groups.pricing.sourceOfficial')
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

const sharedGroupNames = computed(() => {
  if (!boundPolicy.value || !selectedGroupId.value) return [] as string[]
  const ids = (boundPolicy.value.group_ids || []).filter((id) => id !== selectedGroupId.value)
  return ids
    .map((id) => groups.value.find((g) => g.id === id)?.name || `#${id}`)
    .slice(0, 8)
})

const canSave = computed(() => {
  if (!selectedGroup.value || panelLoading.value) return false
  // New policy needs at least one pricing row; existing can update meta/pricing (incl. clear rows → official-like)
  if (!boundPolicy.value && form.model_pricing.length === 0) return false
  return true
})

function formatRate(n: number | null | undefined) {
  if (n == null || Number.isNaN(n)) return '1'
  return Number.isInteger(n) ? String(n) : String(n)
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

function sourceChipLabel(src?: GroupSellPriceSource | null) {
  if (!src) return t('admin.groups.pricing.sourceOfficial')
  if (src.effective) return src.policy_name || t('admin.groups.pricing.sourcePolicy')
  if (src.policy_id) return t('admin.groups.pricing.sourcePolicyInactive')
  return t('admin.groups.pricing.sourceOfficial')
}

function sourceChipClass(src?: GroupSellPriceSource | null) {
  if (src?.effective) {
    return 'bg-violet-100 text-violet-800 dark:bg-violet-950/40 dark:text-violet-200'
  }
  if (src?.policy_id) {
    return 'bg-amber-100 text-amber-800 dark:bg-amber-950/40 dark:text-amber-200'
  }
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
}

function pricingToForm(channel: Channel | null, platform: GroupPlatform): PricingFormEntry[] {
  if (!channel) return []
  return (channel.model_pricing || [])
    .filter((entry) => !entry.platform || entry.platform === platform)
    .map((entry) => ({
      models: [...(entry.models || [])],
      billing_mode: (entry.billing_mode || 'token') as PricingFormEntry['billing_mode'],
      input_price: perTokenToMTok(entry.input_price),
      output_price: perTokenToMTok(entry.output_price),
      cache_write_price: perTokenToMTok(entry.cache_write_price),
      cache_read_price: perTokenToMTok(entry.cache_read_price),
      image_output_price: perTokenToMTok(entry.image_output_price),
      per_request_price: entry.per_request_price,
      intervals: (entry.intervals || []).map((iv, idx) => ({
        min_tokens: iv.min_tokens,
        max_tokens: iv.max_tokens,
        tier_label: iv.tier_label || '',
        input_price: perTokenToMTok(iv.input_price),
        output_price: perTokenToMTok(iv.output_price),
        cache_write_price: perTokenToMTok(iv.cache_write_price),
        cache_read_price: perTokenToMTok(iv.cache_read_price),
        per_request_price: iv.per_request_price,
        sort_order: iv.sort_order ?? idx,
      })),
    }))
}

function formToModelPricing(platform: GroupPlatform): ChannelModelPricing[] {
  const out: ChannelModelPricing[] = []
  for (const entry of form.model_pricing) {
    if (entry.models.length === 0) continue
    out.push({
      platform,
      models: entry.models,
      billing_mode: entry.billing_mode,
      input_price: mTokToPerToken(entry.input_price),
      output_price: mTokToPerToken(entry.output_price),
      cache_write_price: mTokToPerToken(entry.cache_write_price),
      cache_read_price: mTokToPerToken(entry.cache_read_price),
      image_output_price: mTokToPerToken(entry.image_output_price),
      per_request_price:
        entry.per_request_price != null && entry.per_request_price !== ''
          ? Number(entry.per_request_price)
          : null,
      intervals: formIntervalsToAPI(entry.intervals || []),
    })
  }
  return out
}

function resetForm() {
  form.name = ''
  form.description = ''
  form.status = 'active'
  form.model_pricing = []
}

function applyPolicyToForm(channel: Channel | null, group: AdminGroup) {
  if (channel) {
    form.name = channel.name
    form.description = channel.description || ''
    form.status = channel.status || 'active'
    form.model_pricing = pricingToForm(channel, group.platform as GroupPlatform)
  } else {
    form.name = t('admin.channels.defaultPolicyName', { group: group.name })
    form.description = ''
    form.status = 'active'
    form.model_pricing = []
  }
}

async function reloadGroups() {
  groupsLoading.value = true
  try {
    groups.value = await adminAPI.groups.getAllIncludingInactive()
  } catch (error) {
    appStore.showError(
      extractApiErrorMessage(error, t('admin.channels.loadGroupsError')),
    )
  } finally {
    groupsLoading.value = false
  }
}

async function loadPanel(groupId: number) {
  panelLoading.value = true
  summary.value = null
  boundPolicy.value = null
  resetForm()
  try {
    const group = groups.value.find((g) => g.id === groupId)
    if (!group) return

    const data = await adminAPI.groups.getPricingSummary(groupId)
    summary.value = data

    if (data.policy_id) {
      try {
        const policy = await adminAPI.channels.getById(data.policy_id)
        boundPolicy.value = policy
        applyPolicyToForm(policy, group)
      } catch {
        // Policy id present but fetch failed — still allow create-like edit
        applyPolicyToForm(null, group)
        if (data.policy_name) form.name = data.policy_name
      }
    } else {
      applyPolicyToForm(null, group)
    }
  } catch (error) {
    appStore.showError(
      extractApiErrorMessage(error, t('admin.groups.pricing.loadError')),
    )
  } finally {
    panelLoading.value = false
  }
}

async function selectGroup(id: number, opts?: { syncQuery?: boolean }) {
  if (selectedGroupId.value === id) {
    if (opts?.syncQuery !== false) {
      await router.replace({ query: { ...route.query, group: String(id) } })
    }
    return
  }
  selectedGroupId.value = id
  if (opts?.syncQuery !== false) {
    await router.replace({ query: { ...route.query, group: String(id) } })
  }
  await loadPanel(id)
}

async function reloadSelected() {
  if (!selectedGroupId.value) return
  await Promise.all([reloadGroups(), loadPanel(selectedGroupId.value)])
}

function addPricingEntry() {
  form.model_pricing.push({
    models: [],
    billing_mode: 'token',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: [],
  })
}

function updatePricingEntry(idx: number, updated: PricingFormEntry) {
  form.model_pricing.splice(idx, 1, updated)
}

function removePricingEntry(idx: number) {
  form.model_pricing.splice(idx, 1)
}

async function syncLatestModels() {
  const group = selectedGroup.value
  if (!group || syncing.value) return
  syncing.value = true
  try {
    const result = await adminAPI.channels.syncPricingModels(group.platform)
    const existing = new Set<string>()
    for (const entry of form.model_pricing) {
      for (const m of entry.models) existing.add(m)
    }
    const newModels = result.models.filter((m) => !existing.has(m))
    if (newModels.length === 0) {
      appStore.showSuccess(t('admin.channels.form.syncModelsAlreadyUpToDate'))
      return
    }

    let filled = 0
    try {
      const batch = await adminAPI.channels.batchGetModelDefaultPricing(newModels)
      for (const model of newModels) {
        const official = batch.items?.[model]
        const entry: PricingFormEntry = {
          models: [model],
          billing_mode: 'token',
          input_price: null,
          output_price: null,
          cache_write_price: null,
          cache_read_price: null,
          image_output_price: null,
          per_request_price: null,
          intervals: [],
        }
        if (official?.found) {
          entry.input_price = perTokenToMTok(official.input_price ?? null)
          entry.output_price = perTokenToMTok(official.output_price ?? null)
          entry.cache_write_price = perTokenToMTok(official.cache_write_price ?? null)
          entry.cache_read_price = perTokenToMTok(official.cache_read_price ?? null)
          entry.image_output_price = perTokenToMTok(official.image_output_price ?? null)
          filled += 1
        }
        form.model_pricing.push(entry)
      }
    } catch {
      for (const model of newModels) {
        form.model_pricing.push({
          models: [model],
          billing_mode: 'token',
          input_price: null,
          output_price: null,
          cache_write_price: null,
          cache_read_price: null,
          image_output_price: null,
          per_request_price: null,
          intervals: [],
        })
      }
    }

    appStore.showSuccess(
      t('admin.channels.form.syncModelsFilledSuccess', {
        count: newModels.length,
        filled,
      }),
    )
  } catch (error) {
    appStore.showError(
      extractApiErrorMessage(error, t('admin.channels.form.syncModelsError')),
    )
  } finally {
    syncing.value = false
  }
}

function validatePricing(): string | null {
  for (const entry of form.model_pricing) {
    if (entry.models.length === 0) {
      return t('admin.channels.emptyModelsInPricing', {
        platform: selectedGroup.value?.platform || '',
      })
    }
  }

  const allModels: string[] = []
  for (const entry of form.model_pricing) {
    allModels.push(...entry.models)
  }
  const conflict = findModelConflict(allModels)
  if (conflict) {
    return t('admin.channels.modelConflict', {
      model1: conflict[0],
      model2: conflict[1],
    })
  }

  for (const entry of form.model_pricing) {
    if (
      (entry.billing_mode === 'per_request' || entry.billing_mode === 'image') &&
      (entry.per_request_price == null || entry.per_request_price === '') &&
      (!entry.intervals || entry.intervals.length === 0)
    ) {
      return t('admin.channels.form.perRequestPriceRequired')
    }
    if (entry.intervals?.length) {
      const intervalErr = validateIntervals(entry.intervals, entry.billing_mode, t)
      if (intervalErr) {
        const modelLabel = entry.models.join(', ') || t('admin.channels.form.unnamed')
        return `${modelLabel}: ${intervalErr}`
      }
    }
  }
  return null
}

async function handleSave() {
  const group = selectedGroup.value
  if (!group || saving.value) return

  const name = form.name.trim() || defaultPolicyName.value
  if (!name) {
    appStore.showError(t('admin.channels.nameRequired'))
    return
  }

  // Creating new policy requires at least one pricing row
  if (!boundPolicy.value && form.model_pricing.length === 0) {
    appStore.showError(t('admin.channels.needPricingToCreate'))
    return
  }

  const err = validatePricing()
  if (err) {
    appStore.showError(err)
    return
  }

  const model_pricing = formToModelPricing(group.platform as GroupPlatform)
  // Keep other platforms' pricing if editing a shared multi-platform policy
  let mergedPricing = model_pricing
  if (boundPolicy.value) {
    const otherPlatformPricing = (boundPolicy.value.model_pricing || []).filter(
      (p) => p.platform && p.platform !== group.platform,
    )
    mergedPricing = [...otherPlatformPricing, ...model_pricing]
  }

  // group_ids: ensure current group is bound; keep other bound groups
  const groupIds = new Set<number>(boundPolicy.value?.group_ids || [])
  groupIds.add(group.id)

  saving.value = true
  try {
    if (boundPolicy.value) {
      const req: UpdateChannelRequest = {
        name,
        description: form.description.trim() || undefined,
        status: form.status,
        group_ids: Array.from(groupIds),
        model_pricing: mergedPricing,
        model_mapping: {},
        billing_model_source: 'requested',
        restrict_models: false,
        features_config: {},
      }
      await adminAPI.channels.update(boundPolicy.value.id, req)
      appStore.showSuccess(t('admin.channels.updateSuccess'))
    } else {
      const req: CreateChannelRequest = {
        name,
        description: form.description.trim() || undefined,
        group_ids: [group.id],
        model_pricing: mergedPricing,
        model_mapping: {},
        billing_model_source: 'requested',
        restrict_models: false,
        features_config: {},
      }
      await adminAPI.channels.create(req)
      appStore.showSuccess(t('admin.channels.createSuccess'))
    }
    await reloadSelected()
  } catch (error) {
    appStore.showError(
      extractApiErrorMessage(
        error,
        boundPolicy.value
          ? t('admin.channels.updateError')
          : t('admin.channels.createError'),
      ),
    )
  } finally {
    saving.value = false
  }
}

async function unbindOfficial() {
  const group = selectedGroup.value
  if (!group || saving.value) return
  saving.value = true
  try {
    await adminAPI.groups.bindSellPricePolicy(group.id, null)
    appStore.showSuccess(t('admin.groups.pricing.bindSuccess'))
    await reloadSelected()
  } catch (error) {
    appStore.showError(
      extractApiErrorMessage(error, t('admin.groups.pricing.bindError')),
    )
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  await reloadGroups()
  if (!groups.value.length) return

  const q = route.query.group
  const fromQuery = typeof q === 'string' ? Number(q) : NaN
  if (Number.isFinite(fromQuery) && groups.value.some((g) => g.id === fromQuery)) {
    await selectGroup(fromQuery)
    return
  }

  // Prefer a group that already has an effective policy
  const preferred =
    groups.value.find((g) => g.sell_price_source?.effective) || groups.value[0]
  await selectGroup(preferred.id)
})
</script>
