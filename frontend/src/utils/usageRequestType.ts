import type { UsageRequestType } from '@/types'

export interface UsageRequestTypeLike {
  request_type?: string | null
  stream?: boolean | null
  openai_ws_mode?: boolean | null
  ws_payload_bytes?: number | null
}

const VALID_REQUEST_TYPES = new Set<UsageRequestType>(['unknown', 'sync', 'stream', 'ws_v2', 'cyber', 'compact'])

export const isUsageRequestType = (value: unknown): value is UsageRequestType => {
  return typeof value === 'string' && VALID_REQUEST_TYPES.has(value as UsageRequestType)
}

export const resolveUsageRequestType = (value: UsageRequestTypeLike): UsageRequestType => {
  if (isUsageRequestType(value.request_type)) {
    return value.request_type
  }
  if (value.openai_ws_mode) {
    return 'ws_v2'
  }
  return value.stream ? 'stream' : 'sync'
}

export const isUsageWSHTTPBridgeReplay = (value: UsageRequestTypeLike): boolean => {
  return resolveUsageRequestType(value) === 'ws_v2' && Number(value.ws_payload_bytes ?? 0) > 0
}

export const requestTypeToLegacyStream = (requestType?: UsageRequestType | null): boolean | null | undefined => {
  // cyber/compact 与 legacy stream 维度正交，不映射到 legacy stream 过滤。
  if (!requestType || requestType === 'unknown' || requestType === 'cyber' || requestType === 'compact') {
    return null
  }
  if (requestType === 'sync') {
    return false
  }
  return true
}

type UsageRequestTypeFilterParams = {
  request_type?: UsageRequestType | null
  stream?: boolean | null
}

export const preferRequestTypeFilter = <T extends UsageRequestTypeFilterParams>(params: T): T => {
  if (!params.request_type) {
    return params
  }
  return {
    ...params,
    stream: undefined
  } as T
}
