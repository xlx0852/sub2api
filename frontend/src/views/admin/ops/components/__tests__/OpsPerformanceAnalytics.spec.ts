import { describe, expect, it, beforeEach, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OpsPerformanceAnalytics from '../OpsPerformanceAnalytics.vue'

const mockGetDashboardPerformance = vi.fn()
vi.mock('@/api/admin/ops', () => ({
  opsAPI: { getDashboardPerformance: (...args: any[]) => mockGetDashboardPerformance(...args) }
}))

vi.mock('chart.js', () => {
  class MockChart {
    static register() {}
    destroy() {}
  }
  return { Chart: MockChart, LineController: {}, LineElement: {}, PointElement: {}, LinearScale: {}, Tooltip: {}, Legend: {} }
})

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string, params?: Record<string, any>) => params ? `${key}:${JSON.stringify(params)}` : key })
}))

const response = {
  generated_at: '2026-09-18T00:00:00Z',
  timezone: 'UTC',
  window: {
    start_time: '2026-09-18T00:00:00Z',
    end_time: '2026-09-18T01:00:00Z',
    summary: {
      request_count: 121,
      success_count: 106,
      failure_count: 15,
      success_rate: 0.876,
      ttft: { p50: 6100, p90: 17600, unit: 'ms', sample_count: 100, coverage: 0.83 },
      generation_tps: { p50: 50.2, p90: 90, unit: 'tok/s', sample_count: 101, coverage: 0.83 },
      duration: { p50: 12000, p90: 33400, unit: 'ms', sample_count: 106, coverage: 0.88 },
      input_tokens: { p50: 224600, p90: 500000, unit: 'tok', sample_count: 106, coverage: 0.88 },
    },
    points: [{ at: '2026-09-18T00:10:00Z', model: 'gpt-5', success: true, generation_tps: 50.2 }],
    point_limit: 500,
    points_sampled: false,
  },
}

describe('OpsPerformanceAnalytics', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGetDashboardPerformance.mockResolvedValue(response)
  })

  it('renders KPI values and requests the active Ops filters', async () => {
    const wrapper = mount(OpsPerformanceAnalytics, {
      props: { timeRange: '1h', platform: 'openai', groupId: 7, requestType: 'stream', refreshToken: 1 },
      global: { stubs: { EmptyState: { template: '<div />' } } },
    })
    await flushPromises()
    expect(mockGetDashboardPerformance).toHaveBeenCalledWith(expect.objectContaining({ time_range: '1h', platform: 'openai', group_id: 7, request_type: 'stream' }))
    expect(wrapper.text()).toContain('121')
    expect(wrapper.text()).toContain('87.6%')
  })

  it('switches the metric label without changing the loaded data', async () => {
    const wrapper = mount(OpsPerformanceAnalytics, {
      props: { timeRange: '1h', refreshToken: 0 },
      global: { stubs: { EmptyState: { template: '<div />' } } },
    })
    await flushPromises()
    const buttons = wrapper.findAll('button')
    const durationButton = buttons.find((button) => button.text().includes('admin.ops.performance.metrics.duration'))
    expect(durationButton).toBeTruthy()
    await durationButton!.trigger('click')
    expect(wrapper.text()).toContain('admin.ops.performance.metrics.duration')
  })
})
