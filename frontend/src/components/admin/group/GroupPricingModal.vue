<template>
  <BaseDialog
    :show="show"
    :title="t('admin.groups.pricing.title')"
    width="extra-wide"
    @close="emit('close')"
  >
    <GroupSellPriceEditor
      :group="group"
      :active="show"
      @success="emit('success')"
    />

    <template #footer>
      <div class="flex justify-end">
        <button type="button" class="btn btn-secondary" @click="emit('close')">
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { AdminGroup } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import GroupSellPriceEditor from '@/components/admin/group/GroupSellPriceEditor.vue'

defineProps<{
  show: boolean
  group: AdminGroup | null
}>()

const emit = defineEmits<{
  close: []
  success: []
}>()

const { t } = useI18n()
</script>
