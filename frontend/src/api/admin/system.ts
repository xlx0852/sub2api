/**
 * System API endpoints for admin operations
 */

import { apiClient } from '../client'

/**
 * Get current version
 */
export async function getVersion(): Promise<{ version: string }> {
  const { data } = await apiClient.get<{ version: string }>('/admin/system/version')
  return data
}

/**
 * Restart the service
 */
export async function restartService(): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>('/admin/system/restart')
  return data
}

export interface ModelCatalogStatus {
  version: number
  updated_at?: string
  source: 'embedded' | 'local' | 'remote'
  hash: string
  last_check?: string
  last_success?: string
  next_check?: string
  refreshing: boolean
  remote_enabled: boolean
  last_error?: string
  success_count: number
  failure_count: number
}

export async function getModelCatalogStatus(): Promise<ModelCatalogStatus> {
  const { data } = await apiClient.get<ModelCatalogStatus>('/admin/system/model-catalog/status')
  return data
}

export async function refreshModelCatalog(): Promise<ModelCatalogStatus> {
  const { data } = await apiClient.post<ModelCatalogStatus>('/admin/system/model-catalog/refresh')
  return data
}

export const systemAPI = {
  getVersion,
  restartService,
  getModelCatalogStatus,
  refreshModelCatalog
}

export default systemAPI
