import type { AccountPlatform, AccountType } from '@/types'

export type AccountFormMode = 'create' | 'edit' | 'bulk'
export type AccountCategory =
  | 'oauth-based'
  | 'apikey'
  | 'service_account'
  | 'upstream'
  | 'oauth'

export interface FieldContext {
  platform: AccountPlatform | string
  type?: AccountType | string
  category?: AccountCategory | string
  mode: AccountFormMode
}

export interface PlatformDefinition {
  id: AccountPlatform
  label: string
  icon: 'sparkles' | 'bolt' | 'cloud' | 'grok' | 'kimi' | 'cursor' | 'devin'
  activeClass: string
  showInCreatePicker: boolean
  categories: AccountCategory[]
  types: AccountType[]
  fields: {
    headerOverride: boolean
    modelRestriction: boolean
    antigravityProjectId: boolean
    upstreamCredentials: boolean
    vertexServiceAccount: boolean
    mixedScheduling: boolean
    oauthModelRestriction: boolean
    interceptWarmup: boolean
    openAIAdvanced: boolean
    anthropicAdvanced: boolean
    antigravityAdvanced: boolean
    grokAdvanced: boolean
    kimiAdvanced: boolean
    cursorAdvanced?: boolean
    geminiAdvanced: boolean
  }
  apiKey: {
    baseUrl: string
    placeholder: string
    baseUrlHintKey: string
    apiKeyHintKey: string
  }
}

export const ACCOUNT_PLATFORM_REGISTRY: readonly PlatformDefinition[] = [
  {
    id: 'anthropic',
    label: 'Anthropic',
    icon: 'sparkles',
    activeClass: 'bg-white text-orange-600  dark:bg-dark-600 dark:text-orange-400',
    showInCreatePicker: true,
    categories: ['oauth-based', 'apikey', 'service_account'],
    types: ['oauth', 'setup-token', 'apikey', 'service_account'],
    fields: {
      headerOverride: true,
      modelRestriction: true,
      antigravityProjectId: false,
      upstreamCredentials: false,
      vertexServiceAccount: true,
      mixedScheduling: false,
      oauthModelRestriction: true,
      interceptWarmup: true,
      openAIAdvanced: false,
      anthropicAdvanced: true,
      antigravityAdvanced: false,
      grokAdvanced: false,
      kimiAdvanced: false,
      geminiAdvanced: false
    },
    apiKey: {
      baseUrl: 'https://api.anthropic.com',
      placeholder: 'sk-ant-...',
      baseUrlHintKey: 'admin.accounts.baseUrlHint',
      apiKeyHintKey: 'admin.accounts.apiKeyHint'
    }
  },
  {
    id: 'openai',
    label: 'OpenAI',
    icon: 'bolt',
    activeClass: 'bg-white text-green-600  dark:bg-dark-600 dark:text-green-400',
    showInCreatePicker: true,
    categories: ['oauth-based', 'apikey'],
    types: ['oauth', 'setup-token', 'apikey'],
    fields: {
      headerOverride: true,
      modelRestriction: true,
      antigravityProjectId: false,
      upstreamCredentials: false,
      vertexServiceAccount: false,
      mixedScheduling: false,
      oauthModelRestriction: true,
      interceptWarmup: false,
      openAIAdvanced: true,
      anthropicAdvanced: false,
      antigravityAdvanced: false,
      grokAdvanced: false,
      kimiAdvanced: false,
      geminiAdvanced: false
    },
    apiKey: {
      baseUrl: 'https://api.openai.com',
      placeholder: 'sk-proj-...',
      baseUrlHintKey: 'admin.accounts.openai.baseUrlHint',
      apiKeyHintKey: 'admin.accounts.openai.apiKeyHint'
    }
  },
  {
    id: 'antigravity',
    label: 'Antigravity',
    icon: 'cloud',
    activeClass: 'bg-white text-purple-600  dark:bg-dark-600 dark:text-purple-400',
    showInCreatePicker: true,
    categories: ['oauth', 'upstream'],
    types: ['oauth', 'upstream'],
    fields: {
      headerOverride: false,
      modelRestriction: true,
      antigravityProjectId: true,
      upstreamCredentials: true,
      vertexServiceAccount: false,
      mixedScheduling: true,
      oauthModelRestriction: false,
      interceptWarmup: true,
      openAIAdvanced: false,
      anthropicAdvanced: false,
      antigravityAdvanced: true,
      grokAdvanced: false,
      kimiAdvanced: false,
      geminiAdvanced: false
    },
    apiKey: {
      baseUrl: 'https://cloudcode-pa.googleapis.com',
      placeholder: 'sk-...',
      baseUrlHintKey: 'admin.accounts.baseUrlHint',
      apiKeyHintKey: 'admin.accounts.apiKeyHint'
    }
  },
  {
    id: 'grok',
    label: 'Grok',
    icon: 'grok',
    activeClass: 'bg-white text-zinc-900  dark:bg-dark-600 dark:text-zinc-100',
    showInCreatePicker: true,
    categories: ['oauth-based', 'apikey'],
    types: ['oauth', 'setup-token', 'apikey'],
    fields: {
      headerOverride: false,
      modelRestriction: true,
      antigravityProjectId: false,
      upstreamCredentials: false,
      vertexServiceAccount: false,
      mixedScheduling: false,
      oauthModelRestriction: true,
      interceptWarmup: false,
      openAIAdvanced: false,
      anthropicAdvanced: false,
      antigravityAdvanced: false,
      grokAdvanced: true,
      kimiAdvanced: false,
      geminiAdvanced: false
    },
    apiKey: {
      baseUrl: 'https://api.x.ai/v1',
      placeholder: 'xai-...',
      baseUrlHintKey: 'admin.accounts.grok.baseUrlHint',
      apiKeyHintKey: 'admin.accounts.grok.apiKeyHint'
    }
  },
  {
    id: 'kimi',
    label: 'Kimi',
    icon: 'kimi',
    activeClass: 'bg-white text-amber-700 dark:bg-dark-600 dark:text-amber-300',
    showInCreatePicker: true,
    categories: ['oauth-based'],
    types: ['oauth'],
    fields: {
      headerOverride: false,
      modelRestriction: true,
      antigravityProjectId: false,
      upstreamCredentials: false,
      vertexServiceAccount: false,
      mixedScheduling: false,
      oauthModelRestriction: true,
      interceptWarmup: false,
      openAIAdvanced: false,
      anthropicAdvanced: false,
      antigravityAdvanced: false,
      grokAdvanced: false,
      kimiAdvanced: true,
      geminiAdvanced: false
    },
    apiKey: {
      baseUrl: '',
      placeholder: '',
      baseUrlHintKey: 'admin.accounts.baseUrlHint',
      apiKeyHintKey: 'admin.accounts.apiKeyHint'
    }
  },
  {
    id: 'cursor',
    label: 'Cursor',
    icon: 'cursor',
    activeClass: 'bg-white text-cyan-700 dark:bg-dark-600 dark:text-cyan-300',
    showInCreatePicker: true,
    categories: ['apikey'],
    types: ['apikey'],
    fields: {
      headerOverride: false,
      modelRestriction: true,
      antigravityProjectId: false,
      upstreamCredentials: false,
      vertexServiceAccount: false,
      mixedScheduling: false,
      oauthModelRestriction: false,
      interceptWarmup: false,
      openAIAdvanced: false,
      anthropicAdvanced: false,
      antigravityAdvanced: false,
      grokAdvanced: false,
      kimiAdvanced: false,
      cursorAdvanced: false,
      geminiAdvanced: false
    },
    apiKey: {
      baseUrl: 'http://sub2api-cursor-bridge:3000',
      placeholder: 'Cursor access token...',
      baseUrlHintKey: 'admin.accounts.cursor.baseUrlHint',
      apiKeyHintKey: 'admin.accounts.cursor.apiKeyHint'
    }
  },
  {
    id: 'devin',
    label: 'Devin',
    icon: 'devin',
    activeClass: 'bg-white text-indigo-700 dark:bg-dark-600 dark:text-indigo-300',
    showInCreatePicker: true,
    categories: ['apikey'],
    types: ['apikey'],
    fields: {
      headerOverride: false,
      modelRestriction: true,
      antigravityProjectId: false,
      upstreamCredentials: false,
      vertexServiceAccount: false,
      mixedScheduling: false,
      oauthModelRestriction: true,
      interceptWarmup: false,
      openAIAdvanced: false,
      anthropicAdvanced: false,
      antigravityAdvanced: false,
      grokAdvanced: false,
      kimiAdvanced: false,
      cursorAdvanced: false,
      geminiAdvanced: false
    },
    apiKey: {
      baseUrl: 'http://sub2api-devin-bridge:8080',
      placeholder: 'Devin bridge API key...',
      baseUrlHintKey: 'admin.accounts.devin.baseUrlHint',
      apiKeyHintKey: 'admin.accounts.devin.apiKeyHint'
    }
  },
  {
    id: 'gemini',
    label: 'Gemini',
    icon: 'sparkles',
    activeClass: 'bg-white text-blue-600 dark:bg-dark-600 dark:text-blue-400',
    showInCreatePicker: false,
    categories: ['oauth-based', 'apikey', 'service_account'],
    types: ['oauth', 'setup-token', 'apikey', 'service_account'],
    fields: {
      headerOverride: false,
      modelRestriction: true,
      antigravityProjectId: false,
      upstreamCredentials: false,
      vertexServiceAccount: true,
      mixedScheduling: false,
      oauthModelRestriction: true,
      interceptWarmup: false,
      openAIAdvanced: false,
      anthropicAdvanced: false,
      antigravityAdvanced: false,
      grokAdvanced: false,
      kimiAdvanced: false,
      geminiAdvanced: true
    },
    apiKey: {
      baseUrl: 'https://generativelanguage.googleapis.com',
      placeholder: 'AIza...',
      baseUrlHintKey: 'admin.accounts.gemini.baseUrlHint',
      apiKeyHintKey: 'admin.accounts.gemini.apiKeyHint'
    }
  }
] as const

const byId = new Map(ACCOUNT_PLATFORM_REGISTRY.map((item) => [item.id, item]))

export function getPlatformDefinition(platform: string): PlatformDefinition | undefined {
  return byId.get(platform as AccountPlatform)
}

export function createPickerPlatforms(): PlatformDefinition[] {
  return ACCOUNT_PLATFORM_REGISTRY.filter((item) => item.showInCreatePicker)
}

export function platformCategories(platform: string): AccountCategory[] {
  return getPlatformDefinition(platform)?.categories ?? []
}

export function platformSupportsType(platform: string, type: string): boolean {
  return getPlatformDefinition(platform)?.types.includes(type as AccountType) ?? false
}

export function platformAPIKeyConfig(platform: string): PlatformDefinition['apiKey'] {
  return getPlatformDefinition(platform)?.apiKey ?? {
    baseUrl: 'https://api.anthropic.com',
    placeholder: 'sk-ant-...',
    baseUrlHintKey: 'admin.accounts.baseUrlHint',
    apiKeyHintKey: 'admin.accounts.apiKeyHint'
  }
}

export function isHeaderOverridePlatform(platform: string): boolean {
  return getPlatformDefinition(platform)?.fields.headerOverride === true
}

export function isFieldVisible(
  field: keyof PlatformDefinition['fields'],
  ctx: Pick<FieldContext, 'platform'>
): boolean {
  return getPlatformDefinition(ctx.platform)?.fields[field] === true
}

export type CategoryPickerIcon = 'sparkles' | 'key' | 'cloud' | 'platform'
export type CategoryPickerModel = 'accountCategory' | 'antigravityAccountType' | 'none'

export interface CategoryPickerOption {
  id: string
  titleKey?: string
  title?: string
  subtitleKey?: string
  subtitle?: string
  icon: CategoryPickerIcon
  selectedBorder: string
  selectedBg: string
  selectedIconBg: string
  hoverBorder: string
  noteKey?: string
  noteClass?: string
}

export interface CategoryPickerConfig {
  model: CategoryPickerModel
  layoutClass: string
  hintKey?: string
  helpKey?: string
  options: CategoryPickerOption[]
}

const orange = {
  selectedBorder: 'border-orange-500 bg-orange-50 dark:bg-orange-900/20',
  selectedBg: 'bg-orange-500 text-white',
  selectedIconBg: 'bg-orange-500 text-white',
  hoverBorder: 'border-gray-200 hover:border-orange-300 dark:border-dark-600 dark:hover:border-orange-700'
}
const purple = {
  selectedBorder: 'border-purple-500 bg-purple-50 dark:bg-purple-900/20',
  selectedBg: 'bg-purple-500 text-white',
  selectedIconBg: 'bg-purple-500 text-white',
  hoverBorder: 'border-gray-200 hover:border-purple-300 dark:border-dark-600 dark:hover:border-purple-700'
}
const green = {
  selectedBorder: 'border-green-500 bg-green-50 dark:bg-green-900/20',
  selectedBg: 'bg-green-500 text-white',
  selectedIconBg: 'bg-green-500 text-white',
  hoverBorder: 'border-gray-200 hover:border-green-300 dark:border-dark-600 dark:hover:border-green-700'
}
const zinc = {
  selectedBorder: 'border-zinc-800 bg-zinc-50 dark:bg-zinc-900/30',
  selectedBg: 'bg-zinc-900 text-white',
  selectedIconBg: 'bg-zinc-900 text-white',
  hoverBorder: 'border-gray-200 hover:border-zinc-400 dark:border-dark-600 dark:hover:border-zinc-600'
}
const cyan = {
  selectedBorder: 'border-cyan-500 bg-cyan-50 dark:bg-cyan-900/20',
  selectedBg: 'bg-cyan-500 text-white',
  selectedIconBg: 'bg-cyan-500 text-white',
  hoverBorder: 'border-gray-200 hover:border-cyan-300 dark:border-dark-600 dark:hover:border-cyan-700'
}

export const ACCOUNT_CATEGORY_PICKERS: Record<string, CategoryPickerConfig> = {
  cursor: {
    model: 'accountCategory',
    layoutClass: 'grid-cols-1 sm:grid-cols-2',
    hintKey: 'admin.accounts.cursor.accountTypeHint',
    options: [
      {
        id: 'oauth-based',
        title: 'OAuth',
        subtitleKey: 'admin.accounts.types.cursorOauth',
        icon: 'platform',
        ...cyan
      },
      {
        id: 'apikey',
        title: 'API Key',
        subtitleKey: 'admin.accounts.types.cursorApikey',
        icon: 'key',
        ...purple
      }
    ]
  },
  anthropic: {
    model: 'accountCategory',
    layoutClass: 'grid-cols-2',
    options: [
      {
        id: 'oauth-based',
        titleKey: 'admin.accounts.claudeCode',
        subtitleKey: 'admin.accounts.oauthSetupToken',
        icon: 'sparkles',
        ...orange
      },
      {
        id: 'apikey',
        titleKey: 'admin.accounts.claudeConsole',
        subtitleKey: 'admin.accounts.apiKey',
        icon: 'key',
        ...purple
      }
    ]
  },
  openai: {
    model: 'accountCategory',
    layoutClass: 'grid-cols-2',
    options: [
      {
        id: 'oauth-based',
        title: 'OAuth',
        subtitleKey: 'admin.accounts.types.chatgptOauth',
        icon: 'key',
        ...green
      },
      {
        id: 'apikey',
        title: 'API Key',
        subtitleKey: 'admin.accounts.types.responsesApi',
        icon: 'key',
        ...purple
      }
    ]
  },
  grok: {
    model: 'accountCategory',
    layoutClass: 'grid-cols-1 sm:grid-cols-2',
    hintKey: 'admin.accounts.grok.accountTypeHint',
    options: [
      {
        id: 'oauth-based',
        title: 'OAuth',
        subtitleKey: 'admin.accounts.types.grokOauth',
        icon: 'platform',
        ...zinc
      },
      {
        id: 'apikey',
        title: 'API Key',
        subtitleKey: 'admin.accounts.types.responsesApi',
        icon: 'key',
        ...purple
      }
    ]
  },
  kimi: {
    model: 'none',
    layoutClass: '',
    options: [
      {
        id: 'oauth-based',
        title: 'OAuth Device Code',
        subtitle: 'Kimi Coding Subscription',
        icon: 'platform',
        selectedBorder: 'border-amber-400 bg-amber-50 dark:bg-amber-950/20',
        selectedBg: 'bg-amber-500 text-white',
        selectedIconBg: 'bg-amber-500 text-white',
        hoverBorder: 'border-amber-400'
      }
    ]
  },
  antigravity: {
    model: 'antigravityAccountType',
    layoutClass: 'grid-cols-2',
    options: [
      {
        id: 'oauth',
        title: 'OAuth',
        subtitleKey: 'admin.accounts.types.antigravityOauth',
        icon: 'key',
        ...purple
      },
      {
        id: 'upstream',
        title: 'API Key',
        subtitleKey: 'admin.accounts.types.antigravityApikey',
        icon: 'cloud',
        ...purple
      }
    ]
  },
  gemini: {
    model: 'accountCategory',
    layoutClass: 'grid-cols-2',
    helpKey: 'admin.accounts.gemini.helpButton',
    options: [
      {
        id: 'oauth-based',
        titleKey: 'admin.accounts.gemini.accountType.oauthTitle',
        subtitleKey: 'admin.accounts.gemini.accountType.oauthDesc',
        icon: 'key',
        selectedBorder: 'border-blue-500 bg-blue-50 dark:bg-blue-900/20',
        selectedBg: 'bg-blue-500 text-white',
        selectedIconBg: 'bg-blue-500 text-white',
        hoverBorder: 'border-gray-200 hover:border-blue-300 dark:border-dark-600 dark:hover:border-blue-700'
      },
      {
        id: 'apikey',
        titleKey: 'admin.accounts.gemini.accountType.apiKeyTitle',
        subtitleKey: 'admin.accounts.gemini.accountType.apiKeyDesc',
        icon: 'key',
        ...purple
      }
    ]
  }
}

export function platformCategoryPicker(platform: string): CategoryPickerConfig | undefined {
  return ACCOUNT_CATEGORY_PICKERS[platform]
}

export function platformsShareField(
  platforms: string[],
  field: keyof PlatformDefinition['fields']
): boolean {
  return platforms.length > 0 && platforms.every((platform) => isFieldVisible(field, { platform }))
}
