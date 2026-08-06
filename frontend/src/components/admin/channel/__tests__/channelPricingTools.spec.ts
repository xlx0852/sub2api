import { describe, expect, it } from 'vitest'
import {
  clearTokenPriceOverrides,
  formatTokenPriceSummary,
  isTokenPricingEmpty,
  officialPricingToFormMTok,
  previewEffectiveMTok,
  scaleOfficialPricingMTok,
  summarizeChannelPricing,
} from '../channelPricingTools'

describe('channelPricingTools', () => {
  it('converts official per-token prices to $/MTok form fields', () => {
    const form = officialPricingToFormMTok({
      found: true,
      input_price: 2e-7,
      output_price: 1.2e-6,
      cache_write_price: 2.5e-7,
      cache_read_price: 2e-8,
      image_output_price: 0,
    })
    expect(form).toEqual({
      input_price: 0.2,
      output_price: 1.2,
      cache_write_price: 0.25,
      cache_read_price: 0.02,
      image_output_price: 0,
    })
  })

  it('scales official prices by multiplier for sell markup', () => {
    const scaled = scaleOfficialPricingMTok(
      {
        found: true,
        input_price: 2e-7,
        output_price: 1.2e-6,
        cache_write_price: 2.5e-7,
        cache_read_price: 2e-8,
      },
      4,
    )
    expect(scaled?.input_price).toBe(0.8)
    expect(scaled?.output_price).toBe(4.8)
    expect(scaled?.cache_write_price).toBe(1)
    expect(scaled?.cache_read_price).toBe(0.08)
  })

  it('previews effective user price with group multiplier', () => {
    expect(previewEffectiveMTok(0.8, 0.12)).toBe(0.096)
    expect(previewEffectiveMTok(null, 0.12)).toBeNull()
  })

  it('formats token summary and empty fallback', () => {
    expect(
      formatTokenPriceSummary({
        billing_mode: 'token',
        input_price: 0.8,
        output_price: 4.8,
        cache_write_price: null,
        cache_read_price: null,
        image_output_price: null,
        per_request_price: null,
      }),
    ).toContain('in $0.8')
    expect(
      isTokenPricingEmpty({
        billing_mode: 'token',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        image_output_price: null,
        per_request_price: null,
      }),
    ).toBe(true)
    expect(clearTokenPriceOverrides().input_price).toBeNull()
  })

  it('summarizes channel list pricing', () => {
    const summary = summarizeChannelPricing([
      {
        platform: 'openai',
        models: ['gpt-5.6-luna', 'gpt-5.6-sol'],
        billing_mode: 'token',
        input_price: 8e-7,
        output_price: 4.8e-6,
        cache_write_price: null,
        cache_read_price: null,
        image_output_price: null,
        per_request_price: null,
        intervals: [],
      },
      {
        platform: 'openai',
        models: ['gpt-5.4'],
        billing_mode: 'token',
        input_price: null,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        image_output_price: null,
        per_request_price: null,
        intervals: [],
      },
    ])
    expect(summary.ruleCount).toBe(2)
    expect(summary.emptyRuleCount).toBe(1)
    expect(summary.models).toEqual(['gpt-5.6-luna', 'gpt-5.6-sol', 'gpt-5.4'])
    expect(summary.sampleSummary).toContain('in $0.8')
  })
})
