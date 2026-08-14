/**
 * Thin OpenAPI client over the existing axios instance.
 * Keeps 401 refresh, envelope unwrap, and locale/timezone interceptors.
 */
import type { AxiosRequestConfig, AxiosResponse } from 'axios'
import { apiClient } from '../client'
import type { paths } from './control-plane'

export const CONTROL_PLANE_PREFIX = '/api/v1'

type HTTPMethod = 'get' | 'post' | 'put' | 'delete' | 'patch'

export type ControlPlanePath = keyof paths

export interface ControlPlaneRequestOptions {
  pathParams?: Record<string, string | number>
  query?: object
  body?: unknown
  signal?: AbortSignal
}

export function toClientPath(
  specPath: string,
  pathParams?: Record<string, string | number>
): string {
  let path = specPath
  if (path === CONTROL_PLANE_PREFIX) {
    path = '/'
  } else if (path.startsWith(`${CONTROL_PLANE_PREFIX}/`)) {
    path = path.slice(CONTROL_PLANE_PREFIX.length)
  }
  if (pathParams) {
    path = path.replace(/\{([^{}/]+)\}/g, (_, key: string) => {
      const value = pathParams[key]
      if (value === undefined || value === null) {
        throw new Error(`missing OpenAPI path parameter: ${key}`)
      }
      return encodeURIComponent(String(value))
    })
  }
  return path
}

function buildConfig(options?: ControlPlaneRequestOptions): AxiosRequestConfig | undefined {
  if (!options) {
    return undefined
  }
  const config: AxiosRequestConfig = {}
  let used = false
  if (options.query !== undefined) {
    config.params = options.query
    used = true
  }
  if (options.signal) {
    config.signal = options.signal
    used = true
  }
  return used ? config : undefined
}

export function controlPlaneRequest<
  P extends ControlPlanePath,
  M extends Extract<keyof paths[P], HTTPMethod>,
  T = unknown
>(
  method: M,
  path: P,
  options?: ControlPlaneRequestOptions
): Promise<AxiosResponse<T>> {
  const url = toClientPath(String(path), options?.pathParams)
  const config = buildConfig(options)

  switch (method) {
    case 'get':
      return (config ? apiClient.get<T>(url, config) : apiClient.get<T>(url)) as Promise<AxiosResponse<T>>
    case 'post':
      if (config) {
        return apiClient.post<T>(url, options?.body, config)
      }
      if (options && 'body' in options) {
        return apiClient.post<T>(url, options.body)
      }
      return apiClient.post<T>(url)
    case 'put':
      if (config) {
        return apiClient.put<T>(url, options?.body, config)
      }
      if (options && 'body' in options) {
        return apiClient.put<T>(url, options.body)
      }
      return apiClient.put<T>(url)
    case 'patch':
      if (config) {
        return apiClient.patch<T>(url, options?.body, config)
      }
      if (options && 'body' in options) {
        return apiClient.patch<T>(url, options.body)
      }
      return apiClient.patch<T>(url)
    case 'delete':
      if (options?.body !== undefined) {
        return apiClient.delete<T>(url, { ...config, data: options.body })
      }
      return (config ? apiClient.delete<T>(url, config) : apiClient.delete<T>(url)) as Promise<AxiosResponse<T>>
    default:
      throw new Error(`unsupported control-plane method: ${String(method)}`)
  }
}
