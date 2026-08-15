function parseSemver(value: string): [number, number, number] | null {
  const cleaned = value.trim().replace(/^v/i, '').split(/[-+]/)[0]
  const parts = cleaned.split('.')
  if (parts.length < 3) {
    return null
  }
  if (parts.slice(0, 3).some((part) => !/^\d+$/.test(part))) {
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
  return isVersionBelowMin(frontendVersion, minFrontendVersion)
}

export function isBackendBelowMin(backendVersion: string, minBackendVersion: string): boolean {
  return isVersionBelowMin(backendVersion, minBackendVersion)
}

function isVersionBelowMin(version: string, minimumVersion: string): boolean {
  const minimum = minimumVersion.trim()
  if (!minimum) {
    return false
  }
  const current = version.trim()
  if (!current || !parseSemver(current) || !parseSemver(minimum)) {
    return true
  }
  return compareSemver(current, minimum) < 0
}
