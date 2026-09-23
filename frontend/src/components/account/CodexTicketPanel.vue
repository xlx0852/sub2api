<template>
  <section class="space-y-5">
    <div>
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
        {{ t('admin.accounts.usageDetails.tickets.title') }}
      </h3>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.usageDetails.tickets.hint') }}
      </p>
    </div>

    <div v-if="loading && !summary" class="grid gap-2" aria-busy="true">
      <div class="h-14 animate-pulse rounded-md bg-gray-100 dark:bg-dark-700" />
      <div class="h-14 animate-pulse rounded-md bg-gray-100 dark:bg-dark-700" />
    </div>

    <div
      v-else-if="loadFailed && !summary"
      class="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-300"
    >
      {{ t('admin.accounts.usageDetails.tickets.loadFailed') }}
    </div>

    <template v-else-if="summary">
      <!-- 池位 -->
      <div v-if="pools.length">
        <div class="mb-3 flex items-center justify-between gap-3 border-b border-gray-200 pb-2 dark:border-dark-600">
          <span class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.accounts.usageDetails.tickets.poolsTitle') }}
          </span>
          <span class="text-xs text-gray-400 dark:text-gray-500">
            {{ t('admin.accounts.usageDetails.tickets.replayWindow', { minutes: replayMinutes }) }}
          </span>
        </div>
        <ul class="space-y-2">
          <li
            v-for="pool in pools"
            :key="pool.pool"
            class="rounded-md border border-gray-200 px-3 py-2.5 dark:border-dark-600"
          >
            <div class="flex items-center justify-between gap-3">
              <div class="flex min-w-0 items-center gap-2">
                <span class="truncate font-mono text-xs text-gray-900 dark:text-gray-100" :title="pool.pool">{{ pool.pool }}</span>
                <span
                  class="shrink-0 rounded px-1.5 py-0.5 font-mono text-[11px] font-medium tabular-nums"
                  :class="shapeClass(pool.shape)"
                  :title="pool.shape ? t('admin.accounts.usageDetails.tickets.shapeHint') : t('admin.accounts.usageDetails.tickets.clearedHint')"
                >
                  {{ pool.shape || '—' }}
                </span>
              </div>
              <span
                class="shrink-0 rounded px-2 py-0.5 text-[11px] font-medium"
                :class="pool.fresh ? freshClass : staleClass"
              >
                {{ poolStateLabel(pool) }}
              </span>
            </div>
            <div class="mt-2 flex items-center justify-between gap-3 text-[11px] text-gray-500 dark:text-gray-400">
              <span class="truncate" :title="pool.model || ''">
                {{ pool.model ? t('admin.accounts.usageDetails.tickets.servedModel', { model: pool.model }) : t('admin.accounts.usageDetails.tickets.servedModelUnknown') }}
              </span>
              <span class="shrink-0 tabular-nums" :title="pool.captured_at || ''">{{ ageLabel(pool) }}</span>
            </div>
            <!-- 票龄进度：越靠右越接近不再回放；无票时没有可展示的进度 -->
            <div v-if="pool.shape" class="mt-2 h-1 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
              <div class="h-full rounded-full" :class="ageBarClass(pool)" :style="{ width: agePercent(pool) + '%' }" />
            </div>
          </li>
        </ul>
      </div>

      <!-- 会话 cookie -->
      <div v-if="cookies">
        <div class="mb-3 border-b border-gray-200 pb-2 dark:border-dark-600">
          <span class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.accounts.usageDetails.tickets.cookiesTitle') }}
          </span>
        </div>
        <div class="flex flex-wrap items-center gap-x-5 gap-y-2 text-xs text-gray-600 dark:text-gray-300">
          <span class="inline-flex items-center gap-1.5">
            <Icon name="shield" size="xs" />
            {{ t('admin.accounts.usageDetails.tickets.cookieCount', { count: cookies.count }) }}
          </span>
          <span v-if="cookies.age_seconds" class="tabular-nums">
            {{ t('admin.accounts.usageDetails.tickets.cookieRefreshed', { time: relativeLabel(cookies.age_seconds) }) }}
          </span>
          <span v-if="cookies.egress" class="font-mono text-gray-400 dark:text-gray-500" :title="t('admin.accounts.usageDetails.tickets.egressHint')">
            {{ t('admin.accounts.usageDetails.tickets.egress', { fingerprint: cookies.egress }) }}
          </span>
          <span
            class="rounded px-2 py-0.5 text-[11px] font-medium"
            :class="cookies.fresh ? freshClass : staleClass"
          >
            {{ cookies.fresh ? t('admin.accounts.usageDetails.tickets.cookiesLive') : t('admin.accounts.usageDetails.tickets.cookiesExpired') }}
          </span>
        </div>
      </div>
    </template>

    <div
      v-else
      class="rounded-md border border-dashed border-gray-200 py-10 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
    >
      {{ t('admin.accounts.usageDetails.tickets.empty') }}
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account, CodexTicketPool } from '@/types'
import { accountsAPI } from '@/api/admin/accounts'
import { formatRelativeTime } from '@/utils/format'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{ account: Account; active?: boolean }>()
const { t } = useI18n()

// 账号**列表**查询只 select 了部分字段（有意不含 extra 这个重 JSONB 列），
// 所以列表里的 account 没有 codex_ticket。打开本页签时再按 id 取一次详情
// （GET /admin/accounts/:id 会带 extra），既拿到摘要又不给列表加负担。
const detail = ref<Account | null>(null)
const loading = ref(false)
const loadFailed = ref(false)

const summary = computed(() => detail.value?.codex_ticket ?? props.account.codex_ticket ?? null)
const pools = computed(() => summary.value?.pools ?? [])
const cookies = computed(() => summary.value?.cookies ?? null)
const replayMinutes = computed(() => Math.round((summary.value?.replay_ttl_seconds ?? 3600) / 60))

const load = async (id: number) => {
  loading.value = true
  loadFailed.value = false
  try {
    detail.value = await accountsAPI.getById(id)
  } catch {
    // 详情取不到不该把整个面板变成错误页：回退到列表里的 account（可能没有摘要）。
    loadFailed.value = true
    detail.value = null
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.active, props.account?.id] as const,
  ([active, id]) => {
    if (!active || !id) return
    // 换账号必须重取，否则会显示上一个账号的票状态。
    if (detail.value && detail.value.id !== id) detail.value = null
    if (detail.value?.id === id) return
    void load(id)
  },
  { immediate: true }
)

// 形态高亮：292 是当前账号族里被当作目标的形态（实测 astra 落点发 292）。
// 其余形态保持中性——形态与池并非一一对应，不该用颜色暗示不存在的对应关系。
const shapeClass = (shape: number) =>
  shape === 0
    ? 'bg-gray-100 text-gray-400 dark:bg-gray-800 dark:text-gray-500'
    : shape === 292
      ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/60 dark:text-emerald-300'
      : 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-300'

// 三态：可回放 / 已过期 / 已清理（值被失效清理写成 null，键仍在，等待重采）。
// 第三态必须单独说，否则"刚被清理"会被读成"从没打过票"。
const poolStateLabel = (pool: CodexTicketPool) => {
  if (!pool.shape) return t('admin.accounts.usageDetails.tickets.cleared')
  return pool.fresh
    ? t('admin.accounts.usageDetails.tickets.replayable')
    : t('admin.accounts.usageDetails.tickets.expired')
}

const freshClass = 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/60 dark:text-emerald-300'
const staleClass = 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400'

const agePercent = (pool: CodexTicketPool) => {
  const ttl = summary.value?.replay_ttl_seconds ?? 3600
  if (!ttl) return 0
  return Math.min(100, Math.round((pool.age_seconds / ttl) * 100))
}

const ageBarClass = (pool: CodexTicketPool) => {
  if (!pool.fresh) return 'bg-gray-300 dark:bg-gray-600'
  const pct = agePercent(pool)
  if (pct >= 90) return 'bg-red-400 dark:bg-red-500'
  if (pct >= 60) return 'bg-amber-400 dark:bg-amber-500'
  return 'bg-emerald-400 dark:bg-emerald-500'
}

// 复用现有的 i18n 相对时间格式化（utils/format 已带多语言单位），
// 避免在组件里另造一套时间文案。
const translateRelativeTime = (key: string, params?: Record<string, number>) => t(key, params || {})
const relativeLabel = (seconds: number) =>
  formatRelativeTime(new Date(Date.now() - seconds * 1000).toISOString(), translateRelativeTime)

const ageLabel = (pool: CodexTicketPool) =>
  pool.captured_at ? relativeLabel(pool.age_seconds) : t('admin.accounts.usageDetails.tickets.ageUnknown')
</script>
