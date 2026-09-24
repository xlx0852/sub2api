import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'

const {
  updateAccountMock,
  checkMixedChannelRiskMock,
  getAccountMock,
  applyMirasimMappingMock,
  showErrorMock,
  showSuccessMock,
  showInfoMock
} = vi.hoisted(() => ({
  updateAccountMock: vi.fn(),
  getAccountMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn(),
  applyMirasimMappingMock: vi.fn(),
  showErrorMock: vi.fn(),
  showSuccessMock: vi.fn(),
  showInfoMock: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: showErrorMock,
    showSuccess: showSuccessMock,
    showInfo: showInfoMock
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      update: updateAccountMock,
      getById: getAccountMock,
      checkMixedChannelRisk: checkMixedChannelRiskMock,
      applyMirasimModelMapping: applyMirasimMappingMock
    },
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }),
      getSettings: vi.fn().mockResolvedValue({})
    },
    tlsFingerprintProfiles: {
      list: vi.fn().mockResolvedValue([])
    }
  }
}))

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

import EditAccountModal from '../EditAccountModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: {
      type: Boolean,
      default: false
    }
  },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const ModelWhitelistSelectorStub = defineComponent({
  name: 'ModelWhitelistSelector',
  props: {
    modelValue: {
      type: Array,
      default: () => []
    }
  },
  emits: ['update:modelValue'],
  template: `
    <div>
      <button
        type="button"
        data-testid="rewrite-to-snapshot"
        @click="$emit('update:modelValue', ['gpt-5.2-2025-12-11'])"
      >
        rewrite
      </button>
      <span data-testid="model-whitelist-value">
        {{ Array.isArray(modelValue) ? modelValue.join(',') : '' }}
      </span>
    </div>
  `
})

const SelectStub = defineComponent({
  name: 'SelectStub',
  props: {
    modelValue: {
      type: [String, Number, Boolean, null],
      default: ''
    },
    options: {
      type: Array,
      default: () => []
    }
  },
  emits: ['update:modelValue'],
  template: `
    <select
      v-bind="$attrs"
      :value="modelValue"
      @change="$emit('update:modelValue', $event.target.value)"
    >
      <option v-for="option in options" :key="option.value" :value="option.value">
        {{ option.label }}
      </option>
    </select>
  `
})

const GroupSelectorStub = defineComponent({
  name: 'GroupSelector',
  props: {
    modelValue: {
      type: Array,
      default: () => []
    }
  },
  emits: ['update:modelValue'],
  template: `
    <div data-testid="group-selector">
      <button
        type="button"
        data-testid="set-shadow-group"
        @click="$emit('update:modelValue', [7])"
      >
        group
      </button>
    </div>
  `
})

const AccountCostConfigPanelStub = defineComponent({
  name: 'AccountCostConfigPanel',
  emits: ['saved'],
  template: '<button type="button" data-testid="inner-cost-save" @click="$emit(\'saved\', { account_expires_at: \'2026-10-01T00:00:00Z\' })">save cost</button>'
})

function buildAccount() {
  return {
    id: 1,
    name: 'OpenAI Key',
    notes: '',
    platform: 'openai',
    type: 'apikey',
    credentials: {
      api_key: 'sk-test',
      base_url: 'https://api.openai.com',
      model_mapping: {
        'gpt-5.2': 'gpt-5.2'
      }
    },
    extra: {},
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    rate_multiplier: 1,
    status: 'active',
    group_ids: [],
    expires_at: null,
    auto_pause_on_expired: false
  } as any
}

function buildOpenAISparkShadowAccount() {
  const account = buildAccount()
  return {
    ...account,
    id: 4,
    name: 'OpenAI Spark Shadow',
    type: 'oauth',
    parent_account_id: 1,
    credentials: {
      access_token: 'parent-access-token',
      refresh_token: 'parent-refresh-token',
      api_key: 'sk-parent',
      base_url: 'https://api.openai.com',
      model_mapping: {
        'gpt-5.3-codex-spark': 'gpt-5.3-codex-spark'
      },
      compact_model_mapping: {
        'gpt-5.3-codex-spark': 'gpt-5.3-codex-spark-compact'
      }
    },
    group_ids: []
  } as any
}


function buildAntigravityAccount(projectId = 'configured-project') {
  return {
    id: 3,
    name: 'Antigravity OAuth',
    notes: '',
    platform: 'antigravity',
    type: 'oauth',
    credentials: {
      antigravity_project_id: projectId,
      model_mapping: {
        'gemini-2.5-flash': 'gemini-2.5-flash'
      }
    },
    extra: {},
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    rate_multiplier: 1,
    status: 'active',
    group_ids: [],
    expires_at: null,
    auto_pause_on_expired: false
  } as any
}

function buildGrokOAuthAccount() {
  return {
    id: 5,
    name: 'Grok OAuth',
    notes: '',
    platform: 'grok',
    type: 'oauth',
    credentials: {
      refresh_token: 'grok-rt',
      base_url: 'https://api.x.ai/v1',
      model_mapping: {
        'grok-latest': 'grok-4.5'
      }
    },
    extra: {},
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    rate_multiplier: 1,
    status: 'active',
    group_ids: [],
    expires_at: null,
    auto_pause_on_expired: false
  } as any
}

function buildOpenAISetupTokenAccount() {
  return {
    ...buildAccount(),
    type: 'setup-token',
    extra: {
      openai_oauth_responses_websockets_v2_mode: 'ctx_pool',
      openai_oauth_responses_websockets_v2_enabled: true
    }
  } as any
}

function mountModal(account = buildAccount()) {
  getAccountMock.mockResolvedValue(account)
  return mount(EditAccountModal, {
    props: {
      show: true,
      account,
      proxies: [],
      groups: []
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Select: SelectStub,
        Icon: true,
        ProxySelector: true,
        GroupSelector: GroupSelectorStub,
        ModelWhitelistSelector: ModelWhitelistSelectorStub,
        AccountCostConfigPanel: AccountCostConfigPanelStub
      }
    }
  })
}

describe('EditAccountModal', () => {
  it('keeps the cost tab open and does not emit account updated after the inner save', async () => {
    const wrapper = mountModal(buildGrokOAuthAccount())

    await wrapper.get('[data-testid="edit-account-tab-cost"]').trigger('click')
    await wrapper.get('[data-testid="inner-cost-save"]').trigger('click')

    expect(wrapper.get('[data-testid="edit-account-cost-section"]').isVisible()).toBe(true)
    expect(wrapper.emitted('updated')).toBeUndefined()
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('keeps comprehensive cost multiplier operator-controlled and removes legacy auto-sync', async () => {
    const account = buildAccount()
    account.rate_multiplier = 7.1
    account.extra = {
      upstream_billing_probe_enabled: true,
      upstream_billing_rate_sync_enabled: true
    }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)
    await wrapper.get('[data-testid="edit-account-tab-cost"]').trigger('click')
    const input = wrapper.get<HTMLInputElement>('[data-testid="edit-account-rate-multiplier"]')
    expect(input.attributes('disabled')).toBeUndefined()
    expect(input.element.value).toBe('7.1')
    const preview = wrapper.get('[data-testid="account-cost-preview"]')
    expect(preview.text()).toContain('1.0000')
    expect(preview.text()).toContain('7.1000')
    expect(preview.text()).toContain('-6.1000')
    expect(preview.text()).not.toContain('$')
    expect(preview.text()).not.toContain('¥')

    await input.setValue('7.25')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.rate_multiplier).toBe(7.25)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).not.toHaveProperty('upstream_billing_rate_sync_enabled')
  })

  it('reopening the same account rehydrates the OpenAI whitelist from props', async () => {
    const account = buildAccount()
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    expect(wrapper.get('[data-testid="model-whitelist-value"]').text()).toBe('gpt-5.2')

    await wrapper.get('[data-testid="rewrite-to-snapshot"]').trigger('click')
    expect(wrapper.get('[data-testid="model-whitelist-value"]').text()).toBe('gpt-5.2-2025-12-11')

    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })

    expect(wrapper.get('[data-testid="model-whitelist-value"]').text()).toBe('gpt-5.2')

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.model_mapping).toEqual({
      'gpt-5.2': 'gpt-5.2'
    })
  })

  it('preserves model mappings when editing the whitelist', async () => {
    const account = buildAccount()
    account.credentials.model_mapping = {
      'gpt-5.2': 'gpt-5.2',
      'gpt-latest': 'gpt-5.2'
    }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    expect(wrapper.get('[data-testid="model-whitelist-value"]').text()).toBe('gpt-5.2')

    await wrapper.get('[data-testid="rewrite-to-snapshot"]').trigger('click')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.model_mapping).toEqual({
      'gpt-5.2-2025-12-11': 'gpt-5.2-2025-12-11',
      'gpt-latest': 'gpt-5.2'
    })
  })

  it('submits OpenAI compact mode and compact-only model mapping', async () => {
    const account = buildAccount()
    account.extra = {
      openai_compact_mode: 'force_on'
    }
    account.credentials = {
      ...account.credentials,
      compact_model_mapping: {
        'gpt-5.4': 'gpt-5.4-openai-compact'
      }
    }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra?.openai_compact_mode).toBe('force_on')
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.compact_model_mapping).toEqual({
      'gpt-5.4': 'gpt-5.4-openai-compact'
    })
  })

  it('loads and submits Grok OAuth model mapping edits', async () => {
    const account = buildGrokOAuthAccount()
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)
    expect(wrapper.text()).toContain('Imagine Image')
    expect(wrapper.text()).toContain('Imagine Video')

    const inputWithValue = (value: string) => {
      const input = wrapper
        .findAll('input')
        .find((input) => (input.element as HTMLInputElement).value === value)
      expect(input).toBeTruthy()
      return input!
    }

    await inputWithValue('grok-latest').setValue('grok')
    await inputWithValue('grok-4.5').setValue('grok-build-0.1')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.model_mapping).toEqual({
      grok: 'grok-build-0.1'
    })
  })

  it('only submits model mapping credentials when saving an OpenAI spark shadow account', async () => {
    const account = buildOpenAISparkShadowAccount()
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('[data-testid="set-shadow-group"]').trigger('click')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload?.group_ids).toEqual([7])
    expect(payload?.credentials).toEqual({
      model_mapping: {
        'gpt-5.3-codex-spark': 'gpt-5.3-codex-spark'
      },
      compact_model_mapping: {
        'gpt-5.3-codex-spark': 'gpt-5.3-codex-spark-compact'
      }
    })
  })

  it('submits OpenAI APIKey Responses support override mode', async () => {
    const account = buildAccount()
    account.extra = {
      openai_responses_mode: 'force_chat_completions',
      openai_responses_supported: false
    }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('[data-testid="openai-responses-mode-select"]').setValue('force_responses')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra?.openai_responses_mode).toBe('force_responses')
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra?.openai_responses_supported).toBe(false)
  })

  it('clears OpenAI APIKey Responses override when set back to auto', async () => {
    const account = buildAccount()
    account.extra = {
      openai_responses_mode: 'force_chat_completions',
      openai_responses_supported: true
    }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('[data-testid="openai-responses-mode-select"]').setValue('auto')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).not.toHaveProperty('openai_responses_mode')
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra?.openai_responses_supported).toBe(true)
  })

  it('submits OpenAI APIKey endpoint capabilities from credentials', async () => {
    const account = buildAccount()
    account.credentials.openai_capabilities = ['chat_completions']
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    expect(wrapper.findAll('input[type="checkbox"]').some((input) => (input.element as HTMLInputElement).checked)).toBe(true)

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.openai_capabilities).toEqual([
      'chat_completions'
    ])
  })

  it('keeps at least one OpenAI APIKey endpoint capability selected', async () => {
    const account = buildAccount()
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    const chatCheckbox = wrapper.get<HTMLInputElement>(
      '[data-testid="openai-endpoint-capability-chat_completions"]'
    )
    const embeddingsCheckbox = wrapper.get<HTMLInputElement>(
      '[data-testid="openai-endpoint-capability-embeddings"]'
    )

    expect(chatCheckbox.element.checked).toBe(true)
    expect(embeddingsCheckbox.element.checked).toBe(true)

    await embeddingsCheckbox.setValue(false)

    expect(chatCheckbox.element.checked).toBe(true)
    expect(embeddingsCheckbox.element.checked).toBe(false)

    await chatCheckbox.setValue(false)

    expect(chatCheckbox.element.checked).toBe(true)
    expect(embeddingsCheckbox.element.checked).toBe(false)

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.openai_capabilities).toEqual([
      'chat_completions'
    ])
  })

  it('disables text generation protocol when only embeddings requests are accepted', async () => {
    const account = buildAccount()
    account.credentials.openai_capabilities = ['embeddings']
    account.extra = {
      openai_responses_mode: 'force_responses',
      openai_responses_supported: true
    }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    const responsesModeSelect = wrapper.get<HTMLSelectElement>(
      '[data-testid="openai-responses-mode-select"]'
    )

    expect(responsesModeSelect.element.disabled).toBe(true)
    expect(wrapper.find('[data-testid="openai-responses-mode-not-applicable"]').exists()).toBe(true)

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.openai_capabilities).toEqual([
      'embeddings'
    ])
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).not.toHaveProperty('openai_responses_mode')
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra?.openai_responses_supported).toBe(true)
  })

  it('submits Codex image tool force-inject mode as bridge override', async () => {
    const account = buildAccount()
    account.extra = {
      codex_image_generation_bridge: false,
      codex_image_generation_bridge_enabled: true
    }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('button[data-testid="codex-image-tool-enabled"]').trigger('click')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra?.codex_image_generation_bridge).toBe(true)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).not.toHaveProperty('codex_image_generation_bridge_enabled')
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).not.toHaveProperty('codex_image_generation_explicit_tool_policy')
  })

  it('submits Codex image tool no-injection mode without strip policy', async () => {
    const account = buildAccount()
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('button[data-testid="codex-image-tool-disabled"]').trigger('click')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra?.codex_image_generation_bridge).toBe(false)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).not.toHaveProperty('codex_image_generation_explicit_tool_policy')
  })

  it('submits Codex image tool block mode as strip policy and clears bridge override', async () => {
    const account = buildAccount()
    account.extra = {
      codex_image_generation_bridge: true
    }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('button[data-testid="codex-image-tool-block"]').trigger('click')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra?.codex_image_generation_explicit_tool_policy).toBe('strip')
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).not.toHaveProperty('codex_image_generation_bridge')
  })

  it('loads strip policy as block mode and clears both keys when reset to inherit', async () => {
    const account = buildAccount()
    account.extra = {
      codex_image_generation_explicit_tool_policy: 'strip'
    }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('button[data-testid="codex-image-tool-inherit"]').trigger('click')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).not.toHaveProperty('codex_image_generation_explicit_tool_policy')
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).not.toHaveProperty('codex_image_generation_bridge')
  })

  it('setup-token account can select and submit OAuth WS mode', async () => {
    const account = buildOpenAISetupTokenAccount()
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('[data-testid="edit-openai-ws-mode-select"]').setValue('http_bridge')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra?.openai_oauth_responses_websockets_v2_mode).toBe('http_bridge')
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra?.openai_oauth_responses_websockets_v2_enabled).toBe(true)
  })

  it('allows saving apikey account when backend redacted api_key but credentials_status reports it exists', async () => {
    // 新前端 + 新后端：响应已脱敏，credentials 里没有 api_key，credentials_status.has_api_key=true
    const account = buildAccount()
    account.credentials = {
      base_url: 'https://api.openai.com',
      model_mapping: { 'gpt-5.2': 'gpt-5.2' }
    }
    account.credentials_status = { has_api_key: true }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    // 用户未输入新 key 时，payload 不应带 api_key，由后端合并保留旧值
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials).not.toHaveProperty('api_key')
  })

  it('allows saving apikey account against legacy backend without credentials_status', async () => {
    // 新前端 + 旧后端：credentials_status 缺失，但 credentials.api_key 仍是明文，应允许保存
    const account = buildAccount()
    // 显式确保没有 credentials_status
    expect(account.credentials_status).toBeUndefined()
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    // 旧后端响应未脱敏，原 api_key 会随 currentCredentials 一起传回去（旧行为，等价于无操作）
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.api_key).toBe('sk-test')
  })

  it('blocks apikey save when neither credentials_status nor legacy api_key indicates existence', async () => {
    const account = buildAccount()
    account.credentials = {
      base_url: 'https://api.openai.com'
    }
    // 既没有 credentials_status 也没有旧的 api_key
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })

    const wrapper = mountModal(account)

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).not.toHaveBeenCalled()
  })

  it('loads and submits Antigravity configured project fallback', async () => {
    const account = buildAntigravityAccount('configured-project')
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)
    const input = wrapper.get<HTMLInputElement>('[data-testid="antigravity-project-id-input"]')
    expect(input.element.value).toBe('configured-project')

    await input.setValue('  updated-project  ')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.antigravity_project_id).toBe(
      'updated-project'
    )
  })

  it('clears Antigravity configured project fallback when input is empty', async () => {
    const account = buildAntigravityAccount('configured-project')
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)
    const input = wrapper.get<HTMLInputElement>('[data-testid="antigravity-project-id-input"]')

    await input.setValue('')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials).not.toHaveProperty(
      'antigravity_project_id'
    )
  })
})

  it('persists the external bridge switch and kind into account extra', async () => {
    const account = buildAccount()
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)
    await wrapper.get('[data-testid="edit-account-tab-advanced"]').trigger('click')

    expect(wrapper.find('[data-testid="external-bridge-kind"]').exists()).toBe(false)
    await wrapper.get('[data-testid="external-bridge-toggle"]').trigger('click')
    await wrapper.get<HTMLInputElement>('[data-testid="external-bridge-kind"]').setValue(' Devin ')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    const extra = updateAccountMock.mock.calls[0]?.[1]?.extra as Record<string, unknown>
    expect(extra.external_bridge).toBe(true)
    expect(extra.external_bridge_kind).toBe('devin')
  })

  it('clears the external bridge keys when the switch is turned off', async () => {
    const account = buildAccount()
    account.extra = { external_bridge: true, external_bridge_kind: 'devin', keep_me: 1 }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)
    await wrapper.get('[data-testid="edit-account-tab-advanced"]').trigger('click')
    expect(wrapper.get<HTMLInputElement>('[data-testid="external-bridge-kind"]').element.value).toBe('devin')

    await wrapper.get('[data-testid="external-bridge-toggle"]').trigger('click')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    const extra = updateAccountMock.mock.calls[0]?.[1]?.extra as Record<string, unknown>
    expect(extra.external_bridge).toBeUndefined()
    expect(extra.external_bridge_kind).toBeUndefined()
    expect(extra.keep_me).toBe(1)
  })

describe('external bridge review regressions', () => {
  it.each(['oauth', 'setup-token', 'upstream'])('does not expose the switch on %s accounts', (type) => {
    const account = buildAccount()
    account.type = type
    const wrapper = mountModal(account)
    expect(wrapper.find('[data-testid="external-bridge-toggle"]').exists()).toBe(false)
  })

  it('rejects blank bridge kinds before calling the update API', async () => {
    updateAccountMock.mockReset()
    const wrapper = mountModal()
    await wrapper.get('[data-testid="edit-account-tab-advanced"]').trigger('click')
    await wrapper.get('[data-testid="external-bridge-toggle"]').trigger('click')
    await wrapper.get('[data-testid="external-bridge-kind"]').setValue('  ')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()
    expect(updateAccountMock).not.toHaveBeenCalled()
  })

  it('keeps unknown and concurrently added mappings when saving an unrelated field', async () => {
    const account = buildAccount()
    account.credentials.model_mapping = { 'future-model': 'devin/future', 'gpt-5.2': 'gpt-5.2' }
    updateAccountMock.mockReset()
    updateAccountMock.mockResolvedValue(account)
    const wrapper = mountModal(account)
    getAccountMock.mockResolvedValue({ ...account, credentials: { ...account.credentials,
      model_mapping: { ...account.credentials.model_mapping, 'claude-new': 'claude-new' } } })
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()
    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials.model_mapping).toEqual({
      'future-model': 'devin/future', 'gpt-5.2': 'gpt-5.2', 'claude-new': 'claude-new'
    })
  })

  it('does not overwrite a concurrently changed mapping edited in this drawer', async () => {
    updateAccountMock.mockReset()
    const account = buildAccount()
    const wrapper = mountModal(account)
    await wrapper.get('[data-testid="rewrite-to-snapshot"]').trigger('click')
    getAccountMock.mockResolvedValue({ ...account, credentials: { ...account.credentials,
      model_mapping: { 'gpt-5.2': 'new-upstream' } } })
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()
    expect(updateAccountMock).not.toHaveBeenCalled()
  })
})

describe('EditAccountModal Mirasim relay wiring', () => {
  function buildMirasimAccount(overrides: Record<string, unknown> = {}) {
    return {
      id: 9,
      name: 'Mirasim Relay',
      notes: '',
      platform: 'anthropic',
      type: 'apikey',
      credentials: {
        base_url: 'https://relay.mirasim.ai',
        model_mapping: {}
      },
      credentials_status: { has_mirasim_refresh_token: true },
      extra: { mirasim_relay_enabled: true, mirasim_auth_url: 'https://auth.example.com' },
      proxy_id: null,
      concurrency: 1,
      priority: 1,
      rate_multiplier: 1,
      status: 'active',
      group_ids: [],
      expires_at: null,
      auto_pause_on_expired: false,
      // 后端派生只读字段，默认缺失（旧版本后端）——需要时用 overrides 显式传入
      ...overrides
    } as any
  }

  function mountWith(account: any) {
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    applyMirasimMappingMock.mockReset()
    showErrorMock.mockClear()
    showSuccessMock.mockClear()
    showInfoMock.mockClear()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)
    return mountModal(account)
  }

  it('loads an existing relay account into the advanced tab form', async () => {
    const wrapper = mountWith(buildMirasimAccount())
    await wrapper.get('[data-testid="edit-account-tab-advanced"]').trigger('click')

    expect(wrapper.find('[data-testid="edit-mirasim-relay-section"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="edit-mirasim-refresh-token"]').exists()).toBe(true)

    // extra.mirasim_auth_url 有值 → 覆盖面板自动展开
    const relayUrl = wrapper.get<HTMLInputElement>('[data-testid="edit-mirasim-relay-url"]')
    expect(relayUrl.element.value).toBe('')
  })

  it('submits refresh token and relay base url for a newly enabled account without api_key', async () => {
    const account = { ...buildMirasimAccount(), credentials_status: {}, extra: {} }
    account.credentials = { base_url: 'https://api.anthropic.com', model_mapping: {} }
    const wrapper = mountWith(account)
    await wrapper.get('[data-testid="edit-account-tab-advanced"]').trigger('click')

    await wrapper.get('[data-testid="edit-mirasim-relay-toggle"]').trigger('click')
    await wrapper.get('[data-testid="edit-mirasim-refresh-token"]').setValue('rt-new')
    await wrapper.get('[data-testid="edit-mirasim-device-key"]').setValue('  device-pem  ')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload.credentials.mirasim_refresh_token).toBe('rt-new')
    expect(payload.credentials.mirasim_device_key).toBe('device-pem')
    expect(payload.credentials.base_url).toBe('https://relay.mirasim.ai')
    expect(payload.credentials).not.toHaveProperty('api_key')
    expect(payload.extra.mirasim_relay_enabled).toBe(true)
  })

  it('validates that a refresh token is required before enabling', async () => {
    const account = { ...buildMirasimAccount(), credentials_status: {}, extra: {} }
    account.credentials = { base_url: 'https://api.anthropic.com', model_mapping: {} }
    const wrapper = mountWith(account)
    await wrapper.get('[data-testid="edit-account-tab-advanced"]').trigger('click')

    await wrapper.get('[data-testid="edit-mirasim-relay-toggle"]').trigger('click')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).not.toHaveBeenCalled()
  })

  it('writes an explicit false and restores the official base url when relay is disabled', async () => {
    // 关闭 relay 后不再有 refresh token 换 ticket 的能力，必须仍有 api_key 才能提交
    // （否则后端 GetAccessToken 无凭据可用），所以这里模拟一个同时存有 api_key 的账号。
    const account = buildMirasimAccount()
    account.credentials_status = { has_mirasim_refresh_token: true, has_api_key: true }
    const wrapper = mountWith(account)
    await wrapper.get('[data-testid="edit-account-tab-advanced"]').trigger('click')

    await wrapper.get('[data-testid="edit-mirasim-relay-toggle"]').trigger('click')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload.extra.mirasim_relay_enabled).toBe(false)
    expect(payload.extra).not.toHaveProperty('mirasim_auth_url')
    expect(payload.credentials.base_url).toBe('https://api.anthropic.com')
  })

  it('still requires an api_key when disabling relay on an account without one', async () => {
    const wrapper = mountWith(buildMirasimAccount())
    await wrapper.get('[data-testid="edit-account-tab-advanced"]').trigger('click')

    await wrapper.get('[data-testid="edit-mirasim-relay-toggle"]').trigger('click')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).not.toHaveBeenCalled()
  })

  // ==== 空映射风险提示 + 一键填充（mirasim_model_mapping_missing） ====
  // 关键约束（改动前请先读）：该提示是只读派生字段驱动的，
  // 非 relay / 已配置映射的账号绝不能出现提示，否则会诱导用户误覆盖手工映射。
  it('hides the missing-mapping warning for a relay account that already has a mapping', async () => {
    const account = buildMirasimAccount()
    account.credentials = {
      base_url: 'https://relay.mirasim.ai',
      model_mapping: { 'kimi-k3': 'kimi-k3' }
    }
    const wrapper = mountWith(account)
    await wrapper.get('[data-testid="edit-account-tab-advanced"]').trigger('click')

    expect(wrapper.find('[data-testid="edit-mirasim-mapping-missing"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="edit-mirasim-apply-mapping"]').exists()).toBe(false)
  })

  it('warns and one-click fills the identity mapping when the backend flags a missing mapping', async () => {
    const wrapper = mountWith(buildMirasimAccount({ mirasim_model_mapping_missing: true }))
    await wrapper.get('[data-testid="edit-account-tab-advanced"]').trigger('click')

    expect(wrapper.find('[data-testid="edit-mirasim-mapping-missing"]').exists()).toBe(true)

    // 注意：mountWith 会 reset mock，所以返回值必须在挂载之后再设置。
    applyMirasimMappingMock.mockResolvedValue({
      models: ['kimi-k3', 'deepseek-v4-flash'],
      changed: true
    })
    await wrapper.get('[data-testid="edit-mirasim-apply-mapping"]').trigger('click')
    await flushPromises()

    // 本地白名单为空 → 不请求覆盖（overwrite=false），避免抢掉服务端已有映射
    expect(applyMirasimMappingMock).toHaveBeenCalledTimes(1)
    expect(applyMirasimMappingMock.mock.calls[0]?.[0]).toBe(9)
    expect(applyMirasimMappingMock.mock.calls[0]?.[1]).toBe(false)

    // 表单状态同步为白名单模式（服务端写入的就是 identity 映射），并提示成功条数
    await wrapper.get('[data-testid="edit-account-tab-models"]').trigger('click')
    expect(wrapper.get('[data-testid="model-whitelist-value"]').text()).toBe(
      'kimi-k3,deepseek-v4-flash'
    )
    expect(showSuccessMock).toHaveBeenCalledTimes(1)
    expect(showErrorMock).not.toHaveBeenCalled()
  })

  it('requests an overwrite when the form already holds a mapping', async () => {
    const account = buildMirasimAccount({ mirasim_model_mapping_missing: true })
    account.credentials = {
      base_url: 'https://relay.mirasim.ai',
      model_mapping: { 'deepseek-v4.1-flash': 'kimi-k3' }
    }
    const wrapper = mountWith(account)
    await wrapper.get('[data-testid="edit-account-tab-advanced"]').trigger('click')

    applyMirasimMappingMock.mockResolvedValue({ models: ['kimi-k3'], changed: true })
    await wrapper.get('[data-testid="edit-mirasim-apply-mapping"]').trigger('click')
    await flushPromises()

    expect(applyMirasimMappingMock.mock.calls[0]?.[1]).toBe(true)
  })

  it('reports an idempotent no-op as info instead of success', async () => {
    const wrapper = mountWith(buildMirasimAccount({ mirasim_model_mapping_missing: true }))
    await wrapper.get('[data-testid="edit-account-tab-advanced"]').trigger('click')

    applyMirasimMappingMock.mockResolvedValue({ models: ['kimi-k3'], changed: false })
    await wrapper.get('[data-testid="edit-mirasim-apply-mapping"]').trigger('click')
    await flushPromises()

    expect(showInfoMock).toHaveBeenCalledTimes(1)
    expect(showSuccessMock).not.toHaveBeenCalled()
    expect(showErrorMock).not.toHaveBeenCalled()
  })

  it('shows the upstream error message and keeps the form editable when applying fails', async () => {
    const wrapper = mountWith(buildMirasimAccount({ mirasim_model_mapping_missing: true }))
    await wrapper.get('[data-testid="edit-account-tab-advanced"]').trigger('click')

    applyMirasimMappingMock.mockRejectedValue(new Error('Account is not a mirasim relay account'))
    await wrapper.get('[data-testid="edit-mirasim-apply-mapping"]').trigger('click')
    await flushPromises()

    expect(showErrorMock).toHaveBeenCalledTimes(1)
    expect(showSuccessMock).not.toHaveBeenCalled()
    // 失败不破坏编辑态：提示仍在，按钮可重试，模型选择器内容不变
    await wrapper.get('[data-testid="edit-account-tab-models"]').trigger('click')
    expect(wrapper.get('[data-testid="model-whitelist-value"]').text()).toBe('')
    expect(wrapper.find('[data-testid="edit-mirasim-apply-mapping"]').exists()).toBe(true)
    expect(
      wrapper.get('[data-testid="edit-mirasim-apply-mapping"]').attributes('disabled')
    ).toBeUndefined()
  })
})
