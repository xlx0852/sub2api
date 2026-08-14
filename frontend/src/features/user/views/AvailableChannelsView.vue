<template>
  <AppLayout>
    <TablePageLayout compact transparent>
      <template #filters>
        <div class="space-y-3">
          <!-- Header stats -->
          <div class="grid grid-cols-2 overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800 sm:grid-cols-4">
            <div class="border-b border-r border-gray-100 px-4 py-3 dark:border-dark-700 sm:border-b-0">
              <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('availableChannels.stats.models') }}</div>
              <div class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ stats.models }}</div>
            </div>
            <div class="border-b border-gray-100 px-4 py-3 dark:border-dark-700 sm:border-b-0 sm:border-r">
              <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('availableChannels.stats.platforms') }}</div>
              <div class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ stats.platforms }}</div>
            </div>
            <div class="border-r border-gray-100 px-4 py-3 dark:border-dark-700">
              <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('availableChannels.stats.channels') }}</div>
              <div class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ stats.channels }}</div>
            </div>
            <div class="px-4 py-3">
              <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('availableChannels.stats.groups') }}</div>
              <div class="mt-1 text-xl font-semibold text-gray-900 dark:text-white">{{ stats.groups }}</div>
            </div>
          </div>

          <!-- Filters -->
          <div class="flex flex-col justify-between gap-3 rounded-xl border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800 lg:flex-row lg:items-center">
            <div class="flex flex-1 flex-wrap items-center gap-3">
              <div class="relative w-full sm:w-72 lg:w-80">
                <Icon
                  name="search"
                  size="md"
                  class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
                />
                <input
                  v-model="searchQuery"
                  type="text"
                  :placeholder="t('availableChannels.searchPlaceholder')"
                  class="input h-10 pl-10"
                />
              </div>

              <div class="flex flex-wrap gap-2">
                <button
                  v-for="p in platformOptions"
                  :key="p"
                  type="button"
                  class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-medium transition-colors"
                  :class="
                    selectedPlatform === p
                      ? 'border-primary-500 bg-primary-50 text-primary-700 dark:border-primary-400 dark:bg-primary-900/30 dark:text-primary-300'
                      : 'border-gray-200 bg-white text-gray-600 hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700'
                  "
                  @click="togglePlatform(p)"
                >
                  <PlatformIcon v-if="p !== 'all'" :platform="p as any" size="xs" />
                  {{ p === 'all' ? t('availableChannels.filters.allPlatforms') : p }}
                </button>
              </div>
            </div>

            <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
              <select v-model="selectedChannel" class="input h-10 w-full sm:w-48">
                <option value="">{{ t('availableChannels.filters.allChannels') }}</option>
                <option v-for="ch in channelOptions" :key="ch" :value="ch">{{ ch }}</option>
              </select>
              <button
                @click="loadData"
                :disabled="loading"
                class="btn btn-secondary h-10 w-10 p-0"
                :title="t('common.refresh', 'Refresh')"
              >
                <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
              </button>
            </div>
          </div>
        </div>
      </template>

      <template #table>
        <ModelPlazaTable
          :rows="filteredRows"
          :loading="loading"
          :empty-label="t('availableChannels.empty')"
        />
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import ModelPlazaTable from '@/components/channels/ModelPlazaTable.vue'
import userChannelsAPI, { type UserAvailableChannel } from '@/api/channels'
import userGroupsAPI from '@/api/groups'
import userModelCatalogAPI, { type ModelCatalog } from '@/api/modelCatalog'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  buildModelPlazaRows,
  uniqueChannels,
  uniquePlatforms,
  type ModelPlazaRow,
} from '@/utils/modelPlaza'

const { t } = useI18n()
const appStore = useAppStore()

const channels = ref<UserAvailableChannel[]>([])
const userGroupRates = ref<Record<number, number>>({})
const catalog = ref<ModelCatalog | null>(null)
const loading = ref(false)
const searchQuery = ref('')
const selectedPlatform = ref('all')
const selectedChannel = ref('')

const plazaRows = computed(() =>
  buildModelPlazaRows(channels.value, userGroupRates.value, catalog.value),
)

const platformOptions = computed(() => ['all', ...uniquePlatforms(plazaRows.value)])
const channelOptions = computed(() => uniqueChannels(plazaRows.value))

const stats = computed(() => {
  const rows = plazaRows.value
  const groupIds = new Set<number>()
  const channelNames = new Set<string>()
  for (const row of rows) {
    for (const offer of row.offers) {
      groupIds.add(offer.id)
      channelNames.add(offer.channel_name)
    }
  }
  return {
    models: rows.length,
    platforms: uniquePlatforms(rows).length,
    channels: channelNames.size,
    groups: groupIds.size,
  }
})

const filteredRows = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return plazaRows.value.filter((row: ModelPlazaRow) => {
    if (selectedPlatform.value !== 'all' && row.platform.toLowerCase() !== selectedPlatform.value.toLowerCase()) {
      return false
    }
    if (selectedChannel.value) {
      const hit = row.offers.some((o) => o.channel_name === selectedChannel.value)
      if (!hit) return false
    }
    if (!q) return true
    if (row.name.toLowerCase().includes(q)) return true
    if (row.display_name.toLowerCase().includes(q)) return true
    if (row.platform.toLowerCase().includes(q)) return true
    if (row.tags.some((tag) => tag.toLowerCase().includes(q))) return true
    return row.offers.some(
      (o) =>
        o.channel_name.toLowerCase().includes(q) ||
        o.name.toLowerCase().includes(q) ||
        (o.channel_description || '').toLowerCase().includes(q),
    )
  })
})

function togglePlatform(p: string) {
  selectedPlatform.value = selectedPlatform.value === p ? 'all' : p
}

async function loadData() {
  loading.value = true
  try {
    const [list, rates, cat] = await Promise.all([
      userChannelsAPI.getAvailable(),
      userGroupsAPI.getUserGroupRates().catch((err: unknown) => {
        console.error('Failed to load user group rates:', err)
        return {} as Record<number, number>
      }),
      userModelCatalogAPI.get().catch((err: unknown) => {
        console.error('Failed to load model catalog:', err)
        return null
      }),
    ])
    channels.value = list
    userGroupRates.value = rates
    catalog.value = cat
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

onMounted(loadData)
</script>
