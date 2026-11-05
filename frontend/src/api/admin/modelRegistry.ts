import { apiClient } from '../client'

export interface ModelRegistryEntry {
  id: string
  platform: string
  display_name: string
  family: string
  owned_by: string
  context_window: number | null
  max_context_window: number | null
  media: Record<string, boolean>
  is_reasoning: boolean
  enabled: boolean
  retired: boolean
  is_default_chat: boolean
  is_default_test: boolean
  upstream_id: string
  version: number
  notes: string
  price_row_count: number
}

export interface ModelAliasEntry {
  alias: string
  platform: string
  model_id: string
  kind: string
}

export interface ModelPriceRow {
  id: number
  model_id: string
  group_id: number | null
  billing_mode: string
  currency: string
  fx_mode: string
  cost_id: number | null
  fx_rate: number | null
  markup_pct: number
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  input_price_priority: number | null
  output_price_priority: number | null
  image_output_price: number | null
  per_request_price: number | null
  enabled: boolean
  version: number
}

export async function listModels(params?: { platform?: string; search?: string }): Promise<ModelRegistryEntry[]> {
  const { data } = await apiClient.get<{ items: ModelRegistryEntry[] }>('/admin/models', { params })
  return data.items || []
}

export async function upsertModel(entry: Partial<ModelRegistryEntry> & { id: string; platform: string }): Promise<void> {
  await apiClient.put('/admin/models', entry)
}

export async function listModelAliases(modelID: string): Promise<ModelAliasEntry[]> {
  const { data } = await apiClient.get<{ items: ModelAliasEntry[] }>(`/admin/models/${encodeURIComponent(modelID)}/aliases`)
  return data.items || []
}

export async function replaceModelAliases(
  modelID: string,
  aliases: { alias: string; platform: string; kind: string }[],
): Promise<void> {
  await apiClient.put(`/admin/models/${encodeURIComponent(modelID)}/aliases`, { aliases })
}

export async function listModelPrices(modelID: string): Promise<ModelPriceRow[]> {
  const { data } = await apiClient.get<{ items: ModelPriceRow[] }>(`/admin/models/${encodeURIComponent(modelID)}/prices`)
  return data.items || []
}

export async function upsertModelPrice(
  modelID: string,
  row: {
    group_id: number | null
    currency: string
    fx_mode: string
    cost_id?: number | null
    fx_rate?: number | null
    markup_pct?: number
    input_price?: number | null
    output_price?: number | null
    cache_read_price?: number | null
    cache_write_price?: number | null
    input_price_priority?: number | null
    output_price_priority?: number | null
  },
): Promise<void> {
  await apiClient.put(`/admin/models/${encodeURIComponent(modelID)}/price`, row)
}

export async function deleteModelPrice(modelID: string, groupID: number | null): Promise<void> {
  await apiClient.delete(`/admin/models/${encodeURIComponent(modelID)}/price`, {
    params: groupID == null ? {} : { group_id: groupID },
  })
}

export default {
  listModels,
  upsertModel,
  listModelAliases,
  replaceModelAliases,
  listModelPrices,
  upsertModelPrice,
  deleteModelPrice,
}
