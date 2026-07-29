import { describe, expect, it } from 'vitest'
import { resolveGrokMonthlyQuota } from '../grokBillingPresentation'

describe('resolveGrokMonthlyQuota', () => {
  it('以已用金额和明确月限额计算展示', () => {
    expect(resolveGrokMonthlyQuota({
      monthly_limit_cents: 15_000,
      used_cents: 160,
      included_used_cents: 160,
      used_percent: 99,
      billing_period_end: '2026-08-01T00:00:00Z'
    })).toEqual({
      utilization: 160 / 150,
      usedCents: 160,
      remainingCents: 14_840,
      limitCents: 15_000,
      resetsAt: '2026-08-01T00:00:00Z'
    })
  })

  it('超出包含额度时月额度封顶为 100%', () => {
    expect(resolveGrokMonthlyQuota({
      monthly_limit_cents: 15_000,
      used_cents: 18_000,
      included_used_cents: 15_000
    })).toMatchObject({
      utilization: 100,
      usedCents: 15_000,
      remainingCents: 0
    })
  })

  it('兼容只有 used_percent 的旧快照', () => {
    expect(resolveGrokMonthlyQuota({
      monthly_limit_cents: 15_000,
      used_percent: 20
    })).toMatchObject({
      utilization: 20,
      usedCents: 3_000,
      remainingCents: 12_000
    })
  })
})
