/**
 * Gemini admin API (retired)
 * Google discontinued Gemini CLI; endpoints return 410 on backend.
 */
import { apiClient } from "../client"

export interface GeminiOAuthCapabilities {
  [key: string]: unknown
}

export async function generateAuthUrl(payload: Record<string, unknown>) {
  const { data } = await apiClient.post("/admin/gemini/oauth/auth-url", payload)
  return data
}

export async function exchangeCode(payload: Record<string, unknown>) {
  const { data } = await apiClient.post("/admin/gemini/oauth/exchange-code", payload)
  return data
}

export async function getCapabilities(): Promise<GeminiOAuthCapabilities> {
  const { data } = await apiClient.get<GeminiOAuthCapabilities>("/admin/gemini/oauth/capabilities")
  return data
}

const geminiAPI = { generateAuthUrl, exchangeCode, getCapabilities }
export default geminiAPI
