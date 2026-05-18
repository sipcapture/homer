import { useCallback, useMemo, useState } from 'react'
import { sortItems, type SortDirection } from './tableSort'

export interface UseTableSortOptions {
  defaultColumn?: string
  defaultDirection?: SortDirection
}

export function useTableSort<T>(
  items: T[],
  getSortValue: (item: T, columnKey: string) => unknown,
  options?: UseTableSortOptions,
) {
  const [sortCol, setSortCol] = useState<string | null>(options?.defaultColumn ?? null)
  const [sortDir, setSortDir] = useState<SortDirection>(options?.defaultDirection ?? 'asc')

  const toggleSort = useCallback((col: string) => {
    setSortCol((prev) => {
      if (prev === col) {
        setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
        return col
      }
      setSortDir('asc')
      return col
    })
  }, [])

  const sortedItems = useMemo(() => {
    if (!sortCol) return items
    return sortItems(items, (item) => getSortValue(item, sortCol), sortDir)
  }, [items, sortCol, sortDir, getSortValue])

  return { sortCol, sortDir, toggleSort, sortedItems }
}
