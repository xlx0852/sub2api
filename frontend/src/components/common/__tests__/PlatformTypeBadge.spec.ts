import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import PlatformTypeBadge from '../PlatformTypeBadge.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

describe('PlatformTypeBadge', () => {
  it('紧凑模式只保留平台和认证方式，并折叠扩展信息', () => {
    const wrapper = mount(PlatformTypeBadge, {
      props: {
        platform: 'openai',
        type: 'oauth',
        planType: 'pro',
        privacyMode: 'training_off',
        subscriptionExpiresAt: '2026-08-09T00:00:00Z',
        compact: true
      },
      global: {
        stubs: { PlatformIcon: true, Icon: true }
      }
    })

    expect(wrapper.text()).toContain('OpenAI')
    expect(wrapper.text()).toContain('OAuth')
    expect(wrapper.text()).toContain('+3')
    expect(wrapper.text()).not.toContain('Private')
    expect(wrapper.text()).not.toContain('2026-08-09')
    expect(wrapper.get('[title]').attributes('title')).toContain('Pro')
    expect(wrapper.get('[title]').attributes('title')).toContain('Private')
    expect(wrapper.get('[title]').attributes('title')).toContain('2026-08-09')
  })
})
