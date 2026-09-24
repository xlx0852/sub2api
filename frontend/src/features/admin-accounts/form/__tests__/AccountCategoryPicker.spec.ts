import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountCategoryPicker from '../AccountCategoryPicker.vue'
import { platformCategoryPicker } from '../registry'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('AccountCategoryPicker', () => {
  it('anthropic 只剩 APIKey 时渲染静态卡片，不再给出可点击的选择器', () => {
    const wrapper = mount(AccountCategoryPicker, {
      props: { platform: 'anthropic', modelValue: 'apikey' }
    })

    expect(wrapper.findAll('button')).toHaveLength(0)
    expect(wrapper.find('[data-tour="account-form-type"]').exists()).toBe(true)
    // 仍在展示（唯一）类别名称，避免用户看不到账号类型
    expect(wrapper.text()).toContain('admin.accounts.claudeConsole')
    // 静态卡片不带 hover / pointer 交互
    expect(wrapper.find('[data-tour="account-form-type"]').classes()).not.toContain('cursor-pointer')
    expect(wrapper.find('[data-tour="account-form-type"]').classes()).toContain('cursor-default')
  })

  it('kimi 的单一类别同样走静态卡片', () => {
    const wrapper = mount(AccountCategoryPicker, {
      props: { platform: 'kimi', modelValue: 'oauth-based' }
    })

    expect(wrapper.findAll('button')).toHaveLength(0)
    expect(wrapper.find('[data-tour="account-form-type"]').exists()).toBe(true)
  })

  it('多类别平台（openai）保留可点击按钮并照常派发选择', async () => {
    const options = platformCategoryPicker('openai')!.options
    expect(options.length).toBeGreaterThan(1)

    const wrapper = mount(AccountCategoryPicker, {
      props: { platform: 'openai', modelValue: 'oauth-based' }
    })

    const buttons = wrapper.findAll('button')
    expect(buttons).toHaveLength(options.length)

    await buttons[1].trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([options[1].id])
  })
})
