<template>
  <div class="rounded-lg border border-amber-200 bg-amber-50 p-5 dark:border-amber-800 dark:bg-amber-950/30">
    <div class="mb-4 flex items-start gap-3">
      <div class="flex h-10 w-10 items-center justify-center rounded-lg bg-amber-500 text-white">
        <PlatformIcon :platform="props.provider" size="lg" />
      </div>
      <div>
        <h4 class="font-semibold text-gray-900 dark:text-white">{{ text('title') }}</h4>
        <p class="mt-1 text-sm text-gray-600 dark:text-gray-300">{{ text('description') }}</p>
      </div>
    </div>

    <button v-if="!session" type="button" class="btn btn-primary" :disabled="loading" @click="$emit('start')">
      {{ loading ? text('starting') : text('start') }}
    </button>

    <div v-else class="space-y-4">
      <div class="rounded-lg border border-amber-200 bg-white p-4 text-center dark:border-amber-800 dark:bg-dark-700">
        <p class="text-xs uppercase tracking-wide text-gray-500">{{ text('userCode') }}</p>
        <p class="my-2 select-all font-mono text-3xl font-bold tracking-[0.25em] text-amber-700 dark:text-amber-300">{{ session.user_code }}</p>
        <a :href="authorizationUrl" target="_blank" rel="noopener noreferrer" class="btn btn-primary mt-2 inline-flex">
          {{ text('openAuthorization') }}
        </a>
      </div>
      <div class="flex items-center justify-between text-sm">
        <span :class="statusClass">{{ statusText }}</span>
        <span v-if="session.status === 'pending'" class="text-gray-500">{{ text('expiresIn', { seconds: remainingSeconds }) }}</span>
      </div>
      <p v-if="error" class="rounded bg-red-50 p-3 text-sm text-red-600 dark:bg-red-950/40 dark:text-red-300">{{ error }}</p>
      <div class="flex gap-2">
        <button v-if="session.status === 'pending'" type="button" class="btn btn-secondary" @click="$emit('cancel')">{{ t('common.cancel') }}</button>
        <button v-if="session.status !== 'pending' && session.status !== 'authorized'" type="button" class="btn btn-secondary" @click="$emit('start')">{{ text('retry') }}</button>
        <button v-if="session.status === 'authorized' && (!autoComplete || loading || autoCompleteFailed)" type="button" class="btn btn-primary" :disabled="loading" @click="handleManualComplete">
          {{ loading ? (loadingLabel || t('admin.accounts.creating')) : (completeLabel || text('createAccount')) }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import type { DeviceAuthorization } from '@/api/admin/deviceOAuth'
import PlatformIcon from '@/components/common/PlatformIcon.vue'

type DeviceProvider = 'openai' | 'grok' | 'kimi'

const props = withDefaults(defineProps<{
  session: DeviceAuthorization | null
  loading: boolean
  error: string
  remainingSeconds: number
  provider?: DeviceProvider
  completeLabel?: string
  loadingLabel?: string
  // 授权成功后自动创建账号（短延迟让用户看到成功状态）
  autoComplete?: boolean
}>(), {
  provider: 'kimi',
  autoComplete: true
})
const emit = defineEmits<{ start: []; cancel: []; complete: [] }>()
const { t } = useI18n()
const authorizationUrl = computed(() => props.session?.verification_uri_complete || props.session?.verification_uri || '#')
const text = (key: string, params?: Record<string, unknown>) => {
  const base = props.provider === 'kimi'
    ? 'admin.accounts.oauth.kimi'
    : `admin.accounts.oauth.${props.provider}.device`
  return params ? t(`${base}.${key}`, params) : t(`${base}.${key}`)
}
const statusText = computed(() => {
  if (props.session?.status === 'authorized') return text('authorized')
  if (props.session?.status === 'denied') return text('denied')
  return text('waiting')
})
const statusClass = computed(() => props.session?.status === 'authorized' ? 'font-medium text-green-600' : props.session?.status === 'denied' ? 'font-medium text-red-600' : 'font-medium text-amber-700 dark:text-amber-300')

// 自动创建：授权成功 → 短延迟后自动触发 complete；失败（error 出现且不再 loading）→ 显示手动按钮
const autoCompleteFailed = ref(false)
let autoTimer: ReturnType<typeof setTimeout> | null = null
let autoFiredFor: string | null = null

function clearAutoTimer() {
  if (autoTimer) {
    clearTimeout(autoTimer)
    autoTimer = null
  }
}

watch(
  () => [props.session?.status, props.session?.user_code] as const,
  ([status, userCode]) => {
    clearAutoTimer()
    if (status !== 'authorized' || !props.autoComplete) return
    if (!userCode) return
    // 同一授权码只自动触发一次，失败也不要立刻循环重试
    if (autoFiredFor === userCode && autoCompleteFailed.value) return
    if (autoFiredFor === userCode) return
    autoFiredFor = userCode
    autoCompleteFailed.value = false
    autoTimer = setTimeout(() => {
      autoTimer = null
      if (props.session?.status === 'authorized' && !props.loading) {
        emit('complete')
      }
    }, 600)
  },
  { immediate: true }
)

// 自动创建失败后允许手动重试
watch(
  () => [props.loading, props.error] as const,
  ([loading, error]) => {
    if (!loading && error && props.session?.status === 'authorized') {
      autoCompleteFailed.value = true
    }
  }
)

function handleManualComplete() {
  autoCompleteFailed.value = false
  emit('complete')
}

onUnmounted(clearAutoTimer)
</script>
