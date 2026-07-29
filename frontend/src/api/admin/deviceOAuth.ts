import { apiClient } from '../client'
import type { Account } from '@/types'

/** Common response returned by the provider device-authorization endpoints. */
export type DeviceAuthorizationStatus = 'pending' | 'authorized' | 'denied'

export interface DeviceAuthorization {
  session_id: string
  status: DeviceAuthorizationStatus
  verification_uri: string
  verification_uri_complete?: string
  user_code: string
  expires_in: number
  interval: number
  retry_after?: number
  error?: string
}

export interface DeviceCreateAccountRequest {
  session_id: string
  name?: string
  notes?: string
  proxy_id?: number | null
  concurrency: number
  priority: number
  group_ids: number[]
}

export type DeviceOAuthProvider = 'openai' | 'grok' | 'kimi'

const providerPath = (provider: DeviceOAuthProvider) => `/admin/${provider}/oauth/device`

/**
 * Build a small provider-specific client while keeping the device flow itself
 * provider agnostic.  Kimi has its own API module for backwards compatibility;
 * this client is used by OpenAI/Grok and can also be used by future providers.
 */
export function createDeviceOAuthAPI(provider: DeviceOAuthProvider) {
  const path = providerPath(provider)
  return {
    async start(proxyId?: number | null): Promise<DeviceAuthorization> {
      const { data } = await apiClient.post<DeviceAuthorization>(
        `${path}/start`,
        proxyId ? { proxy_id: proxyId } : {}
      )
      return data
    },

    async status(sessionId: string): Promise<DeviceAuthorization> {
      const { data } = await apiClient.get<DeviceAuthorization>(`${path}/status`, {
        params: { session_id: sessionId }
      })
      return data
    },

    async cancel(sessionId: string): Promise<void> {
      await apiClient.delete(`${path}/session`, { params: { session_id: sessionId } })
    },

    async createAccount(payload: DeviceCreateAccountRequest): Promise<Account> {
      const { data } = await apiClient.post<Account>(
        `/admin/${provider}/oauth/create-from-device`,
        payload
      )
      return data
    },

    async reauthorizeAccount(accountId: number, sessionId: string): Promise<Account> {
      const { data } = await apiClient.post<Account>(
        `/admin/${provider}/accounts/${accountId}/reauth-from-device`,
        { session_id: sessionId }
      )
      return data
    }
  }
}

export const openAIDeviceOAuthAPI = createDeviceOAuthAPI('openai')
export const grokDeviceOAuthAPI = createDeviceOAuthAPI('grok')

const deviceOAuthAPI = {
  openai: openAIDeviceOAuthAPI,
  grok: grokDeviceOAuthAPI
}

export default deviceOAuthAPI
