<template>
  <BaseDialog
    :show="show"
    :title="t('admin.groups.pricing.title')"
    width="extra-wide"
    @close="emit('close')"
  >
    <GroupSellPriceEditor
      :group="group"
      :policy-id="policyId ?? null"
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
import BaseDialog from '@/ui/BaseDialog.vue'
import GroupSellPriceEditor from '@/components/admin/group/GroupSellPriceEditor.vue'

defineProps<{
  show: boolean
  group: AdminGroup | null
  /** 售价表模式：直接编辑这张表（独立定价页使用） */
  policyId?: number | null
}>()

const emit = defineEmits<{
  close: []
  success: []
}>()

// policyId 可能为 null，模板里用 ?? null 兜底

const { t } = useI18n()
</script>
