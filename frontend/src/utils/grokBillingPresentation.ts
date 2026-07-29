import type { GrokBillingSnapshot } from '@/api/admin/grok'

export interface GrokMonthlyQuotaPresentation {
  utilization: number
  usedCents: number
  remainingCents: number
  limitCents: number
  resetsAt: string | null
}

const finiteNonNegative = (value: number | null | undefined): number | null => {
  if (value == null || !Number.isFinite(value)) return null
  return Math.max(0, value)
}

/**
 * Build the official Grok monthly allowance view from absolute billing values.
 * The amount fields are the source of truth; used_percent is only a fallback
 * for older snapshots that did not persist the used amount.
 */
export const resolveGrokMonthlyQuota = (
  billing: GrokBillingSnapshot | null | undefined
): GrokMonthlyQuotaPresentation | null => {
  const limitCents = finiteNonNegative(billing?.monthly_limit_cents)
  if (limitCents == null || limitCents <= 0) return null

  const includedUsed = finiteNonNegative(billing?.included_used_cents)
  const totalUsed = finiteNonNegative(billing?.used_cents)
  const amountUsed = includedUsed ?? (totalUsed == null ? null : Math.min(totalUsed, limitCents))
  const fallbackPercent = finiteNonNegative(billing?.used_percent)
  const usedCents = Math.min(
    amountUsed ?? (fallbackPercent == null ? 0 : (limitCents * fallbackPercent) / 100),
    limitCents
  )

  return {
    utilization: Math.max(0, Math.min(100, (usedCents / limitCents) * 100)),
    usedCents,
    remainingCents: Math.max(0, limitCents - usedCents),
    limitCents,
    resetsAt: billing?.billing_period_end || billing?.period_end || null
  }
}
