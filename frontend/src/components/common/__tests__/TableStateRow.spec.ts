import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import TableStateRow from '../TableStateRow.vue'

describe('TableStateRow', () => {
  it('统一表格空状态', () => {
    const wrapper = mount({ template: '<table><tbody><TableStateRow :colspan="4" empty-label="No rows" /></tbody></table>', components: { TableStateRow } }, { global: { stubs: { Icon: true } } })
    expect(wrapper.get('td').attributes('colspan')).toBe('4')
    expect(wrapper.text()).toContain('No rows')
  })

  it('统一表格加载状态', () => {
    const wrapper = mount({ template: '<table><tbody><TableStateRow :colspan="2" loading /></tbody></table>', components: { TableStateRow } }, { global: { stubs: { Icon: { template: '<i data-testid="spinner" />' } } } })
    expect(wrapper.find('[data-testid="spinner"]').exists()).toBe(true)
  })
})
