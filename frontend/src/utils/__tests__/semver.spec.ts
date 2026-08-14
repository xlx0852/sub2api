import { describe, expect, it } from 'vitest'
import { compareSemver, isFrontendBelowMin } from '../semver'

describe('compareSemver', () => {
  it('orders patch versions', () => {
    expect(compareSemver('0.1.318', '0.1.319')).toBe(-1)
    expect(compareSemver('0.1.319', '0.1.319')).toBe(0)
    expect(compareSemver('0.1.320', '0.1.319')).toBe(1)
  })

  it('strips v prefix and pre-release suffix', () => {
    expect(compareSemver('v0.1.319-dirty', '0.1.319')).toBe(0)
  })
})

describe('isFrontendBelowMin', () => {
  it('warns when the console is older than the backend floor', () => {
    expect(isFrontendBelowMin('0.1.300', '0.1.319')).toBe(true)
    expect(isFrontendBelowMin('0.1.319', '0.1.319')).toBe(false)
    expect(isFrontendBelowMin('0.1.400', '0.1.319')).toBe(false)
  })

  it('treats a missing frontend version as incompatible when a floor is set', () => {
    expect(isFrontendBelowMin('', '0.1.319')).toBe(true)
    expect(isFrontendBelowMin('0.1.300', '')).toBe(false)
  })
})
