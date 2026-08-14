/**
 * User Groups API endpoints (non-admin)
 * Handles group-related operations for regular users
 */

import { controlPlaneRequest } from '@/api/generated/request'
import type { Group } from '@/types'

/**
 * Get available groups that the current user can bind to API keys
 * This returns groups based on user's permissions:
 * - Standard groups: public (non-exclusive) or explicitly allowed
 * - Subscription groups: user has active subscription
 * @returns List of available groups
 */
export async function getAvailable(): Promise<Group[]> {
  const { data } = await controlPlaneRequest<'/api/v1/groups/available', 'get', Group[]>(
    'get',
    '/api/v1/groups/available'
  )
  return data
}

/**
 * Get current user's custom group rate multipliers
 * @returns Map of group_id to custom rate_multiplier
 */
export async function getUserGroupRates(): Promise<Record<number, number>> {
  const { data } = await controlPlaneRequest<
    '/api/v1/groups/rates',
    'get',
    Record<number, number> | null
  >('get', '/api/v1/groups/rates')
  return data || {}
}

export const userGroupsAPI = {
  getAvailable,
  getUserGroupRates
}

export default userGroupsAPI
