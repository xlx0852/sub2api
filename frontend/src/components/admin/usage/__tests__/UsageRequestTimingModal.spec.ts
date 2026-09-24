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

  it('renders the new inbound/gateway/upstream segments and the result flags', () => {
    const wrapper = mount(UsageRequestTimingModal, {
      props: {
        modelValue: true,
        log: {
          ...usageLog,
          first_output_stages: {
            ...usageLog.first_output_stages,
            body_first_wait_ms: 5,
            body_receive_ms: 27,
            body_bytes: 67_790_000,
            body_complete: true,
            api_key_auth_ms: 1_980,
            account_select_ms: 12,
            user_queue_ms: 40,
            account_queue_ms: 60,
            handler_body_read_ms: 285,
            handler_total_ms: 52_900,
            upstream_attempts: 1,
            connection_reused: true,
            upstream_write_ms: 13_360,
            upstream_first_byte_ms: 26_510,
            upstream_response_headers_ms: 39_870,
            upstream_stream_transfer_ms: 10_990,
            response_stream_complete: true,
            outcome: 'success',
          },
        },
      },
      global: { stubs: { Icon: true, Teleport: true } },
    })

    const text = wrapper.text()
    expect(text).toContain('usage.timingDetail.bodyFirstWait')
    expect(text).toContain('usage.timingDetail.bodyRate')
    expect(text).toContain('64.65 MiB')
    expect(text).toContain('usage.timingDetail.preprocessAuth')
    expect(text).toContain('usage.timingDetail.handlerTotal')
    expect(text).toContain('usage.timingDetail.outcomeSuccess')
    expect(text).toContain('usage.timingDetail.yes')
    // 未采集的行仍然渲染（安全审核、大请求排队）
    expect(text).toContain('usage.timingDetail.notCollected')
  })
})
