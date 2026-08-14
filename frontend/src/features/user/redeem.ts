/**
 * Redeem code API endpoints
 * Handles redeem code redemption for users
 */

import { controlPlaneRequest } from '@/api/generated/request'
import type { RedeemCodeRequest } from '@/types'

export interface RedeemHistoryItem {
  id: number
  code: string
  type: string
  value: number
  status: string
  used_at: string
  created_at: string
  // Notes from admin for admin_balance/admin_concurrency types
  notes?: string
  // Subscription-specific fields
  group_id?: number
  validity_days?: number
  group?: {
    id: number
    name: string
  }
}

/**
 * Redeem a code
 * @param code - Redeem code string
 * @returns Redemption result with updated balance or concurrency
 */
export async function redeem(code: string): Promise<{
  message: string
  type: string
  value: number
  new_balance?: number
  new_concurrency?: number
}> {
  const payload: RedeemCodeRequest = { code }

  const { data } = await controlPlaneRequest<
    '/api/v1/redeem',
    'post',
    {
      message: string
      type: string
      value: number
      new_balance?: number
      new_concurrency?: number
    }
  >('post', '/api/v1/redeem', { body: payload })

  return data
}

/**
 * Get user's redemption history
 * @returns List of redeemed codes
 */
export async function getHistory(): Promise<RedeemHistoryItem[]> {
  const { data } = await controlPlaneRequest<
    '/api/v1/redeem/history',
    'get',
    RedeemHistoryItem[]
  >('get', '/api/v1/redeem/history')
  return data
}

export const redeemAPI = {
  redeem,
  getHistory
}

export default redeemAPI
