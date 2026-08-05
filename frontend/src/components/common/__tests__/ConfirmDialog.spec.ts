import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { createI18n } from 'vue-i18n'
import ConfirmDialog from '../ConfirmDialog.vue'

describe('ConfirmDialog', () => {
  it('renders above account drawers by default', () => {
    const i18n = createI18n({
      legacy: false,
      locale: 'en',
      messages: { en: { common: { confirm: 'Confirm', cancel: 'Cancel' } } }
    })
    mount(ConfirmDialog, {
      attachTo: document.body,
      props: {
        show: true,
        title: 'Delete plan',
        message: 'Confirm deletion'
      },
      global: {
        plugins: [i18n]
      }
    })

    const overlay = document.body.querySelector<HTMLElement>('.modal-overlay')
    expect(overlay).not.toBeNull()
    expect(overlay?.style.zIndex).toBe('100')
  })
})
