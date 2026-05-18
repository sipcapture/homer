import { describe, expect, it } from 'vitest'
import { compareForSort, sortItems } from './tableSort'

describe('compareForSort', () => {
  it('sorts strings with numeric awareness', () => {
    expect(compareForSort('2', '10')).toBeLessThan(0)
  })

  it('puts nulls last', () => {
    expect(compareForSort(null, 'a')).toBeGreaterThan(0)
    expect(compareForSort('a', null)).toBeLessThan(0)
  })

  it('sorts booleans false before true', () => {
    expect(compareForSort(false, true)).toBeLessThan(0)
  })
})

describe('sortItems', () => {
  it('reverses order for desc', () => {
    const rows = [{ n: 3 }, { n: 1 }, { n: 2 }]
    const sorted = sortItems(rows, (r) => r.n, 'desc')
    expect(sorted.map((r) => r.n)).toEqual([3, 2, 1])
  })
})
