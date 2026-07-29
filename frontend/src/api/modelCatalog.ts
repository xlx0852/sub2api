/**
 * User-facing model catalog (metadata only).
 * Shares the available-channels feature flag on the backend.
 */

import { apiClient } from './client'
import type { ModelCatalog } from './admin/modelCatalog'

export type { ModelCatalog, CatalogModelEntry, CatalogPlatform } from './admin/modelCatalog'

export async function getUserModelCatalog(options?: { signal?: AbortSignal }): Promise<ModelCatalog> {
  const { data } = await apiClient.get<ModelCatalog>('/model-catalog', {
    signal: options?.signal,
  })
  return data
}

export const userModelCatalogAPI = { get: getUserModelCatalog }
export default userModelCatalogAPI
