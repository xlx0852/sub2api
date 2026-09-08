import { cursorDeviceOAuthAPI } from '@/api/admin/deviceOAuth'
import { useDeviceOAuth } from './useDeviceOAuth'

export function useCursorDeviceOAuth() {
  return useDeviceOAuth({
    client: cursorDeviceOAuthAPI,
    provider: 'Cursor',
    startError: 'Failed to start Cursor authorization',
    statusError: 'Cursor authorization status check failed'
  })
}
