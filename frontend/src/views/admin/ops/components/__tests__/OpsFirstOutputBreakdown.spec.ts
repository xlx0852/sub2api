import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import OpsFirstOutputBreakdown from '../OpsFirstOutputBreakdown.vue'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key,
    }),
  }
})

describe('OpsFirstOutputBreakdown', () => {
  it('keeps the breakdown in a rounded card on mobile', () => {
    const wrapper = mount(OpsFirstOutputBreakdown, {
      global: { stubs: { HelpTooltip: true } },
    })

    expect(wrapper.get('section').classes()).toEqual(expect.arrayContaining([
      'rounded-2xl',
      'bg-white',
      'ring-1',
      'dark:bg-dark-800',
    ]))
    expect(wrapper.get('section').classes()).not.toContain('bg-white/45')
  })

  it('renders average stages, total, and coverage', () => {
    const wrapper = mount(OpsFirstOutputBreakdown, {
      props: {
        breakdown: {
          eligible_streaming_requests: 20,
          gateway_pre_upstream: { avg_ms: 100, sample_count: 10, coverage: 0.5 },
          upstream_connect: { avg_ms: 50, sample_count: 10, coverage: 0.5 },
          upstream_first_output: { avg_ms: 800, sample_count: 10, coverage: 0.5 },
          retry_failover: { avg_ms: 40, sample_count: 10, coverage: 0.5 },
          gateway_first_output_write: { avg_ms: 10, sample_count: 10, coverage: 0.5 },
          gateway_first_output_total: { avg_ms: 1000, sample_count: 10, coverage: 0.5 },
        },
      },
      global: {
        stubs: { HelpTooltip: true },
      },
    })

    expect(wrapper.text()).toContain('1.00s')
    expect(wrapper.text()).toContain('800ms')
    expect(wrapper.text()).toContain('50%')
  })

  it('does not turn missing stages into zero milliseconds', () => {
    const wrapper = mount(OpsFirstOutputBreakdown, {
      global: { stubs: { HelpTooltip: true } },
    })

    expect(wrapper.text()).toContain('--')
    expect(wrapper.text()).not.toContain('0ms')
  })
})
