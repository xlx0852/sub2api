import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import DeviceAuthorizationFlow from '../DeviceAuthorizationFlow.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string, params?: Record<string, number>) => params ? `${key}:${params.seconds}` : key })
}))

describe('DeviceAuthorizationFlow', () => {
  it('shows the Kimi user code and emits complete after authorization', async () => {
    const wrapper = mount(DeviceAuthorizationFlow, {
      props: {
        session: {
          session_id: 'session-1', status: 'authorized', verification_uri: 'https://kimi.test/device',
          user_code: 'ABCD-EFGH', expires_in: 600, interval: 5
        },
        loading: false,
        error: '',
        remainingSeconds: 540
      }
    })

    expect(wrapper.text()).toContain('ABCD-EFGH')
    expect(wrapper.get('a').attributes('href')).toBe('https://kimi.test/device')
    await wrapper.get('button.btn-primary').trigger('click')
    expect(wrapper.emitted('complete')).toHaveLength(1)
  })

  it('starts a new device flow when no session exists', async () => {
    const wrapper = mount(DeviceAuthorizationFlow, { props: { session: null, loading: false, error: '', remainingSeconds: 0 } })
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('start')).toHaveLength(1)
  })

  it('supports provider-specific device copy and verification URLs', () => {
    const wrapper = mount(DeviceAuthorizationFlow, {
      props: {
        provider: 'openai',
        session: {
          session_id: 'session-2', status: 'pending', verification_uri: 'https://openai.test/device',
          verification_uri_complete: 'https://openai.test/device?code=ABCD',
          user_code: 'ABCD', expires_in: 600, interval: 5
        },
        loading: false,
        error: '',
        remainingSeconds: 300
      }
    })

    expect(wrapper.text()).toContain('admin.accounts.oauth.openai.device.title')
    expect(wrapper.get('a').attributes('href')).toBe('https://openai.test/device?code=ABCD')
  })
})
