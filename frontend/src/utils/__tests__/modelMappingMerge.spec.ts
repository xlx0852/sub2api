import { describe, expect, it } from 'vitest'
import { mergeModelMappingChanges } from '../modelMappingMerge'

describe('mergeModelMappingChanges', () => {
  it('keeps unknown models, upstream additions and upstream removals on an unrelated save', () => {
    const original = { 'custom-model': 'upstream/custom', removed: 'removed' }
    const latest = { 'custom-model': 'upstream/custom-v2', 'gemini-new': 'gemini-new' }
    expect(mergeModelMappingChanges(original, original, latest)).toEqual({ mapping: latest, conflicts: [] })
  })

  it('applies deliberate local deletions and renames without resurrecting old entries', () => {
    expect(mergeModelMappingChanges(
      { old: 'x', keep: 'keep' }, { renamed: 'y', keep: 'keep' }, { old: 'x', keep: 'keep', remote: 'z' }
    )).toEqual({ mapping: { renamed: 'y', keep: 'keep', remote: 'z' }, conflicts: [] })
  })

  it('rejects conflicting writes and deletions to the same key', () => {
    const result = mergeModelMappingChanges({ m: 'a' }, { m: 'b' }, { m: 'c' })
    expect(result.conflicts).toEqual(['m'])
    expect(result.mapping.m).toBe('c')
    expect(mergeModelMappingChanges({ m: 'a' }, {}, { m: 'c' }).conflicts).toEqual(['m'])
  })

  it('accepts the same change already applied upstream', () => {
    expect(mergeModelMappingChanges({ m: 'a' }, { m: 'b' }, { m: 'b' }).conflicts).toEqual([])
  })
})
