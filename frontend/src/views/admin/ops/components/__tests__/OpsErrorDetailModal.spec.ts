import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import OpsErrorDetailModal from '../OpsErrorDetailModal.vue'
import { opsAPI, type OpsErrorDetail } from '@/api/admin/ops'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/api/admin/ops', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/admin/ops')>()
  return {
    ...actual,
    opsAPI: {
      ...actual.opsAPI,
      getRequestErrorDetail: vi.fn(),
      getUpstreamErrorDetail: vi.fn(),
      listRequestErrorUpstreamErrors: vi.fn().mockResolvedValue({ items: [] }),
    },
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }),
}))

const BaseDialogStub = {
  template: '<div><slot /></div>',
}

function routingErrorDetail(): OpsErrorDetail {
  return {
    id: 59532,
    created_at: '2026-08-20T21:33:27.841+0800',
    phase: 'routing',
    type: 'api_error',
    error_owner: 'platform',
    error_source: 'gateway',
    severity: 'P1',
    status_code: 503,
    platform: 'openai',
    model: 'gpt-5.6-sol',
    resolved: false,
    client_request_id: '',
    request_id: 'fece5b46-d641-4f11-9bc7-4865c61aff82',
    message: 'Service temporarily unavailable',
    user_email: '',
    account_name: '',
    group_name: 'Pro 外接',
    auth_latency_ms: 3,
    routing_latency_ms: null,
    upstream_latency_ms: null,
    response_latency_ms: null,
    is_business_limited: true,
    error_body: '',
  } as OpsErrorDetail
}

function selectedAccountPolicyErrorDetail(): OpsErrorDetail {
  return {
    ...routingErrorDetail(),
    id: 59564,
    phase: 'auth', // 历史数据的错误分类，前端应根据实际阶段证据纠正展示
    status_code: 403,
    message: 'This account only allows Codex official clients',
    user_id: 21,
    user_email: 'user@example.com',
    account_id: 81,
    account_name: 'GPT-自有-1',
    auth_latency_ms: 3,
    routing_latency_ms: 19,
    upstream_latency_ms: null,
    response_latency_ms: 1,
  } as OpsErrorDetail
}

function mountModal(errorType: 'request' | 'upstream' = 'request') {
  return mount(OpsErrorDetailModal, {
    props: { show: true, errorId: 59532, errorType },
    global: { stubs: { BaseDialog: BaseDialogStub, Icon: true } },
  })
}

describe('OpsErrorDetailModal request flow', () => {
  beforeEach(() => {
    vi.mocked(opsAPI.getRequestErrorDetail).mockResolvedValue(routingErrorDetail())
    vi.mocked(opsAPI.listRequestErrorUpstreamErrors).mockResolvedValue({ items: [] })
  })

  it('marks routing as the failed stage when the request died in account selection', async () => {
    const wrapper = mountModal()
    await flushPromises()

    expect(wrapper.text()).toContain('admin.ops.errorDetail.routing')
    expect(wrapper.text()).toContain('admin.ops.errorDetail.diedHere')
    expect(wrapper.text()).toContain('Service temporarily unavailable')
    expect(wrapper.text()).toContain('3ms')
    expect(wrapper.text()).toContain('admin.ops.errorDetail.flowNotReached')
  })

  it('shows auth as passed and upstream as not reached for a routing-phase error', async () => {
    const wrapper = mountModal()
    await flushPromises()

    const text = wrapper.text()
    // auth 有 latency → 通过;routing 是失败阶段;upstream/response 未到达
    expect(text).toContain('admin.ops.errorDetail.auth')
    expect(text).toContain('admin.ops.errorDetail.upstream')
    expect(text).toContain('admin.ops.errorDetail.diedHere')
  })

  it('shows both user and attempted account instead of hiding the account for gateway errors', async () => {
    vi.mocked(opsAPI.getRequestErrorDetail).mockResolvedValue(selectedAccountPolicyErrorDetail())

    const wrapper = mountModal()
    await flushPromises()

    expect(wrapper.text()).toContain('user@example.com')
    expect(wrapper.text()).toContain('GPT-自有-1')
    expect(wrapper.text()).toContain('#81')
  })

  it('renders a selected-account policy rejection in chronological order', async () => {
    vi.mocked(opsAPI.getRequestErrorDetail).mockResolvedValue(selectedAccountPolicyErrorDetail())

    const wrapper = mountModal()
    await flushPromises()

    expect(wrapper.get('[data-flow-step="auth"]').attributes('data-flow-status')).toBe('passed')
    expect(wrapper.get('[data-flow-step="routing"]').attributes('data-flow-status')).toBe('passed')
    expect(wrapper.get('[data-flow-step="upstream"]').attributes('data-flow-status')).toBe('blocked')
    expect(wrapper.get('[data-flow-step="response"]').attributes('data-flow-status')).toBe('passed')
    expect(wrapper.text()).toContain('admin.ops.errorDetail.blockedBeforeUpstream')
  })
})
