<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <!-- Left: Search + Filters -->
          <div class="grid w-full grid-cols-2 items-center gap-3 sm:flex sm:flex-1 sm:flex-wrap">
            <div class="relative min-w-0 sm:flex-1 sm:max-w-64">
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
              />
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('admin.channels.searchChannels', 'Search channels...')"
                class="input pl-10"
                @input="handleSearch"
              />
            </div>

            <Select
              v-model="filters.status"
              :options="statusFilterOptions"
              :placeholder="t('admin.channels.allStatus', 'All Status')"
              class="min-w-0 w-full sm:w-40"
              @change="loadChannels"
            />
          </div>

          <!-- Right: Actions -->
          <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
            <button
              @click="loadChannels"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh', 'Refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button @click="openCreateDialog" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.channels.createChannel', 'Create Channel') }}
            </button>
          </div>
        </div>
        <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.channels.libraryHint') }}
        </p>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="channels"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-name="{ row, value }">
            <div class="min-w-0 space-y-1">
              <div class="flex flex-wrap items-center gap-1.5">
                <span class="font-medium text-gray-900 dark:text-white">{{ value }}</span>
                <span
                  v-if="!row.group_ids?.length"
                  class="rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-semibold text-amber-800 dark:bg-amber-950/50 dark:text-amber-200"
                  :title="t('admin.channels.unboundWarning')"
                >{{ t('admin.channels.unboundBadge') }}</span>
              </div>
              <p
                v-if="!row.group_ids?.length"
                class="text-[11px] text-amber-700/90 dark:text-amber-300/80"
              >{{ t('admin.channels.unboundWarning') }}</p>
            </div>
          </template>

          <template #cell-description="{ value }">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{ value || '-' }}</span>
          </template>

          <template #cell-status="{ row }">
            <Toggle
              :modelValue="row.status === 'active'"
              @update:modelValue="toggleChannelStatus(row)"
            />
          </template>

          <template #cell-group_count="{ row }">
            <div class="flex max-w-[14rem] flex-wrap gap-1">
              <span
                v-for="g in getChannelGroupSummaries(row).slice(0, 3)"
                :key="g.id"
                class="inline-flex items-center gap-1 rounded bg-gray-100 px-1.5 py-0.5 text-[11px] font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300"
                :title="`${g.name} · ${g.rate}x`"
              >
                <span class="max-w-[5.5rem] truncate">{{ g.name }}</span>
                <span class="text-[10px] text-primary-600 dark:text-primary-300">{{ g.rate }}x</span>
              </span>
              <span
                v-if="getChannelGroupSummaries(row).length > 3"
                class="text-[11px] text-gray-400"
              >+{{ getChannelGroupSummaries(row).length - 3 }}</span>
              <span
                v-if="getChannelGroupSummaries(row).length === 0"
                class="inline-flex items-center rounded bg-amber-100 px-1.5 py-0.5 text-[11px] font-medium text-amber-800 dark:bg-amber-950/50 dark:text-amber-200"
                :title="t('admin.channels.unboundWarning')"
              >{{ t('admin.channels.unboundBadge') }}</span>
            </div>
          </template>

          <template #cell-pricing_count="{ row }">
            <div class="min-w-[12rem] space-y-1" :title="t('admin.channels.pricingSummaryHint')">
              <div class="flex flex-wrap gap-1">
                <span
                  v-for="m in getChannelPricingSummary(row).models.slice(0, 3)"
                  :key="m"
                  class="inline-flex rounded bg-sky-50 px-1.5 py-0.5 text-[11px] text-sky-800 dark:bg-sky-950/40 dark:text-sky-200"
                >{{ m }}</span>
                <span
                  v-if="getChannelPricingSummary(row).models.length > 3"
                  class="text-[11px] text-gray-400"
                >+{{ getChannelPricingSummary(row).models.length - 3 }}</span>
              </div>
              <div class="flex flex-wrap items-center gap-1.5 text-[11px]">
                <span class="text-gray-600 dark:text-gray-300">
                  {{ getChannelPricingSummary(row).sampleSummary || t('admin.channels.form.fallbackOfficialShort') }}
                </span>
                <span class="text-gray-400">·</span>
                <span class="text-gray-500">{{ getChannelPricingSummary(row).ruleCount }} {{ t('admin.channels.pricingUnit') }}</span>
                <span
                  v-if="getChannelPricingSummary(row).emptyRuleCount > 0"
                  class="rounded bg-amber-100 px-1 py-0.5 font-medium text-amber-800 dark:bg-amber-950/50 dark:text-amber-200"
                >{{ t('admin.channels.emptyPriceBadge', { count: getChannelPricingSummary(row).emptyRuleCount }) }}</span>
              </div>
            </div>
          </template>

          <template #cell-created_at="{ value }">
            <span class="text-sm text-gray-600 dark:text-gray-400">
              {{ formatDate(value) }}
            </span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button
                @click="openEditDialog(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              >
                <Icon name="edit" size="sm" />
                <span class="text-xs">{{ t('common.edit', 'Edit') }}</span>
              </button>
              <button
                @click="handleDelete(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              >
                <Icon name="trash" size="sm" />
                <span class="text-xs">{{ t('common.delete', 'Delete') }}</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.channels.noChannelsYet', 'No Channels Yet')"
              :description="t('admin.channels.createFirstChannel', 'Create your first channel to manage model pricing')"
              :action-text="t('admin.channels.createChannel', 'Create Channel')"
              @action="openCreateDialog"
            />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <!-- Create/Edit Dialog -->
    <BaseDialog
      :show="showDialog"
      :title="editingChannel ? t('admin.channels.editChannel', 'Edit Channel') : t('admin.channels.createChannel', 'Create Channel')"
      width="extra-wide"
      @close="closeDialog"
    >
      <div class="channel-dialog-body">
        <!-- Tab Bar -->
        <div class="flex items-center border-b border-gray-200 dark:border-dark-700 flex-shrink-0 -mx-4 sm:-mx-6 px-4 sm:px-6 -mt-3 sm:-mt-4 overflow-x-auto">
          <button
            v-for="tab in editorTabs"
            :key="tab.id"
            type="button"
            @click="activeTab = tab.id"
            class="channel-tab whitespace-nowrap"
            :class="activeTab === tab.id ? 'channel-tab-active' : 'channel-tab-inactive'"
          >
            {{ tab.label }}
          </button>
        </div>

        <!-- Tab Content -->
        <!-- novalidate：多 Tab v-show 下隐藏的 number/required 会触发浏览器原生校验且无法聚焦，导致「更新」点了没反应 -->
        <form id="channel-form" novalidate @submit.prevent="handleSubmit" class="flex-1 overflow-y-auto pt-4">
          <!-- Basic -->
          <div v-show="activeTab === 'basic'" class="space-y-5">
            <div>
              <label class="input-label">{{ t('admin.channels.form.name', 'Name') }} <span class="text-red-500">*</span></label>
              <input v-model="form.name" type="text" class="input" :placeholder="t('admin.channels.form.namePlaceholder', 'Enter channel name')" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.channels.form.description', 'Description') }}</label>
              <textarea v-model="form.description" rows="2" class="input" :placeholder="t('admin.channels.form.descriptionPlaceholder', 'Optional description')"></textarea>
            </div>
            <div v-if="editingChannel">
              <label class="input-label">{{ t('admin.channels.form.status', 'Status') }}</label>
              <Select v-model="form.status" :options="statusEditOptions" />
            </div>
            <div class="space-y-3">
              <label class="input-label mb-0">{{ t('admin.channels.form.platformConfig') }}</label>
              <div class="flex flex-wrap gap-2">
                <label
                  v-for="p in platformOrder"
                  :key="p"
                  class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm transition-colors"
                  :class="activePlatforms.includes(p)
                    ? 'bg-primary-50 border-primary-300 dark:bg-primary-900/20 dark:border-primary-700'
                    : 'border-gray-200 hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700'"
                >
                  <input type="checkbox" :checked="activePlatforms.includes(p)" class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500" @change="togglePlatform(p)" />
                  <PlatformIcon :platform="p" size="xs" :class="platformTextClass(p)" />
                  <span :class="platformTextClass(p)">{{ t('admin.groups.platforms.' + p, p) }}</span>
                </label>
              </div>
            </div>

            <div v-for="(section, sIdx) in form.platforms" :key="'groups-' + section.platform" v-show="section.enabled" class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
              <div class="mb-2 flex items-center gap-2 text-sm font-medium">
                <PlatformIcon :platform="section.platform" size="xs" :class="platformTextClass(section.platform)" />
                <span :class="platformTextClass(section.platform)">{{ t('admin.groups.platforms.' + section.platform, section.platform) }}</span>
                <span class="text-xs font-normal text-gray-400">{{ t('admin.channels.form.groups') }}</span>
              </div>
              <div class="max-h-40 overflow-auto rounded-lg border border-gray-200 bg-gray-50 p-2 dark:border-dark-600 dark:bg-dark-900">
                <div v-if="groupsLoading" class="py-2 text-center text-xs text-gray-500">{{ t('common.loading', 'Loading...') }}</div>
                <div v-else-if="getGroupsForPlatform(section.platform).length === 0" class="py-2 text-center text-xs text-gray-500">{{ t('admin.channels.form.noGroupsAvailable', 'No groups available') }}</div>
                <div v-else class="flex flex-wrap gap-1">
                  <label
                    v-for="group in getGroupsForPlatform(section.platform)"
                    :key="group.id"
                    class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-gray-200 px-2 py-1 text-xs transition-colors hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700"
                    :class="[
                      section.group_ids.includes(group.id) ? 'bg-primary-50 border-primary-300 dark:bg-primary-900/20 dark:border-primary-700' : '',
                      isGroupInOtherChannel(group.id, section.platform) ? 'opacity-40' : ''
                    ]"
                  >
                    <input type="checkbox" :checked="section.group_ids.includes(group.id)" :disabled="isGroupInOtherChannel(group.id, section.platform)" class="h-3 w-3 rounded border-gray-300 text-primary-600 focus:ring-primary-500" @change="toggleGroupInSection(sIdx, group.id)" />
                    <span :class="['font-medium', platformTextClass(group.platform)]">{{ group.name }}</span>
                    <span :class="['rounded-full px-1 py-0 text-[10px]', platformBadgeLightClass(group.platform)]">{{ group.rate_multiplier }}x</span>
                    <span class="text-[10px] text-gray-400">{{ group.account_count || 0 }}</span>
                    <span v-if="isGroupInOtherChannel(group.id, section.platform)" class="text-[10px] text-gray-400">{{ getGroupInOtherChannelLabel(group.id) }}</span>
                  </label>
                </div>
              </div>
            </div>
          </div>

          <!-- Sell pricing -->
          <div v-show="activeTab === 'sell'" class="space-y-4">
            <div class="rounded-lg border border-primary-200 bg-primary-50/50 px-3 py-2.5 text-xs leading-5 text-primary-900 dark:border-primary-900/40 dark:bg-primary-950/20 dark:text-primary-100">
              <div class="font-semibold">{{ t('admin.channels.form.billingFlowTitle') }}</div>
              <p class="mt-1">{{ t('admin.channels.form.billingFlowDesc') }}</p>
              <p class="mt-1 text-[11px] opacity-80">{{ t('admin.channels.form.billingFlowCostNote') }}</p>
            </div>

            <div v-if="enabledPlatformSections.length === 0" class="rounded border border-dashed border-gray-300 p-4 text-center text-xs text-gray-400 dark:border-dark-500">
              {{ t('admin.channels.form.enablePlatformFirst') }}
            </div>

            <div v-for="(section, sIdx) in form.platforms" :key="'sell-' + section.platform" v-show="section.enabled" class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600">
              <div class="flex flex-wrap items-center justify-between gap-2">
                <div class="flex items-center gap-2 text-sm font-medium">
                  <PlatformIcon :platform="section.platform" size="xs" :class="platformTextClass(section.platform)" />
                  <span :class="platformTextClass(section.platform)">{{ t('admin.groups.platforms.' + section.platform, section.platform) }}</span>
                  <span class="text-xs font-normal text-gray-400">{{ t('admin.channels.form.modelPricing', 'Model Pricing') }}</span>
                </div>
                <div class="flex items-center gap-2">
                  <button type="button" @click="syncLatestModels(sIdx)" :disabled="syncingPlatform === section.platform" class="text-xs text-gray-500 hover:text-primary-600 disabled:opacity-50">
                    {{ syncingPlatform === section.platform ? t('admin.channels.form.syncingModels') : t('admin.channels.form.syncLatestModels') }}
                  </button>
                  <button type="button" @click="addPricingEntry(sIdx)" class="text-xs text-primary-600 hover:text-primary-700">+ {{ t('common.add', 'Add') }}</button>
                </div>
              </div>

              <div v-if="section.group_ids.length" class="flex flex-wrap gap-1 text-[11px] text-gray-500">
                <span>{{ t('admin.channels.form.linkedGroupRates') }}:</span>
                <span v-for="gid in section.group_ids" :key="gid" class="rounded bg-gray-100 px-1.5 py-0.5 dark:bg-dark-700">
                  {{ getGroupNameById(gid) }} {{ getGroupRateById(gid) }}x
                </span>
              </div>

              <div v-if="section.model_pricing.length === 0" class="rounded border border-dashed border-gray-300 p-2 text-center text-xs text-gray-400 dark:border-dark-500">
                {{ t('admin.channels.form.noPricingRules', 'No pricing rules yet. Click "Add" to create one.') }}
              </div>
              <div v-else class="space-y-2">
                <PricingEntryCard
                  v-for="(entry, idx) in section.model_pricing"
                  :key="idx"
                  :entry="entry"
                  :platform="section.platform"
                  @update="updatePricingEntry(sIdx, idx, $event)"
                  @remove="removePricingEntry(sIdx, idx)"
                />
              </div>
            </div>
          </div>

        </form>
      </div>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeDialog" type="button" class="btn btn-secondary">
            {{ t('common.cancel', 'Cancel') }}
          </button>
          <button
            type="submit"
            form="channel-form"
            :disabled="submitting"
            class="btn btn-primary"
          >
            {{ submitting
              ? t('common.submitting', 'Submitting...')
              : editingChannel
                ? t('common.update', 'Update')
                : t('common.create', 'Create')
            }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Delete Confirmation -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.channels.deleteChannel', 'Delete Channel')"
      :message="deleteConfirmMessage"
      :confirm-text="t('common.delete', 'Delete')"
      :cancel-text="t('common.cancel', 'Cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type { Channel, ChannelModelPricing, CreateChannelRequest, UpdateChannelRequest } from '@/api/admin/channels'
import type { PricingFormEntry } from '@/components/admin/channel/types'
import { mTokToPerToken, perTokenToMTok, formIntervalsToAPI, findModelConflict, validateIntervals } from '@/components/admin/channel/types'
import { summarizeChannelPricing } from '@/components/admin/channel/channelPricingTools'
import type { AdminGroup, GroupPlatform } from '@/types'
import type { Column } from '@/components/common/types'
import { platformTextClass, platformBadgeLightClass } from '@/utils/platformColors'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Toggle from '@/components/common/Toggle.vue'
import PricingEntryCard from '@/components/admin/channel/PricingEntryCard.vue'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'

const { t } = useI18n()
const appStore = useAppStore()

// Web Search global enabled state (loaded once on mount)
const webSearchGlobalEnabled = ref(false)
async function loadWebSearchGlobalState() {
  try {
    const cfg = await adminAPI.settings.getWebSearchEmulationConfig()
    webSearchGlobalEnabled.value = cfg?.enabled === true && (cfg?.providers?.length ?? 0) > 0
  } catch (err: unknown) {
    console.warn('Failed to load web search global state:', err)
    webSearchGlobalEnabled.value = false
  }
}

// ── Platform Section type ──
interface PlatformSection {
  platform: GroupPlatform
  enabled: boolean
  collapsed: boolean
  group_ids: number[]
  model_mapping: Record<string, string>
  model_pricing: PricingFormEntry[]
  web_search_emulation: boolean
  codex_image_generation_bridge: boolean
  bedrock_cc_compat: boolean
}

// ── Table columns ──
const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.channels.columns.name', 'Name'), sortable: true },
  { key: 'description', label: t('admin.channels.columns.description', 'Description'), sortable: false },
  { key: 'status', label: t('admin.channels.columns.status', 'Status'), sortable: true },
  { key: 'group_count', label: t('admin.channels.columns.groups', 'Groups'), sortable: false },
  { key: 'pricing_count', label: t('admin.channels.columns.pricing', 'Pricing'), sortable: false },
  { key: 'created_at', label: t('admin.channels.columns.createdAt', 'Created'), sortable: true },
  { key: 'actions', label: t('admin.channels.columns.actions', 'Actions'), sortable: false }
])

const editorTabs = computed(() => [
  { id: 'basic', label: t('admin.channels.form.basicSettings') },
  { id: 'sell', label: t('admin.channels.form.sellPricingTab') },
])

const enabledPlatformSections = computed(() => form.platforms.filter((s) => s.enabled))

function getChannelPricingSummary(row: Channel) {
  return summarizeChannelPricing(row.model_pricing, t('admin.channels.form.fallbackOfficialShort'))
}

function getChannelGroupSummaries(row: Channel): Array<{ id: number; name: string; rate: number }> {
  return (row.group_ids || []).map((id) => {
    const g = allGroups.value.find((item) => item.id === id)
    return {
      id,
      name: g?.name || `#${id}`,
      rate: typeof g?.rate_multiplier === 'number' ? g.rate_multiplier : 1,
    }
  })
}

function getGroupNameById(groupId: number): string {
  const group = allGroups.value.find(g => g.id === groupId)
  return group ? group.name : `#${groupId}`
}

function getGroupRateById(groupId: number): number {
  const g = allGroups.value.find((item) => item.id === groupId)
  return typeof g?.rate_multiplier === 'number' ? g.rate_multiplier : 1
}

const statusFilterOptions = computed(() => [
  { value: '', label: t('admin.channels.allStatus', 'All Status') },
  { value: 'active', label: t('admin.channels.statusActive', 'Active') },
  { value: 'disabled', label: t('admin.channels.statusDisabled', 'Disabled') }
])

const statusEditOptions = computed(() => [
  { value: 'active', label: t('admin.channels.statusActive', 'Active') },
  { value: 'disabled', label: t('admin.channels.statusDisabled', 'Disabled') }
])


// ── State ──
const channels = ref<Channel[]>([])
const loading = ref(false)
const searchQuery = ref('')
const filters = reactive({ status: '' })
const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0
})
const sortState = reactive({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc'
})

// Dialog state
const showDialog = ref(false)
const editingChannel = ref<Channel | null>(null)
const submitting = ref(false)
const showDeleteDialog = ref(false)
const deletingChannel = ref<Channel | null>(null)
const activeTab = ref<string>('basic')

// Groups
const allGroups = ref<AdminGroup[]>([])
const groupsLoading = ref(false)

// All channels for group-conflict detection (independent of current page)
const allChannelsForConflict = ref<Channel[]>([])

// Form data
const form = reactive({
  name: '',
  description: '',
  status: 'active',
  restrict_models: false,
  billing_model_source: 'channel_mapped' as string,
  platforms: [] as PlatformSection[],
})

let abortController: AbortController | null = null

// ── Platform config ──
const platformOrder: GroupPlatform[] = ['anthropic', 'openai', 'antigravity', 'grok']

// ── Helpers ──
function formatDate(value: string): string {
  if (!value) return '-'
  return new Date(value).toLocaleDateString()
}

// ── Platform section helpers ──
const activePlatforms = computed(() => form.platforms.filter(s => s.enabled).map(s => s.platform))

function addPlatformSection(platform: GroupPlatform) {
  form.platforms.push({
    platform,
    enabled: true,
    collapsed: false,
    group_ids: [],
    model_mapping: {},
    model_pricing: [],
    web_search_emulation: false,
    codex_image_generation_bridge: false,
    bedrock_cc_compat: false,
  })
}

function togglePlatform(platform: GroupPlatform) {
  const section = form.platforms.find(s => s.platform === platform)
  if (section) {
    section.enabled = !section.enabled
  } else {
    addPlatformSection(platform)
  }
}

function getGroupsForPlatform(platform: GroupPlatform): AdminGroup[] {
  return allGroups.value.filter(g => g.platform === platform)
}

// ── Group helpers ──
const groupToChannelMap = computed(() => {
  const map = new Map<number, Channel>()
  for (const ch of allChannelsForConflict.value) {
    if (editingChannel.value && ch.id === editingChannel.value.id) continue
    for (const gid of ch.group_ids || []) {
      map.set(gid, ch)
    }
  }
  return map
})

function isGroupInOtherChannel(groupId: number, _platform: string): boolean {
  return groupToChannelMap.value.has(groupId)
}

function getGroupChannelName(groupId: number): string {
  return groupToChannelMap.value.get(groupId)?.name || ''
}

function getGroupInOtherChannelLabel(groupId: number): string {
  const name = getGroupChannelName(groupId)
  return t('admin.channels.form.inOtherChannel', { name }, `In "${name}"`)
}

const deleteConfirmMessage = computed(() => {
  const name = deletingChannel.value?.name || ''
  return t(
    'admin.channels.deleteConfirm',
    { name },
    `Are you sure you want to delete channel "${name}"? This action cannot be undone.`
  )
})

function toggleGroupInSection(sectionIdx: number, groupId: number) {
  const section = form.platforms[sectionIdx]
  const idx = section.group_ids.indexOf(groupId)
  if (idx >= 0) {
    section.group_ids.splice(idx, 1)
  } else {
    section.group_ids.push(groupId)
  }
}

// ── Pricing helpers ──
function addPricingEntry(sectionIdx: number) {
  form.platforms[sectionIdx].model_pricing.push({
    models: [],
    billing_mode: 'token',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: []
  })
}

const syncingPlatform = ref<string | null>(null)

async function syncLatestModels(sectionIdx: number) {
  const platform = form.platforms[sectionIdx].platform
  if (syncingPlatform.value) return
  syncingPlatform.value = platform
  try {
    const result = await adminAPI.channels.syncPricingModels(platform)
    const existingModels = new Set<string>()
    for (const entry of form.platforms[sectionIdx].model_pricing) {
      for (const m of entry.models) existingModels.add(m)
    }
    const newModels = result.models.filter((m) => !existingModels.has(m))
    if (newModels.length === 0) {
      appStore.showSuccess(t('admin.channels.form.syncModelsAlreadyUpToDate'))
      return
    }

    // 默认：每个新模型单独一行，并尽量填官方价
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
        form.platforms[sectionIdx].model_pricing.push(entry)
      }
    } catch {
      for (const model of newModels) {
        form.platforms[sectionIdx].model_pricing.push({
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
    appStore.showError(extractApiErrorMessage(error, t('admin.channels.form.syncModelsError')))
  } finally {
    syncingPlatform.value = null
  }
}

function updatePricingEntry(sectionIdx: number, idx: number, updated: PricingFormEntry) {
  form.platforms[sectionIdx].model_pricing.splice(idx, 1, updated)
}

function removePricingEntry(sectionIdx: number, idx: number) {
  form.platforms[sectionIdx].model_pricing.splice(idx, 1)
}


// ── Form ↔ API conversion ──
function formToAPI(): { group_ids: number[], model_pricing: ChannelModelPricing[], model_mapping: Record<string, Record<string, string>>, features_config: Record<string, unknown> } {
  const group_ids: number[] = []
  const model_pricing: ChannelModelPricing[] = []
  // Product: sell-price policy only carries sell pricing + group binds.
  const model_mapping: Record<string, Record<string, string>> = {}
  const features_config: Record<string, unknown> = {}

  for (const section of form.platforms) {
    if (!section.enabled) continue
    group_ids.push(...section.group_ids)
    for (const entry of section.model_pricing) {
      if (entry.models.length === 0) continue
      model_pricing.push({
        platform: section.platform,
        models: entry.models,
        billing_mode: entry.billing_mode,
        input_price: mTokToPerToken(entry.input_price),
        output_price: mTokToPerToken(entry.output_price),
        cache_write_price: mTokToPerToken(entry.cache_write_price),
        cache_read_price: mTokToPerToken(entry.cache_read_price),
        image_output_price: mTokToPerToken(entry.image_output_price),
        per_request_price: entry.per_request_price != null && entry.per_request_price !== '' ? Number(entry.per_request_price) : null,
        intervals: formIntervalsToAPI(entry.intervals || [])
      })
    }
  }
  return { group_ids, model_pricing, model_mapping, features_config }
}


function apiToForm(channel: Channel): PlatformSection[] {
  // Collect platforms from pricing + bound groups only (ignore mapping/features).
  const platformSet = new Set<GroupPlatform>()
  for (const pricing of channel.model_pricing || []) {
    const p = pricing.platform as GroupPlatform
    if (p) platformSet.add(p)
  }
  for (const gid of channel.group_ids || []) {
    const g = allGroups.value.find((item) => item.id === gid)
    if (g?.platform) platformSet.add(g.platform as GroupPlatform)
  }
  const sections: PlatformSection[] = []
  for (const platform of platformOrder) {
    if (!platformSet.has(platform) && !(channel.group_ids || []).some((gid) => allGroups.value.find((g) => g.id === gid)?.platform === platform)) {
      continue
    }
    const group_ids = (channel.group_ids || []).filter((gid) => {
      const g = allGroups.value.find((item) => item.id === gid)
      return g?.platform === platform
    })
    const model_pricing: PricingFormEntry[] = (channel.model_pricing || [])
      .filter((entry) => (entry.platform || 'anthropic') === platform)
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
    sections.push({
      platform,
      enabled: true,
      collapsed: false,
      group_ids,
      model_mapping: {},
      model_pricing,
      web_search_emulation: false,
      codex_image_generation_bridge: false,
      bedrock_cc_compat: false,
    })
  }
  // If nothing inferred, leave empty platforms list
  return sections
}

// ── Load data ──
async function loadChannels() {
  if (abortController) abortController.abort()
  const ctrl = new AbortController()
  abortController = ctrl
  loading.value = true

  try {
    const response = await adminAPI.channels.list(pagination.page, pagination.page_size, {
      status: filters.status || undefined,
      search: searchQuery.value || undefined,
      sort_by: sortState.sort_by,
      sort_order: sortState.sort_order
    }, { signal: ctrl.signal })

    if (ctrl.signal.aborted || abortController !== ctrl) return
    channels.value = response.items || []
    pagination.total = response.total
  } catch (error: unknown) {
    const e = error as { name?: string; code?: string }
    if (e?.name === 'AbortError' || e?.code === 'ERR_CANCELED') return
    appStore.showError(extractApiErrorMessage(error, t('admin.channels.loadError', 'Failed to load channels')))
  } finally {
    if (abortController === ctrl) {
      loading.value = false
      abortController = null
    }
  }
}

async function loadGroups() {
  groupsLoading.value = true
  try {
    allGroups.value = await adminAPI.groups.getAll()
  } catch (error) {
    console.error('Error loading groups:', error)
  } finally {
    groupsLoading.value = false
  }
}

async function loadAllChannelsForConflict() {
  try {
    const response = await adminAPI.channels.list(1, 1000)
    allChannelsForConflict.value = response.items || []
  } catch (error) {
    // Fallback to current page data
    allChannelsForConflict.value = channels.value
  }
}

let searchTimeout: ReturnType<typeof setTimeout>
function handleSearch() {
  clearTimeout(searchTimeout)
  searchTimeout = setTimeout(() => {
    pagination.page = 1
    loadChannels()
  }, 300)
}

function handlePageChange(page: number) {
  pagination.page = page
  loadChannels()
}

function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  loadChannels()
}

function handleSort(key: string, order: 'asc' | 'desc') {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  loadChannels()
}

// ── Dialog ──
function resetForm() {
  form.name = ''
  form.description = ''
  form.status = 'active'
  form.restrict_models = false
  form.billing_model_source = 'channel_mapped'
  form.platforms = []
  activeTab.value = 'basic'
}

async function openCreateDialog() {
  editingChannel.value = null
  resetForm()
  await Promise.all([loadGroups(), loadAllChannelsForConflict()])
  showDialog.value = true
}

async function openEditDialog(channel: Channel) {
  editingChannel.value = channel
  form.name = channel.name
  form.description = channel.description || ''
  form.status = channel.status
  form.restrict_models = channel.restrict_models || false
  form.billing_model_source = channel.billing_model_source || 'channel_mapped'
  // Must load groups first so apiToForm can map groupID → platform
  await Promise.all([loadGroups(), loadAllChannelsForConflict()])
  form.platforms = apiToForm(channel)
  // Ignore legacy non-pricing policy capabilities in editor.
  form.restrict_models = false
  form.billing_model_source = 'requested'
  for (const section of form.platforms) {
    section.model_mapping = {}
    section.web_search_emulation = false
    section.codex_image_generation_bridge = false
    section.bedrock_cc_compat = false
  }
  showDialog.value = true
}

function closeDialog() {
  showDialog.value = false
  editingChannel.value = null
  resetForm()
}

async function handleSubmit() {
  if (submitting.value) return
  if (!form.name.trim()) {
    activeTab.value = 'basic'
    appStore.showError(t('admin.channels.nameRequired', 'Please enter a channel name'))
    return
  }

  // Check for pricing entries with empty models (would be silently skipped)
  for (const section of form.platforms.filter(s => s.enabled)) {
    if (section.group_ids.length === 0) {
      const platformLabel = t('admin.groups.platforms.' + section.platform, section.platform)
      appStore.showError(t('admin.channels.noGroupsSelected', { platform: platformLabel }))
      // Tab 重构后是 basic/sell/cost/advanced，不再是平台 id
      activeTab.value = 'basic'
      return
    }
    for (const entry of section.model_pricing) {
      if (entry.models.length === 0) {
        const platformLabel = t('admin.groups.platforms.' + section.platform, section.platform)
        appStore.showError(t('admin.channels.emptyModelsInPricing', { platform: platformLabel }))
        activeTab.value = 'sell'
        return
      }
    }
  }

  // Check model pattern conflicts per platform (duplicate / wildcard overlap)
  for (const section of form.platforms.filter(s => s.enabled)) {
    // Collect all pricing models for this platform
    const allModels: string[] = []
    for (const entry of section.model_pricing) {
      allModels.push(...entry.models)
    }
    const pricingConflict = findModelConflict(allModels)
    if (pricingConflict) {
      appStore.showError(
        t('admin.channels.modelConflict',
          { model1: pricingConflict[0], model2: pricingConflict[1] })
      )
      activeTab.value = 'sell'
      return
    }
    // Check model mapping source pattern conflicts
    const mappingKeys = Object.keys(section.model_mapping)
    if (mappingKeys.length > 0) {
      const mappingConflict = findModelConflict(mappingKeys)
      if (mappingConflict) {
        appStore.showError(
          t('admin.channels.mappingConflict',
            { model1: mappingConflict[0], model2: mappingConflict[1] })
        )
        activeTab.value = 'sell'
        return
      }
    }
  }

  // 校验 per_request/image 模式必须有价格 (只校验启用的平台)
  for (const section of form.platforms.filter(s => s.enabled)) {
    for (const entry of section.model_pricing) {
      if (entry.models.length === 0) continue
      if ((entry.billing_mode === 'per_request' || entry.billing_mode === 'image') &&
          (entry.per_request_price == null || entry.per_request_price === '') &&
          (!entry.intervals || entry.intervals.length === 0)) {
        appStore.showError(t('admin.channels.form.perRequestPriceRequired'))
        activeTab.value = 'sell'
        return
      }
    }
  }

  // 校验区间合法性（范围、重叠等）
  for (const section of form.platforms.filter(s => s.enabled)) {
    for (const entry of section.model_pricing) {
      if (!entry.intervals || entry.intervals.length === 0) continue
      const intervalErr = validateIntervals(entry.intervals, entry.billing_mode, t)
      if (intervalErr) {
        const platformLabel = t('admin.groups.platforms.' + section.platform, section.platform)
        const modelLabel = entry.models.join(', ') || t('admin.channels.form.unnamed')
        appStore.showError(`${platformLabel} - ${modelLabel}: ${intervalErr}`)
        activeTab.value = 'sell'
        return
      }
    }
  }

  const { group_ids, model_pricing } = formToAPI()

  submitting.value = true
  try {
    if (editingChannel.value) {
      const req: UpdateChannelRequest = {
        name: form.name.trim(),
        description: form.description.trim() || undefined,
        status: form.status,
        group_ids,
        model_pricing,
        model_mapping: {},
        billing_model_source: 'requested',
        restrict_models: false,
        features_config: {},
      }
      await adminAPI.channels.update(editingChannel.value.id, req)
      appStore.showSuccess(t('admin.channels.updateSuccess', 'Channel updated'))
    } else {
      const req: CreateChannelRequest = {
        name: form.name.trim(),
        description: form.description.trim() || undefined,
        group_ids,
        model_pricing,
        model_mapping: {},
        billing_model_source: 'requested',
        restrict_models: false,
        features_config: {},
      }
      await adminAPI.channels.create(req)
      appStore.showSuccess(t('admin.channels.createSuccess', 'Channel created'))
    }
    closeDialog()
    loadChannels()
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, editingChannel.value
      ? t('admin.channels.updateError', 'Failed to update channel')
      : t('admin.channels.createError', 'Failed to create channel')))
  } finally {
    submitting.value = false
  }
}

// ── Toggle status ──
async function toggleChannelStatus(channel: Channel) {
  const newStatus = channel.status === 'active' ? 'disabled' : 'active'
  try {
    await adminAPI.channels.update(channel.id, { status: newStatus })
    if (filters.status && filters.status !== newStatus) {
      // Item no longer matches the active filter — reload list
      await loadChannels()
    } else {
      channel.status = newStatus
    }
  } catch (error) {
    appStore.showError(t('admin.channels.updateError', 'Failed to update channel'))
    console.error('Error toggling channel status:', error)
  }
}

// ── Delete ──
function handleDelete(channel: Channel) {
  deletingChannel.value = channel
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (!deletingChannel.value) return

  try {
    await adminAPI.channels.remove(deletingChannel.value.id)
    appStore.showSuccess(t('admin.channels.deleteSuccess', 'Channel deleted'))
    showDeleteDialog.value = false
    deletingChannel.value = null
    loadChannels()
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.channels.deleteError', 'Failed to delete channel')))
  }
}

// ── Lifecycle ──
onMounted(() => {
  loadChannels()
  loadGroups()
  loadWebSearchGlobalState()
})

onUnmounted(() => {
  clearTimeout(searchTimeout)
  abortController?.abort()
})
</script>

<style scoped>
.channel-dialog-body {
  display: flex;
  flex-direction: column;
  height: 70vh;
  min-height: 400px;
}

.channel-tab {
  @apply flex items-center gap-1.5 px-3 py-2.5 text-sm font-medium border-b-2 transition-colors whitespace-nowrap;
}

.channel-tab-active {
  @apply border-primary-600 text-primary-600 dark:border-primary-400 dark:text-primary-400;
}

.channel-tab-inactive {
  @apply border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 dark:text-gray-400 dark:hover:text-gray-300;
}
</style>
