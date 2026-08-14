import { controlPlaneRequest } from '@/api/generated/request'
import type { UserSupportedModelPricing } from '@/api/channels'

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
  const { data } = await controlPlaneRequest<
    '/api/v1/public/pricing',
    'get',
    PublicPricingSnapshot
  >('get', '/api/v1/public/pricing')
  return data
}
