/**
 * Compact zoom selector used by the dashboard mini-games to override
 * the per-cell pixel size computed by `useArenaCellSize`. Renders as
 * a `[− 100% +]` pill: the percentage label is itself a button that
 * resets the zoom to `ZOOM_DEFAULT` (1.0).
 *
 * Kept presentational — the consumer owns the `zoom` state and is
 * expected to persist it via `gameZoomStorage` on every change.
 */
import { Button } from '@/components/ui/button'
import { ZOOM_DEFAULT, ZOOM_MAX, ZOOM_MIN, ZOOM_STEP, clampZoom } from './gameZoomStorage'

export interface ZoomControlsProps {
  zoom: number
  onZoomChange: (next: number) => void
  /** Tooltip override (defaults to a generic "Playfield zoom"). */
  title?: string
}

export function ZoomControls({ zoom, onZoomChange, title }: ZoomControlsProps) {
  const dec = () => onZoomChange(clampZoom(zoom - ZOOM_STEP))
  const inc = () => onZoomChange(clampZoom(zoom + ZOOM_STEP))
  const reset = () => onZoomChange(ZOOM_DEFAULT)
  const pct = Math.round(zoom * 100)
  return (
    <div
      className="flex items-center gap-0.5 rounded-md border border-border bg-card/60 px-0.5 py-0.5"
      title={title ?? 'Playfield zoom — click % to reset'}
    >
      <Button
        type="button"
        variant="ghost"
        size="icon-xs"
        onClick={dec}
        disabled={zoom <= ZOOM_MIN + 1e-6}
        aria-label="Zoom out"
      >
        −
      </Button>
      <button
        type="button"
        onClick={reset}
        className="min-w-[34px] rounded-sm px-1 text-center text-[10px] font-mono tabular-nums text-muted-foreground hover:bg-muted hover:text-foreground"
        aria-label="Reset zoom"
      >
        {pct}%
      </button>
      <Button
        type="button"
        variant="ghost"
        size="icon-xs"
        onClick={inc}
        disabled={zoom >= ZOOM_MAX - 1e-6}
        aria-label="Zoom in"
      >
        +
      </Button>
    </div>
  )
}
