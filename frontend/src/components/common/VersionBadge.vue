<template>
  <div class="relative">
    <template v-if="isAdmin">
      <button
        ref="triggerRef"
        @click="toggleDropdown"
        class="group mt-1 inline-flex h-4 items-center gap-1.5 text-[10px] font-medium text-gray-500 transition-colors hover:text-gray-900 dark:text-gray-400 dark:hover:text-white"
        :title="buttonTitle"
      >
        <span class="h-1 w-1 shrink-0 rounded-full bg-gray-400 transition-colors group-hover:bg-gray-900 dark:bg-gray-500 dark:group-hover:bg-white"></span>
        <span v-if="currentVersion" class="font-mono tabular-nums">v{{ currentVersion }}</span>
        <span
          v-else
          class="h-2.5 w-10 animate-pulse rounded bg-gray-200 dark:bg-dark-600"
        ></span>
      </button>

      <transition name="dropdown">
        <div
          v-if="dropdownOpen"
          ref="dropdownRef"
          class="absolute left-0 z-50 mt-2 w-64 overflow-hidden rounded-xl border border-gray-200 bg-white shadow-lg dark:border-dark-700 dark:bg-dark-800"
        >
          <div class="border-b border-gray-100 px-4 py-3 dark:border-dark-700">
            <span class="text-sm font-medium text-gray-700 dark:text-dark-300">
              {{ t('version.currentVersion') }}
            </span>
          </div>

          <div class="space-y-4 p-4">
            <div class="text-center">
              <div class="text-2xl font-bold text-gray-900 dark:text-white">
                <span v-if="currentVersion">v{{ currentVersion }}</span>
                <span v-else class="text-gray-400 dark:text-dark-500">--</span>
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                {{ t('version.currentVersion') }}
              </p>
            </div>

            <div v-if="catalogStatus" class="rounded-lg bg-gray-50 p-3 text-xs dark:bg-dark-700/60">
              <div class="flex items-center justify-between gap-2">
                <span class="text-gray-500 dark:text-dark-400">Model catalog</span>
                <span class="font-mono text-gray-800 dark:text-dark-200">
                  v{{ catalogStatus.version }} · {{ catalogStatus.source }}
                </span>
              </div>
              <p v-if="catalogStatus.last_error" class="mt-2 break-words text-red-500">
                {{ catalogStatus.last_error }}
              </p>
              <button
                class="mt-2 w-full rounded-md border border-gray-200 px-2 py-1.5 font-medium text-gray-700 hover:bg-white disabled:opacity-50 dark:border-dark-600 dark:text-dark-200 dark:hover:bg-dark-700"
                :disabled="catalogRefreshing || !catalogStatus.remote_enabled"
                @click="handleCatalogRefresh"
              >
                {{ catalogRefreshing ? 'Refreshing…' : 'Refresh model catalog' }}
              </button>
            </div>

            <button
              @click="handleRestart"
              :disabled="restarting"
              class="flex w-full items-center justify-center gap-2 rounded-lg bg-green-500 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-green-600 disabled:cursor-not-allowed disabled:opacity-50"
            >
              <svg
                v-if="restarting"
                class="h-4 w-4 animate-spin"
                fill="none"
                viewBox="0 0 24 24"
              >
                <circle
                  class="opacity-25"
                  cx="12"
                  cy="12"
                  r="10"
                  stroke="currentColor"
                  stroke-width="4"
                ></circle>
                <path
                  class="opacity-75"
                  fill="currentColor"
                  d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                ></path>
              </svg>
              <Icon v-else name="refresh" size="sm" :stroke-width="2" />
              <template v-if="restarting">
                <span>{{ t('version.restarting') }}</span>
                <span v-if="restartCountdown > 0" class="tabular-nums">
                  ({{ restartCountdown }}s)
                </span>
              </template>
              <span v-else>{{ t('version.restartNow') }}</span>
            </button>
          </div>
        </div>
      </transition>
    </template>

    <span v-else-if="currentVersion" class="text-xs text-gray-500 dark:text-dark-400">
      v{{ currentVersion }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores'
import {
  getModelCatalogStatus,
  refreshModelCatalog,
  restartService,
  type ModelCatalogStatus,
} from '@/api/admin/system'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()

const props = defineProps<{
  version?: string
}>()

const authStore = useAuthStore()

const isAdmin = computed(() => authStore.isAdmin)
const currentVersion = computed(() => props.version || '')
const buttonTitle = computed(() =>
  currentVersion.value
    ? `${t('version.currentVersion')}: v${currentVersion.value}`
    : t('version.currentVersion')
)

const dropdownOpen = ref(false)
const dropdownRef = ref<HTMLElement | null>(null)
const triggerRef = ref<HTMLElement | null>(null)

const restarting = ref(false)
const catalogRefreshing = ref(false)
const catalogStatus = ref<ModelCatalogStatus | null>(null)
const restartCountdown = ref(0)

function toggleDropdown() {
  if (!isAdmin.value) return
  dropdownOpen.value = !dropdownOpen.value
	if (dropdownOpen.value && !catalogStatus.value) void loadCatalogStatus()
}

async function loadCatalogStatus() {
  try {
    catalogStatus.value = await getModelCatalogStatus()
  } catch {
    // Version controls remain usable if catalog status is unavailable.
  }
}

async function handleCatalogRefresh() {
  if (catalogRefreshing.value) return
  catalogRefreshing.value = true
  try {
    catalogStatus.value = await refreshModelCatalog()
  } finally {
    catalogRefreshing.value = false
  }
}

function closeDropdown() {
  dropdownOpen.value = false
}

async function handleRestart() {
  if (restarting.value) return

  restarting.value = true
  restartCountdown.value = 8

  try {
    await restartService()
  } catch {
    console.log('Service restarting...')
  }

  const countdownInterval = setInterval(() => {
    restartCountdown.value--
    if (restartCountdown.value <= 0) {
      clearInterval(countdownInterval)
      checkServiceAndReload()
    }
  }, 1000)
}

async function checkServiceAndReload() {
  const maxRetries = 5
  const retryDelay = 1000

  for (let i = 0; i < maxRetries; i++) {
    try {
      const response = await fetch('/health', {
        method: 'GET',
        cache: 'no-cache'
      })
      if (response.ok) {
        window.location.reload()
        return
      }
    } catch {
      // Service not ready yet.
    }

    if (i < maxRetries - 1) {
      await new Promise((resolve) => setTimeout(resolve, retryDelay))
    }
  }

  window.location.reload()
}

function handleClickOutside(event: MouseEvent) {
  const target = event.target as Node
  if (dropdownRef.value?.contains(target) || triggerRef.value?.contains(target)) {
    return
  }
  closeDropdown()
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.2s ease;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
