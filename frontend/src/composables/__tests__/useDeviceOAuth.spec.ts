import { defineComponent, h } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import type { DeviceAuthorization } from '@/api/admin/deviceOAuth'
import { useDeviceOAuth } from '@/composables/useDeviceOAuth'

const pendingSession = (): DeviceAuthorization => ({
  session_id: 'device-session',
  status: 'pending',
  verification_uri: 'https://example.test/device',
  user_code: 'ABCD-EFGH',
  expires_in: 600,
  interval: 5
})

describe('useDeviceOAuth', () => {
  it('starts, polls and creates an account after authorization', async () => {
    const client = {
      start: vi.fn().mockResolvedValue(pendingSession()),
      status: vi.fn().mockResolvedValue({ ...pendingSession(), status: 'authorized' }),
      cancel: vi.fn().mockResolvedValue(undefined),
      createAccount: vi.fn().mockResolvedValue({ id: 1 }),
      reauthorizeAccount: vi.fn()
    }
    const wrapper = mount(defineComponent({
      setup: () => useDeviceOAuth({ client, provider: 'OpenAI' }),
      render: () => h('div')
    }))
    const flow = wrapper.vm as any

    await expect(flow.start(7)).resolves.toBe(true)
    expect(client.start).toHaveBeenCalledWith(7)
    expect(flow.session.session_id).toBe('device-session')

    await expect(flow.poll()).resolves.toBe(true)
    expect(flow.authorized).toBe(true)
    await flow.createAccount({
      name: 'OpenAI device',
      proxy_id: 7,
      concurrency: 1,
      priority: 1,
      group_ids: []
    })
    expect(client.createAccount).toHaveBeenCalledWith(expect.objectContaining({ session_id: 'device-session' }))
    wrapper.unmount()
  })

  it('cancels the active session and clears local state', async () => {
    const client = {
      start: vi.fn().mockResolvedValue(pendingSession()),
      status: vi.fn(),
      cancel: vi.fn().mockResolvedValue(undefined),
      createAccount: vi.fn(),
      reauthorizeAccount: vi.fn()
    }
    const wrapper = mount(defineComponent({
      setup: () => useDeviceOAuth({ client, provider: 'Grok' }),
      render: () => h('div')
    }))
    const flow = wrapper.vm as any
    await flow.start()
    await flow.cancel()
    expect(client.cancel).toHaveBeenCalledWith('device-session')
    expect(flow.session).toBe(null)
    expect(flow.remainingSeconds).toBe(0)
    wrapper.unmount()
  })
})
