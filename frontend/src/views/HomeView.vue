<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="homeContent" class="min-h-screen">
    <!-- iframe mode -->
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <!-- HTML mode - SECURITY: homeContent is admin-only setting, XSS risk is acceptable -->
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Default Home Page -->
  <div v-else class="home-page">
    <div class="home-grid" aria-hidden="true"></div>

    <header class="home-nav">
      <nav class="home-nav-inner" aria-label="Home navigation">
        <router-link to="/home" class="home-logo" :aria-label="siteName">
          <span class="home-logo-mark">
            <img :src="siteLogo || '/logo.png'" alt="" />
          </span>
          <span class="home-logo-text">{{ siteName }}</span>
        </router-link>

        <div class="home-nav-links">
          <a href="#features">{{ t('home.navigation.features') }}</a>
          <a href="#providers">{{ t('home.navigation.models') }}</a>
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">
            {{ t('home.navigation.docs') }}
          </a>
        </div>

        <div class="home-nav-actions">
          <LocaleSwitcher />

          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="home-icon-btn"
            :title="t('home.viewDocs')"
            :aria-label="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>

          <button
            type="button"
            @click="toggleTheme"
            class="home-icon-btn"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>

          <router-link
            v-if="isAuthenticated"
            :to="dashboardPath"
            class="home-dashboard-btn"
          >
            <span class="home-user-dot">{{ userInitial }}</span>
            <span>{{ t('home.dashboard') }}</span>
            <Icon name="arrowRight" size="xs" :stroke-width="2" />
          </router-link>
          <router-link v-else to="/login" class="home-dashboard-btn">
            <span>{{ t('home.login') }}</span>
            <Icon name="arrowRight" size="xs" :stroke-width="2" />
          </router-link>
        </div>
      </nav>
    </header>

    <main class="home-main">
      <section class="hero-section">
        <div class="home-container">
          <div class="hero-layout">
            <div class="hero-copy">
              <div class="hero-eyebrow">
                <span class="status-dot"></span>
                {{ t('home.heroSubtitle') }}
              </div>

              <h1 class="hero-title">
                <span class="hero-brand-line">{{ siteName }}</span>
                <span>
                  <span class="hero-highlight">{{ t('home.heroTitleHighlight') }}</span>
                  {{ t('home.heroTitleSuffix') }}
                </span>
              </h1>

              <p class="hero-description">
                {{ t('home.heroDescription') }}
              </p>

              <div class="hero-actions">
                <router-link
                  :to="isAuthenticated ? dashboardPath : '/login'"
                  class="home-btn home-btn-primary"
                >
                  {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
                  <Icon name="arrowRight" size="sm" :stroke-width="2" />
                </router-link>
                <a
                  v-if="docUrl"
                  :href="docUrl"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="home-btn home-btn-secondary"
                >
                  {{ t('home.docs') }}
                </a>
              </div>
            </div>

            <div class="hero-visual" aria-hidden="true">
              <div class="terminal-card">
                <div class="terminal-header">
                  <div class="terminal-dots">
                    <span></span>
                    <span></span>
                    <span></span>
                  </div>
                  <span class="terminal-title">terminal</span>
                </div>
                <div class="terminal-body">
                  <span class="term-line term-muted"># 在 ~/.sicts/settings.json 中配置一次</span>
                  <span class="term-line">
                    <span class="term-prompt">→</span>
                    <span class="term-path">~/project</span>
                    <span class="term-cmd"> sicts </span>
                    <span class="term-string">"调用任意模型"</span>
                  </span>
                  <span class="term-line">
                    <span class="term-dot"></span>
                    <span class="term-muted">已连接&nbsp;</span>
                    <span class="term-url">api.sicts.io</span>
                    <span class="term-muted">&nbsp;→ Claude / GPT / Gemini</span>
                  </span>
                  <span class="term-line">
                    <span class="term-dot term-dot-muted"></span>
                    <span class="term-muted">路由节点 auto · 会话保持 · 用量统计</span>
                  </span>
                  <span class="term-line">
                    <span class="term-prompt">→</span>
                    <span class="term-path">~/project</span>
                    <span class="term-cursor"></span>
                  </span>
                </div>
              </div>
            </div>
          </div>

          <div class="hero-tags" aria-label="Highlights">
            <span v-for="tag in heroTags" :key="tag.icon">
              <Icon :name="tag.icon" size="sm" />
              {{ tag.label }}
            </span>
          </div>
        </div>
      </section>

      <section id="providers" class="providers-section">
        <div class="home-container">
          <div class="providers-label">{{ t('home.providers.title') }}</div>
          <p class="providers-description">{{ t('home.providers.description') }}</p>
          <div class="providers-row" role="list">
            <span
              v-for="provider in providers"
              :key="provider.name"
              class="provider-item"
              :class="provider.tone"
              role="listitem"
            >
              <span class="provider-name">{{ provider.name }}</span>
              <small>{{ provider.status }}</small>
            </span>
          </div>
        </div>
      </section>

      <section id="features" class="features-section">
        <div class="home-container">
          <div class="section-header">
            <div class="section-label">{{ t('home.featureSection.label') }}</div>
            <h2 class="section-title">
              {{ t('home.featureSection.titlePrefix') }}
              <span>{{ t('home.featureSection.titleHighlight') }}</span>
            </h2>
            <p class="section-description">
              {{ t('home.featureSection.description') }}
            </p>
          </div>

          <div class="features-grid">
            <article
              v-for="feature in features"
              :key="feature.index"
              class="feature-card"
            >
              <div class="feature-meta">
                <span>{{ feature.index }}</span>
                <span class="feature-icon">
                  <Icon :name="feature.icon" size="sm" />
                </span>
              </div>
              <h3>
                {{ feature.title }}
                <span>{{ feature.highlight }}</span>
              </h3>
              <p>{{ feature.description }}</p>
            </article>
          </div>
        </div>
      </section>

      <section class="cta-section">
        <div class="home-container">
          <h2>{{ t('home.cta.title') }}</h2>
          <p>{{ t('home.cta.description') }}</p>
          <div class="cta-actions">
            <router-link
              :to="isAuthenticated ? dashboardPath : '/login'"
              class="home-btn home-btn-primary"
            >
              {{ isAuthenticated ? t('home.goToDashboard') : t('home.cta.button') }}
              <Icon name="arrowRight" size="sm" :stroke-width="2" />
            </router-link>
            <a
              v-if="docUrl"
              :href="docUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="home-btn home-btn-secondary"
            >
              {{ t('home.docs') }}
            </a>
          </div>
        </div>
      </section>
    </main>

    <footer class="home-footer">
      <div class="home-container">
        <div class="footer-inner">
          <div class="footer-brand">
            <div class="footer-logo">
              <span class="home-logo-mark">
                <img :src="siteLogo || '/logo.png'" alt="" />
              </span>
              {{ siteName }}
            </div>
            <p>{{ siteSubtitle }}</p>
          </div>
          <div class="footer-links">
            <a
              v-if="docUrl"
              :href="docUrl"
              target="_blank"
              rel="noopener noreferrer"
            >
              {{ t('home.docs') }}
            </a>
          </div>
        </div>
        <div class="footer-bottom">
          <span>&copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}</span>
          <span class="footer-status">
            <span class="status-dot"></span>
            {{ t('home.footer.operational') }}
          </span>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const legacySubscriptionSubtitle = ['Subscription', 'to API Conversion Platform'].join(' ')

const authStore = useAuthStore()
const appStore = useAppStore()

const defaultSiteName = 'SicTs'
const legacySiteNames = new Set(['Sub2API'])
const legacySubtitles = new Set(['AI API Gateway Platform', legacySubscriptionSubtitle, '订阅转 API 转换平台'])

function normalizeText(value?: string) {
  const text = value?.trim()
  return text || ''
}

// Site settings - directly from appStore (already initialized from injected config)
const siteName = computed(() => {
  const configuredName = normalizeText(appStore.cachedPublicSettings?.site_name || appStore.siteName)
  return configuredName && !legacySiteNames.has(configuredName) ? configuredName : defaultSiteName
})
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const siteSubtitle = computed(() => {
  const configuredSubtitle = normalizeText(appStore.cachedPublicSettings?.site_subtitle)
  return configuredSubtitle && !legacySubtitles.has(configuredSubtitle)
    ? configuredSubtitle
    : t('home.defaultSubtitle')
})
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')

// Check if homeContent is a URL (for iframe display)
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

// Theme
const isDark = ref(document.documentElement.classList.contains('dark'))

// Auth state
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const userInitial = computed(() => {
  const user = authStore.user
  if (!user || !user.email) return ''
  return user.email.charAt(0).toUpperCase()
})

const heroTags = computed(() => [
  { icon: 'swap' as const, label: t('home.tags.modelGateway') },
  { icon: 'shield' as const, label: t('home.tags.stickySession') },
  { icon: 'chart' as const, label: t('home.tags.realtimeBilling') }
])

const providers = computed(() => [
  { name: t('home.providers.claude'), status: t('home.providers.supported'), tone: 'provider-supported' },
  { name: 'GPT', status: t('home.providers.supported'), tone: 'provider-supported' },
  { name: t('home.providers.gemini'), status: t('home.providers.supported'), tone: 'provider-supported' },
  { name: t('home.providers.antigravity'), status: t('home.providers.supported'), tone: 'provider-supported' },
  { name: t('home.providers.more'), status: t('home.providers.soon'), tone: 'provider-muted' }
])

const features = computed(() => [
  {
    index: '01 / Gateway',
    icon: 'server' as const,
    title: t('home.features.unifiedGateway'),
    highlight: t('home.features.unifiedGatewayHighlight'),
    description: t('home.features.unifiedGatewayDesc')
  },
  {
    index: '02 / Routing',
    icon: 'users' as const,
    title: t('home.features.multiAccount'),
    highlight: t('home.features.multiAccountHighlight'),
    description: t('home.features.multiAccountDesc')
  },
  {
    index: '03 / Billing',
    icon: 'dollar' as const,
    title: t('home.features.balanceQuota'),
    highlight: t('home.features.balanceQuotaHighlight'),
    description: t('home.features.balanceQuotaDesc')
  },
  {
    index: '04 / Console',
    icon: 'chart' as const,
    title: t('home.features.teamConsole'),
    highlight: t('home.features.teamConsoleHighlight'),
    description: t('home.features.teamConsoleDesc')
  }
])

// Current year for footer
const currentYear = computed(() => new Date().getFullYear())

// Toggle theme
function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

// Initialize theme
function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

onMounted(() => {
  initTheme()

  // Check auth state
  authStore.checkAuth()

  // Ensure public settings are loaded (will use cache if already loaded from injected config)
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
.home-page {
  --home-bg: #fbfbf8;
  --home-bg-soft: #f4f4ef;
  --home-panel: #ffffff;
  --home-line: #e8e8e2;
  --home-line-strong: #d5d5cd;
  --home-text: #171915;
  --home-text-secondary: #52524c;
  --home-text-muted: #85857b;
  --home-brand: #4f8a63;
  --home-brand-dark: #3f754f;
  --home-brand-wash: #edf7f0;
  --home-brand-border: #b8d8c1;
  --home-button-bg: #171915;
  --home-button-text: #ffffff;
  --home-button-hover: #0f120f;
  --home-serif: Georgia, 'Songti SC', STSong, 'Times New Roman', serif;
  --home-sans: Inter, system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC',
    'Microsoft YaHei', sans-serif;
  --home-mono: 'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  position: relative;
  min-height: 100vh;
  overflow-x: hidden;
  background: var(--home-bg);
  color: var(--home-text);
  font-family: var(--home-sans);
  line-height: 1.6;
}

:global(.dark) .home-page {
  --home-bg: #080a08;
  --home-bg-soft: #10130f;
  --home-panel: #121712;
  --home-line: #242b23;
  --home-line-strong: #3a4438;
  --home-text: #f6f7f1;
  --home-text-secondary: #c7cabf;
  --home-text-muted: #8d9688;
  --home-brand-wash: rgba(79, 138, 99, 0.18);
  --home-brand-border: rgba(115, 174, 132, 0.36);
  --home-button-bg: #f6f7f1;
  --home-button-text: #080a08;
  --home-button-hover: #ffffff;
}

.home-grid {
  position: fixed;
  inset: 0;
  z-index: 0;
  pointer-events: none;
  background-image:
    linear-gradient(var(--home-line) 1px, transparent 1px),
    linear-gradient(90deg, var(--home-line) 1px, transparent 1px);
  background-size: 64px 64px;
  opacity: 0.7;
  mask-image: radial-gradient(80% 60% at 50% 28%, #000 0%, transparent 75%);
}

.home-container {
  position: relative;
  z-index: 1;
  width: 100%;
  max-width: 1280px;
  margin: 0 auto;
  padding: 0 32px;
}

.home-nav {
  position: fixed;
  inset: 0 0 auto;
  z-index: 50;
  border-bottom: 1px solid var(--home-line);
  background: color-mix(in srgb, var(--home-bg) 82%, transparent);
  backdrop-filter: blur(16px) saturate(170%);
}

.home-nav-inner {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  gap: 20px;
  width: 100%;
  max-width: 1280px;
  margin: 0 auto;
  padding: 15px 32px;
}

.home-logo,
.footer-logo {
  display: inline-flex;
  align-items: center;
  min-width: 0;
  gap: 10px;
  color: var(--home-text);
  font-family: var(--home-serif);
  font-size: 22px;
  font-weight: 600;
  letter-spacing: 0;
  text-decoration: none;
}

.home-logo-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 26px;
  height: 26px;
  overflow: hidden;
  border: 1px solid var(--home-line);
  border-radius: 7px;
  background: var(--home-panel);
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.06);
}

.home-logo-mark img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.home-logo-text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.home-nav-links {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 34px;
}

.home-nav-links a,
.footer-links a {
  color: var(--home-text-secondary);
  font-size: 14px;
  font-weight: 500;
  text-decoration: none;
  transition: color 0.15s ease;
}

.home-nav-links a:hover,
.footer-links a:hover {
  color: var(--home-text);
}

.home-nav-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 10px;
  min-width: 0;
}

.home-icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  flex: 0 0 auto;
  border: 1px solid transparent;
  border-radius: 8px;
  color: var(--home-text-muted);
  background: transparent;
  transition:
    color 0.18s ease,
    border-color 0.18s ease,
    background 0.18s ease;
}

.home-icon-btn:hover {
  border-color: var(--home-line);
  background: var(--home-panel);
  color: var(--home-text);
}

.home-dashboard-btn {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 36px;
  padding: 8px 14px;
  border: 1px solid var(--home-button-bg);
  border-radius: 7px;
  background: var(--home-button-bg);
  color: var(--home-button-text);
  font-size: 13px;
  font-weight: 600;
  text-decoration: none;
  transition:
    transform 0.18s ease,
    background 0.18s ease,
    border-color 0.18s ease;
}

.home-dashboard-btn:hover {
  border-color: var(--home-button-hover);
  background: var(--home-button-hover);
  transform: translateY(-1px);
}

.home-user-dot {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background: var(--home-brand);
  color: #f8faf5;
  font-size: 10px;
  font-weight: 800;
}

.home-main {
  position: relative;
  z-index: 1;
}

.hero-section {
  display: flex;
  align-items: center;
  min-height: auto;
  padding: 138px 0 44px;
}

.hero-layout {
  display: grid;
  grid-template-columns: minmax(0, 0.95fr) minmax(480px, 1fr);
  align-items: center;
  gap: 72px;
}

.hero-copy {
  min-width: 0;
}

.hero-eyebrow {
  display: inline-flex;
  align-items: center;
  gap: 9px;
  max-width: 100%;
  margin-bottom: 28px;
  padding: 7px 14px 7px 10px;
  border: 1px solid var(--home-line);
  border-radius: 999px;
  background: var(--home-panel);
  color: var(--home-text-secondary);
  font-family: var(--home-mono);
  font-size: 12px;
  font-weight: 500;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
}

.status-dot {
  display: inline-block;
  width: 7px;
  height: 7px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: var(--home-brand);
  box-shadow: 0 0 0 4px rgba(79, 138, 99, 0.16);
}

.hero-title {
  margin: 0 0 26px;
  color: var(--home-text);
  font-family: var(--home-serif);
  font-size: clamp(52px, 5.6vw, 76px);
  font-weight: 400;
  letter-spacing: 0;
  line-height: 1.05;
}

.hero-title > span {
  display: block;
}

.hero-title > span + span {
  margin-top: 8px;
}

.hero-title .hero-brand-line {
  color: var(--home-text);
}

.hero-title .hero-highlight {
  display: inline;
  color: var(--home-brand-dark);
}

.hero-description {
  max-width: 600px;
  margin: 0 0 36px;
  color: var(--home-text-secondary);
  font-size: 17px;
  line-height: 1.75;
}

.hero-actions,
.cta-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.home-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 9px;
  min-height: 52px;
  padding: 13px 24px;
  border-radius: 10px;
  font-size: 15px;
  font-weight: 750;
  text-decoration: none;
  transition:
    transform 0.18s ease,
    box-shadow 0.18s ease,
    border-color 0.18s ease,
    background 0.18s ease;
}

.home-btn-primary {
  border: 1px solid var(--home-button-bg);
  background: var(--home-button-bg);
  color: var(--home-button-text);
  box-shadow: 0 14px 28px rgba(17, 24, 17, 0.14);
}

.home-btn-primary:hover {
  border-color: var(--home-button-hover);
  background: var(--home-button-hover);
  box-shadow: 0 18px 34px rgba(17, 24, 17, 0.18);
  transform: translateY(-1px);
}

.home-btn-secondary {
  border: 1px solid var(--home-line-strong);
  background: var(--home-panel);
  color: var(--home-text);
}

.home-btn-secondary:hover {
  border-color: var(--home-text-muted);
  background: var(--home-bg-soft);
  transform: translateY(-1px);
}

.hero-tags {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 10px;
  margin-top: 52px;
}

.hero-tags span {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 38px;
  padding: 8px 18px;
  border: 1px solid var(--home-line);
  border-radius: 999px;
  background: color-mix(in srgb, var(--home-panel) 92%, transparent);
  color: var(--home-text-secondary);
  font-size: 13px;
  font-weight: 600;
  box-shadow: 0 8px 18px rgba(15, 23, 42, 0.04);
}

.hero-tags svg {
  color: var(--home-brand-dark);
}

.hero-visual {
  display: flex;
  justify-content: flex-end;
  min-width: 0;
}

.terminal-card {
  width: min(100%, 580px);
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.14);
  border-radius: 14px;
  background: #101318;
  box-shadow:
    0 46px 90px -34px rgba(17, 24, 17, 0.58),
    0 34px 80px -44px rgba(79, 138, 99, 0.42);
  text-align: left;
  transform: rotate(-1.4deg) translateY(0);
  transition: transform 0.35s cubic-bezier(0.2, 0.8, 0.2, 1);
}

.terminal-card:hover {
  transform: rotate(-0.8deg) translateY(-4px);
}

.terminal-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 18px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.07);
  background: #191f28;
}

.terminal-dots {
  display: flex;
  gap: 6px;
}

.terminal-dots span {
  width: 11px;
  height: 11px;
  border-radius: 50%;
}

.terminal-dots span:nth-child(1) {
  background: #ff5f56;
}

.terminal-dots span:nth-child(2) {
  background: #ffbd2e;
}

.terminal-dots span:nth-child(3) {
  background: #27c93f;
}

.terminal-title {
  flex: 1;
  overflow: hidden;
  color: #64748b;
  font-family: var(--home-mono);
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-align: center;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.terminal-body {
  min-height: 242px;
  overflow-x: auto;
  padding: 30px 32px 34px;
  color: #e5e7eb;
  font-family: var(--home-mono);
  font-size: 14px;
  line-height: 2.05;
}

.term-line {
  display: block;
  min-height: 29px;
  white-space: nowrap;
}

.term-muted {
  color: #8d96a3;
}

.term-prompt,
.term-ok {
  color: var(--home-brand);
}

.term-prompt {
  margin-right: 9px;
}

.term-path {
  color: #7dd3fc;
}

.term-cmd {
  color: #c4b5fd;
}

.term-flag {
  color: #a78bfa;
}

.term-url {
  color: #48c78e;
}

.term-string {
  color: #fbbf24;
}

.term-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  margin-right: 10px;
  border-radius: 50%;
  background: #48c78e;
  box-shadow: 0 0 0 4px rgba(72, 199, 142, 0.12);
}

.term-dot-muted {
  background: #64748b;
  box-shadow: none;
}

.term-cursor {
  display: inline-block;
  width: 8px;
  height: 16px;
  margin-left: 4px;
  vertical-align: text-bottom;
  background: var(--home-brand);
  animation: cursor-blink 1.1s step-end infinite;
}

@keyframes cursor-blink {
  0%,
  50% {
    opacity: 1;
  }

  51%,
  100% {
    opacity: 0;
  }
}

.home-footer {
  border-top: 1px solid var(--home-line);
  border-bottom: 1px solid var(--home-line);
  background: var(--home-bg-soft);
}

.providers-section {
  padding: 42px 0 78px;
  background: transparent;
}

.providers-label {
  margin-bottom: 12px;
  color: var(--home-brand-dark);
  font-family: var(--home-mono);
  font-size: 12px;
  font-weight: 650;
  letter-spacing: 0;
  text-align: center;
}

.providers-description {
  margin: 0 0 24px;
  color: var(--home-text-muted);
  font-size: 13px;
  text-align: center;
}

.providers-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 18px 62px;
}

.provider-item {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  min-height: 40px;
  padding: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  color: var(--home-text-secondary);
  font-family: var(--home-serif);
  font-size: clamp(24px, 2.5vw, 34px);
  font-weight: 400;
  letter-spacing: 0;
  box-shadow: none;
}

.provider-name {
  white-space: nowrap;
}

.provider-item small {
  padding: 3px 8px;
  border: 1px solid var(--home-brand-border);
  border-radius: 999px;
  background: var(--home-brand-wash);
  color: var(--home-brand-dark);
  font-family: var(--home-sans);
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0;
}

.provider-muted {
  color: var(--home-text-muted);
}

.provider-muted small {
  background: var(--home-bg-soft);
  color: var(--home-text-muted);
}

.features-section {
  padding: 116px 0;
  border-top: 1px solid var(--home-line);
  border-bottom: 1px solid var(--home-line);
  background:
    linear-gradient(var(--home-line) 1px, transparent 1px),
    linear-gradient(90deg, var(--home-line) 1px, transparent 1px),
    var(--home-bg-soft);
  background-size: 64px 64px;
}

.cta-section {
  padding: 102px 0;
}

.section-header {
  max-width: 720px;
  margin: 0 auto 72px;
  text-align: center;
}

.section-label {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 18px;
  color: var(--home-brand-dark);
  font-family: var(--home-mono);
  font-size: 11px;
  font-weight: 650;
  letter-spacing: 0.18em;
  text-transform: uppercase;
}

.section-label::before {
  width: 18px;
  height: 1px;
  background: var(--home-brand-dark);
  content: '';
}

.section-title,
.cta-section h2 {
  margin: 0;
  color: var(--home-text);
  font-family: var(--home-serif);
  font-size: clamp(42px, 4.8vw, 64px);
  font-weight: 400;
  letter-spacing: 0;
  line-height: 1.1;
}

.section-title span,
.feature-card h3 span {
  color: var(--home-brand-dark);
}

.section-description,
.cta-section p {
  max-width: 650px;
  margin: 20px auto 0;
  color: var(--home-text-secondary);
  font-size: 17px;
  line-height: 1.7;
}

.features-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1px;
  overflow: hidden;
  border: 1px solid var(--home-line);
  border-radius: 16px;
  background: var(--home-line);
}

.feature-card {
  display: flex;
  min-height: 292px;
  min-width: 0;
  flex-direction: column;
  align-items: flex-start;
  padding: 52px 44px 46px;
  border: 0;
  border-radius: 0;
  background: color-mix(in srgb, var(--home-panel) 96%, transparent);
  box-shadow: none;
  transition:
    background 0.2s ease,
    color 0.2s ease;
}

.feature-card:hover {
  background: var(--home-panel);
}

.feature-meta {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  width: 100%;
  gap: 24px;
  margin-bottom: 48px;
  color: var(--home-text-muted);
  font-family: var(--home-mono);
  font-size: 12px;
  font-weight: 600;
}

.feature-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  flex: 0 0 auto;
  border: 1px solid var(--home-brand-border);
  border-radius: 999px;
  background: var(--home-brand-wash);
  color: var(--home-brand-dark);
}

.feature-card h3 {
  margin: 0 0 18px;
  color: var(--home-text);
  font-family: var(--home-serif);
  font-size: clamp(28px, 3vw, 38px);
  font-weight: 400;
  letter-spacing: 0;
  line-height: 1.18;
}

.feature-card p {
  margin: 0;
  color: var(--home-text-secondary);
  font-size: 16px;
  line-height: 1.75;
}

.cta-section {
  text-align: center;
}

.cta-actions {
  justify-content: center;
  margin-top: 36px;
}

.home-footer {
  padding: 54px 0 34px;
  border-bottom: 0;
}

.footer-inner {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 36px;
}

.footer-brand {
  max-width: 420px;
}

.footer-logo {
  margin-bottom: 14px;
}

.footer-brand p {
  margin: 0;
  color: var(--home-text-muted);
  font-size: 14px;
  line-height: 1.7;
}

.footer-links {
  display: flex;
  flex-wrap: wrap;
  gap: 22px;
}

.footer-bottom {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  margin-top: 42px;
  padding-top: 22px;
  border-top: 1px solid var(--home-line);
  color: var(--home-text-muted);
  font-family: var(--home-mono);
  font-size: 12px;
}

.footer-status {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  white-space: nowrap;
}

@media (max-width: 1040px) {
  .home-nav-inner {
    grid-template-columns: minmax(0, 1fr) auto;
  }

  .home-nav-links {
    display: none;
  }

  .hero-layout {
    grid-template-columns: 1fr;
    gap: 54px;
  }

  .hero-visual {
    justify-content: center;
  }

  .terminal-card {
    transform: none;
  }

  .features-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 720px) {
  .home-container {
    padding: 0 20px;
  }

  .home-nav-inner {
    padding: 12px 16px;
  }

  .home-logo {
    font-size: 18px;
  }

  .home-icon-btn {
    display: none;
  }

  .home-dashboard-btn {
    padding: 8px 11px;
  }

  .home-dashboard-btn svg {
    display: none;
  }

  .hero-section {
    min-height: auto;
    padding: 112px 0 62px;
  }

  .hero-title {
    font-size: clamp(40px, 12vw, 52px);
  }

  .hero-tags {
    justify-content: flex-start;
    margin-top: 42px;
  }

  .hero-actions,
  .cta-actions {
    align-items: stretch;
    flex-direction: column;
  }

  .home-btn {
    width: 100%;
  }

  .terminal-body {
    min-height: 220px;
    padding: 20px 18px 24px;
    font-size: 12px;
  }

  .providers-section {
    padding: 44px 0 62px;
  }

  .providers-row {
    gap: 16px 28px;
  }

  .provider-item {
    width: auto;
    justify-content: center;
    font-size: 25px;
  }

  .features-section,
  .cta-section {
    padding: 92px 0;
  }

  .feature-card {
    padding: 34px 26px;
  }

  .footer-inner,
  .footer-bottom {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
