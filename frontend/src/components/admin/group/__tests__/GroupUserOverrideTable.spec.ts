import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import GroupUserOverrideTable from '../GroupUserOverrideTable.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

describe('GroupUserOverrideTable', () => {
  it('统一展示用户信息、状态和业务数值插槽', async () => {
    const wrapper = mount(GroupUserOverrideTable, {
      props: {
        entries: [{ user_id: 7, user_email: 'u@example.com', user_name: 'user', user_status: 'active', rpm_override: 120 }],
        valueLabel: 'RPM', page: 1, pageSize: 10, total: 1
      },
      slots: { value: '<template #value="{ entry }"><span data-testid="value">{{ entry.rpm_override }}</span></template>' },
      global: { stubs: { Pagination: true, Icon: true } }
    })

    expect(wrapper.text()).toContain('u@example.com')
    expect(wrapper.get('[data-testid="value"]').text()).toBe('120')
    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('remove')).toEqual([[7]])
  })
})
