function parseSemver(value: string): [number, number, number] | null {
  const cleaned = value.trim().replace(/^v/i, '').split(/[-+]/)[0]
  const parts = cleaned.split('.')
  if (parts.length < 3) {
    return null
  }
  const nums = parts.slice(0, 3).map((part) => Number(part))
  if (nums.some((n) => !Number.isFinite(n))) {
    return null
  }
  return [nums[0], nums[1], nums[2]]
}

export function compareSemver(a: string, b: string): number {
  const left = parseSemver(a)
  const right = parseSemver(b)
  if (!left || !right) {
    return 0
  }
  for (let i = 0; i < 3; i++) {
    if (left[i] !== right[i]) {
      return left[i] < right[i] ? -1 : 1
    }
  }
  return 0
}

export function isFrontendBelowMin(frontendVersion: string, minFrontendVersion: string): boolean {
  const min = minFrontendVersion.trim()
  if (!min) {
    return false
  }
  const frontend = frontendVersion.trim()
  if (!frontend) {
    return true
  }
  return compareSemver(frontend, min) < 0
}
