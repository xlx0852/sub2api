<!--
  Mirasim OAuth 授权登录（管理员侧 onboarding）。

  关键约束（改动前请先读）：
    1. 本组件只“生产”凭据：拿到 refresh token 后通过 apply 事件交给父组件回填表单，
       账号落库仍走既有的 POST /admin/accounts 路径（含敏感凭据合并与调度器快照同步）。
    2. 绝不把 refresh token / access token 打到 console 或错误提示里；授权链接与回调
       链接同理（回调链接内含 token），只在输入框内展示不落日志。
    3. 登录环节不调用 /auth/refresh（避免轮换导致用户本地客户端掉线）。
    4. redirect_uri 由后端固定为 loopback 占位符 http://127.0.0.1:9/callback（上游白名单
       只接受 loopback 与 *.mirofish.ai），因此“打开授权页”后页面打不开是正常的，
       需要用户从地址栏复制完整回调链接。
-->
<template>
  <div
    class="rounded-lg border border-sky-200 bg-sky-50 p-3 dark:border-sky-800 dark:bg-sky-900/20"
    :data-testid="`${testidPrefix}-panel`"
  >
    <div class="flex items-start justify-between gap-3">
      <div>
        <p class="text-sm font-medium text-sky-900 dark:text-sky-200">
          {{ t('admin.accounts.mirasimOAuth.title') }}
        </p>
        <p class="mt-1 text-xs text-sky-800/80 dark:text-sky-300/80">
          {{ t('admin.accounts.mirasimOAuth.desc') }}
        </p>
      </div>
      <button
        type="button"
        class="flex-shrink-0 text-xs text-sky-700 underline hover:text-sky-900 dark:text-sky-300"
        :data-testid="`${testidPrefix}-toggle`"
        @click="toggleExpanded"
      >
        {{ expanded ? t('admin.accounts.mirasimOAuth.collapse') : t('admin.accounts.mirasimOAuth.expand') }}
      </button>
    </div>

    <div v-if="expanded" class="mt-3 space-y-3">
      <!-- 方式切换 -->
      <div class="flex flex-wrap gap-2 text-xs">
        <button
          v-for="tab in tabs"
          :key="tab.value"
          type="button"
          class="rounded px-2 py-1"
          :class="
            method === tab.value
              ? 'bg-sky-600 text-white'
              : 'bg-white text-sky-800 dark:bg-dark-700 dark:text-sky-200'
          "
          :data-testid="`${testidPrefix}-tab-${tab.value}`"
          @click="method = tab.value"
        >
          {{ tab.label }}
        </button>
      </div>

      <!-- 浏览器授权 -->
      <div v-if="method === 'oauth'" class="space-y-2">
        <div class="flex flex-wrap items-end gap-2">
          <div>
            <label class="input-label">{{ t('admin.accounts.mirasimOAuth.provider') }}</label>
            <select
              v-model="provider"
              class="input text-sm"
              :data-testid="`${testidPrefix}-provider`"
            >
              <option v-for="p in providers" :key="p" :value="p">
                {{ providerLabel(p) }}
              </option>
            </select>
          </div>
          <button
            type="button"
            class="btn btn-secondary text-sm"
            :disabled="generating || providers.length === 0"
            :data-testid="`${testidPrefix}-generate`"
            @click="handleGenerate"
          >
            {{ generating ? t('admin.accounts.mirasimOAuth.generating') : t('admin.accounts.mirasimOAuth.generate') }}
          </button>
        </div>
        <p v-if="providers.length === 0" class="text-xs text-amber-600 dark:text-amber-400">
          {{ t('admin.accounts.mirasimOAuth.noProviders') }}
        </p>

        <div v-if="authLink" class="space-y-2 rounded border border-sky-200 bg-white p-2 dark:border-sky-800 dark:bg-dark-700">
          <div class="flex flex-wrap items-center gap-2">
            <a
              :href="authLink"
              target="_blank"
              rel="noopener noreferrer"
              class="text-xs text-sky-700 underline dark:text-sky-300"
              :data-testid="`${testidPrefix}-auth-link`"
            >{{ t('admin.accounts.mirasimOAuth.openLink') }}</a>
            <button
              type="button"
              class="text-xs text-sky-700 underline dark:text-sky-300"
              :data-testid="`${testidPrefix}-copy-link`"
              @click="copyText(authLink)"
            >{{ copied === 'auth' ? t('admin.accounts.mirasimOAuth.copied') : t('admin.accounts.mirasimOAuth.copyLink') }}</button>
          </div>
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.mirasimOAuth.linkHint') }}</p>
          <div>
            <label class="input-label">{{ t('admin.accounts.mirasimOAuth.callbackUrl') }}</label>
            <textarea
              v-model="callbackUrl"
              rows="2"
              class="input font-mono text-xs"
              :data-testid="`${testidPrefix}-callback-url`"
              :placeholder="t('admin.accounts.mirasimOAuth.callbackUrlPlaceholder')"
            />
          </div>
          <button
            type="button"
            class="btn btn-primary text-sm"
            :disabled="busy"
            :data-testid="`${testidPrefix}-submit-callback`"
            @click="handleSubmitCallback"
          >
            {{ busy ? t('admin.accounts.mirasimOAuth.completing') : t('admin.accounts.mirasimOAuth.complete') }}
          </button>
        </div>
      </div>

      <!-- 邮箱验证码 -->
      <div v-else-if="method === 'email'" class="space-y-2">
        <div class="flex flex-wrap items-end gap-2">
          <div class="min-w-[220px] flex-1">
            <label class="input-label">{{ t('admin.accounts.mirasimOAuth.email') }}</label>
            <input
              v-model="email"
              type="email"
              class="input text-sm"
              :data-testid="`${testidPrefix}-email`"
              :placeholder="t('admin.accounts.mirasimOAuth.emailPlaceholder')"
            />
          </div>
          <button
            type="button"
            class="btn btn-secondary text-sm"
            :disabled="sending"
            :data-testid="`${testidPrefix}-send-code`"
            @click="handleSendCode"
          >
            {{ sending ? t('admin.accounts.mirasimOAuth.sending') : t('admin.accounts.mirasimOAuth.sendCode') }}
          </button>
        </div>
        <p v-if="codeSentTo" class="text-xs text-green-600 dark:text-green-400">
          {{ t('admin.accounts.mirasimOAuth.codeSent', { email: codeSentTo }) }}
        </p>
        <div class="flex flex-wrap items-end gap-2">
          <div>
            <label class="input-label">{{ t('admin.accounts.mirasimOAuth.code') }}</label>
            <input
              v-model="code"
              class="input font-mono text-sm"
              :data-testid="`${testidPrefix}-code`"
              :placeholder="t('admin.accounts.mirasimOAuth.codePlaceholder')"
            />
          </div>
          <button
            type="button"
            class="btn btn-primary text-sm"
            :disabled="busy"
            :data-testid="`${testidPrefix}-submit-code`"
            @click="handleSubmitCode"
          >
            {{ busy ? t('admin.accounts.mirasimOAuth.completing') : t('admin.accounts.mirasimOAuth.complete') }}
          </button>
        </div>
      </div>

      <!-- 粘贴 token -->
      <div v-else class="space-y-2">
        <div>
          <label class="input-label">{{ t('admin.accounts.mirasimOAuth.tokenLabel') }}</label>
          <textarea
            v-model="pastedToken"
            rows="3"
            class="input font-mono text-xs"
            :data-testid="`${testidPrefix}-token`"
            :placeholder="t('admin.accounts.mirasimOAuth.tokenPlaceholder')"
          />
        </div>
        <button
          type="button"
          class="btn btn-primary text-sm"
          :disabled="busy"
          :data-testid="`${testidPrefix}-submit-token`"
          @click="handleSubmitToken"
        >
          {{ busy ? t('admin.accounts.mirasimOAuth.completing') : t('admin.accounts.mirasimOAuth.complete') }}
        </button>
      </div>

      <!-- 结果 / 错误 -->
      <div
        v-if="result"
        class="rounded border border-green-200 bg-green-50 p-2 text-xs text-green-800 dark:border-green-800 dark:bg-green-900/30 dark:text-green-300"
        :data-testid="`${testidPrefix}-result`"
      >
        <p>{{ t('admin.accounts.mirasimOAuth.success', { kind: providerLabel(result.identity.provider || 'manual') }) }}</p>
        <p v-if="result.identity.email">{{ t('admin.accounts.mirasimOAuth.identityEmail', { email: result.identity.email }) }}</p>
        <p v-if="result.identity.plan">{{ t('admin.accounts.mirasimOAuth.identityPlan', { plan: result.identity.plan }) }}</p>
        <p v-if="result.identity.expires_at">
          {{ t('admin.accounts.mirasimOAuth.identityExpiry', { time: formatExpiry(result.identity.expires_at) }) }}
        </p>
      </div>
      <p
        v-if="error"
        class="text-xs text-red-600 dark:text-red-400"
        :data-testid="`${testidPrefix}-error`"
      >{{ error }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import mirasimOAuth from '@/api/admin/mirasim'
import type { MirasimOAuthCredential } from '@/api/admin/mirasim'

const props = withDefaults(
  defineProps<{
    // 表单里的 auth_url 覆盖项（留空用后端默认 auth.mirasim.ai）。
    authUrl?: string
    proxyId?: number | null
    testidPrefix?: string
    defaultExpanded?: boolean
  }>(),
  { authUrl: '', proxyId: null, testidPrefix: 'mirasim-oauth', defaultExpanded: false }
)

const emit = defineEmits<{ apply: [payload: MirasimOAuthCredential] }>()

const { t } = useI18n()

const expanded = ref(props.defaultExpanded)
const method = ref<'oauth' | 'email' | 'token'>('oauth')
const providers = ref<string[]>(['github', 'google'])
const provider = ref<string>('github')
const generating = ref(false)
const busy = ref(false)
const sending = ref(false)
const authLink = ref('')
const callbackUrl = ref('')
const pastedToken = ref('')
const email = ref('')
const code = ref('')
const codeSentTo = ref('')
const copied = ref('')
let providersLoaded = false
const error = ref('')
const result = ref<MirasimOAuthCredential | null>(null)

const tabs = [
  { value: 'oauth' as const, label: t('admin.accounts.mirasimOAuth.tabOauth') },
  { value: 'email' as const, label: t('admin.accounts.mirasimOAuth.tabEmail') },
  { value: 'token' as const, label: t('admin.accounts.mirasimOAuth.tabToken') }
]

function providerLabel(p: string): string {
  const key = p.toLowerCase()
  if (key === 'github') return t('admin.accounts.mirasimOAuth.providerGithub')
  if (key === 'google') return t('admin.accounts.mirasimOAuth.providerGoogle')
  return p
}

function formatExpiry(unix: number): string {
  if (!unix) return '-'
  try {
    return new Date(unix * 1000).toLocaleString()
  } catch {
    return '-'
  }
}

function describeError(err: unknown): string {
  const anyErr = err as { response?: { data?: { message?: string } }; message?: string }
  return anyErr?.response?.data?.message || anyErr?.message || t('admin.accounts.mirasimOAuth.failed')
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    copied.value = 'auth'
  } catch {
    copied.value = ''
  }
}

/** 成功路径统一出口：交给父组件回填表单。 */
function accept(credential: MirasimOAuthCredential) {
  result.value = credential
  error.value = ''
  // 凭据已消费，立即清掉本地输入，避免在内存/界面上多留一份明文。
  callbackUrl.value = ''
  pastedToken.value = ''
  code.value = ''
  emit('apply', credential)
}

async function loadProviders() {
  try {
    const list = await mirasimOAuth.listProviders({
      auth_url: props.authUrl || undefined,
      proxy_id: props.proxyId
    })
    if (list && list.length > 0) {
      providers.value = list
      if (!list.includes(provider.value)) provider.value = list[0]
    }
  } catch (err) {
    // 拉取失败不阻断：回退到内置 github/google 选项，真实错误在生成链接时暴露。
    error.value = describeError(err)
  }
}

async function handleGenerate() {
  generating.value = true
  error.value = ''
  result.value = null
  try {
    const res = await mirasimOAuth.generateAuthUrl({
      provider: provider.value,
      auth_url: props.authUrl || undefined,
      proxy_id: props.proxyId
    })
    authLink.value = res.auth_url
  } catch (err) {
    error.value = describeError(err)
  } finally {
    generating.value = false
  }
}

async function handleSubmitCallback() {
  if (!callbackUrl.value.trim()) {
    error.value = t('admin.accounts.mirasimOAuth.missingCallback')
    return
  }
  busy.value = true
  error.value = ''
  try {
    const credential = await mirasimOAuth.completeOAuth({
      callback_url: callbackUrl.value.trim(),
      provider: provider.value
    })
    accept(credential)
  } catch (err) {
    error.value = describeError(err)
  } finally {
    busy.value = false
  }
}

async function handleSubmitToken() {
  if (!pastedToken.value.trim()) {
    error.value = t('admin.accounts.mirasimOAuth.missingToken')
    return
  }
  busy.value = true
  error.value = ''
  try {
    const credential = await mirasimOAuth.completeOAuth({
      refresh_token: pastedToken.value.trim(),
      provider: provider.value
    })
    accept(credential)
  } catch (err) {
    error.value = describeError(err)
  } finally {
    busy.value = false
  }
}

async function handleSendCode() {
  if (!email.value.trim()) {
    error.value = t('admin.accounts.mirasimOAuth.missingEmail')
    return
  }
  sending.value = true
  error.value = ''
  try {
    const res = await mirasimOAuth.startEmailCode({
      email: email.value.trim(),
      auth_url: props.authUrl || undefined,
      proxy_id: props.proxyId
    })
    codeSentTo.value = res.email || email.value.trim()
  } catch (err) {
    error.value = describeError(err)
  } finally {
    sending.value = false
  }
}

async function handleSubmitCode() {
  if (!email.value.trim()) {
    error.value = t('admin.accounts.mirasimOAuth.missingEmail')
    return
  }
  if (!code.value.trim()) {
    error.value = t('admin.accounts.mirasimOAuth.missingCode')
    return
  }
  busy.value = true
  error.value = ''
  try {
    const credential = await mirasimOAuth.completeEmailCode({
      email: email.value.trim(),
      code: code.value.trim(),
      auth_url: props.authUrl || undefined,
      proxy_id: props.proxyId
    })
    accept(credential)
  } catch (err) {
    error.value = describeError(err)
  } finally {
    busy.value = false
  }
}

function toggleExpanded() {
  expanded.value = !expanded.value
}

// 首次展开时再拉上游登录方式（避免表单打开就发请求）。
watch(expanded, (open) => {
  if (open && !providersLoaded) {
    providersLoaded = true
    void loadProviders()
  }
})

onMounted(() => {
  if (expanded.value && !providersLoaded) {
    providersLoaded = true
    void loadProviders()
  }
})

defineExpose({ loadProviders })
</script>
