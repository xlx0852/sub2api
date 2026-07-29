import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import SemanticBadge from '../SemanticBadge.vue'

describe('SemanticBadge', () => {
  it.each([
    ['neutral', 'bg-gray-100'], ['info', 'bg-blue-100'], ['success', 'bg-emerald-100'],
    ['warning', 'bg-amber-100'], ['danger', 'bg-red-100']
  ] as const)('语义 %s 映射统一色值', (tone, className) => {
    const wrapper = mount(SemanticBadge, { props: { tone }, slots: { default: 'status' } })
    expect(wrapper.get('span').classes()).toContain(className)
  })

  it('支持紧凑非胶囊样式和状态点', () => {
    const wrapper = mount(SemanticBadge, { props: { size: 'xs', pill: false, dot: true }, slots: { default: 'idle' } })
    expect(wrapper.get('span').classes()).toContain('rounded-md')
    expect(wrapper.find('.rounded-full.bg-current').exists()).toBe(true)
  })
})
