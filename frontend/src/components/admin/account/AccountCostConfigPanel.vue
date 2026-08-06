<template v-if="active">
  <div class="space-y-4" data-testid="account-cost-config-panel">
    <div v-if="showAccountHeader" class="text-sm text-gray-600 dark:text-dark-300">
      {{ accountName }} <span class="text-gray-400">#{{ accountId }}</span>
    </div>

    <div class="rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 dark:border-dark-600 dark:bg-dark-700">
      <div class="font-medium text-gray-800 dark:text-dark-100">{{ isSubscription ? t('admin.profit.subscription') : t('admin.profit.metered') }}</div>
      <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ isSubscription ? t('admin.profit.costTypeAutoSubscription') : t('admin.profit.costTypeAutoMetered') }}</p>
    </div>

    <div v-if="isGrokSubscription" class="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800 dark:border-amber-900/60 dark:bg-amber-900/10 dark:text-amber-300">
      <div class="font-semibold">{{ t('admin.profit.grokCycleTitle') }}</div>
      <p>{{ t('admin.profit.grokCycleHint') }}</p>
    </div>

    <template v-if="isSubscription">
      <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
        <div class="mb-3 flex items-center justify-between">
          <span class="text-sm font-semibold text-gray-800 dark:text-dark-100">{{ t('admin.profit.subscriptionCycles') }}</span>
          <span class="text-xs text-gray-400">{{ cycles.length }}</span>
        </div>
        <div v-if="cycles.length" class="space-y-2">
          <div v-for="cycle in cycles" :key="cycle.id" class="rounded-md px-2.5 py-2 text-xs" :class="cycle.termination ? 'border border-red-200 bg-red-50/70 dark:border-red-900/50 dark:bg-red-950/20' : 'bg-gray-50 dark:bg-dark-700'">
            <div class="flex items-start justify-between gap-3">
              <div>
                <div class="flex flex-wrap items-center gap-2 font-medium text-gray-800 dark:text-dark-100">
                  <span>{{ cycle.starts_at.slice(0, 10) }} · {{ cycle.period_days }}{{ t('admin.profit.daysUnit') }}</span>
                  <span v-if="cycle.termination" class="rounded bg-red-100 px-1.5 py-0.5 text-[10px] font-semibold text-red-700 dark:bg-red-950/60 dark:text-red-300">{{ t('admin.profit.banSettled') }}</span>
                </div>
                <div class="mt-0.5 text-gray-500 dark:text-dark-400">${{ cycle.period_fee.toFixed(2) }} {{ cycle.currency }}<span v-if="cycle.notes"> · {{ cycle.notes }}</span></div>
              </div>
              <div class="flex shrink-0 items-center gap-3">
                <button v-if="!cycle.termination && !compact" data-testid="settle-ban" type="button" class="font-medium text-red-600 hover:text-red-700" @click="openTermination(cycle)">{{ t('admin.profit.settleBan') }}</button>
                <button v-if="!cycle.termination" type="button" class="text-red-500 hover:text-red-600" @click="removeCycle(cycle.id)">{{ t('admin.profit.delete') }}</button>
              </div>
            </div>

            <template v-if="cycle.termination && !compact">
              <div class="mt-2 border-t border-red-100 pt-2 text-red-800 dark:border-red-900/50 dark:text-red-200">
                <div>{{ t('admin.profit.bannedAt') }}: {{ formatDateTime(cycle.termination.effective_at) }}</div>
                <div v-if="cycle.termination.notes" class="mt-0.5 text-red-600 dark:text-red-300">{{ cycle.termination.notes }}</div>
              </div>
              <div v-if="cycle.loss_summary" class="mt-2 grid grid-cols-2 gap-2 sm:grid-cols-4">
                <MetricItem :label="t('admin.profit.revenueBeforeBan')" :value="money(cycle.loss_summary.revenue_before_ban)" />
                <MetricItem :label="t('admin.profit.receivedRefund')" :value="money(cycle.loss_summary.refund_total)" />
                <MetricItem :label="t('admin.profit.netPurchaseCost')" :value="money(cycle.loss_summary.net_purchase_cost)" />
                <MetricItem :label="t('admin.profit.confirmedLoss')" :value="money(cycle.loss_summary.realized_loss)" danger />
              </div>
              <div v-if="cycle.loss_summary" class="mt-2">
                <div class="mb-1 flex justify-between text-[10px] text-gray-500 dark:text-dark-400">
                  <span>{{ t('admin.profit.recoveryProgress') }}</span>
                  <span>{{ cycle.loss_summary.recovery_progress.toFixed(1) }}%</span>
                </div>
                <div class="h-1.5 overflow-hidden rounded-full bg-red-100 dark:bg-red-950/50">
                  <div class="h-full rounded-full bg-red-500" :style="{ width: `${Math.min(100, cycle.loss_summary.recovery_progress)}%` }" />
                </div>
              </div>
              <div v-if="cycle.refunds?.length" class="mt-2 space-y-1 border-t border-red-100 pt-2 dark:border-red-900/50">
                <div v-for="refund in cycle.refunds" :key="refund.id" class="flex items-center justify-between gap-2" :class="refund.voided_at ? 'text-gray-400 line-through' : 'text-gray-600 dark:text-dark-300'">
                  <span>{{ formatDateTime(refund.received_at) }} · {{ money(refund.amount) }}<span v-if="refund.notes"> · {{ refund.notes }}</span></span>
                  <button v-if="!refund.voided_at" type="button" class="shrink-0 text-gray-500 hover:text-red-600" @click="openCorrection('void-refund', refund.id)">{{ t('admin.profit.voidRefund') }}</button>
                </div>
              </div>
              <div class="mt-2 flex flex-wrap gap-3">
                <button type="button" class="font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400" @click="openRefund(cycle)">{{ t('admin.profit.addReceivedRefund') }}</button>
                <button type="button" class="text-gray-500 hover:text-red-600" @click="openCorrection('reverse-termination', cycle.termination.id)">{{ t('admin.profit.reverseBanSettlement') }}</button>
              </div>
            </template>
          </div>
        </div>
        <p v-else class="text-xs text-amber-600 dark:text-amber-400">{{ t('admin.profit.noSubscriptionCycles') }}</p>
      </div>

      <div v-if="terminationCycle" data-testid="termination-form" class="rounded-lg border border-red-300 bg-red-50/70 p-3 dark:border-red-900/60 dark:bg-red-950/20">
        <div class="mb-3 flex items-center justify-between">
          <div>
            <div class="text-sm font-semibold text-red-800 dark:text-red-200">{{ t('admin.profit.settleBanTitle') }}</div>
            <p class="mt-0.5 text-xs text-red-600 dark:text-red-300">{{ t('admin.profit.settleBanStopWarning') }}</p>
          </div>
          <button type="button" class="text-gray-400 hover:text-gray-600" @click="terminationCycle = null">×</button>
        </div>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div><label class="label">{{ t('admin.profit.bannedAt') }}</label><input v-model="terminationForm.effective_at" type="datetime-local" class="input" /></div>
          <div><label class="label">{{ t('admin.profit.initialReceivedRefund') }}</label><input v-model.number="terminationForm.initial_refund_amount" type="number" min="0" :max="terminationCycle.period_fee" step="0.01" class="input" /></div>
        </div>
        <div class="mt-3"><label class="label">{{ t('admin.profit.banNotes') }}</label><input v-model="terminationForm.notes" type="text" class="input" /></div>
        <div v-if="terminationPreview" class="mt-3 grid grid-cols-2 gap-2 rounded-md bg-white/80 p-2 dark:bg-dark-800/70 sm:grid-cols-4">
          <MetricItem :label="t('admin.profit.revenueBeforeBan')" :value="money(terminationPreview.revenue_before_ban)" />
          <MetricItem :label="t('admin.profit.netPurchaseCost')" :value="money(terminationPreview.net_purchase_cost)" />
          <MetricItem :label="t('admin.profit.recoveryProgress')" :value="`${terminationPreview.recovery_progress.toFixed(1)}%`" />
          <MetricItem :label="t('admin.profit.estimatedConfirmedLoss')" :value="money(terminationPreview.realized_loss)" danger />
        </div>
        <div class="mt-3 flex justify-end gap-2">
          <button type="button" class="btn-secondary" @click="terminationCycle = null">{{ t('admin.profit.cancel') }}</button>
          <button data-testid="preview-ban-settlement" type="button" class="btn-danger" :disabled="saving || !terminationForm.effective_at" @click="prepareTermination">{{ t('admin.profit.previewAndSettle') }}</button>
        </div>
      </div>

      <div v-if="refundCycle" data-testid="refund-form" class="rounded-lg border border-primary-200 bg-primary-50/40 p-3 dark:border-primary-900/50 dark:bg-primary-900/10">
        <div class="mb-3 flex items-center justify-between">
          <span class="text-sm font-semibold text-gray-800 dark:text-dark-100">{{ t('admin.profit.addReceivedRefund') }}</span>
          <button type="button" class="text-gray-400 hover:text-gray-600" @click="refundCycle = null">×</button>
        </div>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div><label class="label">{{ t('admin.profit.refundAmount') }}</label><input v-model.number="refundForm.amount" type="number" min="0.01" :max="remainingRefundCapacity" step="0.01" class="input" /></div>
          <div><label class="label">{{ t('admin.profit.refundReceivedAt') }}</label><input v-model="refundForm.received_at" type="datetime-local" class="input" /></div>
        </div>
        <div class="mt-3"><label class="label">{{ t('admin.profit.notes') }}</label><input v-model="refundForm.notes" type="text" class="input" /></div>
        <div class="mt-3 flex justify-end gap-2">
          <button type="button" class="btn-secondary" @click="refundCycle = null">{{ t('admin.profit.cancel') }}</button>
          <button data-testid="save-refund" type="button" class="btn-primary" :disabled="saving || refundForm.amount <= 0 || !refundForm.received_at" @click="saveRefund">{{ t('admin.profit.confirmReceivedRefund') }}</button>
        </div>
      </div>

      <div class="flex items-start justify-between gap-3 rounded-lg border border-gray-200 px-3 py-2.5 dark:border-dark-600">
        <div class="min-w-0">
          <div class="text-sm font-semibold text-gray-800 dark:text-dark-100">{{ t('admin.profit.autoRenew') }}</div>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ t('admin.profit.autoRenewHint') }}</p>
        </div>
        <button
          type="button"
          data-testid="auto-renew-toggle"
          class="relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500/40 disabled:opacity-50"
          :class="autoRenew ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'"
          :aria-pressed="autoRenew"
          :disabled="!accountId || savingAutoRenew"
          @click="toggleAutoRenew"
        >
          <span
            class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
            :class="autoRenew ? 'translate-x-5' : 'translate-x-0'"
          />
        </button>
      </div>

      <div class="rounded-lg border border-primary-200 bg-primary-50/40 p-3 dark:border-primary-900/50 dark:bg-primary-900/10">
        <div class="mb-3 flex items-center justify-between">
          <span class="text-sm font-semibold text-gray-800 dark:text-dark-100">{{ t('admin.profit.addSubscriptionCycle') }}</span>
          <button type="button" class="text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400" @click="inferStartDate">{{ t('admin.profit.inferCycleStart') }}</button>
        </div>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <div><label class="label">{{ t('admin.profit.periodFee') }}</label><input v-model.number="form.period_fee" type="number" min="0" step="0.01" class="input" /></div>
          <div><label class="label">{{ t('admin.profit.periodDays') }}</label><input v-model.number="form.period_days" type="number" min="1" class="input" /></div>
          <div><label class="label">{{ t('admin.profit.periodStartAt') }}</label><input v-model="form.starts_at" type="date" class="input" /></div>
        </div>
        <p v-if="inferenceNote" class="mt-2 text-xs" :class="inferenceRisky ? 'text-amber-600 dark:text-amber-400' : 'text-gray-500 dark:text-dark-400'">{{ inferenceNote }}</p>
        <div class="mt-3"><label class="label">{{ t('admin.profit.notes') }}</label><input v-model="form.notes" type="text" class="input" /></div>
        <div v-if="showInlineSave" class="mt-3 flex justify-end">
          <button data-testid="save-cost-config" type="button" class="btn-primary" :disabled="saving || !form.starts_at || !accountId" @click="saveCycle">{{ t('admin.profit.save') }}</button>
        </div>
      </div>
    </template>
  </div>

  <ConfirmDialog
    :show="confirmAction !== null"
    :title="confirmTitle"
    :message="confirmMessage"
    :confirm-text="confirmText"
    :danger="true"
    @confirm="executeConfirmedAction"
    @cancel="closeConfirmation"
  >
    <div v-if="confirmAction !== 'terminate'" class="mt-3">
      <label class="label">{{ t('admin.profit.correctionReason') }}</label>
      <input v-model="correctionReason" data-testid="correction-reason" type="text" class="input" />
    </div>
  </ConfirmDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type { AccountSubscriptionCycle, AccountSubscriptionLossSummary, SubscriptionCycleListResponse } from '@/api/admin/profit'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import MetricItem from './AccountCostMetricItem.vue'

const props = withDefaults(defineProps<{
  active?: boolean
  accountId: number | null
  accountName?: string
  accountType?: string
  accountPlatform?: string
  showAccountHeader?: boolean
  showInlineSave?: boolean
  compact?: boolean
}>(), {
  active: true,
  showAccountHeader: false,
  showInlineSave: true,
  compact: false
})
const emit = defineEmits<{ saved: [payload?: { account_expires_at?: string | null }]; 'update:canSave': [boolean]; 'update:saving': [boolean] }>()
const { t } = useI18n()
const appStore = useAppStore()
const saving = ref(false)
const cycles = ref<AccountSubscriptionCycle[]>([])
const autoRenew = ref(false)
const savingAutoRenew = ref(false)
const hints = ref<SubscriptionCycleListResponse | null>(null)
const inferenceNote = ref('')
const inferenceRisky = ref(false)
const isSubscription = computed(() => props.accountType === 'oauth' || props.accountType === 'setup-token')
const isGrokSubscription = computed(() => isSubscription.value && props.accountPlatform === 'grok')
const form = ref({ period_fee: 0, period_days: 30, starts_at: '', notes: '' })
const terminationCycle = ref<AccountSubscriptionCycle | null>(null)
const terminationPreview = ref<AccountSubscriptionLossSummary | null>(null)
const terminationForm = ref({ effective_at: '', initial_refund_amount: 0, notes: '' })
const refundCycle = ref<AccountSubscriptionCycle | null>(null)
const refundForm = ref({ amount: 0, received_at: '', notes: '' })
const confirmAction = ref<'terminate' | 'reverse-termination' | 'void-refund' | null>(null)
const correctionTargetID = ref<number | null>(null)
const correctionReason = ref('')

const remainingRefundCapacity = computed(() => {
  if (!refundCycle.value) return 0
  return Math.max(0, refundCycle.value.period_fee - (refundCycle.value.loss_summary?.refund_total || 0))
})
const confirmTitle = computed(() => confirmAction.value === 'terminate' ? t('admin.profit.settleBanTitle') : confirmAction.value === 'void-refund' ? t('admin.profit.voidRefund') : t('admin.profit.reverseBanSettlement'))
const confirmText = computed(() => confirmAction.value === 'terminate' ? t('admin.profit.confirmBanAndDisable') : t('common.confirm'))
const confirmMessage = computed(() => {
  if (confirmAction.value === 'terminate') {
    return t('admin.profit.settleBanConfirmMessage', { loss: terminationPreview.value?.realized_loss.toFixed(2) || '0.00' })
  }
  return confirmAction.value === 'void-refund' ? t('admin.profit.voidRefundConfirm') : t('admin.profit.reverseBanConfirm')
})
const canSave = computed(() => isSubscription.value && !!props.accountId && !!form.value.starts_at && !saving.value)

watch(canSave, (v) => emit('update:canSave', v), { immediate: true })
watch(saving, (v) => emit('update:saving', v), { immediate: true })

async function loadCycles() {
  if (!props.accountId || !isSubscription.value) return
  const result = await adminAPI.profit.listSubscriptionCycles(props.accountId)
  cycles.value = result.cycles || []
  autoRenew.value = !!result.auto_renew
  hints.value = result
  return result
}

function emitSaved() {
  emit('saved', { account_expires_at: hints.value?.account_expires_at ?? null })
}

watch(() => [props.active, props.accountId, props.accountType, props.accountPlatform] as const, async ([visible]) => {
  if (!visible) return
  cycles.value = []
  autoRenew.value = false
  hints.value = null
  inferenceNote.value = ''
  form.value = { period_fee: 0, period_days: isGrokSubscription.value ? 31 : 30, starts_at: '', notes: '' }
  terminationCycle.value = null
  refundCycle.value = null
  closeConfirmation()
  if (!props.accountId || !isSubscription.value) return
  try { await loadCycles() } catch { appStore.showError(t('admin.profit.loadFailed')) }
}, { immediate: true })

function inferStartDate() {
  const expiry = hints.value?.subscription_expires_at || hints.value?.oauth_token_expires_at
  if (!expiry) { inferenceNote.value = t('admin.profit.inferUnavailable'); inferenceRisky.value = true; return }
  const end = new Date(expiry)
  end.setUTCDate(end.getUTCDate() - form.value.period_days)
  form.value.starts_at = end.toISOString().slice(0, 10)
  inferenceRisky.value = !hints.value?.subscription_expires_at
  inferenceNote.value = inferenceRisky.value ? t('admin.profit.inferFromTokenWarning') : t('admin.profit.inferFromSubscriptionExpiry')
}

async function toggleAutoRenew() {
  if (!props.accountId || savingAutoRenew.value) return
  const next = !autoRenew.value
  savingAutoRenew.value = true
  try {
    await adminAPI.profit.setSubscriptionAutoRenew(props.accountId, next)
    autoRenew.value = next
    appStore.showSuccess(t('admin.profit.saveSuccess'))
    emitSaved()
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, t('admin.profit.saveFailed')))
  } finally {
    savingAutoRenew.value = false
  }
}

async function saveCycle() {
  if (!props.accountId) return
  saving.value = true
  try {
    await adminAPI.profit.createSubscriptionCycle(props.accountId, { ...form.value, currency: 'USD' })
    appStore.showSuccess(t('admin.profit.saveSuccess'))
    form.value = { ...form.value, starts_at: '', notes: '' }
    await loadCycles()
    emitSaved()
  } catch (err) { appStore.showError(extractApiErrorMessage(err, t('admin.profit.saveFailed'))) } finally { saving.value = false }
}

async function removeCycle(id: number) {
  try { await adminAPI.profit.deleteSubscriptionCycle(id); await loadCycles(); emitSaved() } catch (err) { appStore.showError(extractApiErrorMessage(err, t('admin.profit.saveFailed'))) }
}

function openTermination(cycle: AccountSubscriptionCycle) {
  terminationCycle.value = cycle
  refundCycle.value = null
  terminationPreview.value = null
  const cycleEnd = new Date(cycle.starts_at)
  cycleEnd.setUTCDate(cycleEnd.getUTCDate() + cycle.period_days)
  const effective = new Date(Math.min(Date.now(), cycleEnd.getTime()))
  terminationForm.value = { effective_at: toLocalInput(effective), initial_refund_amount: 0, notes: '' }
}

async function prepareTermination() {
  if (!terminationCycle.value || !terminationForm.value.effective_at) return
  saving.value = true
  try {
    const request = terminationRequest()
    terminationPreview.value = await adminAPI.profit.previewSubscriptionTermination(terminationCycle.value.id, request)
    confirmAction.value = 'terminate'
  } catch (err) { appStore.showError(extractApiErrorMessage(err, t('admin.profit.banSettlementFailed'))) } finally { saving.value = false }
}

function terminationRequest() {
  return {
    effective_at: new Date(terminationForm.value.effective_at).toISOString(),
    reason: 'upstream_banned',
    notes: terminationForm.value.notes,
    initial_refund_amount: terminationForm.value.initial_refund_amount || 0,
    initial_refund_received_at: new Date(terminationForm.value.effective_at).toISOString()
  }
}

function openRefund(cycle: AccountSubscriptionCycle) {
  refundCycle.value = cycle
  terminationCycle.value = null
  refundForm.value = { amount: 0, received_at: toLocalInput(new Date()), notes: '' }
}

async function saveRefund() {
  if (!refundCycle.value?.termination) return
  saving.value = true
  try {
    await adminAPI.profit.addSubscriptionRefund(refundCycle.value.termination.id, {
      amount: refundForm.value.amount,
      received_at: new Date(refundForm.value.received_at).toISOString(),
      notes: refundForm.value.notes
    })
    appStore.showSuccess(t('admin.profit.refundSaved'))
    refundCycle.value = null
    await loadCycles()
    emitSaved()
  } catch (err) { appStore.showError(extractApiErrorMessage(err, t('admin.profit.refundFailed'))) } finally { saving.value = false }
}

function openCorrection(action: 'reverse-termination' | 'void-refund', id: number) {
  confirmAction.value = action
  correctionTargetID.value = id
  correctionReason.value = ''
}

function closeConfirmation() {
  confirmAction.value = null
  correctionTargetID.value = null
  correctionReason.value = ''
}

async function executeConfirmedAction() {
  if (saving.value) return
  if (confirmAction.value !== 'terminate' && !correctionReason.value.trim()) {
    appStore.showError(t('admin.profit.correctionReasonRequired'))
    return
  }
  saving.value = true
  try {
    if (confirmAction.value === 'terminate' && terminationCycle.value) {
      await adminAPI.profit.terminateSubscriptionCycle(terminationCycle.value.id, terminationRequest())
      appStore.showSuccess(t('admin.profit.banSettlementSaved'))
      terminationCycle.value = null
    } else if (confirmAction.value === 'void-refund' && correctionTargetID.value) {
      await adminAPI.profit.voidSubscriptionRefund(correctionTargetID.value, correctionReason.value.trim())
      appStore.showSuccess(t('admin.profit.refundVoided'))
    } else if (confirmAction.value === 'reverse-termination' && correctionTargetID.value) {
      await adminAPI.profit.reverseSubscriptionTermination(correctionTargetID.value, correctionReason.value.trim())
      appStore.showSuccess(t('admin.profit.banSettlementReversed'))
    }
    closeConfirmation()
    await loadCycles()
    emitSaved()
  } catch (err) { appStore.showError(extractApiErrorMessage(err, t('admin.profit.banSettlementFailed'))) } finally { saving.value = false }
}

function toLocalInput(date: Date) {
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000)
  return local.toISOString().slice(0, 16)
}
function formatDateTime(value: string) { return new Date(value).toLocaleString() }
function money(value: number) { return `$${value.toFixed(2)}` }

defineExpose({ saveCycle, canSave, isSubscription })
</script>

<style scoped>
.input { @apply w-full rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-700 dark:text-white; }
.label { @apply mb-1 block text-sm font-medium text-gray-700 dark:text-dark-200; }
.btn-secondary { @apply rounded-lg px-3 py-1.5 text-sm font-medium text-gray-600 hover:bg-gray-100 dark:text-dark-300 dark:hover:bg-dark-700; }
.btn-primary { @apply rounded-lg bg-primary-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-primary-700 disabled:opacity-50; }
.btn-danger { @apply rounded-lg bg-red-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50; }
</style>
