/**
 * Display labels for lake/result column keys.
 * Keys stay as returned by search (e.g. node_id); labels are UI-only.
 *
 * Homer 7 showed HEP chunk 0x000c as CaptureID; Homer 11 stores it as node_id.
 */
const COLUMN_DISPLAY_LABELS: Record<string, string> = {
  node_id: 'Capture ID',
}

/** Human-readable column title; falls back to the raw key. */
export function columnDisplayLabel(col: string): string {
  return COLUMN_DISPLAY_LABELS[col] ?? col
}

/** Tooltip for headers / column picker (includes lake key when aliased). */
export function columnDisplayTitle(col: string): string {
  const label = columnDisplayLabel(col)
  if (label === col) return col
  return `${label} (${col})`
}
