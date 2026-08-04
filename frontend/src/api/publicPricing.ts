import axios from 'axios'
import { buildApiUrl } from './url'
import type { UserSupportedModelPricing } from './channels'

export interface PublicPricingModel {
  name: string
  platform: string
  display_name: string
  family?: string
  is_reasoning: boolean
  media: Record<string, boolean>
  pricing: UserSupportedModelPricing | null
}

export interface PublicPricingSnapshot {
  generated_at: string
  models: PublicPricingModel[]
}

export async function getPublicPricing(): Promise<PublicPricingSnapshot> {
  const response = await axios.get<{ code: number; data: PublicPricingSnapshot }>(buildApiUrl('/public/pricing'), {
    timeout: 15000,
    withCredentials: false,
    headers: { Accept: 'application/json' },
  })
  return response.data.data
}
