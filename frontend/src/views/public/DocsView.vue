<template>
  <div class="docs-page min-h-screen bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white">
    <header class="sticky top-0 z-30 border-b border-gray-200/80 bg-white/90 backdrop-blur dark:border-dark-800 dark:bg-dark-900/90">
      <div class="mx-auto flex max-w-7xl items-center justify-between gap-4 px-4 py-3 sm:px-6">
        <div class="flex min-w-0 items-center gap-3">
          <RouterLink to="/home" class="flex min-w-0 items-center gap-3">
            <span
              class="flex h-10 w-10 flex-shrink-0 items-center justify-center overflow-hidden rounded-xl bg-white ring-1 ring-gray-200 dark:bg-dark-800 dark:ring-dark-700"
            >
              <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
            </span>
            <span class="truncate text-base font-semibold text-gray-950 dark:text-white">
              {{ siteName }}
            </span>
          </RouterLink>
          <span class="hidden text-sm text-gray-400 sm:inline">/</span>
          <span class="hidden truncate text-sm font-medium text-gray-600 dark:text-dark-300 sm:inline">
            {{ t('docs.navLabel') }}
          </span>
        </div>

        <div class="flex items-center gap-2">
          <button
            type="button"
            class="inline-flex items-center justify-center rounded-lg border border-gray-200 bg-white p-2 text-gray-600 transition hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700 lg:hidden"
            :aria-label="t('docs.toggleMenu')"
            @click="mobileNavOpen = !mobileNavOpen"
          >
            <Icon name="menu" size="md" />
          </button>

          <LocaleSwitcher />

          <button
            type="button"
            class="inline-flex items-center justify-center rounded-lg border border-gray-200 bg-white p-2 text-gray-600 transition hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>

          <RouterLink
            v-if="isAuthenticated"
            :to="dashboardPath"
            class="inline-flex items-center justify-center rounded-lg bg-primary-600 px-3 py-2 text-sm font-semibold text-white transition hover:bg-primary-700"
          >
            {{ t('home.dashboard') }}
          </RouterLink>
          <RouterLink
            v-else
            to="/login"
            class="inline-flex items-center justify-center rounded-lg bg-primary-600 px-3 py-2 text-sm font-semibold text-white transition hover:bg-primary-700"
          >
            {{ t('home.login') }}
          </RouterLink>
        </div>
      </div>
    </header>

    <div class="mx-auto flex max-w-7xl gap-0 px-0 lg:gap-8 lg:px-6 lg:py-8">
      <aside class="hidden w-64 flex-shrink-0 lg:block">
        <div class="sticky top-24 max-h-[calc(100vh-7rem)] overflow-y-auto rounded-2xl border border-gray-200 bg-white p-4 dark:border-dark-800 dark:bg-dark-900">
          <p class="mb-3 px-2 text-xs font-bold uppercase tracking-wider text-gray-400 dark:text-dark-500">
            {{ t('docs.quickStart') }}
          </p>
          <nav class="space-y-1" aria-label="Docs navigation">
            <button
              v-for="section in docsSections"
              :key="section.id"
              type="button"
              class="w-full rounded-xl px-3 py-2.5 text-left text-sm font-semibold transition"
              :class="
                activeId === section.id
                  ? 'bg-primary-50 text-primary-700 dark:bg-primary-500/10 dark:text-primary-300'
                  : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white'
              "
              @click="selectSection(section.id)"
            >
              {{ section.title }}
            </button>
          </nav>

          <div class="mt-5 rounded-xl border border-dashed border-gray-200 p-3 text-xs leading-5 text-gray-500 dark:border-dark-700 dark:text-dark-400">
            <p class="font-semibold text-gray-700 dark:text-dark-200">{{ t('docs.apiBase') }}</p>
            <p class="mt-1 break-all font-mono">{{ apiBaseUrl }}</p>
            <RouterLink
              to="/keys"
              class="mt-2 inline-flex font-semibold text-primary-600 hover:underline dark:text-primary-300"
            >
              {{ t('docs.getKey') }}
            </RouterLink>
          </div>
        </div>
      </aside>

      <div
        v-if="mobileNavOpen"
        class="fixed inset-0 z-40 bg-black/40 lg:hidden"
        @click="mobileNavOpen = false"
      />
      <aside
        class="fixed inset-y-0 left-0 z-50 w-72 transform border-r border-gray-200 bg-white p-4 transition-transform dark:border-dark-800 dark:bg-dark-900 lg:hidden"
        :class="mobileNavOpen ? 'translate-x-0' : '-translate-x-full'"
      >
        <div class="mb-4 flex items-center justify-between">
          <p class="text-sm font-bold text-gray-900 dark:text-white">{{ t('docs.quickStart') }}</p>
          <button
            type="button"
            class="rounded-lg p-2 text-gray-500 hover:bg-gray-100 dark:hover:bg-dark-800"
            @click="mobileNavOpen = false"
          >
            <Icon name="x" size="md" />
          </button>
        </div>
        <nav class="space-y-1">
          <button
            v-for="section in docsSections"
            :key="`m-${section.id}`"
            type="button"
            class="w-full rounded-xl px-3 py-2.5 text-left text-sm font-semibold transition"
            :class="
              activeId === section.id
                ? 'bg-primary-50 text-primary-700 dark:bg-primary-500/10 dark:text-primary-300'
                : 'text-gray-600 hover:bg-gray-50 dark:text-dark-300 dark:hover:bg-dark-800'
            "
            @click="selectSection(section.id); mobileNavOpen = false"
          >
            {{ section.title }}
          </button>
        </nav>
      </aside>

      <main class="min-w-0 flex-1 px-4 py-6 sm:px-6 lg:px-0 lg:py-0">
        <article class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-800 dark:bg-dark-900 sm:p-8">
          <div class="mb-6 border-b border-gray-100 pb-6 dark:border-dark-800">
            <p class="text-sm font-medium text-primary-600 dark:text-primary-300">
              {{ t('docs.badge') }}
            </p>
            <h1 class="mt-2 text-3xl font-bold tracking-tight text-gray-950 dark:text-white sm:text-4xl">
              {{ currentSection.title }}
            </h1>
            <p class="mt-3 text-sm leading-6 text-gray-600 dark:text-dark-300 sm:text-base">
              {{ currentSection.summary }}
            </p>
          </div>

          <div
            ref="contentRef"
            class="docs-content"
            v-html="renderedHtml"
          />
        </article>

        <div class="mt-6 flex flex-wrap items-center justify-between gap-3 text-sm text-gray-500 dark:text-dark-400">
          <button
            type="button"
            class="rounded-lg px-3 py-2 font-medium transition hover:bg-white hover:text-gray-900 disabled:opacity-40 dark:hover:bg-dark-900 dark:hover:text-white"
            :disabled="!prevSection"
            @click="prevSection && selectSection(prevSection.id)"
          >
            ← {{ prevSection?.title || t('docs.prev') }}
          </button>
          <button
            type="button"
            class="rounded-lg px-3 py-2 font-medium transition hover:bg-white hover:text-gray-900 disabled:opacity-40 dark:hover:bg-dark-900 dark:hover:text-white"
            :disabled="!nextSection"
            @click="nextSection && selectSection(nextSection.id)"
          >
            {{ nextSection?.title || t('docs.next') }} →
          </button>
        </div>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore, useAuthStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'
import {
  docsSections,
  getDocsSection,
  renderDocsMarkdown,
  resolveDocsSectionId,
  type DocsSectionId
} from '@/data/docsContent'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()

const contentRef = ref<HTMLElement | null>(null)
const mobileNavOpen = ref(false)
const activeId = ref<DocsSectionId>('nodejs')
const isDark = ref(document.documentElement.classList.contains('dark'))

marked.setOptions({
  breaks: true,
  gfm: true
})

const siteName = computed(
  () => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'SicTs'
)
const siteLogo = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', {
    allowRelative: true,
    allowDataUrl: true
  })
)
const apiBaseUrl = computed(
  () =>
    appStore.cachedPublicSettings?.api_base_url ||
    (typeof window !== 'undefined' ? window.location.origin : '')
)

const isAuthenticated = computed(() => authStore.isAuthenticated)
const dashboardPath = computed(() => (authStore.isAdmin ? '/admin/dashboard' : '/dashboard'))

const currentSection = computed(() => getDocsSection(activeId.value))
const currentIndex = computed(() =>
  docsSections.findIndex((section) => section.id === activeId.value)
)
const prevSection = computed(() =>
  currentIndex.value > 0 ? docsSections[currentIndex.value - 1] : null
)
const nextSection = computed(() =>
  currentIndex.value >= 0 && currentIndex.value < docsSections.length - 1
    ? docsSections[currentIndex.value + 1]
    : null
)

const renderedHtml = computed(() => {
  const deploymentMarkdown = renderDocsMarkdown(currentSection.value, apiBaseUrl.value)
  const raw = marked.parse(deploymentMarkdown, { async: false }) as string
  return DOMPurify.sanitize(raw, {
    ADD_ATTR: ['target', 'rel']
  })
})

function selectSection(id: DocsSectionId) {
  activeId.value = id
  const section = getDocsSection(id)
  const nextHash = `#${section.hash}`
  if (window.location.hash !== nextHash) {
    history.replaceState(null, '', router.resolve({ path: '/docs', hash: nextHash }).href)
  }
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

function syncFromHash() {
  const fromHash = resolveDocsSectionId(window.location.hash)
  if (fromHash) {
    activeId.value = fromHash
    return
  }
  const section = getDocsSection(activeId.value)
  history.replaceState(null, '', router.resolve({ path: '/docs', hash: `#${section.hash}` }).href)
}

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function injectCopyButtons() {
  const container = contentRef.value
  if (!container) return

  container.querySelectorAll('pre').forEach((pre) => {
    if (pre.querySelector('.docs-copy-btn')) return
    pre.classList.add('docs-pre')
    const btn = document.createElement('button')
    btn.type = 'button'
    btn.className = 'docs-copy-btn'
    btn.textContent = t('docs.copy')
    btn.addEventListener('click', async () => {
      const code = pre.querySelector('code')?.textContent ?? pre.textContent ?? ''
      try {
        await navigator.clipboard.writeText(code)
        btn.textContent = t('docs.copied')
      } catch {
        btn.textContent = t('docs.copyFailed')
      }
      setTimeout(() => {
        btn.textContent = t('docs.copy')
      }, 1800)
    })
    pre.appendChild(btn)
  })

  container.querySelectorAll('a[href^="#"]').forEach((anchor) => {
    const href = anchor.getAttribute('href') || ''
    const sectionId = resolveDocsSectionId(href)
    if (!sectionId) return
    anchor.addEventListener('click', (event) => {
      event.preventDefault()
      selectSection(sectionId)
    })
  })
}

watch(
  () => activeId.value,
  async () => {
    await nextTick()
    injectCopyButtons()
  },
  { immediate: true }
)

onMounted(async () => {
  if (!appStore.publicSettingsLoaded) {
    try {
      await appStore.fetchPublicSettings()
    } catch {
      // public page can still render offline content
    }
  }
  syncFromHash()
  window.addEventListener('hashchange', syncFromHash)
  await nextTick()
  injectCopyButtons()
})

onUnmounted(() => {
  window.removeEventListener('hashchange', syncFromHash)
})
</script>

<style scoped>
.docs-content {
  color: rgb(55 65 81);
  font-size: 0.95rem;
  line-height: 1.75;
}

.dark .docs-content {
  color: rgb(203 213 225);
}

.docs-content :deep(p) {
  margin: 0.85rem 0;
}

.docs-content :deep(ul),
.docs-content :deep(ol) {
  margin: 0.85rem 0;
  padding-left: 1.35rem;
}

.docs-content :deep(ul) {
  list-style: disc;
}

.docs-content :deep(ol) {
  list-style: decimal;
}

.docs-content :deep(li + li) {
  margin-top: 0.35rem;
}

.docs-content :deep(h2),
.docs-content :deep(h3),
.docs-content :deep(h4) {
  scroll-margin-top: 6rem;
  color: rgb(17 24 39);
  font-weight: 700;
  line-height: 1.3;
  margin: 1.6rem 0 0.75rem;
}

.dark .docs-content :deep(h2),
.dark .docs-content :deep(h3),
.dark .docs-content :deep(h4) {
  color: rgb(248 250 252);
}

.docs-content :deep(h2) {
  font-size: 1.35rem;
}

.docs-content :deep(h3) {
  font-size: 1.15rem;
}

.docs-content :deep(h4) {
  font-size: 1.05rem;
}

.docs-content :deep(a) {
  color: rgb(37 99 235);
  text-decoration: none;
  font-weight: 600;
}

.docs-content :deep(a:hover) {
  text-decoration: underline;
}

.dark .docs-content :deep(a) {
  color: rgb(147 197 253);
}

.docs-content :deep(table) {
  display: block;
  width: 100%;
  overflow-x: auto;
  border-collapse: collapse;
}

.docs-content :deep(th),
.docs-content :deep(td) {
  border: 1px solid rgb(229 231 235);
  padding: 0.55rem 0.75rem;
  text-align: left;
}

.dark .docs-content :deep(th),
.dark .docs-content :deep(td) {
  border-color: rgb(55 65 81);
}

.docs-content :deep(pre.docs-pre),
.docs-content :deep(pre) {
  position: relative;
  overflow-x: auto;
  border-radius: 0.9rem;
  background: #0f172a;
  color: #e2e8f0;
  padding: 1rem;
  margin: 1rem 0;
}

.docs-content :deep(code) {
  font-size: 0.875em;
}

.docs-content :deep(:not(pre) > code) {
  border-radius: 0.35rem;
  background: rgb(243 244 246);
  padding: 0.1rem 0.35rem;
  color: rgb(37 99 235);
}

.dark .docs-content :deep(:not(pre) > code) {
  background: rgb(30 41 59);
  color: rgb(147 197 253);
}

.docs-content :deep(.docs-copy-btn) {
  position: absolute;
  top: 0.55rem;
  right: 0.55rem;
  border: 1px solid rgba(148, 163, 184, 0.35);
  border-radius: 0.5rem;
  background: rgba(15, 23, 42, 0.85);
  color: #e2e8f0;
  font-size: 0.75rem;
  font-weight: 600;
  padding: 0.25rem 0.55rem;
  cursor: pointer;
}

.docs-content :deep(.docs-copy-btn:hover) {
  background: rgba(30, 41, 59, 0.95);
}

.docs-content :deep(blockquote) {
  border-left: 4px solid rgb(59 130 246);
  background: rgb(239 246 255);
  color: rgb(30 64 175);
  padding: 0.75rem 1rem;
  border-radius: 0 0.75rem 0.75rem 0;
}

.dark .docs-content :deep(blockquote) {
  background: rgba(30, 58, 138, 0.25);
  color: rgb(191 219 254);
}
</style>
