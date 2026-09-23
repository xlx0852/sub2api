import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ScheduledTestsPanel from '../ScheduledTestsPanel.vue'
import type { ScheduledTestPlan } from '@/types'

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: { getAvailableModels: vi.fn().mockResolvedValue([]) },
    scheduledTests: {
      listByAccount: vi.fn().mockResolvedValue([]),
      listResults: vi.fn().mockResolvedValue([])
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() })
}))

// Render keys verbatim, with params interpolated so countdown strings are assertable.
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (!params) return key
        return `${key}(${Object.entries(params).map(([k, v]) => `${k}=${v}`).join(',')})`
      }
    })
  }
})

function makePlan(overrides: Partial<ScheduledTestPlan> = {}): ScheduledTestPlan {
  return {
    id: 87,
    account_id: 126,
    model_id: 'gpt-5.6-sol',
    cron_expression: '4-59/5 * * * *',
    enabled: true,
    max_results: 50,
    auto_recover: true,
    last_run_at: new Date(Date.now() - 60_000).toISOString(),
    next_run_at: new Date(Date.now() + 90_000).toISOString(),
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides
  }
}

const mountPanel = (plan: ScheduledTestPlan) =>
  mount(ScheduledTestsPanel, {
    props: {
      show: true,
      accountId: 126,
      plans: [plan],
      loading: false,
      embedded: true
    }
  })

describe('诊断计划的执行倒计时', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-09-23T16:00:00Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('把「下次执行」显示成秒级倒计时，并随时间递减', async () => {
    const plan = makePlan({ next_run_at: new Date(Date.now() + 90_000).toISOString() })
    const wrapper = mountPanel(plan)

    expect(wrapper.text()).toContain('admin.scheduledTests.nextRun')
    expect(wrapper.text()).toContain('admin.scheduledTests.countdownMinutes(minutes=1,seconds=30)')

    await vi.advanceTimersByTimeAsync(30_000)
    expect(wrapper.text()).toContain('admin.scheduledTests.countdownMinutes(minutes=1,seconds=00)')

    await vi.advanceTimersByTimeAsync(60_000)
    // 倒计时归零后显示"正在执行"，而不是负数
    expect(wrapper.text()).toContain('admin.scheduledTests.countdownDue')
    wrapper.unmount()
  })

  it('超过一小时仍按小时计，不会退化成巨大的分钟数', async () => {
    const plan = makePlan({ next_run_at: new Date(Date.now() + 3 * 3600_000 + 5 * 60_000 + 9_000).toISOString() })
    const wrapper = mountPanel(plan)
    expect(wrapper.text()).toContain('admin.scheduledTests.countdownHours(hours=3,minutes=05,seconds=09)')
    wrapper.unmount()
  })

  it('归零后请求父组件刷新一次，让新结果自动出现', async () => {
    const plan = makePlan({ next_run_at: new Date(Date.now() + 2_000).toISOString() })
    const wrapper = mountPanel(plan)

    await vi.advanceTimersByTimeAsync(3_000)
    expect(wrapper.text()).toContain('admin.scheduledTests.countdownDue')

    await vi.advanceTimersByTimeAsync(7_000)
    const refreshes = wrapper.emitted('refresh') ?? []
    expect(refreshes.length).toBe(1)
    wrapper.unmount()
  })

  it('定时器只会触发一次刷新，不会每秒打扰父组件', async () => {
    const plan = makePlan({ next_run_at: new Date(Date.now() + 1_000).toISOString() })
    const wrapper = mountPanel(plan)
    await vi.advanceTimersByTimeAsync(20_000)
    expect((wrapper.emitted('refresh') ?? []).length).toBeLessThanOrEqual(1)
    wrapper.unmount()
  })

  it('关闭后停止计时，避免后台常驻定时器', async () => {
    const plan = makePlan({ next_run_at: new Date(Date.now() + 5_000).toISOString() })
    const wrapper = mountPanel(plan)
    await wrapper.setProps({ show: false })
    const textAfterClose = wrapper.text()
    await vi.advanceTimersByTimeAsync(30_000)
    expect(wrapper.text()).toBe(textAfterClose)
    wrapper.unmount()
  })

  it('没有下次执行时间时不显示倒计时', () => {
    const wrapper = mountPanel(makePlan({ next_run_at: null, enabled: false }))
    expect(wrapper.text()).not.toContain('admin.scheduledTests.countdownMinutes')
    expect(wrapper.text()).not.toContain('admin.scheduledTests.countdownDue')
    wrapper.unmount()
  })
})
