<template>
  <section class="card" data-test="retrospective-risk-panel">
    <div class="flex flex-col gap-4 border-b border-gray-100 px-6 py-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
      <div>
        <div class="flex flex-wrap items-center gap-2">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.retrospective.title') }}</h2>
          <span :class="statusBadgeClass" class="inline-flex rounded-full px-2.5 py-1 text-xs font-medium">{{ statusBadge }}</span>
        </div>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.retrospective.description') }}</p>
      </div>

      <div class="flex flex-wrap gap-2">
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="load">{{ t('common.refresh') }}</button>
        <button type="button" class="btn btn-secondary" :disabled="running || !status?.configured" @click="triggerReview">
          {{ running ? t('admin.riskControl.retrospective.triggering') : t('admin.riskControl.retrospective.trigger') }}
        </button>
        <button type="button" class="btn btn-primary" :disabled="saving || !status?.configured" @click="save">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </div>

    <div class="space-y-5 p-6">
      <div class="grid grid-cols-2 gap-3 lg:grid-cols-5">
        <div v-for="metric in metrics" :key="metric.label" class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700/50">
          <p class="text-xs text-gray-500 dark:text-gray-400">{{ metric.label }}</p>
          <p class="mt-1 truncate text-lg font-semibold text-gray-900 dark:text-white">{{ metric.value }}</p>
        </div>
      </div>

      <div class="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(340px,0.8fr)]">
        <div class="space-y-4 rounded-lg border border-gray-100 p-4 dark:border-dark-700">
          <div class="flex items-center justify-between gap-4">
            <div>
              <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.retrospective.capture') }}</p>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.retrospective.captureHint') }}</p>
            </div>
            <Toggle v-model="form.capture_enabled" />
          </div>
          <div class="flex items-center justify-between gap-4">
            <div>
              <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.retrospective.review') }}</p>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.retrospective.reviewHint') }}</p>
            </div>
            <Toggle v-model="form.review_enabled" />
          </div>
          <div class="flex items-center justify-between gap-4">
            <div>
              <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.riskControl.retrospective.elevation') }}</p>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.retrospective.elevationHint') }}</p>
            </div>
            <Toggle v-model="form.elevation_enabled" :disabled="!form.review_enabled" />
          </div>
          <div class="flex items-center justify-between gap-4 rounded-lg border border-rose-200 bg-rose-50/50 p-3 dark:border-rose-900/40 dark:bg-rose-900/10">
            <div>
              <p class="text-sm font-medium text-rose-800 dark:text-rose-200">{{ t('admin.riskControl.retrospective.enforcement') }}</p>
              <p class="mt-1 text-xs text-rose-600 dark:text-rose-300">{{ t('admin.riskControl.retrospective.enforcementHint') }}</p>
            </div>
            <Toggle v-model="form.enforcement_enabled" :disabled="!form.review_enabled || !form.elevation_enabled" />
          </div>
          <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
            <label class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.riskControl.retrospective.sampleRate') }}
              <input v-model.number="form.baseline_sample_rate" class="input mt-1" type="number" min="0" max="100" />
            </label>
            <label class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.riskControl.retrospective.dailyItems') }}
              <input v-model.number="form.per_user_daily_items" class="input mt-1" type="number" min="1" max="100" />
            </label>
            <label class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.riskControl.retrospective.reviewHour') }}
              <input v-model.number="form.review_hour" class="input mt-1" type="number" min="0" max="23" />
            </label>
            <label class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.riskControl.retrospective.maxItems') }}
              <input v-model.number="form.max_review_items" class="input mt-1" type="number" min="1" max="10000" />
            </label>
            <label class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.riskControl.retrospective.queueSize') }}
              <input v-model.number="form.queue_size" class="input mt-1" type="number" min="1" max="8192" />
            </label>
            <label class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.riskControl.retrospective.dailyBytes') }}
              <input v-model.number="form.per_user_daily_bytes" class="input mt-1" type="number" min="1" max="8388608" />
            </label>
            <label class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.riskControl.retrospective.maxItemBytes') }}
              <input v-model.number="form.max_item_bytes" class="input mt-1" type="number" min="1" max="65536" />
            </label>
            <label class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.riskControl.retrospective.pendingDays') }}
              <input v-model.number="form.pending_retention_days" class="input mt-1" type="number" min="1" max="3" />
            </label>
            <label class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.riskControl.retrospective.flaggedDays') }}
              <input v-model.number="form.flagged_retention_days" class="input mt-1" type="number" min="1" max="30" />
            </label>
            <label class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.riskControl.retrospective.maxTokens') }}
              <input v-model.number="form.max_review_input_tokens" class="input mt-1" type="number" min="1" max="20000000" />
            </label>
            <label class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.riskControl.retrospective.costBudget') }}
              <input v-model.number="form.max_review_cost_units" class="input mt-1" type="number" min="0" step="0.01" />
            </label>
            <label class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.riskControl.retrospective.riskTTL') }}
              <input v-model.number="form.risk_ttl_hours" class="input mt-1" type="number" min="1" max="720" />
            </label>
          </div>
        </div>

        <div class="rounded-lg border border-gray-100 p-4 dark:border-dark-700">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.retrospective.latestRun') }}</h3>
          <dl v-if="status?.latest_review_run" class="mt-3 grid grid-cols-2 gap-3 text-sm">
            <div><dt class="text-xs text-gray-500">{{ t('admin.riskControl.retrospective.runStatus') }}</dt><dd class="mt-1 font-medium">{{ status.latest_review_run.status }}</dd></div>
            <div><dt class="text-xs text-gray-500">{{ t('admin.riskControl.retrospective.reviewed') }}</dt><dd class="mt-1 font-medium">{{ status.latest_review_run.reviewed_count }}</dd></div>
            <div><dt class="text-xs text-gray-500">{{ t('admin.riskControl.retrospective.flagged') }}</dt><dd class="mt-1 font-medium text-rose-600">{{ status.latest_review_run.flagged_count }}</dd></div>
            <div><dt class="text-xs text-gray-500">{{ t('admin.riskControl.retrospective.failed') }}</dt><dd class="mt-1 font-medium text-amber-600">{{ status.latest_review_run.failed_count }}</dd></div>
          </dl>
          <p v-else class="mt-3 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.riskControl.retrospective.noRuns') }}</p>
        </div>
      </div>

      <div class="overflow-hidden rounded-lg border border-gray-100 dark:border-dark-700" data-test="retrospective-evidence-cases">
        <div class="flex items-center justify-between gap-4 border-b border-gray-100 px-4 py-3 dark:border-dark-700">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.retrospective.evidenceCases') }}</h3>
          <span class="shrink-0 text-xs text-gray-500">{{ evidenceCases.length }} / {{ evidenceTotal }}</span>
        </div>
        <div v-if="evidenceCases.length" class="max-h-[420px] overflow-auto">
          <table class="w-full min-w-[860px] table-fixed text-left text-sm">
            <thead class="sticky top-0 z-10 bg-gray-50 text-xs uppercase text-gray-500 dark:bg-dark-700">
              <tr>
                <th class="w-[220px] px-4 py-3">ID</th>
                <th class="w-[220px] px-4 py-3">{{ t('admin.riskControl.retrospective.user') }}</th>
                <th class="px-4 py-3">{{ t('admin.riskControl.retrospective.reason') }}</th>
                <th class="w-[100px] px-4 py-3 text-right">{{ t('admin.riskControl.retrospective.action') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="item in evidenceCases" :key="item.id" class="align-top">
                <td class="px-4 py-3">
                  <p class="truncate font-mono text-xs text-gray-900 dark:text-white" :title="item.id">{{ item.id }}</p>
                  <p class="mt-1 whitespace-nowrap text-xs text-gray-500">{{ formatTime(item.captured_at) }}</p>
                </td>
                <td class="px-4 py-3 text-xs">
                  <p class="truncate text-gray-900 dark:text-white" :title="item.user_email || `#${item.user_id}`">{{ item.user_email || `#${item.user_id}` }}</p>
                </td>
                <td class="px-4 py-3 text-xs text-gray-500">
                  <p class="line-clamp-2 break-words" :title="item.redacted_summary || item.reason_codes.join(', ') || '-'">{{ item.redacted_summary || item.reason_codes.join(', ') || '-' }}</p>
                </td>
                <td class="whitespace-nowrap px-4 py-3 text-right">
                  <button type="button" class="text-xs font-medium text-primary-600 hover:text-primary-700" @click="openEvidenceAuthorization(item)">{{ t('admin.riskControl.retrospective.viewRaw') }}</button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-else class="m-4 rounded-lg bg-gray-50 p-4 text-sm text-gray-500 dark:bg-dark-700/50 dark:text-gray-400">{{ t('admin.riskControl.retrospective.noEvidence') }}</p>
      </div>

      <div>
        <div class="mb-3 flex items-center justify-between">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.retrospective.riskProfiles') }}</h3>
          <span class="text-xs text-gray-500">{{ profiles.length }} / {{ profileTotal }}</span>
        </div>
        <div v-if="profiles.length" class="overflow-x-auto">
          <table class="min-w-full text-left text-sm">
            <thead class="text-xs uppercase text-gray-500"><tr><th class="pb-2 pr-4">{{ t('admin.riskControl.retrospective.user') }}</th><th class="pb-2 pr-4">{{ t('admin.riskControl.retrospective.level') }}</th><th class="pb-2 pr-4">{{ t('admin.riskControl.retrospective.reason') }}</th><th class="pb-2">{{ t('admin.riskControl.retrospective.action') }}</th></tr></thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="profile in profiles" :key="profile.user_id">
                <td class="py-3 pr-4"><p class="font-medium text-gray-900 dark:text-white">{{ profile.email || profile.username || `#${profile.user_id}` }}</p><p class="text-xs text-gray-500">#{{ profile.user_id }}</p></td>
                <td class="py-3 pr-4"><span class="rounded-full bg-rose-50 px-2 py-1 text-xs font-medium text-rose-700 dark:bg-rose-900/20 dark:text-rose-300">{{ profile.level }}</span></td>
                <td class="max-w-[360px] py-3 pr-4 text-xs text-gray-500">{{ profile.reason_codes.join(', ') || '-' }}</td>
                <td class="py-3"><div class="flex flex-wrap gap-2">
                  <button type="button" class="text-xs font-medium text-rose-600" @click="reviewProfile(profile, 'high', 'confirmed')">{{ t('admin.riskControl.retrospective.confirm') }}</button>
                  <button type="button" class="text-xs font-medium text-amber-600" @click="reviewProfile(profile, 'watch', 'confirmed')">{{ t('admin.riskControl.retrospective.downgrade') }}</button>
                  <button type="button" class="text-xs font-medium text-primary-600" @click="reviewProfile(profile, 'normal', 'cleared')">{{ t('admin.riskControl.retrospective.clear') }}</button>
                </div></td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-else class="rounded-lg bg-gray-50 p-4 text-sm text-gray-500 dark:bg-dark-700/50 dark:text-gray-400">{{ t('admin.riskControl.retrospective.noProfiles') }}</p>
      </div>

      <div class="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <div class="rounded-lg border border-gray-100 p-4 dark:border-dark-700">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.retrospective.runHistory') }}</h3>
          <div v-if="runs.length" class="mt-3 space-y-2">
            <div v-for="run in runs" :key="run.id" class="flex items-center justify-between gap-4 rounded-lg bg-gray-50 px-3 py-2 text-xs dark:bg-dark-700/50">
              <div><p class="font-medium text-gray-900 dark:text-white">{{ run.status }}</p><p class="text-gray-500">{{ formatTime(run.started_at) }}</p></div>
              <p class="text-right text-gray-500">{{ run.reviewed_count }} / {{ run.flagged_count }} {{ t('admin.riskControl.retrospective.flagged') }}</p>
            </div>
          </div>
          <p v-else class="mt-3 text-sm text-gray-500">{{ t('admin.riskControl.retrospective.noRuns') }}</p>
        </div>
        <div class="rounded-lg border border-gray-100 p-4 dark:border-dark-700">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.retrospective.riskEvents') }}</h3>
          <div v-if="events.length" class="mt-3 space-y-2">
            <div v-for="event in events" :key="event.id" class="rounded-lg bg-gray-50 px-3 py-2 text-xs dark:bg-dark-700/50">
              <div class="flex items-center justify-between gap-4"><p class="font-medium text-gray-900 dark:text-white">#{{ event.user_id }} · {{ event.previous_level }} → {{ event.new_level }}</p><p class="text-gray-500">{{ formatTime(event.created_at) }}</p></div>
              <p class="mt-1 truncate text-gray-500">{{ event.reason_codes.join(', ') || event.event_type }}</p>
            </div>
          </div>
          <p v-else class="mt-3 text-sm text-gray-500">{{ t('admin.riskControl.retrospective.noEvents') }}</p>
        </div>
      </div>
    </div>

    <div v-if="selectedEvidence" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" @click.self="closeEvidenceModal">
      <div class="w-full max-w-2xl rounded-xl bg-white p-5 shadow-xl dark:bg-dark-800">
        <div class="flex items-center justify-between"><h3 class="font-semibold text-gray-900 dark:text-white">{{ t('admin.riskControl.retrospective.rawEvidenceTitle') }}</h3><button type="button" class="text-gray-500" @click="closeEvidenceModal">×</button></div>
        <template v-if="!rawEvidence">
          <p class="mt-2 text-xs text-amber-600">{{ t('admin.riskControl.retrospective.rawEvidenceWarning') }}</p>
          <label class="mt-4 block text-xs text-gray-500">{{ t('admin.riskControl.retrospective.accessReason') }}<textarea v-model="accessReason" class="input mt-1 min-h-20" maxlength="512" /></label>
          <label class="mt-3 block text-xs text-gray-500">{{ t('admin.riskControl.retrospective.currentPassword') }}<input v-model="accessPassword" class="input mt-1" type="password" autocomplete="current-password" /></label>
          <button type="button" class="btn btn-primary mt-4" :disabled="authorizing || accessReason.trim().length < 8 || !accessPassword" @click="authorizeAndReadEvidence">{{ authorizing ? t('common.loading') : t('admin.riskControl.retrospective.authorizeOnce') }}</button>
        </template>
        <template v-else>
          <div class="mt-4 grid grid-cols-2 gap-2 text-xs text-gray-500"><span>{{ rawEvidence.protocol }}</span><span>{{ rawEvidence.model }}</span></div>
          <pre class="mt-3 max-h-[60vh] overflow-auto whitespace-pre-wrap break-words rounded-lg bg-gray-950 p-4 text-xs text-gray-100">{{ rawEvidence.content }}</pre>
        </template>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/ui/Toggle.vue'
import { riskControlAPI } from '@/api/admin/riskControl'
import type { RawEvidenceView, RetrospectiveEvidenceCase, RetrospectiveReviewRun, RetrospectiveRiskConfig, RetrospectiveRiskStatus, RiskEvent, UserRiskProfile } from '@/api/admin/riskControl'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const saving = ref(false)
const running = ref(false)
const status = ref<RetrospectiveRiskStatus | null>(null)
const profiles = ref<UserRiskProfile[]>([])
const profileTotal = ref(0)
const runs = ref<RetrospectiveReviewRun[]>([])
const events = ref<RiskEvent[]>([])
const evidenceCases = ref<RetrospectiveEvidenceCase[]>([])
const evidenceTotal = ref(0)
const selectedEvidence = ref<RetrospectiveEvidenceCase | null>(null)
const rawEvidence = ref<RawEvidenceView | null>(null)
const accessReason = ref('')
const accessPassword = ref('')
const authorizing = ref(false)

const defaults: RetrospectiveRiskConfig = {
  capture_enabled: false, review_enabled: false, elevation_enabled: false,
  baseline_sample_rate: 2, queue_size: 1024, per_user_daily_items: 20,
  per_user_daily_bytes: 1048576, max_item_bytes: 65536, pending_retention_days: 3,
  flagged_retention_days: 30, review_hour: 2, max_review_items: 1000,
  max_review_input_tokens: 2000000, max_review_cost_units: 0, risk_ttl_hours: 168,
  enforcement_enabled: false,
}
const form = reactive<RetrospectiveRiskConfig>({ ...defaults })

const statusBadge = computed(() => !status.value?.configured
  ? t('admin.riskControl.retrospective.notConfigured')
  : status.value.connectivity_ok
    ? t('admin.riskControl.retrospective.connected')
    : t('admin.riskControl.retrospective.connectionFailed'))
const statusBadgeClass = computed(() => status.value?.connectivity_ok
  ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
  : 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-300')
const metrics = computed(() => [
  { label: t('admin.riskControl.retrospective.queue'), value: `${status.value?.queue_length ?? 0}/${status.value?.queue_size ?? form.queue_size}` },
  { label: t('admin.riskControl.retrospective.stored'), value: String(status.value?.evidence_items ?? status.value?.stored ?? 0) },
  { label: t('admin.riskControl.retrospective.dropped'), value: String(status.value?.dropped ?? 0) },
  { label: t('admin.riskControl.retrospective.errors'), value: String(status.value?.storage_errors ?? 0) },
  { label: t('admin.riskControl.retrospective.activeProfiles'), value: String(status.value?.active_risk_profiles ?? 0) },
])

function formatTime(value: string) {
  return new Date(value).toLocaleString()
}

async function load() {
  loading.value = true
  try {
    const [config, runtime, list, runList, eventList, evidenceList] = await Promise.all([
      riskControlAPI.getConfig(), riskControlAPI.getRetrospectiveStatus(), riskControlAPI.listRiskProfiles({ page: 1, page_size: 20 }),
      riskControlAPI.listRetrospectiveReviewRuns({ page: 1, page_size: 5 }), riskControlAPI.listRiskEvents({ page: 1, page_size: 5 }),
      riskControlAPI.listRetrospectiveEvidence({ page: 1, page_size: 20 }),
    ])
    Object.assign(form, defaults, config.retrospective || {})
    status.value = runtime
    profiles.value = list.items
    profileTotal.value = list.total
    runs.value = runList.items
    events.value = eventList.items
    evidenceCases.value = evidenceList.items
    evidenceTotal.value = evidenceList.total
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.riskControl.retrospective.loadFailed')))
  } finally { loading.value = false }
}

function openEvidenceAuthorization(item: RetrospectiveEvidenceCase) {
  selectedEvidence.value = item
  rawEvidence.value = null
  accessReason.value = ''
  accessPassword.value = ''
}

function closeEvidenceModal() {
  selectedEvidence.value = null
  rawEvidence.value = null
  accessReason.value = ''
  accessPassword.value = ''
}

async function authorizeAndReadEvidence() {
  if (!selectedEvidence.value) return
  authorizing.value = true
  try {
    const grant = await riskControlAPI.authorizeRetrospectiveEvidence(selectedEvidence.value.id, { password: accessPassword.value, reason: accessReason.value.trim() })
    accessPassword.value = ''
    rawEvidence.value = await riskControlAPI.getRawRetrospectiveEvidence(selectedEvidence.value.id, grant.token)
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.riskControl.retrospective.rawEvidenceFailed')))
  } finally { authorizing.value = false }
}

async function save() {
  if (form.elevation_enabled && !form.review_enabled) form.elevation_enabled = false
  if (!form.elevation_enabled || !form.review_enabled) form.enforcement_enabled = false
  saving.value = true
  try {
    await riskControlAPI.updateConfig({ retrospective: { ...form } })
    appStore.showSuccess(t('admin.riskControl.retrospective.saved'))
    await load()
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.riskControl.retrospective.saveFailed')))
  } finally { saving.value = false }
}

async function triggerReview() {
  running.value = true
  try {
    const result = await riskControlAPI.triggerRetrospectiveReview()
    appStore.showSuccess(result.accepted ? t('admin.riskControl.retrospective.triggered') : t('admin.riskControl.retrospective.alreadyRunning'))
    setTimeout(() => void load(), 1500)
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.riskControl.retrospective.triggerFailed')))
  } finally { running.value = false }
}

async function reviewProfile(profile: UserRiskProfile, level: UserRiskProfile['level'], reviewStatus: string) {
  const note = window.prompt(t('admin.riskControl.retrospective.reviewNotePrompt'), '')
  if (note === null || note.trim().length < 3) return
  try {
    await riskControlAPI.reviewRiskProfile(profile.user_id, { level, review_status: reviewStatus, note: note.trim() })
    await load()
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.riskControl.retrospective.saveFailed')))
  }
}

onMounted(() => void load())
</script>
