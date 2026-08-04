<template>
  <section id="pricing" class="pricing-section">
    <div class="pricing-inner">
      <div class="pricing-heading reveal-on-scroll">
        <div>
          <div class="pricing-kicker">{{ t('availableChannels.publicPricing.kicker') }}</div>
          <h2>{{ t('availableChannels.publicPricing.title') }}</h2>
          <p>{{ t('availableChannels.publicPricing.description') }}</p>
        </div>
        <dl class="pricing-summary">
          <div><dt>{{ t('availableChannels.stats.models') }}</dt><dd>{{ rows.length }}</dd></div>
          <div><dt>{{ t('availableChannels.stats.platforms') }}</dt><dd>{{ platforms.length }}</dd></div>
        </dl>
      </div>

      <div class="pricing-toolbar reveal-on-scroll">
        <label class="pricing-search">
          <Icon name="search" size="sm" />
          <input v-model="query" :placeholder="t('availableChannels.publicPricing.searchPlaceholder')" />
        </label>
        <div class="pricing-platforms">
          <button
            v-for="platform in ['all', ...platforms]"
            :key="platform"
            type="button"
            :class="{ active: selectedPlatform === platform }"
            @click="selectedPlatform = platform"
          >
            <PlatformIcon v-if="platform !== 'all'" :platform="platform as any" size="xs" />
            {{ platform === 'all' ? t('availableChannels.filters.allPlatforms') : platform }}
          </button>
        </div>
        <button class="pricing-refresh" type="button" :aria-label="t('common.refresh')" :title="t('common.refresh')" @click="load">
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
        </button>
      </div>

      <div v-if="errorMessage" class="pricing-message error">{{ errorMessage }}</div>
      <div v-else-if="loading && rows.length === 0" class="pricing-grid" aria-hidden="true">
        <div v-for="index in 8" :key="index" class="pricing-skeleton"></div>
      </div>
      <div v-else-if="filteredRows.length === 0" class="pricing-message">{{ t('availableChannels.empty') }}</div>
      <div v-else class="pricing-grid">
        <article v-for="row in filteredRows" :key="row.key" class="pricing-item reveal-on-scroll">
          <header>
            <span class="pricing-model-icon"><ModelIcon :model="row.name" size="20px" /></span>
            <span class="pricing-model-name"><strong>{{ row.displayName }}</strong><small>{{ row.name }}</small></span>
            <span class="pricing-provider"><PlatformIcon :platform="row.platform as any" size="xs" />{{ row.platform }}</span>
          </header>
          <div class="pricing-values">
            <div v-for="price in row.prices" :key="price.label">
              <span>{{ price.label }}</span>
              <strong>{{ price.value }}</strong>
            </div>
          </div>
          <footer>
            <span>{{ row.unit }}</span>
            <button type="button" :aria-label="t('common.copy')" :title="t('common.copy')" @click="copyToClipboard(row.name)">
              <Icon name="copy" size="xs" />
            </button>
          </footer>
        </article>
      </div>

      <div v-if="generatedAt" class="pricing-updated">
        {{ t('availableChannels.publicPricing.updatedAt', { time: generatedAt }) }}
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { getPublicPricing, type PublicPricingModel } from '@/api/publicPricing'
import { BILLING_MODE_IMAGE, BILLING_MODE_TOKEN } from '@/constants/channel'
import { formatScaled } from '@/utils/pricing'

const { t, locale } = useI18n()
const { copyToClipboard } = useClipboard()
const loading = ref(false)
const models = ref<PublicPricingModel[]>([])
const generated = ref('')
const errorMessage = ref('')
const query = ref('')
const selectedPlatform = ref('all')

function displayPrice(value: number | null | undefined, scale = 1_000_000) {
  return value == null ? '-' : formatScaled(value, scale)
}

const rows = computed(() => models.value.map((model) => {
  const pricing = model.pricing
  const perToken = pricing?.billing_mode === BILLING_MODE_TOKEN
  const unit = perToken ? t('availableChannels.pricing.unitPerMillion') : t('availableChannels.pricing.unitPerRequest')
  const prices = perToken
    ? [
        { label: t('availableChannels.pricing.inputPrice'), value: displayPrice(pricing?.input_price) },
        { label: t('availableChannels.pricing.outputPrice'), value: displayPrice(pricing?.output_price) },
        { label: t('availableChannels.pricing.cacheReadPrice'), value: displayPrice(pricing?.cache_read_price) },
      ]
    : [{ label: pricing?.billing_mode === BILLING_MODE_IMAGE ? t('availableChannels.pricing.imageOutputPrice') : t('availableChannels.pricing.perRequestPrice'), value: displayPrice(pricing?.image_output_price ?? pricing?.per_request_price, 1) }]
  return { key: `${model.platform}::${model.name}`, name: model.name, displayName: model.display_name || model.name, platform: model.platform, prices, unit }
}))

const platforms = computed(() => Array.from(new Set(rows.value.map((row) => row.platform))).sort())
const filteredRows = computed(() => {
  const q = query.value.trim().toLowerCase()
  return rows.value.filter((row) => {
    if (selectedPlatform.value !== 'all' && row.platform !== selectedPlatform.value) return false
    return !q || row.name.toLowerCase().includes(q) || row.displayName.toLowerCase().includes(q) || row.platform.toLowerCase().includes(q)
  })
})
const generatedAt = computed(() => generated.value ? new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(generated.value)) : '')

async function load() {
  if (loading.value) return
  loading.value = true
  errorMessage.value = ''
  try {
    const snapshot = await getPublicPricing()
    models.value = snapshot.models || []
    generated.value = snapshot.generated_at || ''
  } catch {
    errorMessage.value = t('availableChannels.publicPricing.loadFailed')
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.pricing-section { position: relative; z-index: 1; padding: 92px 0; border-top: 1px solid var(--home-line); border-bottom: 1px solid var(--home-line); background: color-mix(in srgb, var(--home-panel) 86%, transparent); }
.pricing-inner { width: min(1280px, calc(100% - 64px)); margin: 0 auto; }
.pricing-heading { display: flex; align-items: end; justify-content: space-between; gap: 40px; }
.pricing-kicker { margin-bottom: 10px; color: var(--home-text-muted); font: 600 12px/1 var(--home-mono); text-transform: uppercase; }
.pricing-heading h2 { margin: 0; font-family: var(--home-serif); font-size: clamp(32px, 4vw, 52px); line-height: 1.12; letter-spacing: 0; }
.pricing-heading p { max-width: 620px; margin: 14px 0 0; color: var(--home-text-secondary); font-size: 15px; }
.pricing-summary { display: grid; grid-template-columns: repeat(2, 120px); margin: 0; border-block: 1px solid var(--home-line-strong); }
.pricing-summary div { padding: 14px 16px; }
.pricing-summary div + div { border-left: 1px solid var(--home-line); }
.pricing-summary dt { color: var(--home-text-muted); font-size: 11px; }
.pricing-summary dd { margin: 3px 0 0; font: 600 24px/1.2 var(--home-mono); }
.pricing-toolbar { display: grid; grid-template-columns: minmax(240px, 360px) 1fr 38px; gap: 14px; align-items: center; margin-top: 36px; padding-block: 16px; border-block: 1px solid var(--home-line); }
.pricing-search { display: flex; align-items: center; gap: 9px; height: 38px; padding: 0 12px; border: 1px solid var(--home-line-strong); border-radius: 7px; background: var(--home-panel); color: var(--home-text-muted); }
.pricing-search input { width: 100%; border: 0; outline: 0; background: transparent; color: var(--home-text); font: 13px var(--home-sans); }
.pricing-platforms { display: flex; flex-wrap: wrap; gap: 7px; }
.pricing-platforms button, .pricing-refresh { display: inline-flex; align-items: center; justify-content: center; gap: 6px; height: 34px; border: 0; border-radius: 6px; background: transparent; color: var(--home-text-secondary); font: 600 11px var(--home-sans); cursor: pointer; }
.pricing-platforms button { padding: 0 11px; }
.pricing-platforms button:hover, .pricing-platforms button.active { background: var(--home-brand-wash); color: var(--home-text); }
.pricing-refresh { width: 34px; }
.pricing-refresh:hover { background: var(--home-brand-wash); color: var(--home-text); }
.pricing-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; margin-top: 20px; }
.pricing-item { min-width: 0; padding: 16px; border: 1px solid var(--home-line); border-radius: 7px; background: var(--home-panel); transition: border-color .2s ease, transform .2s ease, box-shadow .2s ease; }
.pricing-item:hover { transform: translateY(-2px); border-color: var(--home-line-strong); box-shadow: 0 10px 24px rgba(23, 25, 21, .06); }
.pricing-item header { display: grid; grid-template-columns: 34px minmax(0, 1fr) auto; gap: 10px; align-items: center; }
.pricing-model-icon { display: grid; place-items: center; width: 34px; height: 34px; border: 1px solid var(--home-line); border-radius: 7px; background: var(--home-bg-soft); }
.pricing-model-name { min-width: 0; }
.pricing-model-name strong, .pricing-model-name small { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.pricing-model-name strong { font-size: 13px; }
.pricing-model-name small { margin-top: 2px; color: var(--home-text-muted); font: 10px var(--home-mono); }
.pricing-provider { display: inline-flex; align-items: center; gap: 4px; color: var(--home-text-muted); font-size: 10px; text-transform: uppercase; }
.pricing-values { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); margin-top: 16px; border-block: 1px solid var(--home-line); }
.pricing-values div { min-width: 0; padding: 10px 6px 10px 0; }
.pricing-values div + div { padding-left: 9px; border-left: 1px solid var(--home-line); }
.pricing-values span, .pricing-values strong { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.pricing-values span { color: var(--home-text-muted); font-size: 9px; }
.pricing-values strong { margin-top: 4px; font: 600 12px var(--home-mono); }
.pricing-item footer { display: flex; align-items: center; justify-content: space-between; margin-top: 10px; color: var(--home-text-muted); font: 10px var(--home-mono); }
.pricing-item footer button { display: grid; place-items: center; width: 26px; height: 26px; border: 0; border-radius: 5px; background: transparent; color: inherit; cursor: pointer; }
.pricing-item footer button:hover { background: var(--home-brand-wash); color: var(--home-text); }
.pricing-message { margin-top: 20px; padding: 50px 20px; border-block: 1px solid var(--home-line); color: var(--home-text-muted); text-align: center; font-size: 13px; }
.pricing-message.error { color: #b42318; }
.pricing-skeleton { height: 165px; border-radius: 7px; background: linear-gradient(90deg, var(--home-bg-soft), var(--home-panel), var(--home-bg-soft)); background-size: 200% 100%; animation: pricing-shimmer 1.4s infinite; }
.pricing-updated { margin-top: 14px; color: var(--home-text-muted); font: 10px var(--home-mono); text-align: right; }
@keyframes pricing-shimmer { to { background-position: -200% 0; } }
@media (max-width: 1080px) { .pricing-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
@media (max-width: 820px) { .pricing-inner { width: min(100% - 32px, 1280px); } .pricing-heading { align-items: start; flex-direction: column; gap: 24px; } .pricing-toolbar { grid-template-columns: 1fr 38px; } .pricing-platforms { grid-column: 1 / -1; grid-row: 2; } .pricing-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 560px) { .pricing-section { padding: 68px 0; } .pricing-summary { width: 100%; grid-template-columns: repeat(2, 1fr); } .pricing-grid { grid-template-columns: 1fr; } }
</style>
