export const GRID_ROW_HEIGHT = 60
export const GRID_MARGIN = 10

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
