import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import UserDashboardQuickActions from '../UserDashboardQuickActions.vue'

const push = vi.fn()

vi.mock('vue-router', () => ({
  useRouter: () => ({ push }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

describe('UserDashboardQuickActions', () => {
  it('shows the affiliate prompt and opens the real affiliate page when enabled', async () => {
    const wrapper = mount(UserDashboardQuickActions, {
      props: { showAffiliate: true },
    })

    expect(wrapper.text()).toContain('dashboard.affiliateCta.title')

    await wrapper.get('button').trigger('click')
    expect(push).toHaveBeenCalledWith('/affiliate')
  })

  it('hides the affiliate prompt when the feature is disabled', () => {
    const wrapper = mount(UserDashboardQuickActions)

    expect(wrapper.text()).not.toContain('dashboard.affiliateCta.title')
  })
})
