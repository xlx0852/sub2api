import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import Pagination from '../Pagination.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const mountPagination = (page = 1) => mount(Pagination, {
  props: { total: 230, page, pageSize: 20 },
  global: {
    stubs: {
      Select: true,
      Icon: true
    }
  }
})

describe('Pagination', () => {
  it('展示首段页码、省略号和末页，并突出当前页', () => {
    const wrapper = mountPagination()
    const current = wrapper.get('[aria-current="page"]')

    expect(current.text()).toBe('1')
    expect(current.classes()).toContain('bg-primary-600')
    expect(wrapper.text()).toContain('...')
    expect(wrapper.text()).toContain('12')
  })

  it('页码按钮使用独立间距而不是相邻边框重叠', () => {
    const wrapper = mountPagination(2)
    const nav = wrapper.get('nav[aria-label="Pagination"]')

    expect(nav.classes()).toContain('gap-1')
    expect(nav.classes()).not.toContain('-space-x-px')
    expect(nav.findAll('.pagination-button').length).toBeGreaterThan(3)
  })

  it('点击下一页会发出页码更新', async () => {
    const wrapper = mountPagination(2)
    const next = wrapper.get(`button[aria-label="pagination.next"]`)
    await next.trigger('click')

    expect(wrapper.emitted('update:page')).toEqual([[3]])
  })
})
