export const TOKENS_PER_MILLION = 1_000_000

interface TokenPriceFormatOptions {
  fractionDigits?: number
  withCurrencySymbol?: boolean
  currency?: string | null
  emptyValue?: string
}

export function normalizePricingCurrency(value: string | null | undefined): 'USD' | 'CNY' {
  return String(value || '').trim().toUpperCase() === 'CNY' ? 'CNY' : 'USD'
}

export function pricingCurrencySymbol(value: string | null | undefined): '$' | '¥' {
  return normalizePricingCurrency(value) === 'CNY' ? '¥' : '$'
}

export function formatPricingAmount(
  value: number | null | undefined,
  currency: string | null | undefined,
  fractionDigits = 6,
): string {
  const amount = typeof value === 'number' && Number.isFinite(value) ? value : 0
  return `${pricingCurrencySymbol(currency)}${amount.toFixed(fractionDigits)}`
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}

export function calculateTokenUnitPrice(
  cost: number | null | undefined,
  tokens: number | null | undefined
): number | null {
  if (!isFiniteNumber(cost) || !isFiniteNumber(tokens) || tokens <= 0) {
    return null
  }

  return cost / tokens
}

export function calculateTokenPricePerMillion(
  cost: number | null | undefined,
  tokens: number | null | undefined
): number | null {
  const unitPrice = calculateTokenUnitPrice(cost, tokens)
  if (unitPrice == null) {
    return null
  }

  return unitPrice * TOKENS_PER_MILLION
}

export function formatTokenPricePerMillion(
  cost: number | null | undefined,
  tokens: number | null | undefined,
  options: TokenPriceFormatOptions = {}
): string {
  const pricePerMillion = calculateTokenPricePerMillion(cost, tokens)
  if (pricePerMillion == null) {
    return options.emptyValue ?? '-'
  }

  const fractionDigits = options.fractionDigits ?? 4
  const formatted = pricePerMillion.toFixed(fractionDigits)
  return options.withCurrencySymbol == false
    ? formatted
    : `${pricingCurrencySymbol(options.currency)}${formatted}`
}
