import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountCostConfigDialog from '../AccountCostConfigDialog.vue'

const { listSubscriptionCycles, createSubscriptionCycle, deleteSubscriptionCycle, showSuccess, showError } = vi.hoisted(() => ({
  listSubscriptionCycles: vi.fn(),
  createSubscriptionCycle: vi.fn(),
  deleteSubscriptionCycle: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showError }),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    profit: { listSubscriptionCycles, createSubscriptionCycle, deleteSubscriptionCycle },
  },
}))

const mountDialog = () => mount(AccountCostConfigDialog, {
  props: {
    show: true,
    accountId: 91,
    accountName: 'Kimi-自有',
    accountType: 'oauth',
  },
  global: {
    stubs: {
      BaseDialog: {
        props: ['show', 'title'],
        emits: ['close'],
        template: '<div v-if="show" role="dialog"><slot /><slot name="footer" /></div>',
      },
    },
  },
})

describe('AccountCostConfigDialog', () => {
  beforeEach(() => {
    listSubscriptionCycles.mockReset().mockResolvedValue({ cycles: [] })
    createSubscriptionCycle.mockReset()
    deleteSubscriptionCycle.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
  })

  it('保存成功后新增独立充值周期', async () => {
    createSubscriptionCycle.mockResolvedValue({})
    const wrapper = mountDialog()
    await flushPromises()

    await wrapper.get('input[type="date"]').setValue('2026-08-01')
    await wrapper.get('[data-testid="save-cost-config"]').trigger('click')
    await flushPromises()

    expect(createSubscriptionCycle).toHaveBeenCalledWith(91, {
      starts_at: '2026-08-01',
      period_fee: 0,
      period_days: 30,
      currency: 'USD',
      notes: '',
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.profit.saveSuccess')
    expect(wrapper.emitted('saved')).toHaveLength(1)
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('保存失败时保留弹窗并展示错误', async () => {
    createSubscriptionCycle.mockRejectedValue({ message: 'failed' })
    const wrapper = mountDialog()
    await flushPromises()

    await wrapper.get('input[type="date"]').setValue('2026-08-01')
    await wrapper.get('[data-testid="save-cost-config"]').trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalled()
    expect(wrapper.emitted('close')).toBeUndefined()
  })

  it('API Key 账号仅展示按量计费说明，不提供订阅配置保存', async () => {
    const wrapper = mount(AccountCostConfigDialog, {
      props: { show: true, accountId: 78, accountName: 'Claude', accountType: 'apikey' },
      global: { stubs: { BaseDialog: { props: ['show', 'title'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' } } },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('admin.profit.metered')
    expect(wrapper.find('[data-testid="save-cost-config"]').exists()).toBe(false)
  })

  it('Grok 账号明确区分真实付款周期和自然月额度重置', async () => {
    createSubscriptionCycle.mockResolvedValue({})
    const wrapper = mount(AccountCostConfigDialog, {
      props: {
        show: true,
        accountId: 104,
        accountName: 'GROK-aihh206887277@gmail.com',
        accountType: 'oauth',
        accountPlatform: 'grok',
      },
      global: {
        stubs: {
          BaseDialog: {
            props: ['show', 'title'],
            emits: ['close'],
            template: '<div v-if="show" role="dialog"><slot /><slot name="footer" /></div>',
          },
        },
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('admin.profit.grokCycleHint')
    await wrapper.get('input[type="number"][min="1"]').setValue(31)
    await wrapper.get('input[type="date"]').setValue('2026-07-17')
    await wrapper.get('[data-testid="save-cost-config"]').trigger('click')
    await flushPromises()

    expect(createSubscriptionCycle).toHaveBeenCalledWith(104, {
      starts_at: '2026-07-17',
      period_fee: 0,
      period_days: 31,
      currency: 'USD',
      notes: '',
    })
  })
})
