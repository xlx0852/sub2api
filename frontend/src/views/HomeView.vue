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

      <section id="stability" class="stability-section">
        <div class="home-container">
          <div class="stability-layout">
            <div class="stability-copy">
              <div class="section-label">{{ stabilityCopy.label }}</div>
              <h2 class="section-title">
                {{ stabilityCopy.titlePrefix }}
                <span>{{ stabilityCopy.titleHighlight }}</span>
              </h2>
              <p class="section-description">
                {{ stabilityCopy.description }}
              </p>
              <div class="stability-stats" role="list">
                <div
                  v-for="stat in stabilityStats"
                  :key="stat.label"
                  class="stability-stat"
                  role="listitem"
                >
                  <strong>{{ stat.value }}</strong>
                  <span>{{ stat.label }}</span>
                </div>
              </div>
            </div>

            <div class="stability-panel" aria-label="SicTs stability monitor">
              <div class="stability-panel-top">
                <div>
                  <span class="stability-kicker">{{ stabilityCopy.panelKicker }}</span>
                  <strong>{{ stabilityCopy.panelValue }}</strong>
                  <small>{{ stabilityCopy.panelNote }}</small>
                </div>
                <span class="stability-live">
                  <span></span>
                  LIVE
                </span>
              </div>

              <div class="stability-chart-card">
                <div class="chart-header">
                  <span>{{ stabilityCopy.chartTitle }}</span>
                  <span>{{ stabilityCopy.chartWindow }}</span>
                </div>
                <div class="chart-body">
                  <svg viewBox="0 0 720 280" role="img" :aria-label="stabilityCopy.chartTitle">
                    <g class="chart-grid-lines">
                      <line x1="64" y1="38" x2="692" y2="38" />
                      <line x1="64" y1="82" x2="692" y2="82" />
                      <line x1="64" y1="126" x2="692" y2="126" />
                      <line x1="64" y1="170" x2="692" y2="170" />
                      <line x1="64" y1="214" x2="692" y2="214" />
                    </g>
                    <g class="chart-axis-labels chart-y-labels">
                      <text x="24" y="43">100</text>
                      <text x="18" y="87">99.8</text>
                      <text x="18" y="131">99.6</text>
                      <text x="18" y="175">99.4</text>
                      <text x="18" y="219">99.2</text>
                    </g>
                    <g class="chart-axis-labels chart-x-labels">
                      <text x="64" y="248">30d</text>
                      <text x="238" y="248">21d</text>
                      <text x="412" y="248">14d</text>
                      <text x="594" y="248">7d</text>
                      <text x="675" y="248">now</text>
                    </g>
                    <path
                      class="chart-recovery-fill"
                      d="M64 42 C138 41 188 43 246 42 C284 42 308 44 326 60 C344 76 368 78 386 56 C406 42 458 42 522 43 C590 44 632 42 692 43 L692 214 L64 214 Z"
                    />
                    <path
                      class="chart-line chart-line-shadow"
                      d="M64 42 C138 41 188 43 246 42 C284 42 308 44 326 60 C344 76 368 78 386 56 C406 42 458 42 522 43 C590 44 632 42 692 43"
                    />
                    <path
                      class="chart-line"
                      d="M64 42 C138 41 188 43 246 42 C284 42 308 44 326 60 C344 76 368 78 386 56 C406 42 458 42 522 43 C590 44 632 42 692 43"
                    />
                    <path
                      class="chart-line chart-line-success"
                      d="M64 47 C184 47 304 46 424 47 C544 48 612 47 692 47"
                    />
                  </svg>
                </div>
                <div class="chart-footer">
                  <span>
                    <i></i>
                    {{ stabilityCopy.legendPrimary }}
                  </span>
                  <span>{{ stabilityCopy.generatedBy }}</span>
                </div>
              </div>
            </div>
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

const { t, locale } = useI18n()
const legacySubscriptionSubtitle = ['Subscription', 'to API Conversion Platform'].join(' ')
const legacyZhSubscriptionSubtitle = ['订阅转', 'API 转换平台'].join(' ')

const authStore = useAuthStore()
const appStore = useAppStore()

const defaultSiteName = 'SicTs'
const legacySiteNames = new Set(['Sub2API'])
const legacySubtitles = new Set([
  'AI API Gateway Platform',
  legacySubscriptionSubtitle,
  legacyZhSubscriptionSubtitle
])

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
  { name: t('home.providers.grok'), status: t('home.providers.supported'), tone: 'provider-supported' },
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

const isZhLocale = computed(() => String(locale.value).toLowerCase().startsWith('zh'))

const stabilityCopy = computed(() => isZhLocale.value
  ? {
      label: '稳定性',
      titlePrefix: '请求成功率，',
      titleHighlight: '持续可见',
      description: 'SicTs 会持续观察上游通道状态，并在异常时切到健康账号或节点，让业务请求尽量保持连续。',
      panelKicker: '近 30 天平均成功率',
      panelValue: '99.89%',
      panelNote: '跨模型、跨上游通道聚合展示',
      chartTitle: 'Uptime',
      chartWindow: '30d',
      legendPrimary: '请求成功率',
      generatedBy: 'SicTs Monitor'
    }
  : {
      label: 'Stability',
      titlePrefix: 'Request success, ',
      titleHighlight: 'always visible',
      description: 'SicTs continuously watches upstream channel health and routes around failures so production requests stay steady.',
      panelKicker: 'Average success rate, last 30 days',
      panelValue: '99.89%',
      panelNote: 'Aggregated across models and upstream channels',
      chartTitle: 'Uptime',
      chartWindow: '30d',
      legendPrimary: 'Request success',
      generatedBy: 'SicTs Monitor'
    })

const stabilityStats = computed(() => isZhLocale.value
  ? [
      { value: '30d', label: '滚动窗口' },
      { value: '< 2s', label: '异常切换' },
      { value: '24/7', label: '通道观察' }
    ]
  : [
      { value: '30d', label: 'Rolling window' },
      { value: '< 2s', label: 'Failover' },
      { value: '24/7', label: 'Channel watch' }
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

.stability-section {
  padding: 112px 0;
  background: var(--home-bg);
}

.stability-layout {
  display: grid;
  grid-template-columns: minmax(0, 0.72fr) minmax(560px, 1fr);
  align-items: center;
  gap: 76px;
}

.stability-copy {
  min-width: 0;
}

.stability-copy .section-header,
.stability-copy .section-description {
  text-align: left;
}

.stability-copy .section-description {
  margin: 22px 0 0;
}

.stability-stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 1px;
  overflow: hidden;
  margin-top: 38px;
  border: 1px solid var(--home-line);
  border-radius: 14px;
  background: var(--home-line);
}

.stability-stat {
  min-width: 0;
  padding: 22px 18px;
  background: var(--home-panel);
}

.stability-stat strong {
  display: block;
  color: var(--home-text);
  font-family: var(--home-serif);
  font-size: clamp(28px, 3vw, 38px);
  font-weight: 400;
  line-height: 1;
}

.stability-stat span {
  display: block;
  margin-top: 10px;
  color: var(--home-text-muted);
  font-size: 13px;
}

.stability-panel {
  min-width: 0;
}

.stability-panel-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  margin-bottom: 14px;
  padding: 24px 28px;
  border: 1px solid var(--home-line);
  border-radius: 14px;
  background: color-mix(in srgb, var(--home-panel) 92%, transparent);
  box-shadow: 0 18px 48px rgba(17, 24, 17, 0.04);
}

.stability-panel-top div {
  min-width: 0;
}

.stability-kicker,
.stability-panel-top small {
  display: block;
  color: var(--home-text-muted);
  font-size: 13px;
  line-height: 1.45;
}

.stability-panel-top strong {
  display: block;
  margin: 7px 0 4px;
  color: var(--home-text);
  font-family: var(--home-serif);
  font-size: clamp(42px, 4.6vw, 58px);
  font-weight: 400;
  line-height: 1;
}

.stability-live {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  flex: 0 0 auto;
  padding: 7px 10px;
  border-radius: 999px;
  background: #3fbd65;
  color: #ffffff;
  font-family: var(--home-mono);
  font-size: 11px;
  font-weight: 800;
  line-height: 1;
}

.stability-live span {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: currentColor;
  box-shadow: 0 0 0 4px rgba(255, 255, 255, 0.18);
}

.stability-chart-card {
  overflow: hidden;
  border: 1px solid var(--home-line);
  border-radius: 14px;
  background: var(--home-panel);
  box-shadow: 0 24px 70px rgba(17, 24, 17, 0.06);
}

.chart-header,
.chart-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  color: var(--home-text-secondary);
  font-size: 13px;
}

.chart-header {
  padding: 12px 18px;
  border-bottom: 1px solid var(--home-line);
}

.chart-header span:first-child {
  color: var(--home-text);
  font-size: 16px;
  font-weight: 700;
}

.chart-body {
  padding: 16px 18px 8px;
}

.chart-body svg {
  display: block;
  width: 100%;
  height: auto;
}

.chart-grid-lines line {
  stroke: var(--home-line);
  stroke-width: 1;
}

.chart-axis-labels text {
  fill: var(--home-text-secondary);
  font-family: var(--home-sans);
  font-size: 12px;
}

.chart-x-labels text {
  fill: var(--home-text-muted);
  font-size: 11px;
}

.chart-x-labels text:last-child {
  text-anchor: end;
}

.chart-recovery-fill {
  fill: rgba(79, 138, 99, 0.08);
}

.chart-line {
  fill: none;
  stroke: #34985a;
  stroke-linecap: round;
  stroke-linejoin: round;
  stroke-width: 3.2;
}

.chart-line-shadow {
  stroke: rgba(52, 152, 90, 0.16);
  stroke-width: 9;
}

.chart-line-success {
  stroke: rgba(52, 152, 90, 0.42);
  stroke-width: 1.6;
}

.chart-footer {
  padding: 11px 18px 14px;
}

.chart-footer span {
  display: inline-flex;
  align-items: center;
  gap: 9px;
}

.chart-footer i {
  display: inline-block;
  width: 11px;
  height: 11px;
  border-radius: 3px;
  background: #34985a;
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

  .stability-layout {
    grid-template-columns: 1fr;
    gap: 42px;
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
  .stability-section,
  .cta-section {
    padding: 92px 0;
  }

  .feature-card {
    padding: 34px 26px;
  }

  .stability-stats {
    grid-template-columns: 1fr;
  }

  .stability-panel-top {
    align-items: flex-start;
    flex-direction: column;
    padding: 22px;
  }

  .chart-body {
    padding: 14px 10px 6px;
  }

  .chart-axis-labels text {
    font-size: 15px;
  }

  .chart-footer {
    align-items: flex-start;
    flex-direction: column;
  }

  .footer-inner,
  .footer-bottom {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
