export const GRID_ROW_HEIGHT = 60
export const GRID_MARGIN = 10
/** Canonical dashboard column count (matches dashboard config.columns). */
export const GRID_COLS = 12

export interface GridLayoutPosition {
  i: string
  x: number
  y: number
  w: number
  h: number
}

export interface WidgetGridPosition {
  id: string
  x?: number
  y?: number
  w?: number
  h?: number
}

/** How many grid rows fit in the given pixel height. */
export function computeAvailableRows(
  containerHeightPx: number,
  rowHeight = GRID_ROW_HEIGHT,
  margin = GRID_MARGIN,
): number {
  if (containerHeightPx <= 0) return 1
  return Math.floor(containerHeightPx / (rowHeight + margin))
}

/** Clamp a widget's default height so it doesn't exceed available rows. */
export function fitWidgetHeight(
  defaultH: number,
  minH: number,
  availableRows: number,
): number {
  if (availableRows <= 0) return defaultH
  return Math.max(minH, Math.min(defaultH, availableRows))
}

/**
 * Apply react-grid-layout positions onto widgets.
 * Returns a new array when any x/y/w/h changed, otherwise null.
 */
export function mergeLayoutIntoWidgets<T extends WidgetGridPosition>(
  widgets: T[],
  newLayout: GridLayoutPosition[],
): T[] | null {
  let changed = false
  const updated = widgets.map((w) => {
    const item = newLayout.find((l) => l.i === w.id)
    if (
      item &&
      (w.x !== item.x || w.y !== item.y || w.w !== item.w || w.h !== item.h)
    ) {
      changed = true
      return { ...w, x: item.x, y: item.y, w: item.w, h: item.h }
    }
    return w
  })
  return changed ? updated : null
}
