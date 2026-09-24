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
  it('renders quick action entries for keys, usage, and redeem', () => {
    const wrapper = mount(UserDashboardQuickActions)

    expect(wrapper.text()).toContain('dashboard.createApiKey')
    expect(wrapper.text()).toContain('dashboard.viewUsage')
    expect(wrapper.text()).toContain('dashboard.redeemCode')
  })

  it('navigates to /keys when the create-key action is clicked', async () => {
    const wrapper = mount(UserDashboardQuickActions)

    await wrapper.get('button').trigger('click')
    expect(push).toHaveBeenCalledWith('/keys')
  })
})
