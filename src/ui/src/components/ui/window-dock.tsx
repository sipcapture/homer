import { useEffect } from 'react'
import { createPortal } from 'react-dom'
import { SquareIcon, XIcon } from 'lucide-react'
import { useWindowRegistry } from '@/dashboard/stores/window-registry'

const DOCK_HEIGHT = 40

export function WindowDock() {
  const minimized = useWindowRegistry((s) => s.minimized)
  const restore = useWindowRegistry((s) => s.restore)
  const remove = useWindowRegistry((s) => s.remove)

  useEffect(() => {
    if (minimized.length === 0) {
      document.documentElement.style.removeProperty('--dock-h')
      return
    }
    document.documentElement.style.setProperty('--dock-h', `${DOCK_HEIGHT}px`)
    return () => {
      document.documentElement.style.removeProperty('--dock-h')
    }
  }, [minimized.length])

  if (minimized.length === 0) return null

  return createPortal(
    <div className="pointer-events-none fixed inset-x-0 bottom-0 z-[9999] flex w-full flex-nowrap items-center gap-2 overflow-x-auto overflow-y-hidden border-t border-border bg-background/80 px-2 py-1.5 backdrop-blur">
      {minimized.map((entry) => (
        <div
          key={entry.id}
          className="pointer-events-auto flex w-[200px] shrink-0 items-stretch overflow-hidden rounded-md border border-border bg-card/90 text-[11px] shadow-md backdrop-blur"
        >
          <button
            onClick={() => {
              entry.onRestore()
              restore(entry.id)
            }}
            className="flex min-w-0 flex-1 items-center gap-2 px-2 py-1 text-left hover:bg-accent"
            title="Restore"
          >
            <SquareIcon className="h-3 w-3 shrink-0" />
            <span className="truncate">{entry.title}</span>
          </button>
          <button
            onClick={() => {
              entry.onClose()
              remove(entry.id)
            }}
            className="flex w-6 items-center justify-center border-l border-border text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
            aria-label="Close"
            title="Close"
          >
            <XIcon className="h-3 w-3" />
          </button>
        </div>
      ))}
    </div>,
    document.body,
  )
}
