<template>
  <BaseDialog :show="show" :title="t('admin.profit.configTitle')" @close="emit('close')">
    <div class="space-y-4">
      <div class="text-sm text-gray-600 dark:text-dark-300">
        {{ accountName }} <span class="text-gray-400">#{{ accountId }}</span>
      </div>

      <div class="rounded-lg border border-gray-200 bg-gray-50 px-3 py-2 dark:border-dark-600 dark:bg-dark-700">
        <div class="font-medium text-gray-800 dark:text-dark-100">{{ isSubscription ? t('admin.profit.subscription') : t('admin.profit.metered') }}</div>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">{{ isSubscription ? t('admin.profit.costTypeAutoSubscription') : t('admin.profit.costTypeAutoMetered') }}</p>
      </div>

      <template v-if="isSubscription">
        <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
          <div class="mb-3 flex items-center justify-between">
            <span class="text-sm font-semibold text-gray-800 dark:text-dark-100">{{ t('admin.profit.subscriptionCycles') }}</span>
            <span class="text-xs text-gray-400">{{ cycles.length }}</span>
          </div>
          <div v-if="cycles.length" class="space-y-2">
            <div v-for="cycle in cycles" :key="cycle.id" class="flex items-center justify-between gap-3 rounded-md bg-gray-50 px-2.5 py-2 text-xs dark:bg-dark-700">
              <div>
                <div class="font-medium text-gray-800 dark:text-dark-100">{{ cycle.starts_at.slice(0, 10) }} · {{ cycle.period_days }}{{ t('admin.profit.daysUnit') }}</div>
                <div class="mt-0.5 text-gray-500 dark:text-dark-400">${{ cycle.period_fee.toFixed(2) }} {{ cycle.currency }}<span v-if="cycle.notes"> · {{ cycle.notes }}</span></div>
              </div>
              <button class="text-red-500 hover:text-red-600" @click="removeCycle(cycle.id)">{{ t('admin.profit.delete') }}</button>
            </div>
          </div>
          <p v-else class="text-xs text-amber-600 dark:text-amber-400">{{ t('admin.profit.noSubscriptionCycles') }}</p>
        </div>

        <div class="rounded-lg border border-primary-200 bg-primary-50/40 p-3 dark:border-primary-900/50 dark:bg-primary-900/10">
          <div class="mb-3 flex items-center justify-between">
            <span class="text-sm font-semibold text-gray-800 dark:text-dark-100">{{ t('admin.profit.addSubscriptionCycle') }}</span>
            <button class="text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400" @click="inferStartDate">{{ t('admin.profit.inferCycleStart') }}</button>
          </div>
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <div><label class="label">{{ t('admin.profit.periodFee') }}</label><input v-model.number="form.period_fee" type="number" min="0" step="0.01" class="input" /></div>
            <div><label class="label">{{ t('admin.profit.periodDays') }}</label><input v-model.number="form.period_days" type="number" min="1" class="input" /></div>
            <div><label class="label">{{ t('admin.profit.periodStartAt') }}</label><input v-model="form.starts_at" type="date" class="input" /></div>
          </div>
          <p v-if="inferenceNote" class="mt-2 text-xs" :class="inferenceRisky ? 'text-amber-600 dark:text-amber-400' : 'text-gray-500 dark:text-dark-400'">{{ inferenceNote }}</p>
          <div class="mt-3"><label class="label">{{ t('admin.profit.notes') }}</label><input v-model="form.notes" type="text" class="input" /></div>
        </div>
      </template>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <button class="btn-secondary" @click="emit('close')">{{ t('admin.profit.cancel') }}</button>
        <button v-if="isSubscription" data-testid="save-cost-config" class="btn-primary" :disabled="saving || !form.starts_at" @click="saveCycle">{{ t('admin.profit.save') }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import type { AccountSubscriptionCycle, SubscriptionCycleListResponse } from '@/api/admin/profit'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{ show: boolean; accountId: number | null; accountName?: string; accountType?: string }>()
const emit = defineEmits(['close', 'saved'])
const { t } = useI18n()
const appStore = useAppStore()
const saving = ref(false)
const cycles = ref<AccountSubscriptionCycle[]>([])
const hints = ref<SubscriptionCycleListResponse | null>(null)
const inferenceNote = ref('')
const inferenceRisky = ref(false)
const isSubscription = computed(() => props.accountType === 'oauth' || props.accountType === 'setup-token')
const form = ref({ period_fee: 0, period_days: 30, starts_at: '', notes: '' })

async function loadCycles() {
  if (!props.accountId || !isSubscription.value) return
  const result = await adminAPI.profit.listSubscriptionCycles(props.accountId)
  cycles.value = result.cycles || []
  hints.value = result
}

watch(() => [props.show, props.accountId, props.accountType], async ([visible]) => {
  if (!visible) return
  cycles.value = []
  hints.value = null
  inferenceNote.value = ''
  form.value = { period_fee: 0, period_days: 30, starts_at: '', notes: '' }
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

async function saveCycle() {
  if (!props.accountId) return
  saving.value = true
  try {
    await adminAPI.profit.createSubscriptionCycle(props.accountId, { ...form.value, currency: 'USD' })
    appStore.showSuccess(t('admin.profit.saveSuccess'))
    await loadCycles()
    emit('saved')
  } catch (err) { appStore.showError(extractApiErrorMessage(err, t('admin.profit.saveFailed'))) } finally { saving.value = false }
}

async function removeCycle(id: number) {
  try { await adminAPI.profit.deleteSubscriptionCycle(id); await loadCycles(); emit('saved') } catch (err) { appStore.showError(extractApiErrorMessage(err, t('admin.profit.saveFailed'))) }
}
</script>

<style scoped>
.input { @apply w-full rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-gray-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20 dark:border-dark-600 dark:bg-dark-700 dark:text-white; }
.label { @apply mb-1 block text-sm font-medium text-gray-700 dark:text-dark-200; }
.btn-secondary { @apply rounded-lg px-3 py-1.5 text-sm font-medium text-gray-600 hover:bg-gray-100 dark:text-dark-300 dark:hover:bg-dark-700; }
.btn-primary { @apply rounded-lg bg-primary-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-primary-700 disabled:opacity-50; }
</style>
