import { apiClient } from '../client'

export interface CatalogModelEntry {
  id: string
  name?: string
  display_name?: string
  object?: string
  owned_by?: string
  type?: string
  created?: number
  created_at?: string
  family?: string
  is_reasoning?: boolean
  media?: Record<string, boolean>
}

export interface CatalogPlatform {
  default_test_model?: string
  default_chat_model?: string
  models?: CatalogModelEntry[]
  aliases?: Record<string, string>
  retired_ids?: string[]
  default_mapping?: Record<string, string>
  id_overrides?: Record<string, string>
  id_reverse_overrides?: Record<string, string>
}

export interface CatalogUIPreset {
  label: string
  from: string
  to: string
  color?: string
}

export interface ModelCatalog {
  version: number
  updated_at?: string
  platforms: Record<string, CatalogPlatform>
  ui_presets?: Record<string, CatalogUIPreset[]>
  fallback_pricing?: Record<string, unknown>
  image_defaults?: {
    base_price_usd?: number
    size_multipliers?: Record<string, number>
  }
}

/**
 * Fetch global model catalog (JSON-driven first-party models, mappings, presets).
 */
export async function getModelCatalog(): Promise<ModelCatalog> {
  const { data } = await apiClient.get<ModelCatalog>('/admin/model-catalog')
  return data
}
