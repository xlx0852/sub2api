import type {
  UserAvailableChannel,
  UserAvailableGroup,
  UserSupportedModel,
  UserSupportedModelPricing,
} from '@/api/channels'
import type { CatalogModelEntry, ModelCatalog } from '@/api/admin/modelCatalog'
import {
  BILLING_MODE_TOKEN,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_IMAGE,
  type BillingMode,
} from '@/constants/channel'

export interface ModelPlazaGroupOffer {
  id: number
  name: string
  platform: string
  subscription_type: string
  rate_multiplier: number
  peak_rate_enabled: boolean
  peak_start: string
  peak_end: string
  peak_rate_multiplier: number
  is_exclusive: boolean
  channel_name: string
  channel_description: string
  /** 乘上用户有效倍率后的展示价（用于列表直接显示） */
  effective_pricing: UserSupportedModelPricing | null
  base_pricing: UserSupportedModelPricing | null
}

export interface ModelPlazaRow {
  key: string
  name: string
  platform: string
  display_name: string
  family?: string
  is_reasoning: boolean
  media: Record<string, boolean>
  tags: string[]
  offers: ModelPlazaGroupOffer[]
  /** 代表价：取最低有效输入/按次价所在 offer */
  best_offer: ModelPlazaGroupOffer | null
}

function scalePrice(value: number | null | undefined, multiplier: number): number | null {
  if (value == null) return null
  return value * multiplier
}

export function applyRateToPricing(
  pricing: UserSupportedModelPricing | null,
  multiplier: number,
): UserSupportedModelPricing | null {
  if (!pricing) return null
  const m = Number.isFinite(multiplier) && multiplier > 0 ? multiplier : 1
  return {
    ...pricing,
    input_price: scalePrice(pricing.input_price, m),
    output_price: scalePrice(pricing.output_price, m),
    cache_write_price: scalePrice(pricing.cache_write_price, m),
    cache_read_price: scalePrice(pricing.cache_read_price, m),
    image_output_price: scalePrice(pricing.image_output_price, m),
    per_request_price: scalePrice(pricing.per_request_price, m),
    intervals: (pricing.intervals || []).map((iv) => ({
      ...iv,
      input_price: scalePrice(iv.input_price, m),
      output_price: scalePrice(iv.output_price, m),
      cache_write_price: scalePrice(iv.cache_write_price, m),
      cache_read_price: scalePrice(iv.cache_read_price, m),
      per_request_price: scalePrice(iv.per_request_price, m),
    })),
  }
}

function effectiveMultiplier(
  group: UserAvailableGroup,
  userGroupRates: Record<number, number>,
): number {
  const userRate = userGroupRates[group.id]
  if (typeof userRate === 'number' && userRate > 0) return userRate
  if (typeof group.rate_multiplier === 'number' && group.rate_multiplier > 0) {
    return group.rate_multiplier
  }
  return 1
}

function catalogIndex(catalog: ModelCatalog | null | undefined): Map<string, CatalogModelEntry> {
  const map = new Map<string, CatalogModelEntry>()
  if (!catalog?.platforms) return map
  for (const [platform, cfg] of Object.entries(catalog.platforms)) {
    for (const model of cfg.models || []) {
      if (!model?.id) continue
      map.set(`${platform.toLowerCase()}::${model.id.toLowerCase()}`, model)
      map.set(model.id.toLowerCase(), model)
    }
    // aliases → canonical entry when possible
    for (const [alias, target] of Object.entries(cfg.aliases || {})) {
      const hit =
        map.get(`${platform.toLowerCase()}::${String(target).toLowerCase()}`) ||
        map.get(String(target).toLowerCase())
      if (hit) {
        map.set(`${platform.toLowerCase()}::${alias.toLowerCase()}`, hit)
        map.set(alias.toLowerCase(), hit)
      }
    }
  }
  return map
}

function inferTags(model: UserSupportedModel, meta?: CatalogModelEntry): string[] {
  const tags: string[] = []
  const name = model.name.toLowerCase()
  if (meta?.is_reasoning || /thinking|reason|o1|o3|o4|r1/.test(name)) tags.push('reasoning')
  if (meta?.media?.image || /image|dall-e|imagen|gpt-image|flux|midjourney/.test(name)) tags.push('image')
  if (meta?.media?.video || /video|veo|sora|cogvideo/.test(name)) tags.push('video')
  if (meta?.media?.audio || /audio|tts|whisper|voice/.test(name)) tags.push('audio')
  if (/code|codestral|codex|devstral/.test(name)) tags.push('code')
  if (model.pricing?.billing_mode === BILLING_MODE_IMAGE) tags.push('image')
  if (model.pricing?.billing_mode === BILLING_MODE_PER_REQUEST) tags.push('per_request')
  return Array.from(new Set(tags))
}

function offerSortKey(offer: ModelPlazaGroupOffer): number {
  const p = offer.effective_pricing
  if (!p) return Number.POSITIVE_INFINITY
  if (p.billing_mode === BILLING_MODE_TOKEN && p.input_price != null) return p.input_price
  if (p.per_request_price != null) return p.per_request_price
  if (p.image_output_price != null) return p.image_output_price
  if (p.output_price != null) return p.output_price
  return Number.POSITIVE_INFINITY
}

export function buildModelPlazaRows(
  channels: UserAvailableChannel[],
  userGroupRates: Record<number, number>,
  catalog?: ModelCatalog | null,
): ModelPlazaRow[] {
  const metaMap = catalogIndex(catalog)
  const byKey = new Map<string, ModelPlazaRow>()

  for (const channel of channels) {
    for (const section of channel.platforms || []) {
      for (const model of section.supported_models || []) {
        const platform = (model.platform || section.platform || '').toLowerCase()
        const key = `${platform}::${model.name.toLowerCase()}`
        let row = byKey.get(key)
        if (!row) {
          const meta =
            metaMap.get(`${platform}::${model.name.toLowerCase()}`) ||
            metaMap.get(model.name.toLowerCase())
          row = {
            key,
            name: model.name,
            platform: model.platform || section.platform,
            display_name: meta?.display_name || meta?.name || model.name,
            family: meta?.family,
            is_reasoning: Boolean(meta?.is_reasoning),
            media: meta?.media || {},
            tags: inferTags(model, meta),
            offers: [],
            best_offer: null,
          }
          byKey.set(key, row)
        }

        for (const group of section.groups || []) {
          const multiplier = effectiveMultiplier(group, userGroupRates)
          row.offers.push({
            id: group.id,
            name: group.name,
            platform: group.platform || section.platform,
            subscription_type: group.subscription_type || 'standard',
            rate_multiplier: group.rate_multiplier,
            peak_rate_enabled: group.peak_rate_enabled,
            peak_start: group.peak_start,
            peak_end: group.peak_end,
            peak_rate_multiplier: group.peak_rate_multiplier,
            is_exclusive: group.is_exclusive,
            channel_name: channel.name,
            channel_description: channel.description || '',
            base_pricing: model.pricing,
            effective_pricing: applyRateToPricing(model.pricing, multiplier),
          })
        }
      }
    }
  }

  const rows = Array.from(byKey.values())
  for (const row of rows) {
    row.offers.sort((a, b) => offerSortKey(a) - offerSortKey(b) || a.name.localeCompare(b.name))
    row.best_offer = row.offers[0] || null
  }
  rows.sort((a, b) => {
    if (a.platform !== b.platform) return a.platform.localeCompare(b.platform)
    return a.display_name.localeCompare(b.display_name)
  })
  return rows
}

export function billingModeOf(row: ModelPlazaRow): BillingMode | null {
  return row.best_offer?.effective_pricing?.billing_mode ?? null
}

export function uniquePlatforms(rows: ModelPlazaRow[]): string[] {
  return Array.from(new Set(rows.map((r) => r.platform).filter(Boolean))).sort()
}

export function uniqueChannels(rows: ModelPlazaRow[]): string[] {
  const set = new Set<string>()
  for (const row of rows) {
    for (const offer of row.offers) {
      if (offer.channel_name) set.add(offer.channel_name)
    }
  }
  return Array.from(set).sort()
}
