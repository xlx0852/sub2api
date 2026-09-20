import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../VersionBadge.vue')
const componentSource = readFileSync(componentPath, 'utf8')

describe('VersionBadge retired GitHub update checks', () => {
  it('renders only the local build version and restart control', () => {
    expect(componentSource).toContain('v{{ currentVersion }}')
    expect(componentSource).toContain("t('version.currentVersion')")
    expect(componentSource).toContain("t('version.restartNow')")
    expect(componentSource).toContain('restartService')
  })

  it('does not probe GitHub or expose online update/rollback UI', () => {
    expect(componentSource).not.toContain('has_update')
    expect(componentSource).not.toContain('latestVersion')
    expect(componentSource).not.toContain('updateNow')
    expect(componentSource).not.toContain('/admin/system/update')
    expect(componentSource).not.toContain('/admin/system/rollback')
    expect(componentSource).not.toContain('api.github.com')
    expect(componentSource).not.toContain('checkUpdate')
  })
})
