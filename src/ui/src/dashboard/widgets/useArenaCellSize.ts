/**
 * Compute a per-cell pixel size for a fixed-grid playfield (Tetris,
 * Netris, …) so the board fills its parent container instead of being
 * pinned to a hard-coded `CELL_PX` constant.
 *
 * The hook subscribes to a `ResizeObserver` on the supplied container
 * ref and returns the largest integer cell size that still keeps a
 * `rows × cols` grid (plus any `reserveWidth`/`reserveHeight` taken
 * by sibling chrome — opponent panel, padding, sidebar) inside the
 * container, clamped to `[min, max]`. The returned `autoFitPx` is
 * the unmodified auto-fit value; `cellPx` additionally folds in the
 * caller-supplied `zoom` factor.
 *
 * When the container hasn't laid out yet (first paint, no observer
 * callback), we fall back to `Math.max(min, Math.floor(max / 2))` so
 * something visible renders before the observer fires.
 */
import { useEffect, useState } from 'react'

export interface UseArenaCellSizeOpts {
  /** Container whose inner size we measure (typically the arena's flex parent). */
  containerRef: React.RefObject<HTMLElement | null>
  /** Pixels reserved horizontally inside the container (sidebar, separator, padding). */
  reserveWidth?: number
  /** Pixels reserved vertically inside the container (header, footer, padding). */
  reserveHeight?: number
  /** Lower clamp for the cell pixel size (auto-fit and zoomed). */
  min?: number
  /** Upper clamp for the cell pixel size (auto-fit and zoomed). */
  max?: number
  /** Multiplier applied on top of the auto-fit value. 1.0 = pure auto-fit. */
  zoom?: number
}

export interface UseArenaCellSizeResult {
  /** Auto-fit value before the zoom factor is applied (clamped to [min, max]). */
  autoFitPx: number
  /** Final per-cell size including the zoom factor (clamped to [min, max]). */
  cellPx: number
}

const DEFAULT_MIN = 10
const DEFAULT_MAX = 64

export function useArenaCellSize(
  rows: number,
  cols: number,
  opts: UseArenaCellSizeOpts,
): UseArenaCellSizeResult {
  const {
    containerRef,
    reserveWidth = 0,
    reserveHeight = 0,
    min = DEFAULT_MIN,
    max = DEFAULT_MAX,
    zoom = 1,
  } = opts

  const [autoFitPx, setAutoFitPx] = useState<number>(() => Math.max(min, Math.floor(max / 2)))

  useEffect(() => {
    const el = containerRef.current
    if (!el || typeof ResizeObserver === 'undefined') return
    const recompute = () => {
      const rect = el.getBoundingClientRect()
      const usableW = Math.max(0, rect.width - reserveWidth)
      const usableH = Math.max(0, rect.height - reserveHeight)
      const byW = cols > 0 ? usableW / cols : 0
      const byH = rows > 0 ? usableH / rows : 0
      const raw = Math.floor(Math.min(byW, byH))
      const clamped = Math.min(max, Math.max(min, raw || min))
      setAutoFitPx((prev) => (prev === clamped ? prev : clamped))
    }
    recompute()
    const ro = new ResizeObserver(recompute)
    ro.observe(el)
    return () => ro.disconnect()
  }, [containerRef, rows, cols, reserveWidth, reserveHeight, min, max])

  const cellPx = Math.min(max, Math.max(min, Math.floor(autoFitPx * zoom)))
  return { autoFitPx, cellPx }
}
