import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

import RequestTimingWaterfall from '../RequestTimingWaterfall.vue'

const segments = [
  { key: 'body_wait', lane: 'client', start_ms: 0, end_ms: 5 },
  { key: 'body_receive', lane: 'client', start_ms: 5, end_ms: 32 },
  { key: 'api_key_auth', lane: 'gateway', start_ms: 32, end_ms: 2036 },
  { key: 'account_select', lane: 'gateway', start_ms: 2100, end_ms: 2112 },
  { key: 'upstream_connect', lane: 'upstream', start_ms: 2140, end_ms: 2141, attempt: 1 },
  { key: 'upstream_first_byte', lane: 'upstream', start_ms: 2186, end_ms: 2820, attempt: 1 },
  { key: 'upstream_stream', lane: 'upstream', start_ms: 2820, end_ms: 50810, attempt: 1 }
]

describe('RequestTimingWaterfall', () => {
  it('按泳道渲染色块，宽度与偏移取自 segment 的起止时间', () => {
    const wrapper = mount(RequestTimingWaterfall, {
      props: { segments, totalMs: 52_910, firstTokenMs: 44_200, bodyBytes: 67_790_000 }
    })

    // 三条泳道
    expect(wrapper.text()).toContain('usage.timingDetail.clientToGateway')
    expect(wrapper.text()).toContain('usage.timingDetail.gatewayInternal')
    expect(wrapper.text()).toContain('usage.timingDetail.gatewayToUpstream')
    // 体积标注
    expect(wrapper.text()).toContain('64.65 MiB')
    // 色块数量 = 阶段数
    expect(wrapper.findAll('[data-segment]')).toHaveLength(segments.length)
    // 首字节色块起点 = 2186/52910 ≈ 4.13%
    const firstByte = wrapper.find('[data-segment="upstream_first_byte"]')
    expect(firstByte.attributes('style')).toContain('left: 4.1315')
    // 宽度 = 634/52910 ≈ 1.198%
    expect(firstByte.attributes('style')).toContain('width: 1.1982')
    // 首 Token 标记按比例定位（44200/52910 ≈ 83.54%）
    expect(wrapper.html()).toContain('left: 83.538')
  })

  it('没有分段数据时显示未采集占位', () => {
    const wrapper = mount(RequestTimingWaterfall, { props: { segments: null, totalMs: 1000 } })
    expect(wrapper.text()).toContain('usage.timingDetail.notCollected')
    expect(wrapper.findAll('[data-segment]')).toHaveLength(0)
  })

  it('多次上游尝试分成独立泳道', () => {
    const wrapper = mount(RequestTimingWaterfall, {
      props: { segments: [
        { key: 'upstream_first_byte', lane: 'upstream', start_ms: 0, end_ms: 100, attempt: 1 },
        { key: 'upstream_first_byte', lane: 'upstream', start_ms: 100, end_ms: 300, attempt: 2 }
      ], totalMs: 300 }
    })
    expect(wrapper.text()).toContain('#1')
    expect(wrapper.text()).toContain('#2')
  })
})
