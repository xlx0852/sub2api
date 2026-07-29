import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import CapacityBadge from '../CapacityBadge.vue'

describe('CapacityBadge', () => {
  it.each([
    { current: 0, max: 10, color: 'bg-gray-100' },
    { current: 3, max: 10, color: 'bg-yellow-100' },
    { current: 10, max: 10, color: 'bg-red-100' }
  ])('容量 $current / $max 使用统一状态色', ({ current, max, color }) => {
    const wrapper = mount(CapacityBadge, {
      props: { current, max },
      slots: { default: '<svg data-testid="capacity-icon" />' }
    })

    expect(wrapper.get('span').classes()).toContain('min-h-7')
    expect(wrapper.get('span').classes()).toContain('rounded-lg')
    expect(wrapper.get('span').classes()).toContain(color)
    expect(wrapper.text()).toContain(`${current}/${max}`)
  })

  it('允许业务场景覆盖自动状态色', () => {
    const wrapper = mount(CapacityBadge, {
      props: { current: '$8', max: '$10', colorClass: 'bg-orange-100 text-orange-700' }
    })

    expect(wrapper.get('span').classes()).toContain('bg-orange-100')
    expect(wrapper.text()).toContain('$8/$10')
  })
})
