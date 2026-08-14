/**
 * TOTP (2FA) API endpoints
 * Handles Two-Factor Authentication with Google Authenticator
 */

import { controlPlaneRequest } from '@/api/generated/request'
import type {
  TotpStatus,
  TotpSetupRequest,
  TotpSetupResponse,
  TotpEnableRequest,
  TotpEnableResponse,
  TotpDisableRequest,
  TotpVerificationMethod
} from '@/types'

/**
 * Get TOTP status for current user
 * @returns TOTP status including enabled state and feature availability
 */
export async function getStatus(): Promise<TotpStatus> {
  const { data } = await controlPlaneRequest<'/api/v1/user/totp/status', 'get', TotpStatus>(
    'get',
    '/api/v1/user/totp/status'
  )
  return data
}

/**
 * Get verification method for TOTP operations
 * @returns Method ('email' or 'password') required for setup/disable
 */
export async function getVerificationMethod(): Promise<TotpVerificationMethod> {
  const { data } = await controlPlaneRequest<
    '/api/v1/user/totp/verification-method',
    'get',
    TotpVerificationMethod
  >('get', '/api/v1/user/totp/verification-method')
  return data
}

/**
 * Send email verification code for TOTP operations
 * @returns Success response
 */
export async function sendVerifyCode(): Promise<{ success: boolean }> {
  const { data } = await controlPlaneRequest<
    '/api/v1/user/totp/send-code',
    'post',
    { success: boolean }
  >('post', '/api/v1/user/totp/send-code')
  return data
}

/**
 * Initiate TOTP setup - generates secret and QR code
 * @param request - Email code or password depending on verification method
 * @returns Setup response with secret, QR code URL, and setup token
 */
export async function initiateSetup(request?: TotpSetupRequest): Promise<TotpSetupResponse> {
  const { data } = await controlPlaneRequest<
    '/api/v1/user/totp/setup',
    'post',
    TotpSetupResponse
  >('post', '/api/v1/user/totp/setup', { body: request || {} })
  return data
}

/**
 * Complete TOTP setup by verifying the code
 * @param request - TOTP code and setup token
 * @returns Enable response with success status and enabled timestamp
 */
export async function enable(request: TotpEnableRequest): Promise<TotpEnableResponse> {
  const { data } = await controlPlaneRequest<
    '/api/v1/user/totp/enable',
    'post',
    TotpEnableResponse
  >('post', '/api/v1/user/totp/enable', { body: request })
  return data
}

/**
 * Disable TOTP for current user
 * @param request - Email code or password depending on verification method
 * @returns Success response
 */
export async function disable(request: TotpDisableRequest): Promise<{ success: boolean }> {
  const { data } = await controlPlaneRequest<
    '/api/v1/user/totp/disable',
    'post',
    { success: boolean }
  >('post', '/api/v1/user/totp/disable', { body: request })
  return data
}

export const totpAPI = {
  getStatus,
  getVerificationMethod,
  sendVerifyCode,
  initiateSetup,
  enable,
  disable
}

export default totpAPI
