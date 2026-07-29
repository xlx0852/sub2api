import { grokDeviceOAuthAPI } from '@/api/admin/deviceOAuth'
import { useDeviceOAuth } from './useDeviceOAuth'

export function useGrokDeviceOAuth() {
  return useDeviceOAuth({
    client: grokDeviceOAuthAPI,
    provider: 'Grok',
    startError: 'Failed to start Grok device authorization',
    statusError: 'Grok authorization status check failed'
  })
}
