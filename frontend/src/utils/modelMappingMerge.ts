export type ModelMapping = Record<string, string>

/** Apply only the drawer's explicit edits to a fresh mapping snapshot.
 * Unknown catalog IDs are ordinary keys, not disposable UI metadata.
 * A conflicting edit of the same key must be resolved by the operator.
 */
export function mergeModelMappingChanges(
  original: ModelMapping,
  edited: ModelMapping,
  latest: ModelMapping
): { mapping: ModelMapping; conflicts: string[] } {
  const mapping: ModelMapping = { ...latest }
  const conflicts: string[] = []
  const has = (value: ModelMapping, key: string) => Object.prototype.hasOwnProperty.call(value, key)
  const equal = (a: ModelMapping, b: ModelMapping, key: string) =>
    has(a, key) === has(b, key) && a[key] === b[key]

  for (const key of new Set([...Object.keys(original), ...Object.keys(edited)])) {
    if (equal(original, edited, key)) continue
    if (!equal(original, latest, key) && !equal(edited, latest, key)) {
      conflicts.push(key)
      continue
    }
    if (has(edited, key)) {
      Object.defineProperty(mapping, key, { value: edited[key], enumerable: true, configurable: true, writable: true })
    } else delete mapping[key]
  }
  return { mapping, conflicts }
}
