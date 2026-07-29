import { describe, expect, it } from 'vitest'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { resolve } from 'node:path'

import en from '../locales/en'
import zh from '../locales/zh'

type Messages = Record<string, unknown>

function leafKeys(value: unknown, prefix = ''): string[] {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    return prefix ? [prefix] : []
  }

  return Object.entries(value as Messages).flatMap(([key, child]) =>
    leafKeys(child, prefix ? `${prefix}.${key}` : key)
  )
}

function sourceFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const file = resolve(dir, name)
    if (statSync(file).isDirectory()) return sourceFiles(file)
    return /\.(?:ts|vue)$/.test(name) && !file.includes('/i18n/locales/') ? [file] : []
  })
}

function staticallyReferencedKeys(): string[] {
  const src = resolve(__dirname, '../..')
  const keys = new Set<string>()
  const callPattern = /(?<![\w$])(?:\$t|t|i18n\.global\.t)\(\s*(['"])([^'"\n]+)\1/g

  for (const file of sourceFiles(src)) {
    const source = readFileSync(file, 'utf8')
    for (const match of source.matchAll(callPattern)) {
      if (match[2].includes('.') && !match[2].endsWith('.')) keys.add(match[2])
    }
  }

  return [...keys].sort()
}

describe('locale completeness', () => {
  it('every statically referenced message exists in both locales', () => {
    const referenced = staticallyReferencedKeys()
    const enKeys = new Set(leafKeys(en))
    const zhKeys = new Set(leafKeys(zh))

    expect(referenced.filter((key) => !enKeys.has(key))).toEqual([])
    expect(referenced.filter((key) => !zhKeys.has(key))).toEqual([])
  })

  it('defines labels for every group platform in both locales', () => {
    const platforms = ['all', 'anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi']
    const required = platforms.map((platform) => `admin.groups.platforms.${platform}`)

    expect(required.filter((key) => !new Set(leafKeys(en)).has(key))).toEqual([])
    expect(required.filter((key) => !new Set(leafKeys(zh)).has(key))).toEqual([])
  })
})
