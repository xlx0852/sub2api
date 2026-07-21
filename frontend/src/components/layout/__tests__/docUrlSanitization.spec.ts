import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const headerSource = readFileSync(resolve(dir, '../AppHeader.vue'), 'utf8')
const homeViewSource = readFileSync(resolve(dir, '../../../views/HomeView.vue'), 'utf8')
const keyUsageViewSource = readFileSync(resolve(dir, '../../../views/KeyUsageView.vue'), 'utf8')

describe('doc_url sanitization', () => {
  it('AppHeader imports sanitizeUrl', () => {
    expect(headerSource).toContain("import { sanitizeUrl } from '@/utils/url'")
  })

  it('AppHeader applies sanitizeUrl to docUrl', () => {
    expect(headerSource).toContain('sanitizeUrl(appStore.docUrl)')
  })

  it('HomeView imports sanitizeUrl', () => {
    expect(homeViewSource).toContain("import { sanitizeUrl } from '@/utils/url'")
  })

  it('HomeView applies sanitizeUrl to docUrl', () => {
    expect(homeViewSource).toContain('sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl')
  })

  it('KeyUsageView imports sanitizeUrl', () => {
    expect(keyUsageViewSource).toContain("import { sanitizeUrl } from '@/utils/url'")
  })

  it('KeyUsageView applies sanitizeUrl to docUrl', () => {
    expect(keyUsageViewSource).toContain('sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl')
  })
})

describe('doc_url fallback to built-in /docs', () => {
  it('HomeView falls back to /docs when external doc_url is empty', () => {
    expect(homeViewSource).toContain("externalDocUrl.value || '/docs'")
    expect(homeViewSource).toContain('isExternalDocUrl')
  })

  it('AppHeader falls back to /docs when external doc_url is empty', () => {
    expect(headerSource).toContain("externalDocUrl.value || '/docs'")
    expect(headerSource).toContain('isExternalDocUrl')
  })

  it('KeyUsageView falls back to /docs when external doc_url is empty', () => {
    expect(keyUsageViewSource).toContain("externalDocUrl.value || '/docs'")
    expect(keyUsageViewSource).toContain('isExternalDocUrl')
  })
})

