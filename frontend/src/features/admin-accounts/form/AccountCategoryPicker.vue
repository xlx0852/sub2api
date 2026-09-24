<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { GroupPlatform } from '@/types'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import {
  platformCategoryPicker,
  type CategoryPickerOption
} from '@/features/admin-accounts/form/registry'

const props = defineProps<{
  platform: GroupPlatform
  modelValue: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
  help: []
}>()

const { t } = useI18n()
const config = computed(() => platformCategoryPicker(props.platform))
// 只有一个类别时没有可选项：渲染成静态卡片（不给 hover / 选中交互）。
const isSingleOption = computed(() => (config.value?.options.length ?? 0) <= 1)
const isStatic = computed(() => config.value?.model === 'none' || isSingleOption.value)

function titleOf(option: CategoryPickerOption): string {
  return option.titleKey ? t(option.titleKey) : option.title ?? ''
}

function subtitleOf(option: CategoryPickerOption): string {
  return option.subtitleKey ? t(option.subtitleKey) : option.subtitle ?? ''
}

function isSelected(option: CategoryPickerOption): boolean {
  return config.value?.model === 'none' || props.modelValue === option.id
}

function select(option: CategoryPickerOption) {
  if (config.value?.model === 'none') return
  emit('update:modelValue', option.id)
}
</script>

<template>
  <div v-if="config">
    <div class="flex items-center justify-between">
      <label class="input-label">{{ t('admin.accounts.accountType') }}</label>
      <button
        v-if="config.helpKey"
        type="button"
        class="flex items-center gap-1 rounded px-2 py-1 text-xs text-blue-600 hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
        @click="emit('help')"
      >
        <Icon name="questionCircle" size="sm" />
        {{ t(config.helpKey) }}
      </button>
    </div>
    <div
      v-if="isStatic"
      class="mt-2 cursor-default rounded-lg border-2 p-3"
      :class="config.options[0]?.selectedBorder"
      data-tour="account-form-type"
    >
      <div class="flex items-center gap-3">
        <div
          class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg"
          :class="config.options[0]?.selectedIconBg"
        >
          <Icon v-if="config.options[0]?.icon !== 'platform'" :name="config.options[0]!.icon" size="sm" />
          <PlatformIcon v-else :platform="platform" size="sm" />
        </div>
        <div>
          <span class="block text-sm font-medium text-gray-900 dark:text-white">{{
            titleOf(config.options[0])
          }}</span>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{
            subtitleOf(config.options[0])
          }}</span>
        </div>
      </div>
    </div>
    <div
      v-else
      class="mt-2 grid gap-3"
      :class="config.layoutClass"
      data-tour="account-form-type"
    >
      <button
        v-for="option in config.options"
        :key="option.id"
        type="button"
        @click="select(option)"
        :class="[
          'flex items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
          isSelected(option) ? option.selectedBorder : option.hoverBorder
        ]"
      >
        <div
          :class="[
            'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
            isSelected(option)
              ? option.selectedIconBg
              : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400'
          ]"
        >
          <Icon v-if="option.icon !== 'platform'" :name="option.icon" size="sm" />
          <PlatformIcon v-else :platform="platform" size="sm" />
        </div>
        <div>
          <span class="block text-sm font-medium text-gray-900 dark:text-white">{{
            titleOf(option)
          }}</span>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ subtitleOf(option) }}</span>
        </div>
      </button>
    </div>
    <p v-if="config.hintKey" class="mt-2 text-xs text-gray-500 dark:text-gray-400">
      {{ t(config.hintKey) }}
    </p>
    <div
      v-for="option in config.options.filter((item) => item.noteKey && isSelected(item))"
      :key="`note-${option.id}`"
      :class="option.noteClass"
    >
      <p>{{ t(option.noteKey!) }}</p>
    </div>
  </div>
</template>
