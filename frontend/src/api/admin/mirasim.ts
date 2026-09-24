/**
 * Admin Mirasim OAuth onboarding API endpoints.
 *
 * These endpoints only *produce* credentials (refresh token + identity) for the
 * account form; they never create accounts. Backend: internal/handler/admin/mirasim_oauth_handler.go
 */

import { apiClient } from '../client'

export type MirasimOAuthProvider = 'github' | 'google'

/** Local, non-verified view of the credential — parsed from local JWT claims only. */
export interface MirasimOAuthIdentity {
  provider?: string
  email?: string
  name?: string
  plan?: string
  plan_exp?: number
  expires_at?: number
}

export interface MirasimOAuthCredential {
  refresh_token: string
  identity: MirasimOAuthIdentity
}

export interface MirasimOAuthAuthURL {
  provider: string
  auth_url: string
  redirect_uri: string
  state: string
  expires_at: number
}

export interface MirasimEmailCodeStarted {
  email: string
  message?: string
}

export async function listProviders(payload?: {
  auth_url?: string
  proxy_id?: number | null
}): Promise<string[]> {
  const { data } = await apiClient.get<{ providers: string[] }>('/admin/mirasim/oauth/providers', {
    params: payload,
  })
  return data.providers ?? []
}

export async function generateAuthUrl(payload: {
  provider: MirasimOAuthProvider | string
  auth_url?: string
  proxy_id?: number | null
}): Promise<MirasimOAuthAuthURL> {
  const { data } = await apiClient.post<MirasimOAuthAuthURL>('/admin/mirasim/oauth/auth-url', payload)
  return data
}

export async function completeOAuth(payload: {
  callback_url?: string
  refresh_token?: string
  provider?: string
}): Promise<MirasimOAuthCredential> {
  const { data } = await apiClient.post<MirasimOAuthCredential>(
    '/admin/mirasim/oauth/complete',
    payload
  )
  return data
}

export async function startEmailCode(payload: {
  email: string
  auth_url?: string
  proxy_id?: number | null
}): Promise<MirasimEmailCodeStarted> {
  const { data } = await apiClient.post<MirasimEmailCodeStarted>(
    '/admin/mirasim/oauth/email/code',
    payload
  )
  return data
}

export async function completeEmailCode(payload: {
  email: string
  code: string
  auth_url?: string
  proxy_id?: number | null
}): Promise<MirasimOAuthCredential> {
  const { data } = await apiClient.post<MirasimOAuthCredential>(
    '/admin/mirasim/oauth/email/verify',
    payload
  )
  return data
}

export default {
  listProviders,
  generateAuthUrl,
  completeOAuth,
  startEmailCode,
  completeEmailCode,
}
