<template>
  <div v-if="visible">
    <div class="mb-3 flex items-start justify-between gap-4">
      <div class="min-w-0">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.accounts.usageDetails.openCodeGo.title') }}
        </h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.usageDetails.openCodeGo.hint') }}
        </p>
      </div>
      <button
        type="button"
        class="shrink-0 rounded-md border border-gray-200 px-2.5 py-1 text-xs font-medium text-gray-600 hover:bg-gray-50 disabled:opacity-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
        :disabled="loading"
        @click="refresh"
      >
        {{ loading ? t('admin.accounts.usageDetails.openCodeGo.querying') : t('admin.accounts.usageDetails.openCodeGo.query') }}
      </button>
    </div>

    <div v-if="loading && !usage" class="grid gap-2" aria-busy="true">
      <div class="h-12 animate-pulse rounded-md bg-gray-100 dark:bg-dark-700" />
      <div class="h-12 animate-pulse rounded-md bg-gray-100 dark:bg-dark-700" />
    </div>

    <div
      v-else-if="failureReason"
      class="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-300"
    >
      {{ t('admin.accounts.usageDetails.openCodeGo.failed', { reason: failureReason }) }}
      <span v-if="fetchedLabel" class="ml-2 text-amber-600/80 dark:text-amber-400/80">{{ fetchedLabel }}</span>
    </div>

    <div v-else class="space-y-3">
      <div v-for="win in windows" :key="win.key">
        <div class="flex items-center justify-between gap-3 text-xs">
          <span class="font-medium text-gray-700 dark:text-gray-200">
            {{ t(`admin.accounts.usageDetails.openCodeGo.${win.key}`) }}
          </span>
          <span class="tabular-nums text-gray-500 dark:text-gray-400">
            {{ formatPercent(win.percent) }}
            <span v-if="win.resetsAt" class="ml-2">{{ t('admin.accounts.usageDetails.openCodeGo.resetsAt', { time: win.resetsAt }) }}</span>
          </span>
        </div>
        <div class="mt-1.5 h-1.5 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
          <div class="h-full rounded-full" :class="barClass(win.percent)" :style="{ width: Math.min(100, Math.max(0, win.percent)) + '%' }" />
        </div>
      </div>
      <p v-if="fetchedLabel" class="text-[11px] text-gray-400 dark:text-gray-500">{{ fetchedLabel }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import { accountsAPI } from '@/api/admin/accounts'
import { formatRelativeTime } from '@/utils/format'

const props = defineProps<{ account: Account; active?: boolean }>()
const { t } = useI18n()

const OPENCODE_GO_HOST = 'opencode.ai'
const OPENCODE_GO_PATH = '/zen/go/v1'

// 本地先做一次轻量判断（host+path 精确匹配），服务端仍会再判一次并返回 applicable。
// 目的只是"不是这类账号就不发请求、不显示空面板"。
const looksLikeOpenCodeGo = computed(() => {
  const base = String((props.account?.credentials as Record<string, unknown> | undefined)?.base_url ?? '').trim()
  if (!base) return false
  try {
    const parsed = new URL(base)
    const path = parsed.pathname.replace(/\/+$/, '')
    return parsed.protocol === 'https:' && parsed.hostname === OPENCODE_GO_HOST &&
      (path === OPENCODE_GO_PATH || path === `${OPENCODE_GO_PATH}/usage`)
  } catch {
    return false
  }
})

const visible = ref(false)
const loading = ref(false)
const usage = ref<Awaited<ReturnType<typeof accountsAPI.getOpenCodeGoUsage>>['usage'] | null>(null)
const failureReason = ref('')

const translateRelativeTime = (key: string, params?: Record<string, number>) => t(key, params || {})

const windows = computed(() => {
  const u = usage.value
  if (!u) return []
  const rows: Array<{ key: 'rolling' | 'weekly' | 'monthly'; percent: number; resetsAt: string }> = []
  for (const key of ['rolling', 'weekly', 'monthly'] as const) {
    const win = u[key]
    if (!win) continue
    rows.push({ key, percent: Number(win.percent) || 0, resetsAt: win.resets_at ? relative(win.resets_at) : '' })
  }
  return rows
})

const relative = (iso: string) => {
  const ts = new Date(iso)
  if (Number.isNaN(ts.getTime())) return ''
  return formatRelativeTime(ts.toISOString(), translateRelativeTime)
}

const fetchedLabel = computed(() => {
  const at = usage.value?.fetched_at
  if (!at) return ''
  const ts = new Date(at)
  if (Number.isNaN(ts.getTime())) return ''
  return t('admin.accounts.usageDetails.openCodeGo.fetchedAt', {
    time: formatRelativeTime(ts.toISOString(), translateRelativeTime)
  })
})

const formatPercent = (value: number) => `${(Number(value) || 0).toFixed(1)}%`

// 只按用量分档，不做彩虹配色。
const barClass = (percent: number) => {
  if (percent >= 90) return 'bg-red-400 dark:bg-red-500'
  if (percent >= 70) return 'bg-amber-400 dark:bg-amber-500'
  return 'bg-emerald-400 dark:bg-emerald-500'
}

const load = async (id: number) => {
  loading.value = true
  try {
    const result = await accountsAPI.getOpenCodeGoUsage(id)
    visible.value = result?.applicable === true
    usage.value = result?.usage ?? null
    failureReason.value = result?.usage?.error || result?.error || ''
  } catch {
    // 请求本身失败：保持已判定过的可见性，并显示失败原因。
    failureReason.value = t('admin.accounts.usageDetails.openCodeGo.requestFailed')
  } finally {
    loading.value = false
  }
}

const refresh = () => {
  const id = props.account?.id
  if (id) void load(id)
}

watch(
  () => [props.active, props.account?.id] as const,
  ([active, id]) => {
    if (!id || !looksLikeOpenCodeGo.value) {
      visible.value = false
      return
    }
    if (!active) return
    void load(id)
  },
  { immediate: true }
)
</script>
