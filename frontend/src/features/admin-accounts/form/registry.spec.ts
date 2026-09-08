import { describe, expect, it } from 'vitest'
import {
  ACCOUNT_PLATFORM_REGISTRY,
  createPickerPlatforms,
  isFieldVisible,
  isHeaderOverridePlatform,
  platformAPIKeyConfig,
  platformCategories,
  platformCategoryPicker,
  platformsShareField,
  platformSupportsType
} from './registry'

describe('account form registry', () => {
  it('lists every supported platform once', () => {
    expect(ACCOUNT_PLATFORM_REGISTRY.map((item) => item.id)).toEqual([
      'anthropic',
      'openai',
      'antigravity',
      'grok',
      'kimi',
      'cursor',
      'gemini'
    ])
  })

  it('keeps the create picker aligned with the current CreateAccountModal buttons', () => {
    expect(createPickerPlatforms().map((item) => item.id)).toEqual([
      'anthropic',
      'openai',
      'antigravity',
      'grok',
      'kimi',
      'cursor'
    ])
  })

  it('exposes platform types and header-override eligibility for all three modals', () => {
    expect(platformCategories('anthropic')).not.toContain('bedrock')
    expect(platformSupportsType('anthropic', 'bedrock')).toBe(false)
    expect(platformSupportsType('openai', 'apikey')).toBe(true)
    expect(platformSupportsType('kimi', 'apikey')).toBe(false)
    expect(isHeaderOverridePlatform('anthropic')).toBe(true)
    expect(isHeaderOverridePlatform('openai')).toBe(true)
    expect(isHeaderOverridePlatform('gemini')).toBe(false)
    expect(isFieldVisible('antigravityProjectId', { platform: 'antigravity' })).toBe(true)
    expect(isFieldVisible('antigravityProjectId', { platform: 'openai' })).toBe(false)
  })

  it('drives every platform subtype block from registry without exposing Gemini in the create picker', () => {
    expect(platformCategoryPicker('anthropic')?.options.map((item) => item.id)).toEqual([
      'oauth-based',
      'apikey'
    ])
    expect(platformCategoryPicker('openai')?.model).toBe('accountCategory')
    expect(platformCategoryPicker('grok')?.hintKey).toBe('admin.accounts.grok.accountTypeHint')
    expect(platformCategoryPicker('kimi')?.model).toBe('none')
    expect(platformCategoryPicker('antigravity')?.model).toBe('antigravityAccountType')
    expect(platformCategoryPicker('gemini')?.options.map((item) => item.id)).toEqual([
      'oauth-based',
      'apikey'
    ])
    expect(platformCategoryPicker('gemini')?.helpKey).toBe('admin.accounts.gemini.helpButton')
    expect(platformCategoryPicker('anthropic')?.options.some((item) => item.id === 'service_account')).toBe(false)
    expect(platformCategoryPicker('gemini')?.options.some((item) => item.id === 'service_account')).toBe(false)
    expect(platformCategoryPicker('anthropic')?.layoutClass).toBe('grid-cols-2')
    expect(platformCategoryPicker('gemini')?.layoutClass).toBe('grid-cols-2')
    expect(createPickerPlatforms().some((item) => item.id === 'gemini')).toBe(false)
  })

  it('requires every selected platform to share a field', () => {
    expect(platformsShareField(['anthropic', 'openai'], 'headerOverride')).toBe(true)
    expect(platformsShareField(['anthropic', 'gemini'], 'headerOverride')).toBe(false)
    expect(platformsShareField(['antigravity'], 'antigravityProjectId')).toBe(true)
    expect(platformsShareField([], 'headerOverride')).toBe(false)
  })

  it('centralizes platform blocks and API key presentation metadata', () => {
    expect(isFieldVisible('vertexServiceAccount', { platform: 'gemini' })).toBe(true)
    expect(isFieldVisible('upstreamCredentials', { platform: 'antigravity' })).toBe(true)
    expect(isFieldVisible('oauthModelRestriction', { platform: 'antigravity' })).toBe(false)
    expect(platformsShareField(['openai'], 'openAIAdvanced')).toBe(true)
    expect(platformsShareField(['openai', 'grok'], 'openAIAdvanced')).toBe(false)
    expect(platformAPIKeyConfig('grok')).toMatchObject({
      baseUrl: 'https://api.x.ai/v1',
      placeholder: 'xai-...'
    })
  })
})
