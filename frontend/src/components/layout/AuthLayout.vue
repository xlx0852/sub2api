<template>
  <div class="relative flex min-h-screen items-center justify-center overflow-hidden p-4">
    <!-- Background -->
    <div
      class="absolute inset-0 bg-[#f4f4f0] dark:bg-dark-950"
    ></div>

    <!-- Decorative Elements -->
    <div class="pointer-events-none absolute inset-0 overflow-hidden">
      <!-- Ink wash -->
      <div
        class="absolute -right-40 -top-40 h-80 w-80 rounded-full bg-black/[0.06] blur-3xl dark:bg-white/[0.04]"
      ></div>
      <div
        class="absolute -bottom-40 -left-40 h-80 w-80 rounded-full bg-black/[0.05] blur-3xl dark:bg-white/[0.03]"
      ></div>
      <div
        class="absolute left-1/2 top-1/2 h-96 w-96 -translate-x-1/2 -translate-y-1/2 rounded-full bg-white/60 blur-3xl dark:bg-black/20"
      ></div>

      <!-- Grid Pattern -->
      <div
        class="absolute inset-0 bg-[linear-gradient(rgba(24,24,23,0.025)_1px,transparent_1px),linear-gradient(90deg,rgba(24,24,23,0.025)_1px,transparent_1px)] bg-[size:72px_72px] dark:opacity-30"
      ></div>
    </div>

    <!-- Content Container -->
    <div class="relative z-10 w-full max-w-md">
      <!-- Logo/Brand -->
      <div class="mb-8 text-center">
        <!-- Custom Logo or Default Logo -->
        <template v-if="settingsLoaded">
          <div
            class="mb-4 inline-flex h-16 w-16 items-center justify-center overflow-hidden rounded-2xl bg-transparent shadow-none"
          >
            <img :src="siteLogo || '/logo.png'" alt="Logo" class="brand-logo-monochrome h-full w-full object-contain" />
          </div>
          <h1 class="text-gradient mb-2 text-3xl font-bold">
            {{ siteName }}
          </h1>
          <p class="text-sm text-gray-500 dark:text-dark-400">
            {{ siteSubtitle }}
          </p>
        </template>
      </div>

      <!-- Card Container -->
      <div class="card-glass rounded-xl border-black/10 bg-[#fbfbf8]/90 p-8 dark:border-white/10 dark:bg-dark-900/90">
        <slot />
      </div>

      <!-- Footer Links -->
      <div class="mt-6 text-center text-sm">
        <slot name="footer" />
      </div>

      <!-- Copyright -->
      <div class="mt-8 text-center text-xs text-gray-400 dark:text-dark-500">
        &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'SicTs')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || '大模型接入商')
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
.text-gradient {
  @apply text-primary-950 dark:text-primary-100;
}
</style>
