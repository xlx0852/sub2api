export function roundUsagePercent(value: number): number {
  if (!Number.isFinite(value)) return value
  return Math.round(value * 100) / 100
}

export function formatUsagePercent(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return '—'
  const percent = roundUsagePercent(value)
  if (percent > 999) return '>999%'
  return `${percent.toFixed(2)}%`
}
