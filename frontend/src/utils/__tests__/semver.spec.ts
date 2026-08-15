import { describe, expect, it } from 'vitest'
import { compareSemver, isBackendBelowMin, isFrontendBelowMin } from '../semver'

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

  it('fails closed for malformed component versions', () => {
    expect(isFrontendBelowMin('dev', '0.1.319')).toBe(true)
    expect(isFrontendBelowMin('0.1.319', 'invalid')).toBe(true)
  })
})

describe('isBackendBelowMin', () => {
  it('allows a compatible backend for a frontend-only release', () => {
    expect(isBackendBelowMin('0.1.320', '0.1.319')).toBe(false)
    expect(isBackendBelowMin('0.1.318', '0.1.319')).toBe(true)
  })

  it('treats missing backend metadata as incompatible when a floor is set', () => {
    expect(isBackendBelowMin('', '0.1.319')).toBe(true)
    expect(isBackendBelowMin('0.1.319', '')).toBe(false)
  })
})
