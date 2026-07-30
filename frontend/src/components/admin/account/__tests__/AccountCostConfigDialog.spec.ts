import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountCostConfigDialog from '../AccountCostConfigDialog.vue'

const { listSubscriptionCycles, createSubscriptionCycle, deleteSubscriptionCycle, previewSubscriptionTermination, terminateSubscriptionCycle, addSubscriptionRefund, voidSubscriptionRefund, reverseSubscriptionTermination, showSuccess, showError } = vi.hoisted(() => ({
  listSubscriptionCycles: vi.fn(),
  createSubscriptionCycle: vi.fn(),
  deleteSubscriptionCycle: vi.fn(),
  previewSubscriptionTermination: vi.fn(),
  terminateSubscriptionCycle: vi.fn(),
  addSubscriptionRefund: vi.fn(),
  voidSubscriptionRefund: vi.fn(),
  reverseSubscriptionTermination: vi.fn(),
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
    profit: { listSubscriptionCycles, createSubscriptionCycle, deleteSubscriptionCycle, previewSubscriptionTermination, terminateSubscriptionCycle, addSubscriptionRefund, voidSubscriptionRefund, reverseSubscriptionTermination },
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
    previewSubscriptionTermination.mockReset()
    terminateSubscriptionCycle.mockReset()
    addSubscriptionRefund.mockReset()
    voidSubscriptionRefund.mockReset()
    reverseSubscriptionTermination.mockReset()
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

  it('封禁结算先预览亏损，二次确认后才停号', async () => {
    listSubscriptionCycles.mockResolvedValue({
      cycles: [{ id: 8, account_id: 91, starts_at: '2026-07-01T00:00:00Z', period_fee: 865, period_days: 60, currency: 'USD', notes: '', created_at: '', updated_at: '' }]
    })
    previewSubscriptionTermination.mockResolvedValue({
      purchase_cost: 865, revenue_before_ban: 300, refund_total: 200, net_purchase_cost: 665,
      recovered_amount: 500, recovery_progress: 57.8, realized_profit: -365, realized_loss: 365
    })
    terminateSubscriptionCycle.mockResolvedValue({ cycle: {} })
    const wrapper = mountDialog()
    await flushPromises()

    await wrapper.get('[data-testid="settle-ban"]').trigger('click')
    await wrapper.get('[data-testid="termination-form"] input[type="number"]').setValue(200)
    await wrapper.get('[data-testid="preview-ban-settlement"]').trigger('click')
    await flushPromises()

    expect(previewSubscriptionTermination).toHaveBeenCalledWith(8, expect.objectContaining({ initial_refund_amount: 200 }))
    expect(wrapper.text()).toContain('$365.00')
    const confirmButton = wrapper.findAll('button').find((button) => button.text() === 'admin.profit.confirmBanAndDisable')
    expect(confirmButton).toBeTruthy()
    await confirmButton!.trigger('click')
    await flushPromises()

    expect(terminateSubscriptionCycle).toHaveBeenCalledWith(8, expect.objectContaining({ reason: 'upstream_banned', initial_refund_amount: 200 }))
    expect(showSuccess).toHaveBeenCalledWith('admin.profit.banSettlementSaved')
  })

  it('已封禁周期可追加实际到账退款', async () => {
    listSubscriptionCycles.mockResolvedValue({
      cycles: [{
        id: 8, account_id: 91, starts_at: '2026-07-01T00:00:00Z', period_fee: 865, period_days: 60, currency: 'USD', notes: '', created_at: '', updated_at: '',
        termination: { id: 19, cycle_id: 8, account_id: 91, effective_at: '2026-07-30T02:00:00Z', reason: 'upstream_banned', notes: '', created_at: '', updated_at: '' },
        loss_summary: { purchase_cost: 865, revenue_before_ban: 300, refund_total: 0, net_purchase_cost: 865, recovered_amount: 300, recovery_progress: 34.68, realized_profit: -565, realized_loss: 565 }
      }]
    })
    addSubscriptionRefund.mockResolvedValue({ cycle: {} })
    const wrapper = mountDialog()
    await flushPromises()

    const addRefundButton = wrapper.findAll('button').find((button) => button.text() === 'admin.profit.addReceivedRefund')
    await addRefundButton!.trigger('click')
    await wrapper.get('[data-testid="refund-form"] input[type="number"]').setValue(200)
    await wrapper.get('[data-testid="save-refund"]').trigger('click')
    await flushPromises()

    expect(addSubscriptionRefund).toHaveBeenCalledWith(19, expect.objectContaining({ amount: 200 }))
    expect(showSuccess).toHaveBeenCalledWith('admin.profit.refundSaved')
  })
})
