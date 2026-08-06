import type { ChannelModelPricing } from '@/api/admin/channels'
import type { PricingFormEntry } from './types'
import { perTokenToMTok, toNullableNumber } from './types'

export type OfficialTokenPricing = {
  found: boolean
  input_price?: number | null
  output_price?: number | null
  cache_write_price?: number | null
  cache_read_price?: number | null
  image_output_price?: number | null
}

export type TokenPriceFields = {
  input_price: number | string | null
  output_price: number | string | null
  cache_write_price: number | string | null
  cache_read_price: number | string | null
  image_output_price: number | string | null
}

const TOKEN_PRICE_KEYS = [
  'input_price',
  'output_price',
  'cache_write_price',
  'cache_read_price',
  'image_output_price',
] as const

export function isTokenPricingEmpty(entry: Pick<PricingFormEntry, (typeof TOKEN_PRICE_KEYS)[number] | 'billing_mode' | 'per_request_price'>): boolean {
  if (entry.billing_mode === 'token') {
    return TOKEN_PRICE_KEYS.every((key) => toNullableNumber(entry[key]) == null)
  }
  return toNullableNumber(entry.per_request_price) == null
}

export function hasAnyTokenPrice(entry: Pick<PricingFormEntry, (typeof TOKEN_PRICE_KEYS)[number]>): boolean {
  return TOKEN_PRICE_KEYS.some((key) => toNullableNumber(entry[key]) != null)
}

/** 折叠卡片/列表用的价格摘要（已是 $/MTok 表单值） */
export function formatTokenPriceSummary(
  entry: Pick<PricingFormEntry, (typeof TOKEN_PRICE_KEYS)[number] | 'billing_mode' | 'per_request_price'>,
  emptyLabel = '回退官方',
): string {
  if (entry.billing_mode === 'per_request' || entry.billing_mode === 'image') {
    const price = toNullableNumber(entry.per_request_price)
    return price == null ? emptyLabel : `$${trimNum(price)} / 次`
  }
  const input = toNullableNumber(entry.input_price)
  const output = toNullableNumber(entry.output_price)
  if (input == null && output == null) return emptyLabel
  const parts: string[] = []
  if (input != null) parts.push(`in $${trimNum(input)}`)
  if (output != null) parts.push(`out $${trimNum(output)}`)
  return `${parts.join(' / ')} / MTok`
}

export function officialPricingToFormMTok(official: OfficialTokenPricing): TokenPriceFields | null {
  if (!official.found) return null
  return {
    input_price: perTokenToMTok(official.input_price ?? null),
    output_price: perTokenToMTok(official.output_price ?? null),
    cache_write_price: perTokenToMTok(official.cache_write_price ?? null),
    cache_read_price: perTokenToMTok(official.cache_read_price ?? null),
    image_output_price: perTokenToMTok(official.image_output_price ?? null),
  }
}

/** 按官方价（$/MTok）× multiplier 生成售价覆盖 */
export function scaleOfficialPricingMTok(
  official: OfficialTokenPricing,
  multiplier: number,
): TokenPriceFields | null {
  const base = officialPricingToFormMTok(official)
  if (!base) return null
  if (!(multiplier > 0) || !Number.isFinite(multiplier)) return null
  const scale = (v: number | string | null) => {
    const n = toNullableNumber(v)
    if (n == null) return null
    return parseFloat((n * multiplier).toPrecision(10))
  }
  return {
    input_price: scale(base.input_price),
    output_price: scale(base.output_price),
    cache_write_price: scale(base.cache_write_price),
    cache_read_price: scale(base.cache_read_price),
    image_output_price: scale(base.image_output_price),
  }
}

export function clearTokenPriceOverrides(): TokenPriceFields {
  return {
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_output_price: null,
  }
}

/** 用户实扣预览：$ / MTok × 分组倍率 */
export function previewEffectiveMTok(
  channelMTok: number | string | null | undefined,
  rateMultiplier: number | null | undefined,
): number | null {
  const price = toNullableNumber(channelMTok)
  if (price == null) return null
  const rate = typeof rateMultiplier === 'number' && Number.isFinite(rateMultiplier) ? rateMultiplier : 1
  return parseFloat((price * rate).toPrecision(10))
}

export type ChannelPricingListSummary = {
  models: string[]
  sampleSummary: string | null
  emptyRuleCount: number
  ruleCount: number
}

export function summarizeChannelPricing(
  pricing: ChannelModelPricing[] | null | undefined,
  emptyLabel = '回退官方',
): ChannelPricingListSummary {
  const rules = pricing || []
  const models: string[] = []
  const seen = new Set<string>()
  let emptyRuleCount = 0
  let sampleSummary: string | null = null

  for (const rule of rules) {
    for (const model of rule.models || []) {
      const key = model.toLowerCase()
      if (seen.has(key)) continue
      seen.add(key)
      models.push(model)
    }
    const formLike = {
      billing_mode: rule.billing_mode,
      input_price: perTokenToMTok(rule.input_price),
      output_price: perTokenToMTok(rule.output_price),
      cache_write_price: perTokenToMTok(rule.cache_write_price),
      cache_read_price: perTokenToMTok(rule.cache_read_price),
      image_output_price: perTokenToMTok(rule.image_output_price),
      per_request_price: rule.per_request_price,
    }
    if (isTokenPricingEmpty(formLike)) emptyRuleCount += 1
    if (!sampleSummary && hasAnyTokenPrice(formLike)) {
      sampleSummary = formatTokenPriceSummary(formLike, emptyLabel)
    } else if (!sampleSummary && (formLike.billing_mode === 'per_request' || formLike.billing_mode === 'image') && formLike.per_request_price != null) {
      sampleSummary = formatTokenPriceSummary(formLike, emptyLabel)
    }
  }

  return {
    models,
    sampleSummary,
    emptyRuleCount,
    ruleCount: rules.length,
  }
}

function trimNum(n: number): string {
  if (Number.isInteger(n)) return String(n)
  const s = n.toPrecision(6)
  return String(parseFloat(s))
}
