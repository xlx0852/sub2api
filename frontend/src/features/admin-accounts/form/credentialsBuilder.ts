import type { OpenAICompactMode, OpenAIEndpointCapability } from '@/types'

export function applyInterceptWarmup(
  credentials: Record<string, unknown>,
  enabled: boolean,
  mode: 'create' | 'edit' | 'bulk'
): void {
  if (enabled) {
    credentials.intercept_warmup_requests = true
  } else if (mode === 'edit') {
    delete credentials.intercept_warmup_requests
  } else if (mode === 'bulk') {
    credentials.intercept_warmup_requests = false
  }
}

export const ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY = 'antigravity_project_id'

export function applyAntigravityProjectID(
  credentials: Record<string, unknown>,
  projectId: string,
  mode: 'create' | 'edit'
): void {
  const trimmed = projectId.trim()
  if (trimmed) {
    credentials[ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY] = trimmed
  } else if (mode === 'edit') {
    delete credentials[ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY]
  }
}

export const GROK_API_BASE_URL = 'https://api.x.ai/v1'

export function applyGrokAPIKeyRouting(
  credentials: Record<string, unknown>,
  platform: string,
  accountType: string
): void {
  if (platform !== 'grok' || accountType !== 'apikey') return

  const configuredBaseURL =
    typeof credentials.base_url === 'string' ? credentials.base_url.trim() : ''
  credentials.base_url = configuredBaseURL || GROK_API_BASE_URL
  credentials.using_api = true
}

export function applyAntigravityExtra(
  extra: Record<string, unknown>,
  mixedScheduling: boolean,
  allowOverages: boolean
): void {
  if (mixedScheduling) {
    extra.mixed_scheduling = true
  } else {
    delete extra.mixed_scheduling
  }
  if (allowOverages) {
    extra.allow_overages = true
  } else {
    delete extra.allow_overages
  }
}

export function applyOpenAIPassthrough(
  extra: Record<string, unknown>,
  enabled: boolean,
  mode: 'create' | 'edit' | 'bulk'
): void {
  if (enabled) {
    extra.openai_passthrough = true
    return
  }
  if (mode === 'bulk') {
    extra.openai_passthrough = false
    extra.openai_oauth_passthrough = false
  } else {
    delete extra.openai_passthrough
    delete extra.openai_oauth_passthrough
  }
}

/**
 * 「外部桥」标记：账号由独立部署的 OpenAI 兼容桥服务（Devin / Cursor…）承载。
 *
 * 该标记是 opt-in 的显式开关：打开后网关按外部桥语义处理（命名空间工具摊平、
 * Devin 专属工具转换/描述裁剪、用量投影走桥的状态端点），关闭则回到
 * 普通 OpenAI 兼容上游行为。kind 用于区分具体桥（影响用量端点与模型特例）；
 * 关闭时一并清除，避免留下会被其它逻辑读到的旧 kind。
 */
export function applyOpenAIExternalBridge(
  extra: Record<string, unknown>,
  enabled: boolean,
  kind: string
): void {
  if (!enabled) {
    delete extra.external_bridge
    delete extra.external_bridge_kind
    return
  }
  const normalizedKind = kind.trim().toLowerCase()
  if (!normalizedKind) throw new Error('external bridge kind is required')
  extra.external_bridge = true
  extra.external_bridge_kind = normalizedKind
}

export function applyOpenAIWebSocketExtra<T extends string>(
  extra: Record<string, unknown>,
  accountType: string,
  oauthMode: T,
  apiKeyMode: T,
  isEnabled: (mode: T) => boolean
): void {
  if (accountType === 'oauth' || accountType === 'setup-token' || accountType === 'oauth-based') {
    extra.openai_oauth_responses_websockets_v2_mode = oauthMode
    extra.openai_oauth_responses_websockets_v2_enabled = isEnabled(oauthMode)
  } else if (accountType === 'apikey') {
    extra.openai_apikey_responses_websockets_v2_mode = apiKeyMode
    extra.openai_apikey_responses_websockets_v2_enabled = isEnabled(apiKeyMode)
  }
  delete extra.responses_websockets_v2_enabled
  delete extra.openai_ws_enabled
}

export function normalizeOpenAIEndpointCapabilities(
  values: OpenAIEndpointCapability[]
): OpenAIEndpointCapability[] {
  const allowed: OpenAIEndpointCapability[] = ['chat_completions', 'embeddings']
  const selected = allowed.filter((value) => values.includes(value))
  return selected.length > 0 ? selected : allowed
}

export function applyOpenAIEndpointCapabilities(
  credentials: Record<string, unknown>,
  values: OpenAIEndpointCapability[]
): void {
  const capabilities = normalizeOpenAIEndpointCapabilities(values)
  if (capabilities.length === 2) {
    delete credentials.openai_capabilities
  } else {
    credentials.openai_capabilities = capabilities
  }
}

export function applyCustomBaseURL(
  extra: Record<string, unknown>,
  enabled: boolean,
  value: string,
  mode: 'create' | 'edit'
): void {
  const trimmed = value.trim()
  if (enabled && trimmed) {
    extra.custom_base_url_enabled = true
    extra.custom_base_url = trimmed
  } else if (mode === 'edit') {
    delete extra.custom_base_url_enabled
    delete extra.custom_base_url
  }
}

export function applyAnthropicExtra(
  extra: Record<string, unknown>,
  passthroughEnabled: boolean,
  authScheme: string,
  webSearchMode: string
): void {
  if (passthroughEnabled) extra.anthropic_passthrough = true
  else delete extra.anthropic_passthrough

  if (authScheme === 'authorization_bearer') {
    extra.anthropic_apikey_auth_scheme = authScheme
  } else {
    delete extra.anthropic_apikey_auth_scheme
  }

  if (webSearchMode === 'default') delete extra.web_search_emulation
  else extra.web_search_emulation = webSearchMode
}

// ========== 请求头覆写（仅 anthropic/openai 平台的 api_key 账号） ==========

export const HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY = 'header_override_enabled'
export const HEADER_OVERRIDES_CREDENTIAL_KEY = 'header_overrides'

export interface HeaderOverrideRow {
  name: string
  value: string
}

export {
  isFieldVisible,
  isHeaderOverridePlatform,
  platformsShareField
} from '@/features/admin-accounts/form/registry'

/** 禁止覆写的请求头（与后端 headerOverrideBlockedNames 保持一致） */
const HEADER_OVERRIDE_BLOCKED_NAMES = new Set([
  'host',
  'content-length',
  'content-type',
  'transfer-encoding',
  'connection',
  'keep-alive',
  'proxy-authenticate',
  'proxy-authorization',
  'proxy-connection',
  'te',
  'trailer',
  'upgrade',
  'authorization',
  'x-api-key',
  'x-goog-api-key',
  'cookie',
  'accept-encoding',
  'sec-websocket-key',
  'sec-websocket-version',
  'sec-websocket-extensions',
  'sec-websocket-protocol',
  'sec-websocket-accept',
  'session_id',
  'conversation_id',
  'x-codex-turn-state',
  'x-codex-turn-metadata',
  'chatgpt-account-id',
  'x-claude-code-session-id',
  'x-client-request-id'
])

/** RFC 7230 token：合法的 HTTP header 名称字符集 */
const HEADER_NAME_PATTERN = /^[!#$%&'*+\-.^_`|~0-9A-Za-z]+$/

function isValidHeaderOverrideName(name: string): boolean {
  return HEADER_NAME_PATTERN.test(name)
}

/** 模板：Claude Code CLI 标准客户端请求头（值留空由管理员填写） */
const ANTHROPIC_HEADER_OVERRIDE_TEMPLATE = [
  'user-agent',
  'x-app',
  'anthropic-beta',
  'anthropic-version',
  'anthropic-dangerous-direct-browser-access',
  'x-stainless-lang',
  'x-stainless-package-version',
  'x-stainless-os',
  'x-stainless-arch',
  'x-stainless-runtime',
  'x-stainless-runtime-version',
  'x-stainless-retry-count',
  'x-stainless-timeout'
]

/** 模板：Codex CLI 标准客户端请求头（值留空由管理员填写） */
const OPENAI_HEADER_OVERRIDE_TEMPLATE = [
  'user-agent',
  'originator',
  'openai-beta',
  'version',
  'accept',
  'accept-language'
]

export function getHeaderOverrideTemplate(platform: string): HeaderOverrideRow[] {
  const names =
    platform === 'openai' ? OPENAI_HEADER_OVERRIDE_TEMPLATE : ANTHROPIC_HEADER_OVERRIDE_TEMPLATE
  return names.map((name) => ({ name, value: '' }))
}

/** 与后端 maxHeaderOverride* 常量保持一致 */
const HEADER_OVERRIDE_MAX_ENTRIES = 64
const HEADER_OVERRIDE_MAX_NAME_LENGTH = 200
const HEADER_OVERRIDE_MAX_VALUE_LENGTH = 8192

/** header value 不允许包含控制字符（与后端 httpguts.ValidHeaderFieldValue 对齐） */
// eslint-disable-next-line no-control-regex
const HEADER_VALUE_INVALID_PATTERN = /[\x00-\x08\x0a-\x1f\x7f]/

/** 长度限制按 UTF-8 字节计（与后端 Go len() 对齐，避免多字节值前端放行后端 400） */
const HEADER_TEXT_ENCODER = new TextEncoder()
function utf8ByteLength(value: string): number {
  return HEADER_TEXT_ENCODER.encode(value).length
}

/**
 * 校验请求头覆写行，返回首个错误的 i18n key（无错误返回 null）。
 * 名称为空但值非空 → invalidName；名称非法 → invalidName；
 * 禁止覆写 → blockedName；大小写不敏感重名 → duplicateName；
 * 值含控制字符或超长 → invalidValue；条目过多 → tooManyEntries。
 */
export function validateHeaderOverrideRows(
  rows: HeaderOverrideRow[]
): 'invalidName' | 'blockedName' | 'duplicateName' | 'invalidValue' | 'tooManyEntries' | null {
  const seen = new Set<string>()
  for (const row of rows) {
    const name = row.name.trim()
    const value = row.value.trim()
    if (!name) {
      if (value) return 'invalidName'
      continue
    }
    if (!isValidHeaderOverrideName(name) || name.length > HEADER_OVERRIDE_MAX_NAME_LENGTH) {
      return 'invalidName'
    }
    const lower = name.toLowerCase()
    if (HEADER_OVERRIDE_BLOCKED_NAMES.has(lower)) return 'blockedName'
    if (seen.has(lower)) return 'duplicateName'
    if (
      HEADER_VALUE_INVALID_PATTERN.test(value) ||
      utf8ByteLength(value) > HEADER_OVERRIDE_MAX_VALUE_LENGTH
    ) {
      return 'invalidValue'
    }
    seen.add(lower)
  }
  if (seen.size > HEADER_OVERRIDE_MAX_ENTRIES) return 'tooManyEntries'
  return null
}

/** 行数组 → credentials 存储对象（名称小写化，丢弃空行） */
export function buildHeaderOverridesObject(rows: HeaderOverrideRow[]): Record<string, string> {
  const result: Record<string, string> = {}
  for (const row of rows) {
    const name = row.name.trim().toLowerCase()
    if (!name) continue
    result[name] = row.value.trim()
  }
  return result
}

/** credentials 存储对象 → 行数组（按名称排序保证稳定展示） */
export function splitHeaderOverridesObject(record: unknown): HeaderOverrideRow[] {
  if (!record || typeof record !== 'object' || Array.isArray(record)) return []
  return Object.entries(record as Record<string, unknown>)
    .filter(([, value]) => typeof value === 'string')
    .map(([name, value]) => ({ name, value: value as string }))
    .sort((a, b) => a.name.localeCompare(b.name))
}

/**
 * 将请求头覆写写入 credentials。
 * create 模式：关闭时不写入任何字段；edit 模式：关闭时删除字段（全量替换语义）。
 */
export function applyHeaderOverride(
  credentials: Record<string, unknown>,
  enabled: boolean,
  rows: HeaderOverrideRow[],
  mode: 'create' | 'edit' | 'bulk'
): void {
  if (enabled) {
    credentials[HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY] = true
    credentials[HEADER_OVERRIDES_CREDENTIAL_KEY] = buildHeaderOverridesObject(rows)
  } else if (mode === 'edit') {
    delete credentials[HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY]
    delete credentials[HEADER_OVERRIDES_CREDENTIAL_KEY]
  } else if (mode === 'bulk') {
    // Bulk updates merge JSONB keys, so explicitly write empty values to clear
    // the selected accounts' existing override configuration.
    credentials[HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY] = false
    credentials[HEADER_OVERRIDES_CREDENTIAL_KEY] = {}
  }
}

// Bulk JSON merge requires an explicit auto value to clear an existing override.
export function applyOpenAICompactMode(
  extra: Record<string, unknown>,
  value: OpenAICompactMode,
  mode: 'create' | 'edit' | 'bulk'
): void {
  if (value === 'auto' && mode !== 'bulk') {
    delete extra.openai_compact_mode
  } else {
    extra.openai_compact_mode = value
  }
}

// ===== Mirasim relay（Anthropic API Key 账号的第三方中继上游） =====
//
// 关键约束（改动前请先读）：
//   - 后端识别规则（backend/internal/service/mirasim_relay.go isMirasimRelayAccount）是
//     "platform=anthropic && type=apikey && credentials.mirasim_refresh_token 非空，
//      且 extra.mirasim_relay_enabled 未显式置 false"；前端的写入必须与此一致。
//   - relay 地址放在 credentials.base_url（复用既有自定义 base URL 机制与 URL 白名单），
//     不新增凭据键；extra.mirasim_relay_url 只是可选覆盖。
//   - mirasim 账号没有 api_key：留空串会让凭据面板出现误导性空值，且后端
//     GetAccessToken 的 mirasim 分支不读 api_key，因此空值时直接不写该键。
//   - 敏感键（mirasim_refresh_token / mirasim_device_key）后端不回显；编辑时留空
//     表示"保留原值"（后端 MergePreservingSensitiveCreds），不能写成空串。

/** 凭据键（与后端 mirasimCredentialRefreshToken / mirasimCredentialDeviceKey 一致） */
export const MIRASIM_CREDENTIAL_REFRESH_TOKEN = 'mirasim_refresh_token'
export const MIRASIM_CREDENTIAL_DEVICE_KEY = 'mirasim_device_key'

/** extra 键（与后端 mirasimExtra* 常量一致） */
export const MIRASIM_EXTRA_ENABLED = 'mirasim_relay_enabled'
export const MIRASIM_EXTRA_RELAY_URL = 'mirasim_relay_url'
export const MIRASIM_EXTRA_AUTH_URL = 'mirasim_auth_url'
export const MIRASIM_EXTRA_CLIENT_VERSION = 'mirasim_client_version'
export const MIRASIM_EXTRA_SEAL_PUBKEY = 'mirasim_seal_pubkey'

/** 默认端点与协议参数（与后端 mirasimDefault* 常量一致） */
export const MIRASIM_DEFAULT_RELAY_URL = 'https://relay.mirasim.ai'
export const MIRASIM_DEFAULT_AUTH_URL = 'https://auth.mirasim.ai'
export const MIRASIM_DEFAULT_CLIENT_VERSION = '0.0.322'
export const MIRASIM_DEFAULT_SEAL_PUBKEY = 'HlyNMMeGXryasYLJuYQ/9ksCD4AYVVy1zXKAtJdpJn4='
export const ANTHROPIC_OFFICIAL_BASE_URL = 'https://api.anthropic.com'

export interface MirasimRelayInput {
  enabled: boolean
  refreshToken: string
  deviceKey: string
  relayUrl: string
  authUrl: string
  clientVersion: string
  sealPubkey: string
}

export type MirasimRelayValidationError =
  | 'refreshTokenRequired'
  | 'invalidRelayUrl'
  | 'invalidAuthUrl'
  | 'invalidSealPubkey'

/** 空值表示"用默认"，只校验非空值的可解析性。 */
function isMirasimEndpointValid(value: string): boolean {
  const trimmed = value.trim()
  if (!trimmed) return true
  try {
    const parsed = new URL(trimmed)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
}

/**
 * 校验 Mirasim relay 表单。
 * hasExistingRefreshToken：编辑场景下账号已有 refresh token（后端不回显），此时留空合法。
 */
export function validateMirasimRelayInput(
  input: MirasimRelayInput,
  hasExistingRefreshToken: boolean
): MirasimRelayValidationError | null {
  if (!input.enabled) return null
  if (!input.refreshToken.trim() && !hasExistingRefreshToken) return 'refreshTokenRequired'
  if (!isMirasimEndpointValid(input.relayUrl)) return 'invalidRelayUrl'
  if (!isMirasimEndpointValid(input.authUrl)) return 'invalidAuthUrl'
  // seal 公钥是 relay 静态 X25519 公钥的 base64，长度固定 32 字节 → 44 字符含 '='。
  const sealPubkey = input.sealPubkey.trim()
  if (sealPubkey && !/^[A-Za-z0-9+/]{43}=$/.test(sealPubkey)) return 'invalidSealPubkey'
  return null
}

/** 账号 extra/凭据状态 → 表单初始值（只读，不改动入参）。 */
export function mirasimRelayInputFromAccount(
  extra: Record<string, unknown> | undefined,
  hasRefreshToken: boolean
): MirasimRelayInput {
  const flag = extra?.[MIRASIM_EXTRA_ENABLED]
  const explicitlyDisabled = flag !== undefined && flag !== true
  return {
    enabled: !explicitlyDisabled && (flag === true || hasRefreshToken),
    refreshToken: '',
    deviceKey: '',
    relayUrl: String(extra?.[MIRASIM_EXTRA_RELAY_URL] ?? ''),
    authUrl: String(extra?.[MIRASIM_EXTRA_AUTH_URL] ?? ''),
    clientVersion: String(extra?.[MIRASIM_EXTRA_CLIENT_VERSION] ?? ''),
    sealPubkey: String(extra?.[MIRASIM_EXTRA_SEAL_PUBKEY] ?? '')
  }
}

function writeOptionalMirasimExtra(
  extra: Record<string, unknown>,
  key: string,
  value: string
): void {
  const trimmed = value.trim()
  if (trimmed) {
    extra[key] = trimmed
  } else {
    delete extra[key]
  }
}

/**
 * 把 Mirasim relay 表单写入 credentials 与 extra（原地修改）。
 *
 * options.currentBaseUrl：表单当前的 base URL，用于判断是否由本功能自动填写；
 * options.previousEnabled：编辑场景下账号原本是否已启用（关闭时写显式 false 以生效）。
 */
export function applyMirasimRelay(
  credentials: Record<string, unknown>,
  extra: Record<string, unknown>,
  input: MirasimRelayInput,
  mode: 'create' | 'edit',
  options: { currentBaseUrl?: string; previousEnabled?: boolean } = {}
): void {
  const currentBaseUrl = (options.currentBaseUrl ?? '').trim().replace(/\/+$/, '')
  const relayOverride = input.relayUrl.trim().replace(/\/+$/, '')

  if (!input.enabled) {
    if (options.previousEnabled || mode === 'edit') {
      // 显式 false 才能让后端停止按 mirasim 处理；凭据保留，便于再次开启。
      extra[MIRASIM_EXTRA_ENABLED] = false
    } else {
      delete extra[MIRASIM_EXTRA_ENABLED]
    }
    delete extra[MIRASIM_EXTRA_RELAY_URL]
    delete extra[MIRASIM_EXTRA_AUTH_URL]
    delete extra[MIRASIM_EXTRA_CLIENT_VERSION]
    delete extra[MIRASIM_EXTRA_SEAL_PUBKEY]

    // 关闭后若 base_url 仍指向 relay（本功能自动填入或用户填的 relay 覆盖值），
    // 请求会打到 relay 却不带 mirasim 签名头，表现为持续 401 → 换回官方地址。
    const relayEndpoints = [MIRASIM_DEFAULT_RELAY_URL, relayOverride].filter(Boolean)
    if (!currentBaseUrl || relayEndpoints.includes(currentBaseUrl)) {
      credentials.base_url = ANTHROPIC_OFFICIAL_BASE_URL
    }
    return
  }

  extra[MIRASIM_EXTRA_ENABLED] = true
  writeOptionalMirasimExtra(extra, MIRASIM_EXTRA_RELAY_URL, relayOverride)
  writeOptionalMirasimExtra(extra, MIRASIM_EXTRA_AUTH_URL, input.authUrl)
  writeOptionalMirasimExtra(extra, MIRASIM_EXTRA_CLIENT_VERSION, input.clientVersion)
  writeOptionalMirasimExtra(extra, MIRASIM_EXTRA_SEAL_PUBKEY, input.sealPubkey)

  const refreshToken = input.refreshToken.trim()
  if (refreshToken) {
    credentials[MIRASIM_CREDENTIAL_REFRESH_TOKEN] = refreshToken
  } else {
    // 编辑留空 = 保留后端已有值；删除键避免把空串写入凭据。
    delete credentials[MIRASIM_CREDENTIAL_REFRESH_TOKEN]
  }
  const deviceKey = input.deviceKey.trim()
  if (deviceKey) {
    credentials[MIRASIM_CREDENTIAL_DEVICE_KEY] = deviceKey
  } else {
    delete credentials[MIRASIM_CREDENTIAL_DEVICE_KEY]
  }

  // relay 账号不需要 api_key（后端 GetAccessToken 的 mirasim 分支直接取 ticket）。
  if (!String(credentials.api_key ?? '').trim()) {
    delete credentials.api_key
  }

  // base_url 是 relay 地址的首选来源；仅在用户未填/仍是官方地址时用默认值补齐。
  if (relayOverride) {
    credentials.base_url = relayOverride
  } else if (!currentBaseUrl || currentBaseUrl === ANTHROPIC_OFFICIAL_BASE_URL) {
    credentials.base_url = MIRASIM_DEFAULT_RELAY_URL
  }
}
