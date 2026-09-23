<template>
  <div v-if="visible" class="flex flex-wrap items-center gap-1.5" :title="tooltip">
    <template v-if="windows.length">
      <span
        v-for="win in windows"
        :key="win.key"
        class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] font-medium tabular-nums"
        :class="toneClass(win.percent)"
      >
        <span class="opacity-70">{{ win.label }}</span>
        <span>{{ win.percent.toFixed(1) }}%</span>
      </span>
    </template>
    <span
      v-else-if="failure"
      class="inline-flex items-center rounded bg-amber-100 px-1.5 py-0.5 text-[11px] font-medium text-amber-700 dark:bg-amber-950/60 dark:text-amber-300"
    >
      {{ t('admin.accounts.openCodeGo.failedShort') }}
    </span>
    <span v-else class="text-[11px] text-gray-400 dark:text-gray-500">
      {{ t('admin.accounts.openCodeGo.notFetched') }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'

const props = defineProps<{ account: Account }>()
const { t } = useI18n()

const OPENCODE_GO_HOST = 'opencode.ai'
const OPENCODE_GO_PATH = '/zen/go/v1'

// 与抽屉面板同款判定：只对"base URL 精确指向官方 Go 基址"的 API-key 账号展示。
const visible = computed(() => {
  if (props.account?.type !== 'apikey') return false
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

/** 列表数据来自账号列表接口里的 extra，不需要额外交互请求。 */
const snapshot = computed(() => {
  const raw = (props.account?.extra as Record<string, unknown> | undefined)?.opencode_go_usage
  if (typeof raw !== 'string' || !raw.trim()) return null
  try {
    return JSON.parse(raw) as {
      weekly?: { percent?: number }
      monthly?: { percent?: number }
      error?: string
      fetched_at?: string
    }
  } catch {
    return null
  }
})

const failure = computed(() => Boolean(snapshot.value?.error))

const windows = computed(() => {
  const snap = snapshot.value
  if (!snap) return []
  const out: Array<{ key: string; label: string; percent: number }> = []
  if (snap.weekly) out.push({ key: 'weekly', label: t('admin.accounts.openCodeGo.weeklyShort'), percent: Number(snap.weekly.percent) || 0 })
  if (snap.monthly) out.push({ key: 'monthly', label: t('admin.accounts.openCodeGo.monthlyShort'), percent: Number(snap.monthly.percent) || 0 })
  return out
})

const toneClass = (percent: number) => {
  if (percent >= 90) return 'bg-red-100 text-red-700 dark:bg-red-950/60 dark:text-red-300'
  if (percent >= 70) return 'bg-amber-100 text-amber-700 dark:bg-amber-950/60 dark:text-amber-300'
  return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/60 dark:text-emerald-300'
}

const tooltip = computed(() => {
  if (failure.value) return t('admin.accounts.openCodeGo.failedTooltip', { reason: snapshot.value?.error ?? '' })
  if (!windows.value.length) return t('admin.accounts.openCodeGo.notFetchedTooltip')
  return t('admin.accounts.openCodeGo.tooltip')
})
</script>
