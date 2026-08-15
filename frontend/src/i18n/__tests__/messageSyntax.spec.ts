import { baseCompile } from '@intlify/message-compiler'
import { describe, expect, it } from 'vitest'

import enAdminSettings from '../locales/en/admin/settings'
import zhAdminSettings from '../locales/zh/admin/settings'

describe.each([
  ['en', enAdminSettings],
  ['zh', zhAdminSettings]
] as const)('%s admin settings message syntax', (locale, adminSettings) => {
  it('compiles every message with the runtime message grammar', () => {
    const messages: Array<[string, string]> = []
    const visit = (value: unknown, path: string): void => {
      if (typeof value === 'string') {
        messages.push([path, value])
        return
      }
      if (value && typeof value === 'object') {
        for (const [key, child] of Object.entries(value)) {
          visit(child, path ? `${path}.${key}` : key)
        }
      }
    }
    visit(adminSettings, '')

    for (const [path, message] of messages) {
      expect(() => baseCompile(message), path).not.toThrow()
    }
  })
})
