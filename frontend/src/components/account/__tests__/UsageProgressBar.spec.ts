import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import UsageProgressBar from '../UsageProgressBar.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('UsageProgressBar', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-17T00:00:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('showNowWhenIdle=true 且利用率为 0 时显示“现在”', () => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: '5h',
        utilization: 0,
        resetsAt: '2026-03-17T02:30:00Z',
        showNowWhenIdle: true,
        color: 'indigo'
      }
    })

    expect(wrapper.text()).toContain('usage.resetNow')
    expect(wrapper.text()).not.toContain('2h 30m')
  })

  it('showNowWhenIdle=true 但利用率大于 0 时显示倒计时', () => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: '7d',
        utilization: 12,
        resetsAt: '2026-03-17T02:30:00Z',
        showNowWhenIdle: true,
        color: 'emerald'
      }
    })

    expect(wrapper.text()).toContain('2h 30m')
    expect(wrapper.text()).not.toContain('usage.resetNow')
    expect(wrapper.text()).not.toContain('usage.resetPending')
  })

  it('showNowWhenIdle=false 时保持原有倒计时行为', () => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: '1d',
        utilization: 0,
        resetsAt: '2026-03-17T02:30:00Z',
        showNowWhenIdle: false,
        color: 'indigo'
      }
    })

    expect(wrapper.text()).toContain('2h 30m')
    expect(wrapper.text()).not.toContain('usage.resetNow')
  })

  it('resetsAt 已过期且利用率大于 0 时显示「待刷新」', () => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: '5h',
        utilization: 53,
        // 早于 fake system time 2026-03-17T00:00:00Z
        resetsAt: '2026-03-16T22:00:00Z',
        color: 'indigo'
      }
    })

    expect(wrapper.text()).toContain('usage.resetPending')
    expect(wrapper.text()).not.toContain('usage.resetNow')
  })

  it('resetsAt 已过期且利用率为 0 时仍显示「现在」', () => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: '5h',
        utilization: 0,
        resetsAt: '2026-03-16T22:00:00Z',
        color: 'indigo'
      }
    })

    expect(wrapper.text()).toContain('usage.resetNow')
    expect(wrapper.text()).not.toContain('usage.resetPending')
  })

  it('将窗口状态和用量指标拆成可扫读的统一区块', () => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: '5h',
        utilization: 4,
        resetsAt: '2026-03-17T04:26:00Z',
        color: 'indigo',
        windowStats: {
          requests: 100,
          tokens: 21_800_000,
          cost: 20.66,
          user_cost: 20.66
        }
      }
    })

    expect(wrapper.get('[data-testid="usage-window-row"]').text()).toContain('5h')
    expect(wrapper.get('[data-testid="usage-stat-volume"]').text()).toContain('100 req')
    expect(wrapper.get('[data-testid="usage-stat-volume"]').text()).toContain('21.8M')
    expect(wrapper.get('[data-testid="usage-stat-billing"]').text()).toContain('A $20.66')
    expect(wrapper.get('[data-testid="usage-stat-billing"]').text()).toContain('U $20.66')
    expect(wrapper.get('[data-testid="usage-progress-fill"]').classes()).toContain('bg-emerald-500')
    expect(wrapper.findAll('.usage-metric')).toHaveLength(8)
    expect(wrapper.html()).not.toContain('shadow')
  })

  it('根据当前消耗和已用比例展示满额线性预估', () => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: '7d',
        utilization: 20,
        color: 'emerald',
        windowStats: {
          requests: 6_000,
          tokens: 760_000_000,
          cost: 600,
          user_cost: 72
        }
      }
    })

    const estimate = wrapper.get('[data-testid="usage-full-estimate"]')
    expect(estimate.text()).toContain('usage.fullUtilizationEstimate')
    expect(estimate.text()).toContain('30.0K req')
    expect(estimate.text()).toContain('3.8B')
    expect(estimate.text()).toContain('A $3000.00')
    expect(estimate.text()).toContain('U $360.00')
  })

  it('满额预估请求数四舍五入为整数', () => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: '5h',
        utilization: 13,
        color: 'indigo',
        windowStats: {
          requests: 5,
          tokens: 392_900,
          cost: 1.18,
          user_cost: 0.14
        }
      }
    })

    const estimate = wrapper.get('[data-testid="usage-full-estimate"]')
    expect(estimate.text()).toContain('38 req')
    expect(estimate.text()).not.toContain('38.461')
  })

  it.each([0, 120])('利用率为 %s 时不展示满额预估', (utilization) => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: '5h',
        utilization,
        color: 'indigo',
        windowStats: { requests: 100, tokens: 1_000, cost: 1 }
      }
    })

    expect(wrapper.find('[data-testid="usage-full-estimate"]').exists()).toBe(false)
  })

  it.each([
    { utilization: 79, colorClass: 'bg-emerald-500' },
    { utilization: 80, colorClass: 'bg-amber-500' },
    { utilization: 99, colorClass: 'bg-amber-500' },
    { utilization: 100, colorClass: 'bg-red-500' }
  ])('利用率 $utilization% 使用 $colorClass', ({ utilization, colorClass }) => {
    const wrapper = mount(UsageProgressBar, {
      props: {
        label: '5h',
        utilization,
        color: 'indigo'
      }
    })

    expect(wrapper.get('[data-testid="usage-progress-fill"]').classes()).toContain(colorClass)
  })
})
