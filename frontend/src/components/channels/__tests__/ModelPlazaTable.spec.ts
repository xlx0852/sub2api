import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ModelPlazaTable from '../ModelPlazaTable.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: vi.fn() }) }))

describe('ModelPlazaTable', () => {
  it('用响应式卡片展示模型与价格', () => {
    const wrapper = mount(ModelPlazaTable, {
      props: {
        loading: false,
        emptyLabel: 'empty',
        rows: [{
          key: 'openai:gpt-5.4', name: 'gpt-5.4', display_name: 'GPT-5.4', platform: 'openai', tags: ['reasoning'],
          offers: [{ id: 4, name: 'GPT Pro', channel_name: '模型列表示例', channel_description: '', rate_multiplier: 0.8, is_exclusive: false, pricing: null, base_pricing: null, effective_pricing: { billing_mode: 'token', input_price: 0.0000025, output_price: 0.000015, cache_read_price: 0.00000025, cache_write_price: null, image_output_price: null, per_request_price: null, intervals: [] } }],
          best_offer: { id: 4, name: 'GPT Pro', channel_name: '模型列表示例', channel_description: '', rate_multiplier: 0.8, is_exclusive: false, pricing: null, base_pricing: null, effective_pricing: { billing_mode: 'token', input_price: 0.0000025, output_price: 0.000015, cache_read_price: 0.00000025, cache_write_price: null, image_output_price: null, per_request_price: null, intervals: [] } }
        }]
      },
      global: { stubs: { Icon: true, ModelIcon: true, PlatformIcon: true } }
    })
    expect(wrapper.find('[data-testid="model-plaza-grid"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="model-plaza-grid"]').classes()).toContain('xl:grid-cols-4')
    expect(wrapper.text()).toContain('GPT-5.4')
    expect(wrapper.text()).toContain('2.5')
    expect(wrapper.text()).toContain('20%')
  })

  it('公开价格模式不展示渠道信息', () => {
    const pricing = { billing_mode: 'token' as const, input_price: 0.0000025, output_price: 0.000015, cache_read_price: null, cache_write_price: null, image_output_price: null, per_request_price: null, intervals: [] }
    const offer = { id: 1, name: '内部渠道', platform: 'openai', subscription_type: 'standard', channel_name: '内部渠道名', channel_description: '', rate_multiplier: 1, peak_rate_enabled: false, peak_start: '', peak_end: '', peak_rate_multiplier: 1, is_exclusive: false, base_pricing: pricing, effective_pricing: pricing }
    const wrapper = mount(ModelPlazaTable, {
      props: {
        loading: false,
        emptyLabel: 'empty',
        showOffers: false,
        rows: [{ key: 'openai:gpt-public', name: 'gpt-public', display_name: 'GPT Public', platform: 'openai', is_reasoning: false, media: {}, tags: [], offers: [offer], best_offer: offer }],
      },
      global: { stubs: { Icon: true, ModelIcon: true, PlatformIcon: true } },
    })

    expect(wrapper.text()).toContain('availableChannels.publicPricing.publicRate')
    expect(wrapper.text()).not.toContain('内部渠道')
    expect(wrapper.text()).not.toContain('availableChannels.columns.channels')
  })
})
