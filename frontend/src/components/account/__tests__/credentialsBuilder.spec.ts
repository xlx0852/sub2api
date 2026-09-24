import { describe, it, expect } from 'vitest'
import {
  ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY,
  MIRASIM_DEFAULT_RELAY_URL,
  MIRASIM_DEFAULT_SEAL_PUBKEY,
  applyMirasimRelay,
  mirasimRelayInputFromAccount,
  validateMirasimRelayInput,
  HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY,
  HEADER_OVERRIDES_CREDENTIAL_KEY,
  applyAntigravityExtra,
  applyAnthropicExtra,
  applyCustomBaseURL,
  applyOpenAIPassthrough,
  applyOpenAIExternalBridge,
  applyOpenAICompactMode,
  applyOpenAIWebSocketExtra,
  applyOpenAIEndpointCapabilities,
  normalizeOpenAIEndpointCapabilities,
  applyAntigravityProjectID,
  applyGrokAPIKeyRouting,
  applyHeaderOverride,
  applyInterceptWarmup,
  buildHeaderOverridesObject,
  getHeaderOverrideTemplate,
  isHeaderOverridePlatform,
  splitHeaderOverridesObject,
  validateHeaderOverrideRows
} from '@/features/admin-accounts/form/credentialsBuilder'

describe('applyOpenAICompactMode', () => {
  it.each(['create', 'edit', 'bulk'] as const)('%s preserves unrelated fields and resets overrides correctly', (mode) => {
    const extra: Record<string, unknown> = { keep: 42, openai_compact_mode: 'force_on' }
    applyOpenAICompactMode(extra, 'auto', mode)
    expect(extra).toEqual(mode === 'bulk' ? { keep: 42, openai_compact_mode: 'auto' } : { keep: 42 })
    applyOpenAICompactMode(extra, 'force_off', mode)
    expect(extra).toEqual({ keep: 42, openai_compact_mode: 'force_off' })
  })
})

describe('applyAntigravityExtra', () => {
  it('sets enabled flags and removes stale disabled flags', () => {
    const extra: Record<string, unknown> = {
      mixed_scheduling: true,
      allow_overages: true,
      keep: 'value'
    }

    applyAntigravityExtra(extra, false, true)
    expect(extra).toEqual({ allow_overages: true, keep: 'value' })

    applyAntigravityExtra(extra, true, false)
    expect(extra).toEqual({ mixed_scheduling: true, keep: 'value' })
  })
})

describe('applyAnthropicExtra', () => {
  it('sets selected values and removes stale optional values', () => {
    const extra: Record<string, unknown> = {
      anthropic_passthrough: true,
      anthropic_apikey_auth_scheme: 'authorization_bearer',
      web_search_emulation: 'default'
    }
    applyAnthropicExtra(extra, false, 'api_key', 'native')
    expect(extra).toEqual({ web_search_emulation: 'native' })
  })
})

describe('applyOpenAIPassthrough', () => {
  it('keeps bulk false values explicit and clears legacy keys for single updates', () => {
    const bulk: Record<string, unknown> = {}
    applyOpenAIPassthrough(bulk, false, 'bulk')
    expect(bulk).toEqual({ openai_passthrough: false, openai_oauth_passthrough: false })

    const edit: Record<string, unknown> = {
      openai_passthrough: true,
      openai_oauth_passthrough: true
    }
    applyOpenAIPassthrough(edit, false, 'edit')
    expect(edit).toEqual({})
  })
})

describe('applyOpenAIWebSocketExtra', () => {
  it('writes type-specific modes and removes legacy keys', () => {
    const extra: Record<string, unknown> = {
      responses_websockets_v2_enabled: true,
      openai_ws_enabled: true
    }
    applyOpenAIWebSocketExtra(extra, 'oauth', 'ctx_pool', 'off', (mode) => mode !== 'off')
    expect(extra).toEqual({
      openai_oauth_responses_websockets_v2_mode: 'ctx_pool',
      openai_oauth_responses_websockets_v2_enabled: true
    })
  })
})

describe('OpenAI endpoint capabilities', () => {
  it('normalizes unknown/empty values and omits the all-capabilities override', () => {
    expect(normalizeOpenAIEndpointCapabilities([])).toEqual(['chat_completions', 'embeddings'])
    expect(normalizeOpenAIEndpointCapabilities(['embeddings'])).toEqual(['embeddings'])

    const credentials: Record<string, unknown> = { openai_capabilities: ['embeddings'] }
    applyOpenAIEndpointCapabilities(credentials, ['chat_completions', 'embeddings'])
    expect(credentials).toEqual({})
  })
})

describe('applyCustomBaseURL', () => {
  it('trims and writes create values, while edit clears disabled values', () => {
    const created: Record<string, unknown> = {}
    applyCustomBaseURL(created, true, ' https://upstream.example/v1 ', 'create')
    expect(created).toEqual({
      custom_base_url_enabled: true,
      custom_base_url: 'https://upstream.example/v1'
    })

    applyCustomBaseURL(created, false, '', 'edit')
    expect(created).toEqual({})
  })
})

describe('applyGrokAPIKeyRouting', () => {
  it('selects the official xAI API for Grok API Key accounts', () => {
    const credentials: Record<string, unknown> = { api_key: 'xai-key', base_url: '  ' }
    applyGrokAPIKeyRouting(credentials, 'grok', 'apikey')
    expect(credentials.base_url).toBe('https://api.x.ai/v1')
    expect(credentials.using_api).toBe(true)
  })

  it('preserves an explicit compatible upstream', () => {
    const credentials: Record<string, unknown> = {
      api_key: 'xai-key',
      base_url: ' https://xai.example.com/v1 '
    }
    applyGrokAPIKeyRouting(credentials, 'grok', 'apikey')
    expect(credentials.base_url).toBe('https://xai.example.com/v1')
    expect(credentials.using_api).toBe(true)
  })

  it('does not change Grok OAuth or non-Grok accounts', () => {
    const oauth: Record<string, unknown> = { access_token: 'token' }
    const openai: Record<string, unknown> = { api_key: 'key' }
    applyGrokAPIKeyRouting(oauth, 'grok', 'oauth')
    applyGrokAPIKeyRouting(openai, 'openai', 'apikey')
    expect(oauth).toEqual({ access_token: 'token' })
    expect(openai).toEqual({ api_key: 'key' })
  })
})

describe('applyInterceptWarmup', () => {
  it('create + enabled=true: should set intercept_warmup_requests to true', () => {
    const creds: Record<string, unknown> = { access_token: 'tok' }
    applyInterceptWarmup(creds, true, 'create')
    expect(creds.intercept_warmup_requests).toBe(true)
  })

  it('create + enabled=false: should not add the field', () => {
    const creds: Record<string, unknown> = { access_token: 'tok' }
    applyInterceptWarmup(creds, false, 'create')
    expect('intercept_warmup_requests' in creds).toBe(false)
  })

  it('edit + enabled=true: should set intercept_warmup_requests to true', () => {
    const creds: Record<string, unknown> = { api_key: 'sk' }
    applyInterceptWarmup(creds, true, 'edit')
    expect(creds.intercept_warmup_requests).toBe(true)
  })

  it('edit + enabled=false + field exists: should delete the field', () => {
    const creds: Record<string, unknown> = { api_key: 'sk', intercept_warmup_requests: true }
    applyInterceptWarmup(creds, false, 'edit')
    expect('intercept_warmup_requests' in creds).toBe(false)
  })

  it('edit + enabled=false + field absent: should not throw', () => {
    const creds: Record<string, unknown> = { api_key: 'sk' }
    applyInterceptWarmup(creds, false, 'edit')
    expect('intercept_warmup_requests' in creds).toBe(false)
  })

  it('should not affect other fields', () => {
    const creds: Record<string, unknown> = {
      api_key: 'sk',
      base_url: 'url',
      intercept_warmup_requests: true
    }
    applyInterceptWarmup(creds, false, 'edit')
    expect(creds.api_key).toBe('sk')
    expect(creds.base_url).toBe('url')
    expect('intercept_warmup_requests' in creds).toBe(false)
  })
})

describe('applyAntigravityProjectID', () => {
  it('create + project id: trims and stores configured project fallback', () => {
    const creds: Record<string, unknown> = { access_token: 'tok' }
    applyAntigravityProjectID(creds, '  configured-project  ', 'create')
    expect(creds[ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY]).toBe('configured-project')
  })

  it('create + empty project id: should not add the field', () => {
    const creds: Record<string, unknown> = { access_token: 'tok' }
    applyAntigravityProjectID(creds, '   ', 'create')
    expect(ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY in creds).toBe(false)
  })

  it('edit + empty project id: deletes existing fallback', () => {
    const creds: Record<string, unknown> = {
      access_token: 'tok',
      [ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY]: 'old-project'
    }
    applyAntigravityProjectID(creds, '', 'edit')
    expect(ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY in creds).toBe(false)
  })

  it('does not affect onboard project_id or other credentials', () => {
    const creds: Record<string, unknown> = {
      project_id: 'onboard-project',
      model_mapping: { 'gemini-*': 'gemini-2.5-flash' }
    }
    applyAntigravityProjectID(creds, 'configured-project', 'edit')
    expect(creds.project_id).toBe('onboard-project')
    expect(creds.model_mapping).toEqual({ 'gemini-*': 'gemini-2.5-flash' })
    expect(creds[ANTIGRAVITY_PROJECT_ID_CREDENTIAL_KEY]).toBe('configured-project')
  })
})

describe('isHeaderOverridePlatform', () => {
  it('only anthropic and openai are supported', () => {
    expect(isHeaderOverridePlatform('anthropic')).toBe(true)
    expect(isHeaderOverridePlatform('openai')).toBe(true)
    expect(isHeaderOverridePlatform('gemini')).toBe(false)
    expect(isHeaderOverridePlatform('grok')).toBe(false)
    expect(isHeaderOverridePlatform('antigravity')).toBe(false)
    expect(isHeaderOverridePlatform('')).toBe(false)
  })
})

describe('validateHeaderOverrideRows', () => {
  it('accepts valid rows and empty placeholder rows', () => {
    expect(
      validateHeaderOverrideRows([
        { name: 'user-agent', value: 'my-agent/1.0' },
        { name: 'x-app', value: '' },
        { name: '', value: '' }
      ])
    ).toBeNull()
  })

  it('rejects empty name with non-empty value', () => {
    expect(validateHeaderOverrideRows([{ name: '', value: 'v' }])).toBe('invalidName')
  })

  it('rejects invalid header names', () => {
    expect(validateHeaderOverrideRows([{ name: 'bad name', value: '' }])).toBe('invalidName')
    expect(validateHeaderOverrideRows([{ name: 'bad:name', value: '' }])).toBe('invalidName')
    expect(validateHeaderOverrideRows([{ name: '名称', value: '' }])).toBe('invalidName')
  })

  it('rejects blocked header names case-insensitively', () => {
    expect(validateHeaderOverrideRows([{ name: 'Authorization', value: '' }])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'X-Api-Key', value: '' }])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'host', value: '' }])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'Content-Length', value: '' }])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'Content-Type', value: '' }])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'Cookie', value: '' }])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'x-goog-api-key', value: '' }])).toBe('blockedName')
  })

  it('rejects duplicate names case-insensitively', () => {
    expect(
      validateHeaderOverrideRows([
        { name: 'User-Agent', value: 'a' },
        { name: 'user-agent', value: 'b' }
      ])
    ).toBe('duplicateName')
  })
})

describe('buildHeaderOverridesObject / splitHeaderOverridesObject', () => {
  it('lowercases names, trims values and drops empty-name rows', () => {
    expect(
      buildHeaderOverridesObject([
        { name: ' User-Agent ', value: ' my-agent ' },
        { name: 'X-App', value: '' },
        { name: '', value: 'ignored' }
      ])
    ).toEqual({ 'user-agent': 'my-agent', 'x-app': '' })
  })

  it('splits an object into sorted rows and ignores non-string values', () => {
    expect(
      splitHeaderOverridesObject({ 'x-app': 'cli', 'user-agent': 'ua', bogus: 42 })
    ).toEqual([
      { name: 'user-agent', value: 'ua' },
      { name: 'x-app', value: 'cli' }
    ])
    expect(splitHeaderOverridesObject(null)).toEqual([])
    expect(splitHeaderOverridesObject(['a'])).toEqual([])
    expect(splitHeaderOverridesObject('str')).toEqual([])
  })

  it('roundtrips through build and split', () => {
    const rows = [
      { name: 'user-agent', value: 'ua' },
      { name: 'x-app', value: 'cli' }
    ]
    expect(splitHeaderOverridesObject(buildHeaderOverridesObject(rows))).toEqual(rows)
  })
})

describe('getHeaderOverrideTemplate', () => {
  it('returns Claude Code CLI headers with empty values for anthropic', () => {
    const rows = getHeaderOverrideTemplate('anthropic')
    expect(rows.every((r) => r.value === '')).toBe(true)
    const names = rows.map((r) => r.name)
    expect(names).toContain('user-agent')
    expect(names).toContain('x-app')
    expect(names).toContain('anthropic-beta')
    expect(names).toContain('x-stainless-lang')
    expect(validateHeaderOverrideRows(rows)).toBeNull()
  })

  it('returns Codex CLI headers with empty values for openai', () => {
    const rows = getHeaderOverrideTemplate('openai')
    expect(rows.every((r) => r.value === '')).toBe(true)
    const names = rows.map((r) => r.name)
    expect(names).toContain('user-agent')
    expect(names).toContain('originator')
    expect(names).toContain('openai-beta')
    expect(validateHeaderOverrideRows(rows)).toBeNull()
  })
})

describe('applyHeaderOverride', () => {
  it('create + enabled: writes enabled flag and overrides object', () => {
    const creds: Record<string, unknown> = { api_key: 'sk' }
    applyHeaderOverride(creds, true, [{ name: 'User-Agent', value: 'ua' }], 'create')
    expect(creds[HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY]).toBe(true)
    expect(creds[HEADER_OVERRIDES_CREDENTIAL_KEY]).toEqual({ 'user-agent': 'ua' })
  })

  it('create + disabled: does not add fields', () => {
    const creds: Record<string, unknown> = { api_key: 'sk' }
    applyHeaderOverride(creds, false, [{ name: 'user-agent', value: 'ua' }], 'create')
    expect(HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY in creds).toBe(false)
    expect(HEADER_OVERRIDES_CREDENTIAL_KEY in creds).toBe(false)
  })

  it('edit + disabled: deletes existing fields', () => {
    const creds: Record<string, unknown> = {
      api_key: 'sk',
      [HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY]: true,
      [HEADER_OVERRIDES_CREDENTIAL_KEY]: { 'user-agent': 'ua' }
    }
    applyHeaderOverride(creds, false, [], 'edit')
    expect(HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY in creds).toBe(false)
    expect(HEADER_OVERRIDES_CREDENTIAL_KEY in creds).toBe(false)
    expect(creds.api_key).toBe('sk')
  })

  it('edit + enabled: replaces overrides object wholesale', () => {
    const creds: Record<string, unknown> = {
      [HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY]: true,
      [HEADER_OVERRIDES_CREDENTIAL_KEY]: { 'x-old': 'old' }
    }
    applyHeaderOverride(creds, true, [{ name: 'x-new', value: 'new' }], 'edit')
    expect(creds[HEADER_OVERRIDES_CREDENTIAL_KEY]).toEqual({ 'x-new': 'new' })
  })

  it('bulk + disabled: writes explicit empty values for JSON merge updates', () => {
    const creds: Record<string, unknown> = {
      [HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY]: true,
      [HEADER_OVERRIDES_CREDENTIAL_KEY]: { 'x-old': 'old' }
    }
    applyHeaderOverride(creds, false, [], 'bulk')
    expect(creds[HEADER_OVERRIDE_ENABLED_CREDENTIAL_KEY]).toBe(false)
    expect(creds[HEADER_OVERRIDES_CREDENTIAL_KEY]).toEqual({})
  })
})

describe('validateHeaderOverrideRows value/entry limits', () => {
  it('rejects websocket handshake headers', () => {
    expect(validateHeaderOverrideRows([{ name: 'Sec-WebSocket-Key', value: '' }])).toBe(
      'blockedName'
    )
  })

  it('rejects control characters in values', () => {
    expect(validateHeaderOverrideRows([{ name: 'x-app', value: 'a\x0bb' }])).toBe('invalidValue')
  })

  it('rejects oversized values', () => {
    expect(validateHeaderOverrideRows([{ name: 'x-app', value: 'a'.repeat(8193) }])).toBe(
      'invalidValue'
    )
  })

  it('measures value length in UTF-8 bytes to match backend', () => {
    // 3000 个 CJK 字符 = 3000 UTF-16 code units，但 9000 UTF-8 字节 > 8192
    expect(validateHeaderOverrideRows([{ name: 'x-app', value: '测'.repeat(3000) }])).toBe(
      'invalidValue'
    )
    expect(validateHeaderOverrideRows([{ name: 'x-app', value: '测'.repeat(2000) }])).toBeNull()
  })

  it('rejects too many entries', () => {
    const rows = Array.from({ length: 65 }, (_, i) => ({ name: `x-h-${i}`, value: 'v' }))
    expect(validateHeaderOverrideRows(rows)).toBe('tooManyEntries')
  })
})

describe('validateHeaderOverrideRows session isolation headers', () => {
  it('rejects per-request session headers', () => {
    expect(validateHeaderOverrideRows([{ name: 'session_id', value: '' }])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'Conversation_ID', value: '' }])).toBe('blockedName')
    expect(validateHeaderOverrideRows([{ name: 'x-codex-turn-state', value: '' }])).toBe(
      'blockedName'
    )
    expect(validateHeaderOverrideRows([{ name: 'X-Claude-Code-Session-Id', value: '' }])).toBe(
      'blockedName'
    )
    expect(validateHeaderOverrideRows([{ name: 'x-client-request-id', value: '' }])).toBe(
      'blockedName'
    )
  })

  it('allows tab inside value', () => {
    expect(validateHeaderOverrideRows([{ name: 'x-app', value: 'a\tb' }])).toBeNull()
  })

  it('rejects oversized names', () => {
    expect(validateHeaderOverrideRows([{ name: 'x'.repeat(201), value: 'v' }])).toBe('invalidName')
  })
})

describe('applyOpenAIExternalBridge', () => {
  it('enables the bridge flag and normalizes the kind', () => {
    const extra: Record<string, unknown> = {}
    applyOpenAIExternalBridge(extra, true, '  Devin ')
    expect(extra.external_bridge).toBe(true)
    expect(extra.external_bridge_kind).toBe('devin')
  })

  it('rejects empty kinds before mutating the existing extra', () => {
    const extra: Record<string, unknown> = { external_bridge_kind: 'cursor' }
    expect(() => applyOpenAIExternalBridge(extra, true, '   ')).toThrow('kind is required')
    expect(extra).toEqual({ external_bridge_kind: 'cursor' })
  })

  it('clears both keys when disabled', () => {
    const extra: Record<string, unknown> = { external_bridge: true, external_bridge_kind: 'devin', keep: 1 }
    applyOpenAIExternalBridge(extra, false, 'devin')
    expect(extra.external_bridge).toBeUndefined()
    expect(extra.external_bridge_kind).toBeUndefined()
    expect(extra.keep).toBe(1)
  })
})

const mirasimInput = (overrides: Partial<Record<string, unknown>> = {}) => ({
  enabled: true,
  refreshToken: '',
  deviceKey: '',
  relayUrl: '',
  authUrl: '',
  clientVersion: '',
  sealPubkey: '',
  ...overrides
}) as Parameters<typeof applyMirasimRelay>[2]

describe('applyMirasimRelay', () => {
  it('writes credentials and extra on enable, defaulting base_url to the relay', () => {
    const credentials: Record<string, unknown> = { base_url: 'https://api.anthropic.com' }
    const extra: Record<string, unknown> = {}

    applyMirasimRelay(
      credentials,
      extra,
      mirasimInput({ refreshToken: ' rt-1 ' }),
      'create',
      { currentBaseUrl: 'https://api.anthropic.com' }
    )

    expect(credentials.mirasim_refresh_token).toBe('rt-1')
    expect(credentials.base_url).toBe(MIRASIM_DEFAULT_RELAY_URL)
    expect(extra.mirasim_relay_enabled).toBe(true)
    expect(extra.mirasim_relay_url).toBeUndefined()
  })

  it('honours relay/auth overrides and drops an empty api_key', () => {
    const credentials: Record<string, unknown> = { base_url: 'https://api.anthropic.com', api_key: '' }
    const extra: Record<string, unknown> = { keep: 1 }

    applyMirasimRelay(
      credentials,
      extra,
      mirasimInput({
        refreshToken: 'rt',
        relayUrl: 'https://relay.internal.example/',
        authUrl: 'https://auth.internal.example',
        clientVersion: '9.9.9',
        sealPubkey: MIRASIM_DEFAULT_SEAL_PUBKEY
      }),
      'create',
      { currentBaseUrl: 'https://api.anthropic.com' }
    )

    expect(credentials.base_url).toBe('https://relay.internal.example')
    expect('api_key' in credentials).toBe(false)
    expect(extra).toEqual({
      keep: 1,
      mirasim_relay_enabled: true,
      mirasim_relay_url: 'https://relay.internal.example',
      mirasim_auth_url: 'https://auth.internal.example',
      mirasim_client_version: '9.9.9',
      mirasim_seal_pubkey: MIRASIM_DEFAULT_SEAL_PUBKEY
    })
  })

  it('keeps a real api_key and an explicit base_url untouched', () => {
    const credentials: Record<string, unknown> = { base_url: 'https://custom.example', api_key: 'sk-x' }
    applyMirasimRelay(credentials, {}, mirasimInput({ refreshToken: 'rt' }), 'edit', {
      currentBaseUrl: 'https://custom.example'
    })
    expect(credentials.api_key).toBe('sk-x')
    expect(credentials.base_url).toBe('https://custom.example')
  })

  it('treats an empty refresh token in edit mode as "keep existing" instead of writing an empty string', () => {
    const credentials: Record<string, unknown> = {
      base_url: MIRASIM_DEFAULT_RELAY_URL,
      mirasim_refresh_token: 'stale-from-redacted-spread'
    }
    applyMirasimRelay(credentials, {}, mirasimInput({ deviceKey: 'pem' }), 'edit', {
      previousEnabled: true
    })
    expect('mirasim_refresh_token' in credentials).toBe(false)
    expect(credentials.mirasim_device_key).toBe('pem')
  })

  it('disables explicitly and restores the official base URL', () => {
    const credentials: Record<string, unknown> = { base_url: MIRASIM_DEFAULT_RELAY_URL }
    const extra: Record<string, unknown> = {
      mirasim_relay_enabled: true,
      mirasim_auth_url: 'https://auth.internal.example',
      keep: 'v'
    }

    applyMirasimRelay(credentials, extra, mirasimInput({ enabled: false, refreshToken: 'rt' }), 'edit', {
      currentBaseUrl: MIRASIM_DEFAULT_RELAY_URL,
      previousEnabled: true
    })

    expect(credentials.base_url).toBe('https://api.anthropic.com')
    expect(extra).toEqual({ keep: 'v', mirasim_relay_enabled: false })
    // 凭据保留：再次开启无需重新粘贴 refresh token。写入由调用方负责，这里只断言未新增敏感键。
    expect('mirasim_refresh_token' in credentials).toBe(false)
  })

  it('writes no flag on create-disable but clears an auto-filled relay base URL', () => {
    const credentials: Record<string, unknown> = { base_url: MIRASIM_DEFAULT_RELAY_URL }
    const extra: Record<string, unknown> = {}
    applyMirasimRelay(credentials, extra, mirasimInput({ enabled: false }), 'create', {
      currentBaseUrl: MIRASIM_DEFAULT_RELAY_URL
    })
    expect('mirasim_relay_enabled' in extra).toBe(false)
    expect(credentials.base_url).toBe('https://api.anthropic.com')
  })

  it('leaves an unrelated custom base URL alone when disabled', () => {
    const credentials: Record<string, unknown> = { base_url: 'https://custom.example' }
    applyMirasimRelay(credentials, {}, mirasimInput({ enabled: false }), 'create', {
      currentBaseUrl: 'https://custom.example'
    })
    expect(credentials.base_url).toBe('https://custom.example')
  })
})

describe('validateMirasimRelayInput', () => {
  it('requires a refresh token only when none is stored yet', () => {
    expect(validateMirasimRelayInput(mirasimInput(), false)).toBe('refreshTokenRequired')
    expect(validateMirasimRelayInput(mirasimInput(), true)).toBeNull()
    expect(validateMirasimRelayInput(mirasimInput({ enabled: false }), false)).toBeNull()
  })

  it('rejects malformed endpoints and seal keys', () => {
    expect(validateMirasimRelayInput(mirasimInput({ refreshToken: 'rt', relayUrl: 'relay.ai' }), false)).toBe(
      'invalidRelayUrl'
    )
    expect(validateMirasimRelayInput(mirasimInput({ refreshToken: 'rt', authUrl: 'ftp://a' }), false)).toBe(
      'invalidAuthUrl'
    )
    expect(validateMirasimRelayInput(mirasimInput({ refreshToken: 'rt', sealPubkey: 'short' }), false)).toBe(
      'invalidSealPubkey'
    )
    expect(
      validateMirasimRelayInput(mirasimInput({ refreshToken: 'rt', sealPubkey: MIRASIM_DEFAULT_SEAL_PUBKEY }), false)
    ).toBeNull()
    expect(MIRASIM_DEFAULT_SEAL_PUBKEY).toMatch(/^[A-Za-z0-9+/]{43}=$/)
  })
})

describe('mirasimRelayInputFromAccount', () => {
  it('derives enabled state from the flag or a stored refresh token', () => {
    expect(mirasimRelayInputFromAccount({ mirasim_relay_enabled: true }, false).enabled).toBe(true)
    expect(mirasimRelayInputFromAccount({}, true).enabled).toBe(true)
    expect(mirasimRelayInputFromAccount({}, false).enabled).toBe(false)
    expect(mirasimRelayInputFromAccount({ mirasim_relay_enabled: false }, true).enabled).toBe(false)
  })

  it('restores override fields and never echoes sensitive values', () => {
    const input = mirasimRelayInputFromAccount(
      {
        mirasim_relay_url: 'https://relay.internal.example',
        mirasim_client_version: '1.2.3'
      },
      true
    )
    expect(input).toEqual({
      enabled: true,
      refreshToken: '',
      deviceKey: '',
      relayUrl: 'https://relay.internal.example',
      authUrl: '',
      clientVersion: '1.2.3',
      sealPubkey: ''
    })
  })
})
