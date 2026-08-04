import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import UserDashboardAvailability from '../UserDashboardAvailability.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('UserDashboardAvailability', () => {
  it('聚合展示成功率、平均响应和样本状态', async () => {
    const wrapper = mount(UserDashboardAvailability, {
      props: {
        loading: false,
        platform: '',
        data: {
          start_at: '2026-08-02T00:00:00Z', end_at: '2026-08-03T00:00:00Z', bucket_minutes: 10,
          success_count: 1, failure_count: 1, sample_count: 2, success_rate: 50, average_latency_ms: 200,
          buckets: [
            { start_at: '2026-08-03T00:00:00Z', success_count: 1, failure_count: 0, sample_count: 1, success_rate: 100, average_latency_ms: 100, status: 'healthy' },
            { start_at: '2026-08-03T00:10:00Z', success_count: 0, failure_count: 1, sample_count: 1, success_rate: 0, average_latency_ms: null, status: 'attention' }
          ]
        }
      },
      global: {
        stubs: { RouterLink: true, Icon: true }
      }
    })

    expect(wrapper.text()).toContain('50.00%')
    expect(wrapper.text()).toContain('200 ms')
    expect(wrapper.text()).toContain('channelStatus.availabilityTitle')
    expect(wrapper.text()).not.toContain('dashboard.channelStatus')
    expect(wrapper.findAll('[data-testid="availability-grid"] span')).toHaveLength(2)
    await wrapper.find('button').trigger('click')
    expect(wrapper.emitted('refresh')).toHaveLength(1)
    const platformButtons = wrapper.findAll('button')
    await platformButtons[2].trigger('click')
    expect(wrapper.emitted('update:platform')?.[0]).toEqual(['openai'])
  })
})
