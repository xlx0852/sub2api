<template>
  <Teleport to="body">
    <Transition :name="transitionName">
      <div
        v-if="show"
        :class="overlayClasses"
        :style="zIndexStyle"
        :aria-labelledby="dialogId"
        role="dialog"
        aria-modal="true"
        data-testid="base-dialog-overlay"
        :data-variant="variant"
        @click.self="handleClose"
      >
        <!-- Modal / drawer panel -->
        <div
          ref="dialogRef"
          :class="['modal-content', panelClasses, widthClasses]"
          tabindex="-1"
          data-testid="base-dialog-panel"
          @click.stop
          @invalid.capture="suppressHiddenInvalid"
        >
          <!-- Header -->
          <div class="modal-header">
            <h3 :id="dialogId" class="modal-title">
              {{ title }}
            </h3>
            <button
              v-if="showCloseButton"
              @click="emit('close')"
              class="modal-close-button -mr-2 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 focus-visible:ring-offset-2 dark:text-dark-500 dark:hover:bg-dark-700 dark:hover:text-dark-300 dark:focus-visible:ring-offset-dark-900"
              aria-label="Close modal"
            >
              <Icon name="x" size="md" />
            </button>
          </div>

          <!-- Body -->
          <div class="modal-body">
            <slot></slot>
          </div>

          <!-- Footer -->
          <div v-if="$slots.footer" class="modal-footer">
            <slot name="footer"></slot>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, watch, onMounted, onUnmounted, ref, nextTick } from 'vue'
import Icon from '@/components/icons/Icon.vue'

// 生成唯一ID以避免多个对话框时ID冲突
let dialogIdCounter = 0
const dialogId = `modal-title-${++dialogIdCounter}`

// 焦点管理
const dialogRef = ref<HTMLElement | null>(null)
let previousActiveElement: HTMLElement | null = null

type DialogWidth = 'narrow' | 'normal' | 'wide' | 'extra-wide' | 'full'
type DialogVariant = 'modal' | 'drawer'

interface Props {
  show: boolean
  title: string
  width?: DialogWidth
  /** modal=居中弹窗；drawer=右侧抽屉（适合多 Tab 长表单） */
  variant?: DialogVariant
  closeOnEscape?: boolean
  closeOnClickOutside?: boolean
  showCloseButton?: boolean
  zIndex?: number
}

interface Emits {
  (e: 'close'): void
}

const props = withDefaults(defineProps<Props>(), {
  width: 'normal',
  variant: 'modal',
  closeOnEscape: true,
  closeOnClickOutside: false,
  showCloseButton: true,
  zIndex: 50
})

const emit = defineEmits<Emits>()

const isDrawer = computed(() => props.variant === 'drawer')
const transitionName = computed(() => (isDrawer.value ? 'drawer' : 'modal'))
const overlayClasses = computed(() =>
  isDrawer.value ? 'modal-overlay modal-overlay--drawer' : 'modal-overlay'
)
const panelClasses = computed(() =>
  isDrawer.value ? 'modal-content--drawer' : ''
)

// Custom z-index style (overrides the default z-50 from CSS)
const zIndexStyle = computed(() => {
  return props.zIndex !== 50 ? { zIndex: props.zIndex } : undefined
})

const widthClasses = computed(() => {
  // Width guidance: narrow=confirm/short prompts, normal=standard forms,
  // wide=multi-section forms or rich content, extra-wide=analytics/tables,
  // full=full-screen or very dense layouts.
  // Drawer uses the same tokens but prefers a fixed right-rail width on desktop.
  if (isDrawer.value) {
    // 抽屉宽度按侧栏场景收敛，避免 wide 在大屏拉到 4xl 占半屏。
    const drawerWidths: Record<DialogWidth, string> = {
      narrow: 'w-full sm:max-w-md',
      normal: 'w-full sm:max-w-lg sm:w-[32rem]',
      wide: 'w-full sm:max-w-xl sm:w-[36rem]',
      'extra-wide': 'w-full sm:max-w-2xl sm:w-[42rem]',
      full: 'w-full sm:max-w-3xl sm:w-[48rem]'
    }
    return drawerWidths[props.width]
  }
  const widths: Record<DialogWidth, string> = {
    narrow: 'max-w-md',
    normal: 'max-w-lg',
    wide: 'w-full sm:max-w-2xl md:max-w-3xl lg:max-w-4xl',
    'extra-wide': 'w-full sm:max-w-3xl md:max-w-4xl lg:max-w-5xl xl:max-w-6xl',
    full: 'w-full sm:max-w-4xl md:max-w-5xl lg:max-w-6xl xl:max-w-7xl'
  }
  return widths[props.width]
})

const handleClose = () => {
  if (props.closeOnClickOutside) {
    emit('close')
  }
}

const handleEscape = (event: KeyboardEvent) => {
  if (props.show && props.closeOnEscape && event.key === 'Escape') {
    emit('close')
  }
}

// 多 Tab / v-show 表单：隐藏的 required/min 控件会触发
// "An invalid form control with name='' is not focusable."
// 统一给弹层内 form 开 novalidate，并把不可见控件的 invalid 事件吞掉。
const disableNativeFormValidation = () => {
  const root = dialogRef.value
  if (!root) return
  root.querySelectorAll('form').forEach((form) => {
    form.setAttribute('novalidate', '')
  })
}

const suppressHiddenInvalid = (event: Event) => {
  const el = event.target
  if (!(el instanceof HTMLElement)) return
  // 不可见（含 display:none / 祖先 v-show 隐藏）时禁止浏览器尝试 focus
  if (el.offsetParent === null || el.getClientRects().length === 0) {
    event.preventDefault()
    event.stopPropagation()
  }
}

// Prevent body scroll when modal is open and manage focus
watch(
  () => props.show,
  async (isOpen) => {
    if (isOpen) {
      // 保存当前焦点元素
      previousActiveElement = document.activeElement as HTMLElement
      // 使用CSS类而不是直接操作style,更易于管理多个对话框
      document.body.classList.add('modal-open')

      // 等待DOM更新后设置焦点到对话框
      await nextTick()
      disableNativeFormValidation()
      if (dialogRef.value) {
        const firstFocusable = dialogRef.value.querySelector<HTMLElement>(
          'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
        )
        firstFocusable?.focus()
      }
    } else {
      document.body.classList.remove('modal-open')
      // 恢复之前的焦点
      if (previousActiveElement && typeof previousActiveElement.focus === 'function') {
        previousActiveElement.focus()
      }
      previousActiveElement = null
    }
  },
  { immediate: true }
)

onMounted(() => {
  document.addEventListener('keydown', handleEscape)
})

onUnmounted(() => {
  document.removeEventListener('keydown', handleEscape)
  // 确保组件卸载时移除滚动锁定
  document.body.classList.remove('modal-open')
})
</script>
