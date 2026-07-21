import { describe, expect, it } from 'vitest'
import {
  docsSections,
  getDocsSection,
  renderDocsMarkdown,
  resolveDocsSectionId
} from '../docsContent'

describe('docsContent', () => {
  it('has expected quick-start sections', () => {
    const ids = docsSections.map((s) => s.id)
    expect(ids).toContain('nodejs')
    expect(ids).toContain('codex')
    expect(ids).toContain('codex-imagegen')
    expect(ids).toContain('claude')
    expect(ids).toContain('api-scripts')
    expect(ids).toContain('faq')
  })

  it('resolves hash aliases like apikey.fun docs', () => {
    expect(resolveDocsSectionId('#Nodejs')).toBe('nodejs')
    expect(resolveDocsSectionId('#Codex')).toBe('codex')
    expect(resolveDocsSectionId('#CodexImagegen')).toBe('codex-imagegen')
    expect(resolveDocsSectionId('#imagegen')).toBe('codex-imagegen')
    expect(resolveDocsSectionId('#Claude')).toBe('claude')
    expect(resolveDocsSectionId('#TraeSolo')).toBe('trae-solo')
    expect(resolveDocsSectionId('#ApiScripts')).toBe('api-scripts')
    expect(resolveDocsSectionId('#PiDroid')).toBe('pi-droid')
    expect(resolveDocsSectionId('#FAQ')).toBe('faq')
  })

  it('returns content for known section ids', () => {
    const section = getDocsSection('codex')
    expect(section.title).toContain('Codex')
    const markdown = renderDocsMarkdown(section, 'https://self-hosted.example/')
    expect(markdown).toContain('https://self-hosted.example/v1')
    expect(markdown).not.toContain('code.sicts.shop')
    expect(markdown).not.toContain('apikey.fun')
  })
})
