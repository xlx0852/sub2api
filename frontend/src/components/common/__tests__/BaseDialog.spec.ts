import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import BaseDialog from '../BaseDialog.vue'

describe('BaseDialog', () => {
  it('renders drawer variant on the right rail', async () => {
    const wrapper = mount(BaseDialog, {
      props: {
        show: true,
        title: 'Edit',
        variant: 'drawer',
        width: 'wide'
      },
      slots: {
        default: '<div data-testid="body">body</div>',
        footer: '<button type="button">ok</button>'
      },
      attachTo: document.body
    })

    await nextTick()

    const overlay = document.querySelector('[data-testid="base-dialog-overlay"]')
    const panel = document.querySelector('[data-testid="base-dialog-panel"]')
    expect(overlay).not.toBeNull()
    expect(overlay?.getAttribute('data-variant')).toBe('drawer')
    expect(overlay?.className).toContain('modal-overlay--drawer')
    expect(panel?.className).toContain('modal-content--drawer')
    expect(document.body.textContent).toContain('body')

    wrapper.unmount()
  })

  it('keeps centered modal as the default variant', async () => {
    const wrapper = mount(BaseDialog, {
      props: {
        show: true,
        title: 'Confirm'
      },
      attachTo: document.body
    })
    await nextTick()

    const overlay = document.querySelector('[data-testid="base-dialog-overlay"]')
    expect(overlay?.getAttribute('data-variant')).toBe('modal')
    expect(overlay?.className).not.toContain('modal-overlay--drawer')

    wrapper.unmount()
  })
})
