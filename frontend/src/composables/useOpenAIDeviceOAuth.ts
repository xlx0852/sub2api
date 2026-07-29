import { openAIDeviceOAuthAPI } from '@/api/admin/deviceOAuth'
import { useDeviceOAuth } from './useDeviceOAuth'

export function useOpenAIDeviceOAuth() {
  return useDeviceOAuth({
    client: openAIDeviceOAuthAPI,
    provider: 'OpenAI',
    startError: 'Failed to start OpenAI device authorization',
    statusError: 'OpenAI authorization status check failed'
  })
}
