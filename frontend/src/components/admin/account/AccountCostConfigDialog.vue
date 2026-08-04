<template>
  <BaseDialog :show="show" :title="t('admin.profit.configTitle')" @close="emit('close')">
    <AccountCostConfigPanel
      ref="panelRef"
      :active="show"
      :account-id="accountId"
      :account-name="accountName"
      :account-type="accountType"
      :account-platform="accountPlatform"
      :show-account-header="true"
      :show-inline-save="false"
      @saved="emit('saved')"
      @update:can-save="canSave = $event"
      @update:saving="saving = $event"
    />
    <template #footer>
      <div class="flex justify-end gap-2">
        <button class="btn-secondary" type="button" @click="emit('close')">{{ t('admin.profit.cancel') }}</button>
        <button
          v-if="isSubscription"
          data-testid="save-cost-config"
          type="button"
          class="btn-primary"
          :disabled="saving || !canSave"
          @click="panelRef?.saveCycle()"
        >{{ t('admin.profit.save') }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import AccountCostConfigPanel from './AccountCostConfigPanel.vue'

const props = defineProps<{ show: boolean; accountId: number | null; accountName?: string; accountType?: string; accountPlatform?: string }>()
const emit = defineEmits(['close', 'saved'])
const { t } = useI18n()
const panelRef = ref<InstanceType<typeof AccountCostConfigPanel> | null>(null)
const canSave = ref(false)
const saving = ref(false)
const isSubscription = computed(() => props.accountType === 'oauth' || props.accountType === 'setup-token')
</script>

<style scoped>
.btn-secondary { @apply rounded-lg px-3 py-1.5 text-sm font-medium text-gray-600 hover:bg-gray-100 dark:text-dark-300 dark:hover:bg-dark-700; }
.btn-primary { @apply rounded-lg bg-primary-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-primary-700 disabled:opacity-50; }
</style>
