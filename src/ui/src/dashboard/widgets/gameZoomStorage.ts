/**
 * Per-game zoom factor stored in localStorage so each dashboard
 * mini-game (Netris, SIPetris, …) remembers how big the user wanted
 * the playfield. Kept separate from `gameScoreStorage` because the
 * key namespace is different and the value range/clamp is its own.
 *
 * The factor is multiplied against the auto-fit cell pixel size
 * computed by `useArenaCellSize`, so 1.0 means "fill the widget",
 * <1.0 shrinks below the auto-fit, and >1.0 grows past it (the
 * arena scrolls internally when the grid no longer fits).
 */

const PREFIX = 'homer_game_zoom_'

export const ZOOM_MIN = 0.5
export const ZOOM_MAX = 2.0
export const ZOOM_STEP = 0.1
export const ZOOM_DEFAULT = 1.0

export function clampZoom(z: number): number {
  if (!Number.isFinite(z)) return ZOOM_DEFAULT
  return Math.min(ZOOM_MAX, Math.max(ZOOM_MIN, Math.round(z * 100) / 100))
}

export function readZoom(gameId: string): number {
  try {
    const v = localStorage.getItem(PREFIX + gameId)
    if (v == null || v === '') return ZOOM_DEFAULT
    const n = Number(v)
    return clampZoom(n)
  } catch {
    return ZOOM_DEFAULT
  }
}

export function writeZoom(gameId: string, zoom: number): number {
  const next = clampZoom(zoom)
  try {
    localStorage.setItem(PREFIX + gameId, String(next))
  } catch {
    /* ignore quota / private mode */
  }
  return next
}
