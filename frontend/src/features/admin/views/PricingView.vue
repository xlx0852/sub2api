<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-3 lg:flex-row lg:items-start">
          <div class="flex flex-wrap items-center gap-2">
            <div class="flex rounded-lg border border-gray-200 p-0.5 dark:border-dark-600">
              <button
                v-for="tab in tabs"
                :key="tab.key"
                type="button"
                class="rounded-md px-3 py-1.5 text-sm font-medium"
                :class="activeTab === tab.key
                  ? 'bg-primary-600 text-white'
                  : 'text-gray-600 hover:text-gray-900 dark:text-gray-300'"
                @click="activeTab = tab.key"
              >
                {{ t(tab.label) }}
              </button>
            </div>
            <button type="button" class="btn btn-secondary" :disabled="loading" @click="load">
              <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
              {{ t('common.refresh') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
      <!-- Tab: 售价表 -->
      <section v-show="activeTab === 'policies'" class="overflow-x-auto">
        <table class="min-w-full text-sm">
          <thead class="table-header bg-[#f0f0ec] text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:bg-dark-800 dark:text-dark-400">
            <tr>
              <th class="px-3 py-2">{{ t('admin.pricing.colPolicy') }}</th>
              <th class="px-3 py-2">{{ t('admin.pricing.colStatus') }}</th>
              <th class="px-3 py-2 text-right">{{ t('admin.pricing.colModels') }}</th>
              <th class="px-3 py-2">{{ t('admin.pricing.colBoundGroups') }}</th>
              <th class="px-3 py-2">{{ t('admin.pricing.colExample') }}</th>
              <th class="px-3 py-2" />
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 text-gray-700 dark:divide-dark-700 dark:text-gray-200">
            <tr v-for="policy in overview?.policies || []" :key="policy.id" class="transition-colors hover:bg-black/[0.02] dark:hover:bg-white/[0.03]">
              <td class="px-3 py-2">
                <div class="font-medium">#{{ policy.id }} {{ policy.name }}</div>
                <div class="text-[11px] text-gray-400">{{ (policy.platforms || []).join(', ') || '-' }}</div>
              </td>
              <td class="px-3 py-2">
                <span class="rounded px-1.5 py-0.5 text-[11px]"
                  :class="policy.status === 'active'
                    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
                    : 'bg-gray-100 text-gray-500 dark:bg-dark-700'">
                  {{ policy.status === 'active' ? t('admin.pricing.statusActive') : policy.status }}
                </span>
                <span v-if="policy.priced_row_count === 0"
                  class="ml-1 rounded bg-amber-100 px-1.5 py-0.5 text-[11px] text-amber-800 dark:bg-amber-950/40 dark:text-amber-200">
                  {{ t('admin.pricing.listOnly') }}
                </span>
              </td>
              <td class="px-3 py-2 text-right">{{ policy.model_count }}</td>
              <td class="px-3 py-2">
                <template v-if="policy.bound_groups.length">
                  <span v-for="g in policy.bound_groups" :key="g.id"
                    class="mr-1 inline-flex rounded bg-gray-100 px-1.5 py-0.5 text-[11px] dark:bg-dark-700">
                    {{ g.name }} ×{{ g.rate_multiplier }}
                  </span>
                </template>
                <span v-else class="text-[11px] text-gray-400">{{ t('admin.pricing.unbound') }}</span>
              </td>
              <td class="px-3 py-2 whitespace-nowrap">
                <template v-if="policy.example_model">
                  <span class="text-gray-400">{{ policy.example_model }}</span>
                  {{ money(policy.example_sell_input, policy.example_currency) }}
                  <span v-if="policy.example_effective_input != null">→ {{ money(policy.example_effective_input, policy.example_currency) }}</span>
                </template>
                <span v-else class="text-gray-400">-</span>
              </td>
              <td class="px-3 py-2 text-right">
                <button type="button" class="text-primary-600 hover:text-primary-700"
                  @click="openEditor(policy.id)">
                  {{ t('admin.pricing.edit') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </section>

      <!-- Tab: 按模型的价目矩阵 -->
      <section v-show="activeTab === 'models'" class="space-y-2">
        <div class="overflow-x-auto">
          <table class="min-w-full text-sm">
            <thead class="table-header bg-[#f0f0ec] text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:bg-dark-800 dark:text-dark-400">
              <tr>
                <th class="px-3 py-2">{{ t('admin.pricing.colModelName') }}</th>
                <th class="px-3 py-2">{{ t('admin.pricing.colOffers') }}</th>
                <th class="px-3 py-2 text-right">{{ t('admin.pricing.colSpread') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-200 text-gray-700 dark:divide-dark-700 dark:text-gray-200">
              <tr v-for="row in modelMatrix" :key="row.model">
                <td class="px-3 py-2 font-mono">{{ row.model }}</td>
                <td class="px-3 py-2">
                  <div v-for="(offer, i) in row.offers" :key="i" class="flex flex-wrap items-center gap-1 py-0.5">
                    <span class="rounded bg-gray-100 px-1.5 text-[11px] dark:bg-dark-700">#{{ offer.policy_id }} {{ offer.policy_name }}</span>
                    <span class="font-medium">{{ money(offer.input, offer.currency) }}</span>
                    <span class="text-gray-400">/M</span>
                    <span v-if="offer.groups.length" class="text-[11px] text-gray-400">
                      → {{ offer.groups.map((g) => `${g.name} ¥${(Number(offer.input || 0) * g.rate).toFixed(2)}`).join(' · ') }}
                    </span>
                    <span v-else class="text-[11px] text-gray-400">{{ t('admin.pricing.unbound') }}</span>
                  </div>
                </td>
                <td class="px-3 py-2 text-right">
                  <span v-if="row.pricedOffers > 1" class="rounded bg-amber-100 px-1.5 py-0.5 text-[11px] text-amber-800 dark:bg-amber-950/40 dark:text-amber-200">
                    {{ t('admin.pricing.diverged', { n: row.pricedOffers }) }}
                  </span>
                  <span v-else class="text-gray-400">-</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
      </template>
    </TablePageLayout>

    <GroupPricingModal
      :show="editorPolicyId !== null"
      :group="editorGroup"
      :policy-id="editorPolicyId"
      @close="editorPolicyId = null"
      @success="load"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import GroupPricingModal from '@/components/admin/group/GroupPricingModal.vue'
import adminAPI from '@/api/admin'
import { useAppStore } from '@/stores/app'
import { apiErrorMessage } from '@/api/client'
import type { PricingOverview } from '@/api/admin/channels'

const { t } = useI18n()
const appStore = useAppStore()
const route = useRoute()

const loading = ref(false)
const overview = ref<PricingOverview | null>(null)
const activeTab = ref<'policies' | 'models'>('policies')
const editorPolicyId = ref<number | null>(null)
const editorGroup = ref<null>(null)

const tabs = [
  { key: 'policies' as const, label: 'admin.pricing.tabPolicies' },
  { key: 'models' as const, label: 'admin.pricing.tabModels' }
]

function money(value: number | null | undefined, currency?: string) {
  if (value == null) return '-'
  const symbol = currency === 'CNY' ? '¥' : '$'
  return `${symbol}${Number(value).toFixed(value >= 1 ? 2 : 3)}`
}

// 按模型汇总所有售价表：同一模型出现多个价就是"分叉"，需要人工确认。
const modelMatrix = computed(() => {
  const map = new Map<string, {
    model: string
    pricedOffers: number
    offers: { policy_id: number; policy_name: string; input: number | null; currency: string; groups: { name: string; rate: number }[] }[]
  }>()
  for (const policy of overview.value?.policies || []) {
    for (const row of policy.rows || []) {
      for (const model of row.models || []) {
        const key = model.toLowerCase()
        if (!map.has(key)) {
          map.set(key, { model, pricedOffers: 0, offers: [] })
        }
        const bucket = map.get(key)!
        const hasPrice = row.input_per_mtok != null
        if (hasPrice) bucket.pricedOffers += 1
        bucket.offers.push({
          policy_id: policy.id,
          policy_name: policy.name,
          input: row.input_per_mtok ?? null,
          currency: row.currency,
          groups: (policy.bound_groups || []).map((g) => ({ name: g.name, rate: g.rate_multiplier }))
        })
      }
    }
  }
  return [...map.values()].sort((a, b) => a.model.localeCompare(b.model))
})

function openEditor(policyId: number) {
  editorPolicyId.value = policyId
}

async function load() {
  loading.value = true
  try {
    overview.value = await adminAPI.channels.getPricingOverview()
  } catch (error) {
    appStore.showError(apiErrorMessage(error))
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await load()
  // ?policy=12 直接在编辑弹窗里打开这张表
  const fromQuery = Number(route.query.policy)
  if (Number.isFinite(fromQuery) && fromQuery > 0) {
    editorPolicyId.value = fromQuery
  }
})
</script>
