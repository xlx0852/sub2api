<template>
  <BaseDialog :show="show" :title="title" width="full" :close-on-click-outside="true" @close="close">
    <div v-if="loading" class="flex items-center justify-center py-16">
      <div class="flex flex-col items-center gap-3">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
        <div class="text-sm font-medium text-gray-500 dark:text-gray-400">{{ t('admin.ops.errorDetail.loading') }}</div>
      </div>
    </div>

    <div v-else-if="!detail" class="py-10 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ emptyText }}
    </div>

    <div v-else class="space-y-6 p-6">
      <!-- Summary (compact definition grid) -->
      <dl class="grid grid-cols-2 gap-x-6 gap-y-2 rounded-xl bg-gray-50 px-5 py-4 sm:grid-cols-3 lg:grid-cols-4 dark:bg-dark-900">
        <div class="min-w-0">
          <dt class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.requestId') }}</dt>
          <dd class="truncate font-mono text-xs font-medium text-gray-900 dark:text-white" :title="requestId">{{ requestId || '—' }}</dd>
        </div>
        <div class="min-w-0">
          <dt class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.time') }}</dt>
          <dd class="truncate text-xs font-medium text-gray-900 dark:text-white">{{ formatDateTime(detail.created_at) }}</dd>
        </div>
        <div class="min-w-0">
          <dt class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.user') }}</dt>
          <dd class="truncate text-xs font-medium text-gray-900 dark:text-white">{{ detail.user_email || (detail.user_id != null ? String(detail.user_id) : '—') }}</dd>
        </div>
        <div class="min-w-0">
          <dt class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.attemptedAccount') }}</dt>
          <dd class="truncate text-xs font-medium text-gray-900 dark:text-white">
            {{ detail.account_name || (detail.account_id != null ? `#${detail.account_id}` : t('admin.ops.errorDetail.accountNotSelected')) }}
            <span v-if="detail.account_id != null && detail.account_name" class="font-mono text-gray-400">#{{ detail.account_id }}</span>
          </dd>
        </div>
        <div class="min-w-0">
          <dt class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.platform') }}</dt>
          <dd class="truncate text-xs font-medium text-gray-900 dark:text-white">{{ detail.platform || '—' }}</dd>
        </div>
        <div class="min-w-0">
          <dt class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.group') }}</dt>
          <dd class="truncate text-xs font-medium text-gray-900 dark:text-white">{{ detail.group_name || (detail.group_id != null ? String(detail.group_id) : '—') }}</dd>
        </div>
        <div class="min-w-0">
          <dt class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.model') }}</dt>
          <dd class="truncate font-mono text-xs font-medium text-gray-900 dark:text-white">
            <template v-if="hasModelMapping(detail)">
              {{ detail.requested_model }}<span class="mx-1 text-gray-400">→</span><span class="text-primary-600 dark:text-primary-400">{{ detail.upstream_model }}</span>
            </template>
            <template v-else>{{ displayModel(detail) || '—' }}</template>
          </dd>
        </div>
        <div class="min-w-0">
          <dt class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.inboundEndpoint') }}</dt>
          <dd class="truncate font-mono text-xs font-medium text-gray-900 dark:text-white" :title="detail.inbound_endpoint">{{ detail.inbound_endpoint || '—' }}</dd>
        </div>
        <div class="min-w-0">
          <dt class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.upstreamEndpoint') }}</dt>
          <dd class="truncate font-mono text-xs font-medium text-gray-900 dark:text-white" :title="detail.upstream_endpoint">{{ detail.upstream_endpoint || '—' }}</dd>
        </div>
        <div class="min-w-0">
          <dt class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.status') }}</dt>
          <dd>
            <span :class="['inline-flex items-center rounded px-1.5 py-0.5 text-[11px] font-black ring-1 ring-inset', statusClass]">
              {{ detail.status_code }}
            </span>
          </dd>
        </div>
        <div class="min-w-0">
          <dt class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.requestType') }}</dt>
          <dd class="truncate text-xs font-medium text-gray-900 dark:text-white">{{ formatRequestTypeLabel(detail.request_type) }}</dd>
        </div>
        <div class="min-w-0">
          <dt class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.apiKeyPrefix') }}</dt>
          <dd class="truncate font-mono text-xs font-medium text-gray-900 dark:text-white">{{ detail.api_key_prefix || detail.attempted_key_prefix || '—' }}</dd>
        </div>
        <div class="min-w-0">
          <dt class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.message') }}</dt>
          <dd class="truncate text-xs font-medium text-gray-900 dark:text-white" :title="detail.message">{{ detail.message || '—' }}</dd>
        </div>
        <div v-if="detail.deleted_key_owner_email" class="min-w-0">
          <dt class="text-[10px] font-bold uppercase tracking-wider text-gray-400">{{ t('admin.ops.errorDetail.deletedKeyOwner') }}</dt>
          <dd class="truncate text-xs font-medium text-gray-900 dark:text-white">
            {{ detail.deleted_key_owner_email }}
            <span v-if="detail.deleted_key_name" class="text-gray-500 dark:text-gray-400">({{ detail.deleted_key_name }})</span>
            <span class="ml-1 inline-flex items-center rounded px-1 py-0.5 text-[10px] font-bold ring-1 ring-inset bg-red-50 text-red-700 ring-red-600/20 dark:bg-red-900/30 dark:text-red-400 dark:ring-red-500/30">
              {{ t('admin.ops.errorDetail.keyDeletedBadge') }}
            </span>
          </dd>
        </div>
      </dl>

      <!-- Request flow: which stage the request died at -->
      <div class="rounded-xl bg-gray-50 p-6 dark:bg-dark-900">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <h3 class="text-sm font-black uppercase tracking-wider text-gray-900 dark:text-white">{{ t('admin.ops.errorDetail.requestFlow') }}</h3>
          <div class="font-mono text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.ops.errorDetail.latency') }}: {{ requestDurationLabel }}
          </div>
        </div>
        <ol class="mt-5 space-y-0">
          <li
            v-for="(step, idx) in flowSteps"
            :key="step.key"
            class="relative flex gap-3 pb-6 last:pb-0"
            :data-flow-step="step.key"
            :data-flow-status="step.status"
          >
            <div v-if="idx < flowSteps.length - 1" class="absolute left-[11px] top-6 bottom-0 w-px bg-gray-200 dark:bg-dark-700"></div>
            <div
              class="relative mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-white"
              :class="
                step.status === 'failed'
                  ? 'bg-red-500'
                  : step.status === 'blocked'
                    ? 'bg-amber-500'
                    : step.status === 'passed'
                      ? 'bg-emerald-500'
                      : 'bg-gray-300 dark:bg-dark-600'
              "
            >
              <Icon
                :name="step.status === 'failed' || step.status === 'blocked' ? 'x' : step.status === 'passed' ? 'check' : 'clock'"
                size="xs"
                :stroke-width="3"
              />
            </div>
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-2">
                <span class="text-sm font-bold text-gray-900 dark:text-white">{{ step.label }}</span>
                <span
                  v-if="step.status === 'failed'"
                  class="inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-black ring-1 ring-inset bg-red-50 text-red-700 ring-red-600/20 dark:bg-red-900/30 dark:text-red-400 dark:ring-red-500/30"
                >
                  {{ t('admin.ops.errorDetail.diedHere') }}
                </span>
                <span
                  v-else-if="step.status === 'blocked'"
                  class="inline-flex items-center rounded bg-amber-50 px-1.5 py-0.5 text-[10px] font-black text-amber-700 ring-1 ring-inset ring-amber-600/20 dark:bg-amber-900/30 dark:text-amber-400 dark:ring-amber-500/30"
                >
                  {{ t('admin.ops.errorDetail.blockedBeforeUpstream') }}
                </span>
                <span v-else-if="step.latency != null" class="font-mono text-[11px] text-gray-500 dark:text-gray-400">
                  {{ step.latency }}ms
                </span>
                <span v-else-if="step.status === 'passed'" class="text-[11px] text-emerald-600 dark:text-emerald-400">
                  {{ t('admin.ops.errorDetail.flowPassed') }}
                </span>
                <span v-else class="text-[11px] text-gray-400 dark:text-gray-500">
                  {{ t('admin.ops.errorDetail.flowNotReached') }}
                </span>
              </div>
              <div
                v-if="(step.status === 'failed' || step.status === 'blocked') && step.detail"
                class="mt-1 break-words text-sm font-medium"
                :class="step.status === 'blocked' ? 'text-amber-700 dark:text-amber-400' : 'text-red-700 dark:text-red-400'"
              >
                {{ step.detail }}
              </div>

              <!-- 环节内部细节 -->
              <div v-if="step.subline" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ step.subline }}
              </div>

              <!-- 上游尝试链：每次选号/调用的账号、结果与耗时 -->
              <ul
                v-if="step.key === 'upstream' && upstreamAttempts.length"
                class="mt-2 space-y-1 rounded-lg border border-gray-200 bg-white p-2 dark:border-dark-700 dark:bg-dark-800"
              >
                <li
                  v-for="(a, i) in upstreamAttempts"
                  :key="i"
                  class="flex flex-wrap items-baseline gap-x-2 font-mono text-[11px] leading-5 text-gray-600 dark:text-gray-300"
                >
                  <span class="text-gray-400">#{{ i + 1 }}</span>
                  <span v-if="a.atLabel" class="text-gray-400">{{ a.atLabel }}</span>
                  <span class="font-semibold text-gray-800 dark:text-gray-100">{{ a.accountLabel }}</span>
                  <span
                    :class="a.statusCode && a.statusCode < 400
                      ? 'text-emerald-600 dark:text-emerald-400'
                      : 'text-red-600 dark:text-red-400'"
                  >{{ a.statusCode ?? '—' }}</span>
                  <span v-if="a.kind" class="rounded bg-gray-100 px-1 text-[10px] text-gray-500 dark:bg-dark-700 dark:text-gray-400">{{ a.kind }}</span>
                  <span v-if="a.deltaMs != null" class="text-gray-400">+{{ a.deltaMs }}ms</span>
                  <span v-if="a.message" class="w-full truncate text-gray-500 dark:text-gray-400" :title="a.message">{{ a.message }}</span>
                </li>
              </ul>
            </div>
          </li>
        </ol>
      </div>

      <!-- Response content (client request -> error_body; upstream -> upstream_error_detail/message) -->
      <div class="rounded-xl bg-gray-50 p-6 dark:bg-dark-900">
        <h3 class="text-sm font-black uppercase tracking-wider text-gray-900 dark:text-white">{{ t('admin.ops.errorDetail.responseBody') }}</h3>
        <pre class="mt-4 max-h-[520px] overflow-auto rounded-xl border border-gray-200 bg-white p-4 text-xs text-gray-800 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-100"><code>{{ prettyJSON(primaryResponseBody || '') }}</code></pre>
      </div>

      <!-- Upstream errors list (only for request errors) -->
      <div v-if="showUpstreamList" class="rounded-xl bg-gray-50 p-6 dark:bg-dark-900">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <h3 class="text-sm font-black uppercase tracking-wider text-gray-900 dark:text-white">{{ t('admin.ops.errorDetails.upstreamErrors') }}</h3>
          <div class="text-xs text-gray-500 dark:text-gray-400" v-if="correlatedUpstreamLoading">{{ t('common.loading') }}</div>
        </div>

        <div v-if="!correlatedUpstreamLoading && !correlatedUpstreamErrors.length" class="mt-3 text-sm text-gray-500 dark:text-gray-400">
          {{ t('common.noData') }}
        </div>

        <div v-else class="mt-4 space-y-3">
          <div
            v-for="(ev, idx) in correlatedUpstreamErrors"
            :key="ev.id"
            class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800"
          >
            <div class="flex flex-wrap items-center justify-between gap-2">
              <div class="text-xs font-black text-gray-900 dark:text-white">
                #{{ idx + 1 }}
                <span v-if="ev.type" class="ml-2 rounded-md bg-gray-100 px-2 py-0.5 font-mono text-[10px] font-bold text-gray-700 dark:bg-dark-700 dark:text-gray-200">{{ ev.type }}</span>
              </div>
              <div class="flex items-center gap-2">
                <div class="font-mono text-xs text-gray-500 dark:text-gray-400">
                  {{ ev.status_code ?? '—' }}
                </div>
                <button
                  type="button"
                  class="inline-flex items-center gap-1.5 rounded-md px-1.5 py-1 text-[10px] font-bold text-primary-700 hover:bg-primary-50 disabled:cursor-not-allowed disabled:opacity-60 dark:text-primary-200 dark:hover:bg-dark-700"
                  :disabled="!getUpstreamResponsePreview(ev)"
                  :title="getUpstreamResponsePreview(ev) ? '' : t('common.noData')"
                  @click="toggleUpstreamDetail(ev.id)"
                >
                  <Icon
                    :name="expandedUpstreamDetailIds.has(ev.id) ? 'chevronDown' : 'chevronRight'"
                    size="xs"
                    :stroke-width="2"
                  />
                  <span>
                    {{
                      expandedUpstreamDetailIds.has(ev.id)
                        ? t('admin.ops.errorDetail.responsePreview.collapse')
                        : t('admin.ops.errorDetail.responsePreview.expand')
                    }}
                  </span>
                </button>
              </div>
            </div>

            <div class="mt-3 grid grid-cols-1 gap-2 text-xs text-gray-600 dark:text-gray-300 sm:grid-cols-2">
              <div>
                <span class="text-gray-400">{{ t('admin.ops.errorDetail.upstreamEvent.status') }}:</span>
                <span class="ml-1 font-mono">{{ ev.status_code ?? '—' }}</span>
              </div>
              <div>
                <span class="text-gray-400">{{ t('admin.ops.errorDetail.upstreamEvent.requestId') }}:</span>
                <span class="ml-1 font-mono">{{ ev.request_id || ev.client_request_id || '—' }}</span>
              </div>
            </div>

            <div v-if="ev.message" class="mt-3 break-words text-sm font-medium text-gray-900 dark:text-white">{{ ev.message }}</div>

            <pre
              v-if="expandedUpstreamDetailIds.has(ev.id)"
              class="mt-3 max-h-[240px] overflow-auto rounded-xl border border-gray-200 bg-gray-50 p-3 text-xs text-gray-800 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-100"
            ><code>{{ prettyJSON(getUpstreamResponsePreview(ev)) }}</code></pre>
          </div>
        </div>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/ui/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { opsAPI, type OpsErrorDetail } from '@/api/admin/ops'
import { formatDateTime } from '@/utils/format'
import { resolvePrimaryResponseBody, resolveUpstreamPayload } from '../utils/errorDetailResponse'

interface Props {
  show: boolean
  errorId: number | null
  errorType?: 'request' | 'upstream'
}

interface Emits {
  (e: 'update:show', value: boolean): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const detail = ref<OpsErrorDetail | null>(null)

const showUpstreamList = computed(() => props.errorType === 'request')

const requestId = computed(() => detail.value?.request_id || detail.value?.client_request_id || '')

// 请求总时长优先取 response 延迟；没有则用最近一个有值的阶段延迟兜底。
const requestDurationLabel = computed(() => {
  const d = detail.value
  if (!d) return '—'
  const candidates = [d.response_latency_ms, d.upstream_latency_ms, d.routing_latency_ms, d.auth_latency_ms]
  for (const v of candidates) {
    if (v != null) return `${v}ms`
  }
  return '—'
})

const primaryResponseBody = computed(() => {
  return resolvePrimaryResponseBody(detail.value, props.errorType)
})




const title = computed(() => {
  if (!props.errorId) return t('admin.ops.errorDetail.title')
  return t('admin.ops.errorDetail.titleWithId', { id: String(props.errorId) })
})

const emptyText = computed(() => t('admin.ops.errorDetail.noErrorSelected'))

function formatRequestTypeLabel(type: number | null | undefined): string {
  switch (type) {
    case 1: return t('admin.ops.errorDetail.requestTypeSync')
    case 2: return t('admin.ops.errorDetail.requestTypeStream')
    case 3: return t('admin.ops.errorDetail.requestTypeWs')
    default: return t('admin.ops.errorDetail.requestTypeUnknown')
  }
}

function hasModelMapping(d: OpsErrorDetail | null): boolean {
  if (!d) return false
  const requested = String(d.requested_model || '').trim()
  const upstream = String(d.upstream_model || '').trim()
  return !!requested && !!upstream && requested !== upstream
}

function displayModel(d: OpsErrorDetail | null): string {
  if (!d) return ''
  const upstream = String(d.upstream_model || '').trim()
  if (upstream) return upstream
  const requested = String(d.requested_model || '').trim()
  if (requested) return requested
  return String(d.model || '').trim()
}

type FlowStepStatus = 'passed' | 'failed' | 'blocked' | 'not_reached'

interface FlowStep {
  key: string
  label: string
  status: FlowStepStatus
  latency?: number | null
  detail?: string
  subline?: string
}

interface UpstreamAttempt {
  atUnixMs?: number
  atLabel?: string
  accountLabel: string
  statusCode?: number | null
  kind?: string
  message?: string
  deltaMs?: number | null
}

function pad3(n: number): string {
  return String(n).padStart(3, '0')
}

function formatAttemptTime(ms?: number): string {
  if (ms == null) return ''
  const d = new Date(ms)
  const p2 = (v: number) => String(v).padStart(2, '0')
  return `${p2(d.getHours())}:${p2(d.getMinutes())}:${p2(d.getSeconds())}.${pad3(d.getMilliseconds())}`
}

const upstreamAttempts = computed<UpstreamAttempt[]>(() => {
  const raw = String(detail.value?.upstream_errors || '').trim()
  if (!raw || raw === '[]' || raw === '{}') return []
  let events: any[] = []
  try {
    const parsed = JSON.parse(raw)
    events = Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
  const attempts: UpstreamAttempt[] = []
  let prevMs: number | null = null
  for (const ev of events) {
    const at = typeof ev?.at_unix_ms === 'number' ? ev.at_unix_ms : undefined
    const accountLabel = ev?.account_name
      ? `${ev.account_name} #${ev.account_id ?? ''}`.trim()
      : ev?.account_id != null
        ? `#${ev.account_id}`
        : '—'
    attempts.push({
      atUnixMs: at,
      atLabel: formatAttemptTime(at),
      accountLabel,
      statusCode: ev?.upstream_status_code ?? null,
      kind: typeof ev?.kind === 'string' ? ev.kind : undefined,
      message: typeof ev?.message === 'string' ? ev.message : undefined,
      deltaMs: at != null && prevMs != null ? Math.max(0, at - prevMs) : null
    })
    if (at != null) prevMs = at
  }
  return attempts
})

// 选号摘要：最终账号 + 尝试次数/去重账号数，挂到 routing 步骤
const selectionSummary = computed(() => {
  const d = detail.value
  if (!d) return ''
  const parts: string[] = []
  if (d.account_id != null || d.account_name) {
    const label = d.account_name || `#${d.account_id}`
    parts.push(t('admin.ops.errorDetail.flowSelectedAccount', { account: d.account_name ? `${label} #${d.account_id}` : label }))
  }
  const n = upstreamAttempts.value.length
  if (n > 0) {
    const distinct = new Set(upstreamAttempts.value.map((a) => a.accountLabel)).size
    parts.push(t('admin.ops.errorDetail.flowAttemptsSummary', { n, m: distinct }))
  }
  return parts.join(' · ')
})

const flowSteps = computed<FlowStep[]>(() => {
  const d = detail.value
  if (!d) return []
  const phase = String(d.phase || '').toLowerCase()
  const authLatency = d.auth_latency_ms
  const routingLatency = d.routing_latency_ms
  const upstreamLatency = d.upstream_latency_ms
  const responseLatency = d.response_latency_ms
  const accountSelected = d.account_id != null

  // 旧数据会把「选号后的本地账号策略拦截」记成 auth。
  // 一旦已有路由/账号/返回客户端证据，入站认证就不可能是失败点。
  const reachedRouting = routingLatency != null || accountSelected || upstreamLatency != null
  const reachedUpstream = upstreamLatency != null || phase === 'upstream'
  const locallyRejected = d.is_business_limited && String(d.error_source || '').toLowerCase() !== 'upstream_http'
  const localBlockBeforeUpstream =
    locallyRejected && reachedRouting && !reachedUpstream && responseLatency != null && phase !== 'routing'

  const authStatus: FlowStepStatus = phase === 'auth' && !reachedRouting ? 'failed' : 'passed'
  const routingStatus: FlowStepStatus = phase === 'routing' ? 'failed' : reachedRouting ? 'passed' : 'not_reached'
  const upstreamStatus: FlowStepStatus =
    phase === 'upstream' ? 'failed' : upstreamLatency != null ? 'passed' : localBlockBeforeUpstream ? 'blocked' : 'not_reached'
  const responseStatus: FlowStepStatus = responseLatency != null ? 'passed' : 'not_reached'

  // 失败原因优先用上游细节；网关本地失败则展示 phase + 消息，便于一眼定位。
  const failureDetail = d.upstream_error_message || d.message

  return [
    {
      key: 'auth',
      label: t('admin.ops.errorDetail.auth'),
      status: authStatus,
      latency: authLatency,
      detail: authStatus === 'failed' ? failureDetail : undefined,
    },
    {
      key: 'routing',
      label: t('admin.ops.errorDetail.routing'),
      status: routingStatus,
      latency: routingLatency,
      detail: routingStatus === 'failed' ? failureDetail : undefined,
      subline: selectionSummary.value || undefined,
    },
    {
      key: 'upstream',
      label: t('admin.ops.errorDetail.upstream'),
      status: upstreamStatus,
      latency: upstreamLatency,
      detail: upstreamStatus === 'failed' || upstreamStatus === 'blocked' ? failureDetail : undefined,
      subline: upstreamAttempts.value.length
        ? t('admin.ops.errorDetail.flowAttemptsSummary', { n: upstreamAttempts.value.length, m: new Set(upstreamAttempts.value.map((a) => a.accountLabel)).size })
        : undefined,
    },
    {
      key: 'response',
      label: t('admin.ops.errorDetail.response'),
      status: responseStatus,
      latency: responseLatency,
    },
  ]
})

const correlatedUpstream = ref<OpsErrorDetail[]>([])
const correlatedUpstreamLoading = ref(false)

const correlatedUpstreamErrors = computed<OpsErrorDetail[]>(() => correlatedUpstream.value)

const expandedUpstreamDetailIds = ref(new Set<number>())

function getUpstreamResponsePreview(ev: OpsErrorDetail): string {
  const upstreamPayload = resolveUpstreamPayload(ev)
  if (upstreamPayload) return upstreamPayload
  return String(ev.error_body || '').trim()
}

function toggleUpstreamDetail(id: number) {
  const next = new Set(expandedUpstreamDetailIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  expandedUpstreamDetailIds.value = next
}

async function fetchCorrelatedUpstreamErrors(requestErrorId: number) {
  correlatedUpstreamLoading.value = true
  try {
    const res = await opsAPI.listRequestErrorUpstreamErrors(
      requestErrorId,
      { page: 1, page_size: 100, view: 'all' },
      { include_detail: true }
    )
    correlatedUpstream.value = res.items || []
  } catch (err) {
    console.error('[OpsErrorDetailModal] Failed to load correlated upstream errors', err)
    correlatedUpstream.value = []
  } finally {
    correlatedUpstreamLoading.value = false
  }
}

function close() {
  emit('update:show', false)
}

function prettyJSON(raw?: string): string {
  if (!raw) return 'N/A'
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  } catch {
    return raw
  }
}

async function fetchDetail(id: number) {
  loading.value = true
  try {
    const kind = props.errorType || (detail.value?.phase === 'upstream' ? 'upstream' : 'request')
    const d = kind === 'upstream' ? await opsAPI.getUpstreamErrorDetail(id) : await opsAPI.getRequestErrorDetail(id)
    detail.value = d
  } catch (err: any) {
    detail.value = null
    appStore.showError(err?.message || t('admin.ops.failedToLoadErrorDetail'))
  } finally {
    loading.value = false
  }
}

watch(
  () => [props.show, props.errorId] as const,
  ([show, id]) => {
    if (!show) {
      detail.value = null
      return
    }
    if (typeof id === 'number' && id > 0) {
      expandedUpstreamDetailIds.value = new Set()
      fetchDetail(id)
      if (props.errorType === 'request') {
        fetchCorrelatedUpstreamErrors(id)
      } else {
        correlatedUpstream.value = []
      }
    }
  },
  { immediate: true }
)

const statusClass = computed(() => {
  const code = detail.value?.status_code ?? 0
  if (code >= 500) return 'bg-red-50 text-red-700 ring-red-600/20 dark:bg-red-900/30 dark:text-red-400 dark:ring-red-500/30'
  if (code === 429) return 'bg-purple-50 text-purple-700 ring-purple-600/20 dark:bg-purple-900/30 dark:text-purple-400 dark:ring-purple-500/30'
  if (code >= 400) return 'bg-amber-50 text-amber-700 ring-amber-600/20 dark:bg-amber-900/30 dark:text-amber-400 dark:ring-amber-500/30'
  return 'bg-gray-50 text-gray-700 ring-gray-600/20 dark:bg-gray-900/30 dark:text-gray-400 dark:ring-gray-500/30'
})

</script>
