import { apiClient } from '../client'
import type { Account } from '@/types'

export interface KimiDeviceAuthorization {
  session_id: string
  status: 'pending' | 'authorized' | 'denied'
  verification_uri: string
  verification_uri_complete?: string
  user_code: string
  expires_in: number
  interval: number
  retry_after?: number
  error?: string
}

export interface KimiCreateAccountRequest {
  session_id: string
  name: string
  notes?: string
  proxy_id?: number | null
  concurrency: number
  priority: number
  group_ids: number[]
  model_mapping?: Record<string, string>
}

export async function startDeviceAuthorization(proxyId?: number | null): Promise<KimiDeviceAuthorization> {
  const { data } = await apiClient.post<KimiDeviceAuthorization>('/admin/kimi/oauth/device/start', proxyId ? { proxy_id: proxyId } : {})
  return data
}

export async function getDeviceAuthorizationStatus(sessionId: string): Promise<KimiDeviceAuthorization> {
  const { data } = await apiClient.get<KimiDeviceAuthorization>('/admin/kimi/oauth/device/status', { params: { session_id: sessionId } })
  return data
}

export async function cancelDeviceAuthorization(sessionId: string): Promise<void> {
  await apiClient.delete('/admin/kimi/oauth/device/session', { params: { session_id: sessionId } })
}

export async function createAccountFromDevice(payload: KimiCreateAccountRequest): Promise<void> {
  await apiClient.post('/admin/kimi/oauth/create-from-device', payload)
}

export async function reauthorizeAccountFromDevice(accountId: number, sessionId: string): Promise<Account> {
  const { data } = await apiClient.post<Account>(`/admin/kimi/accounts/${accountId}/reauth-from-device`, { session_id: sessionId })
  return data
}

export async function refreshAccountToken(accountId: number): Promise<Account> {
  const { data } = await apiClient.post<Account>(`/admin/kimi/accounts/${accountId}/refresh`)
  return data
}

export default { startDeviceAuthorization, getDeviceAuthorizationStatus, cancelDeviceAuthorization, createAccountFromDevice, reauthorizeAccountFromDevice, refreshAccountToken }
