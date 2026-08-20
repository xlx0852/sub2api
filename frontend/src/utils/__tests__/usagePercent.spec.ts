import { describe, expect, it } from 'vitest'
import { formatUsagePercent, roundUsagePercent } from '../usagePercent'

describe('usagePercent', () => {
  it('rounds quota used% to two decimal places', () => {
    expect(roundUsagePercent(12.345)).toBe(12.35)
    expect(roundUsagePercent(7)).toBe(7)
    expect(roundUsagePercent(0.42)).toBe(0.42)
  })

  it('formats quota used% with two decimal places', () => {
    expect(formatUsagePercent(12.345)).toBe('12.35%')
    expect(formatUsagePercent(7)).toBe('7.00%')
    expect(formatUsagePercent(1000)).toBe('>999%')
    expect(formatUsagePercent(null)).toBe('—')
  })
})
