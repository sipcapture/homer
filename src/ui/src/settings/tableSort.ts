export type SortDirection = 'asc' | 'desc'

/** Compare two cell values for table sorting (nulls last). */
export function compareForSort(a: unknown, b: unknown): number {
  const aEmpty = a === null || a === undefined || a === ''
  const bEmpty = b === null || b === undefined || b === ''
  if (aEmpty && bEmpty) return 0
  if (aEmpty) return 1
  if (bEmpty) return -1

  if (typeof a === 'boolean' && typeof b === 'boolean') {
    return Number(a) - Number(b)
  }
  if (typeof a === 'number' && typeof b === 'number') {
    if (Number.isNaN(a) && Number.isNaN(b)) return 0
    if (Number.isNaN(a)) return 1
    if (Number.isNaN(b)) return -1
    return a - b
  }
  if (typeof a === 'object' || typeof b === 'object') {
    const sa = typeof a === 'object' ? JSON.stringify(a) : String(a)
    const sb = typeof b === 'object' ? JSON.stringify(b) : String(b)
    return sa.localeCompare(sb, undefined, { numeric: true, sensitivity: 'base' })
  }
  return String(a).localeCompare(String(b), undefined, {
    numeric: true,
    sensitivity: 'base',
  })
}

export function sortItems<T>(
  items: T[],
  getValue: (item: T) => unknown,
  direction: SortDirection,
): T[] {
  const mult = direction === 'asc' ? 1 : -1
  return [...items].sort((x, y) => mult * compareForSort(getValue(x), getValue(y)))
}
