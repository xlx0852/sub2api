<template>
  <Teleport to="body">
    <div
      v-if="modelValue && log"
      class="fixed inset-0 z-[100] flex items-center justify-center bg-black/40 p-4"
      role="dialog"
      aria-modal="true"
      @click.self="close"
    >
      <div class="max-h-[min(900px,calc(100vh-2rem))] w-full max-w-5xl overflow-y-auto rounded-2xl bg-white shadow-2xl dark:bg-dark-800">
        <header class="flex items-center justify-between border-b border-gray-200 px-6 py-5 dark:border-dark-600">
          <h2 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('usage.timingDetail.title') }}</h2>
          <button type="button" class="rounded-lg p-2 text-gray-400 hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-700" @click="close">
            <Icon name="x" size="md" />
          </button>
        </header>

        <div class="space-y-5 p-6">
          <section class="rounded-xl bg-gray-50 px-5 py-4 dark:bg-dark-700/60">
            <div class="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ log.model || '-' }}</h3>
                <div class="mt-1 break-all font-mono text-xs text-gray-500 dark:text-gray-400">{{ log.request_id || '-' }}</div>
              </div>
              <time class="text-sm text-gray-500 dark:text-gray-400">{{ formatDate(log.created_at) }}</time>
            </div>
          </section>

          <section class="grid grid-cols-1 gap-3 md:grid-cols-3">
            <MetricCard :label="t('usage.timingDetail.firstToken')" :value="formatDuration(log.first_token_ms)" />
            <MetricCard :label="t('usage.timingDetail.totalDuration')" :value="formatDuration(log.duration_ms)" />
            <MetricCard :label="t('usage.timingDetail.firstOutputTotal')" :value="formatDuration(stages?.gateway_first_output_total_ms)" />
          </section>

          <section class="grid grid-cols-1 gap-4 lg:grid-cols-3">
            <TimingSection :title="t('usage.timingDetail.clientToGateway')">
              <TimingRow :label="t('usage.timingDetail.bodyFirstWait')" :value="msValue(stages?.body_first_wait_ms)" />
              <TimingRow :label="t('usage.timingDetail.bodyReceive')" :value="msValue(stages?.body_receive_ms)" />
              <TimingRow :label="t('usage.timingDetail.bodySize')" :value="bodySize" />
              <TimingRow :label="t('usage.timingDetail.bodyRate')" :value="bodyRate" />
              <TimingRow :label="t('usage.timingDetail.inboundPath')" :value="log.inbound_endpoint || '-'" />
              <TimingRow :label="t('usage.timingDetail.requestType')" :value="requestTypeLabel" />
            </TimingSection>

            <TimingSection :title="t('usage.timingDetail.gatewayInternal')">
              <TimingRow :label="t('usage.timingDetail.handlerBodyRead')" :value="msValue(stages?.handler_body_read_ms)" />
              <TimingRow :label="t('usage.timingDetail.preprocessAuth')" :value="msValue(stages?.api_key_auth_ms)" />
              <TimingRow :label="t('usage.timingDetail.securityAudit')" :value="notCollected" />
              <TimingRow :label="t('usage.timingDetail.routing')" :value="msValue(stages?.account_select_ms)" />
              <TimingRow :label="t('usage.timingDetail.userQueue')" :value="msValue(stages?.user_queue_ms)" />
              <TimingRow :label="t('usage.timingDetail.accountQueue')" :value="msValue(stages?.account_queue_ms)" />
              <TimingRow :label="t('usage.timingDetail.largeBodyQueue')" :value="notCollected" />
              <TimingRow :label="t('usage.timingDetail.handlerTotal')" :value="msValue(stages?.handler_total_ms)" />
            </TimingSection>

            <TimingSection :title="t('usage.timingDetail.gatewayToUpstream')">
              <TimingRow :label="t('usage.timingDetail.connectionAcquire')" :value="msValue(stages?.upstream_connect_ms)" />
              <TimingRow :label="t('usage.timingDetail.upstreamWrite')" :value="msValue(stages?.upstream_write_ms)" />
              <TimingRow :label="t('usage.timingDetail.upstreamFirstByte')" :value="msValue(stages?.upstream_first_byte_ms)" />
              <TimingRow :label="t('usage.timingDetail.upstreamResponseHeaders')" :value="msValue(stages?.upstream_response_headers_ms)" />
              <TimingRow :label="t('usage.timingDetail.firstToken')" :value="msValue(log.first_token_ms)" />
              <TimingRow :label="t('usage.timingDetail.streamTransfer')" :value="msValue(stages?.upstream_stream_transfer_ms)" />
              <TimingRow :label="t('usage.timingDetail.upstreamPath')" :value="log.upstream_endpoint || '-'" />
            </TimingSection>
          </section>

          <section class="rounded-xl border border-gray-200 p-5 dark:border-dark-600">
            <h3 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">{{ t('usage.timingDetail.result') }}</h3>
            <div class="grid grid-cols-1 gap-x-8 gap-y-3 text-sm md:grid-cols-2">
              <TimingRow :label="t('usage.timingDetail.status')" :value="outcomeLabel" />
              <TimingRow :label="t('usage.timingDetail.attempts')" :value="countValue(stages?.upstream_attempts)" />
              <TimingRow :label="t('usage.timingDetail.connectionReused')" :value="boolValue(stages?.connection_reused)" />
              <TimingRow :label="t('usage.timingDetail.bodyComplete')" :value="boolValue(stages?.body_complete)" />
              <TimingRow :label="t('usage.timingDetail.responseComplete')" :value="boolValue(stages?.response_stream_complete)" />
              <TimingRow :label="t('usage.timingDetail.outputRate')" :value="outputRate" />
              <TimingRow :label="t('usage.tokens')" :value="`${log.input_tokens || 0} / ${log.output_tokens || 0}`" />
              <TimingRow :label="t('usage.cacheHit')" :value="cacheHit" />
              <TimingRow :label="t('usage.timingDetail.mapping')" :value="mapping" />
            </div>
          </section>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, defineComponent, h } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { UsageLog } from '@/types'

defineOptions({ name: 'UsageRequestTimingModal' })

const props = defineProps<{
  modelValue: boolean
  log: UsageLog | null
}>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
const { t } = useI18n()

const MetricCard = defineComponent({
  props: { label: String, value: String },
  setup(cardProps) {
    return () => h('div', { class: 'rounded-xl border border-gray-200 px-5 py-4 dark:border-dark-600' }, [
      h('div', { class: 'text-sm text-gray-500 dark:text-gray-400' }, cardProps.label),
      h('div', { class: 'mt-2 text-2xl font-semibold text-gray-900 dark:text-white' }, cardProps.value),
    ])
  },
})
const TimingSection = defineComponent({
  props: { title: String },
  setup(sectionProps, { slots }) {
    return () => h('section', { class: 'rounded-xl border border-gray-200 p-5 dark:border-dark-600' }, [
      h('h3', { class: 'mb-4 text-lg font-semibold text-gray-900 dark:text-white' }, sectionProps.title),
      h('div', { class: 'space-y-3' }, slots.default?.()),
    ])
  },
})
const TimingRow = defineComponent({
  props: { label: String, value: String },
  setup(rowProps) {
    return () => h('div', { class: 'flex items-start justify-between gap-3 text-sm' }, [
      h('span', { class: 'text-gray-500 dark:text-gray-400' }, rowProps.label),
      h('span', { class: 'break-all text-right font-medium text-gray-900 dark:text-white' }, rowProps.value),
    ])
  },
})

const stages = computed(() => props.log?.first_output_stages || null)
const requestTypeLabel = computed(() => {
  if (!props.log) return '-'
  return props.log.request_type || (props.log.stream ? t('usage.stream') : t('usage.sync'))
})
const mapping = computed(() => {
  const log = props.log
  if (!log) return '-'
  const admin = log as UsageLog & { model_mapping_chain?: string | null; upstream_model?: string | null }
  return admin.model_mapping_chain || admin.upstream_model || log.model || '-'
})
const cacheHit = computed(() => {
  if (!props.log) return '-'
  return props.log.cache_read_tokens > 0 ? formatTokens(props.log.cache_read_tokens) : t('usage.timingDetail.noCacheHit')
})
const outputRate = computed(() => {
  if (!props.log || props.log.duration_ms == null || props.log.first_token_ms == null || props.log.output_tokens <= 0) return '-'
  const generationMs = props.log.duration_ms - props.log.first_token_ms
  if (generationMs <= 0) return '-'
  return `${((props.log.output_tokens * 1000) / generationMs).toFixed(2)} tok/s`
})

function close() {
  emit('update:modelValue', false)
}
function formatDuration(ms: number | null | undefined): string {
  if (ms == null) return '-'
  if (ms < 1000) return `${ms}ms`
  if (ms >= 60_000) return `${Math.floor(ms / 60_000)}m ${Math.floor((ms % 60_000) / 1000)}s`
  return `${(ms / 1000).toFixed(2)}s`
}
const notCollected = computed(() => t('usage.timingDetail.notCollected'))
function msValue(ms: number | null | undefined): string {
  if (ms == null) return t('usage.timingDetail.notCollected')
  return formatDuration(ms)
}
function countValue(value: number | null | undefined): string {
  if (value == null) return t('usage.timingDetail.notCollected')
  return String(value)
}
function boolValue(value: boolean | null | undefined): string {
  if (value == null) return t('usage.timingDetail.notCollected')
  return value ? t('usage.timingDetail.yes') : t('usage.timingDetail.no')
}
const outcomeLabel = computed(() => {
  const outcome = stages.value?.outcome
  if (!outcome) return t('usage.timingDetail.notCollected')
  const map: Record<string, string> = {
    success: 'usage.timingDetail.outcomeSuccess',
    failed: 'usage.timingDetail.outcomeFailed',
    canceled: 'usage.timingDetail.outcomeCanceled',
    timeout: 'usage.timingDetail.outcomeTimeout',
    transport_error: 'usage.timingDetail.outcomeTransportError',
    incomplete: 'usage.timingDetail.outcomeIncomplete'
  }
  const key = map[outcome]
  return key ? t(key) : outcome
})
const bodySize = computed(() => {
  const bytes = stages.value?.body_bytes
  if (bytes == null) return t('usage.timingDetail.notCollected')
  return formatBytes(bytes)
})
const bodyRate = computed(() => {
  const bytes = stages.value?.body_bytes
  const receive = stages.value?.body_receive_ms
  if (bytes == null || receive == null) return t('usage.timingDetail.notCollected')
  if (receive < 1) return t('usage.timingDetail.notCollected')
  return `${(bytes / 1024 / 1024 / (receive / 1000)).toFixed(2)} MiB/s`
})
function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`
  return `${(bytes / 1024 / 1024).toFixed(2)} MiB`
}
function formatTokens(value: number): string {
  return value.toLocaleString()
}
function formatDate(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}
</script>
