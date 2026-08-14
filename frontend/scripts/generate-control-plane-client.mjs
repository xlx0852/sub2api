#!/usr/bin/env node
/**
 * Emits path/method types from openapi/control-plane.yaml.
 * Field-level schemas stay envelope-level; this file only locks the operation surface.
 */
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const frontendRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const specPath = path.resolve(frontendRoot, '../openapi/control-plane.yaml')
const outPath = path.join(frontendRoot, 'src/api/generated/control-plane.ts')
const check = process.argv.includes('--check')

const HTTP_METHODS = new Set(['get', 'post', 'put', 'delete', 'patch', 'head', 'options', 'trace'])

function parseOperations(yaml) {
  /** @type {Map<string, string[]>} */
  const paths = new Map()
  let currentPath = null
  for (const line of yaml.split(/\r?\n/)) {
    const pathMatch = line.match(/^  (\/[A-Za-z0-9/_{}-]+):$/)
    if (pathMatch) {
      currentPath = pathMatch[1]
      if (!paths.has(currentPath)) {
        paths.set(currentPath, [])
      }
      continue
    }
    const methodMatch = line.match(/^    ([a-z]+):$/)
    if (methodMatch && currentPath && HTTP_METHODS.has(methodMatch[1])) {
      const methods = paths.get(currentPath)
      if (methods && !methods.includes(methodMatch[1])) {
        methods.push(methodMatch[1])
      }
    }
  }
  return [...paths.entries()].filter(([, methods]) => methods.length > 0)
}

function renderTypes(operations) {
  const pathBlocks = operations.map(([specPath, methods]) => {
    const methodBlocks = methods.map(
      (method) => `    ${method}: {
      parameters: {
        query?: Record<string, unknown>
        header?: never
        path?: Record<string, string | number>
        cookie?: never
      }
      requestBody?: {
        content: {
          'application/json': Record<string, unknown>
        }
      }
      responses: {
        200: {
          content: {
            'application/json': Record<string, unknown>
          }
        }
      }
    }`
    )
    return `  '${specPath}': {
${methodBlocks.join('\n')}
  }`
  })

  return `/**
 * Generated from openapi/control-plane.yaml. Do not edit.
 * Regenerate with: npm run generate:api
 */
export interface paths {
${pathBlocks.join('\n')}
}

export interface components {
  schemas: Record<string, unknown>
}

export interface operations {
  [operationId: string]: unknown
}

export interface webhooks {}
`
}

function generate() {
  const yaml = readFileSync(specPath, 'utf8')
  const operations = parseOperations(yaml)
  if (operations.length === 0) {
    throw new Error(`no control-plane operations parsed from ${specPath}`)
  }
  return renderTypes(operations)
}

const next = generate()
if (!check) {
  writeFileSync(outPath, next)
  process.exit(0)
}

const dir = mkdtempSync(path.join(tmpdir(), 'control-plane-'))
try {
  const current = readFileSync(outPath, 'utf8')
  if (current !== next) {
    console.error('src/api/generated/control-plane.ts is stale. Re-run npm run generate:api')
    process.exit(1)
  }
} finally {
  rmSync(dir, { recursive: true, force: true })
}
