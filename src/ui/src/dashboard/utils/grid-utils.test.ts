import { describe, expect, it } from 'vitest'
import { fitWidgetHeight, mergeLayoutIntoWidgets } from './grid-utils'

describe('mergeLayoutIntoWidgets', () => {
  const widgets = [
    { id: 'a', type: 'search', x: 0, y: 0, w: 6, h: 3 },
    { id: 'b', type: 'result', x: 6, y: 0, w: 6, h: 3 },
  ]

  it('returns null when layout positions are unchanged', () => {
    const layout = [
      { i: 'a', x: 0, y: 0, w: 6, h: 3 },
      { i: 'b', x: 6, y: 0, w: 6, h: 3 },
    ]
    expect(mergeLayoutIntoWidgets(widgets, layout)).toBeNull()
  })

  it('updates only widgets whose grid position changed', () => {
    const layout = [
      { i: 'a', x: 0, y: 0, w: 6, h: 3 },
      { i: 'b', x: 0, y: 3, w: 12, h: 4 },
    ]
    expect(mergeLayoutIntoWidgets(widgets, layout)).toEqual([
      { id: 'a', type: 'search', x: 0, y: 0, w: 6, h: 3 },
      { id: 'b', type: 'result', x: 0, y: 3, w: 12, h: 4 },
    ])
  })

  it('ignores layout entries for unknown widget ids', () => {
    const layout = [
      { i: 'a', x: 1, y: 2, w: 4, h: 5 },
      { i: 'ghost', x: 0, y: 0, w: 12, h: 1 },
    ]
    expect(mergeLayoutIntoWidgets(widgets, layout)).toEqual([
      { id: 'a', type: 'search', x: 1, y: 2, w: 4, h: 5 },
      { id: 'b', type: 'result', x: 6, y: 0, w: 6, h: 3 },
    ])
  })
})

describe('fitWidgetHeight', () => {
  it('clamps default height into available rows', () => {
    expect(fitWidgetHeight(8, 2, 5)).toBe(5)
    expect(fitWidgetHeight(1, 2, 5)).toBe(2)
    expect(fitWidgetHeight(3, 2, 0)).toBe(3)
  })
})
