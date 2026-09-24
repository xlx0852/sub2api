import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

import UsageRequestTimingModal from '../UsageRequestTimingModal.vue'

const usageLog = {
  request_id: 'client:test-1',
  model: 'gpt-5.6-sol',
  created_at: '2026-09-23T16:02:21+08:00',
  inbound_endpoint: '/v1/responses',
  upstream_endpoint: '/v1/responses',
  request_type: 'stream',
  duration_ms: 50810,
  first_token_ms: 44200,
  first_output_stages: {
    gateway_pre_upstream_ms: 285,
    upstream_connect_ms: 0,
    upstream_first_output_ms: 26510,
    gateway_first_output_write_ms: 10990,
    gateway_first_output_total_ms: 50810,
  },
  input_tokens: 100,
  output_tokens: 20,
  cache_read_tokens: 4096,
  cache_creation_tokens: 0,
  stream: true,
} as any

describe('UsageRequestTimingModal', () => {
  it('renders request paths and recorded timing stages', () => {
    const wrapper = mount(UsageRequestTimingModal, {
      props: { modelValue: true, log: usageLog },
      global: { stubs: { Icon: true, Teleport: true } },
    })

    expect(wrapper.text()).toContain('gpt-5.6-sol')
    expect(wrapper.text()).toContain('/v1/responses')
    expect(wrapper.text()).toContain('44.20s')
    expect(wrapper.text()).toContain('50.81s')
    expect(wrapper.text()).toContain('4,096')
  })

  it('shows dash for missing historical timing stages', () => {
    const wrapper = mount(UsageRequestTimingModal, {
      props: { modelValue: true, log: { ...usageLog, first_output_stages: null, first_token_ms: null, duration_ms: null } },
      global: { stubs: { Icon: true, Teleport: true } },
    })

    expect(wrapper.text()).toContain('-')
  })
})
