import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { XIcon, Minus, Maximize2, Minimize2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { useWindowRegistry } from '@/dashboard/stores/window-registry'

let topZ = 200
let cascadeCounter = 0

/** Applied while dragging/resizing so this window stays above siblings (e.g. OTLP detail over Transaction). */
const FLOAT_INTERACTION_Z_BOOST = 60_000

interface FloatingWindowProps {
  open: boolean
  onClose: () => void
  title?: ReactNode
  headerActions?: ReactNode
  children?: ReactNode
  className?: string
  id?: string
  defaultWidth?: number
  defaultHeight?: number
  minWidth?: number
  minHeight?: number
}

export function FloatingWindow({
  open,
  onClose,
  title,
  headerActions,
  children,
  className,
  id,
  defaultWidth,
  defaultHeight,
  minWidth = 320,
  minHeight = 200,
}: FloatingWindowProps) {
  const autoId = useId()
  const resolvedId = id ?? autoId

  const cascadeSlotRef = useRef<number | null>(null)
  if (cascadeSlotRef.current === null) {
    cascadeSlotRef.current = cascadeCounter++ % 8
  }
  const cascadeOffset = cascadeSlotRef.current * 28

  const [pos, setPos] = useState<{ x: number; y: number } | null>(null)
  const [size, setSize] = useState<{ w: number; h: number } | null>(
    defaultWidth && defaultHeight ? { w: defaultWidth, h: defaultHeight } : null,
  )
  const [z, setZ] = useState(() => ++topZ)
  const [isMaximized, setIsMaximized] = useState(false)
  const [isMinimized, setIsMinimized] = useState(false)

  const dragRef = useRef<{ startX: number; startY: number; origX: number; origY: number } | null>(null)
  const resizeRef = useRef<{ startX: number; startY: number; origW: number; origH: number } | null>(null)
  const prevRectRef = useRef<{ pos: { x: number; y: number } | null; size: { w: number; h: number } | null } | null>(null)
  const windowRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) {
      setPos(null)
      setSize(defaultWidth && defaultHeight ? { w: defaultWidth, h: defaultHeight } : null)
      setIsMaximized(false)
      setIsMinimized(false)
      prevRectRef.current = null
      useWindowRegistry.getState().remove(resolvedId)
    }
  }, [open, defaultWidth, defaultHeight, resolvedId])

  useEffect(() => {
    return () => {
      useWindowRegistry.getState().remove(resolvedId)
    }
  }, [resolvedId])

  useEffect(() => {
    if (!isMaximized) return
    const onResize = () => {
      setPos({ x: 8, y: 8 })
      setSize({ w: window.innerWidth - 16, h: window.innerHeight - 16 })
    }
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [isMaximized])

  const bringToFront = useCallback(() => {
    setZ(++topZ)
  }, [])

  /** After drag/resize, assign a fresh stacking value so order stays coherent without leaving a huge gap. */
  const settleOnTop = useCallback(() => {
    setZ(++topZ)
  }, [])

  const resolvePos = useCallback(() => {
    let origin = pos
    if (!origin && windowRef.current) {
      const rect = windowRef.current.getBoundingClientRect()
      origin = { x: rect.left, y: rect.top }
      setPos(origin)
    }
    return origin
  }, [pos])

  const handlePointerDown = useCallback(
    (e: React.PointerEvent) => {
      if ((e.target as HTMLElement).closest('button')) return
      if (isMaximized) return
      // Large temporary z-index so pointer moves stay above other FloatingWindows (cursor often tracks over them).
      setZ(++topZ + FLOAT_INTERACTION_Z_BOOST)
      const origin = resolvePos()
      dragRef.current = {
        startX: e.clientX,
        startY: e.clientY,
        origX: origin?.x ?? 0,
        origY: origin?.y ?? 0,
      }
      ;(e.target as HTMLElement).setPointerCapture(e.pointerId)
      e.preventDefault()
    },
    [resolvePos, isMaximized],
  )

  const handlePointerMove = useCallback((e: React.PointerEvent) => {
    if (!dragRef.current) return
    setPos({
      x: dragRef.current.origX + (e.clientX - dragRef.current.startX),
      y: dragRef.current.origY + (e.clientY - dragRef.current.startY),
    })
  }, [])

  const handlePointerUp = useCallback(() => {
    const wasDragging = !!dragRef.current
    dragRef.current = null
    if (wasDragging) settleOnTop()
  }, [settleOnTop])

  const handleResizePointerDown = useCallback(
    (e: React.PointerEvent) => {
      setZ(++topZ + FLOAT_INTERACTION_Z_BOOST)
      resolvePos()
      const rect = windowRef.current?.getBoundingClientRect()
      resizeRef.current = {
        startX: e.clientX,
        startY: e.clientY,
        origW: rect?.width ?? defaultWidth ?? 600,
        origH: rect?.height ?? defaultHeight ?? 400,
      }
      ;(e.target as HTMLElement).setPointerCapture(e.pointerId)
      e.preventDefault()
      e.stopPropagation()
    },
    [resolvePos, defaultWidth, defaultHeight],
  )

  const handleResizePointerMove = useCallback(
    (e: React.PointerEvent) => {
      if (!resizeRef.current) return
      setSize({
        w: Math.max(minWidth, resizeRef.current.origW + (e.clientX - resizeRef.current.startX)),
        h: Math.max(minHeight, resizeRef.current.origH + (e.clientY - resizeRef.current.startY)),
      })
    },
    [minWidth, minHeight],
  )

  const handleResizePointerUp = useCallback(() => {
    const wasResizing = !!resizeRef.current
    resizeRef.current = null
    if (wasResizing) settleOnTop()
  }, [settleOnTop])

  const toggleMaximize = useCallback(() => {
    setIsMaximized((prev) => {
      if (!prev) {
        prevRectRef.current = { pos, size }
        setPos({ x: 8, y: 8 })
        setSize({ w: window.innerWidth - 16, h: window.innerHeight - 16 })
        return true
      }
      const prevRect = prevRectRef.current
      setPos(prevRect?.pos ?? null)
      setSize(
        prevRect?.size ??
          (defaultWidth && defaultHeight ? { w: defaultWidth, h: defaultHeight } : null),
      )
      prevRectRef.current = null
      return false
    })
  }, [pos, size, defaultWidth, defaultHeight])

  const handleHeaderDoubleClick = useCallback(
    (e: React.MouseEvent) => {
      if ((e.target as HTMLElement).closest('button')) return
      toggleMaximize()
    },
    [toggleMaximize],
  )

  const handleMinimize = useCallback(() => {
    setIsMinimized(true)
    useWindowRegistry.getState().minimize({
      id: resolvedId,
      title,
      onRestore: () => setIsMinimized(false),
      onClose,
    })
  }, [resolvedId, title, onClose])

  if (!open) return null
  if (isMinimized) return null

  const headerStyle = isMaximized ? 'cursor-default' : 'cursor-move'

  return createPortal(
    <div
      ref={windowRef}
      className={cn(
        'fixed flex flex-col overflow-hidden bg-popover text-popover-foreground shadow-2xl ring-1 ring-foreground/15',
        className,
      )}
      style={{
        zIndex: z,
        ...(size ? { width: size.w, height: size.h } : {}),
        ...(pos
          ? { left: pos.x, top: pos.y }
          : {
              left: `calc(50% + ${cascadeOffset}px)`,
              top: `calc(50% + ${cascadeOffset}px)`,
              transform: 'translate(-50%, -50%)',
            }),
      }}
      onPointerDownCapture={bringToFront}
    >
      <div
        className={cn(
          'flex shrink-0 items-center gap-2 border-b border-border bg-card/80 px-3 py-2',
          headerStyle,
        )}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onDoubleClick={handleHeaderDoubleClick}
      >
        <div className="min-w-0 flex-1 select-none truncate text-xs font-medium">{title}</div>
        {headerActions}
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={handleMinimize}
          className="shrink-0"
          title="Minimize"
        >
          <Minus />
          <span className="sr-only">Minimize</span>
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={toggleMaximize}
          className="shrink-0"
          title={isMaximized ? 'Restore' : 'Maximize'}
        >
          {isMaximized ? <Minimize2 /> : <Maximize2 />}
          <span className="sr-only">{isMaximized ? 'Restore' : 'Maximize'}</span>
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={onClose}
          className="shrink-0"
          title="Close"
        >
          <XIcon />
          <span className="sr-only">Close</span>
        </Button>
      </div>
      {children}
      {!isMaximized && (
        <div
          className="absolute bottom-0 right-0 z-10 h-4 w-4 cursor-nwse-resize touch-none"
          onPointerDown={handleResizePointerDown}
          onPointerMove={handleResizePointerMove}
          onPointerUp={handleResizePointerUp}
        >
          <svg
            viewBox="0 0 16 16"
            className="h-full w-full text-muted-foreground/40 hover:text-muted-foreground/70"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
          >
            <line x1="14" y1="6" x2="6" y2="14" />
            <line x1="14" y1="10" x2="10" y2="14" />
            <line x1="14" y1="14" x2="14" y2="14" />
          </svg>
        </div>
      )}
    </div>,
    document.body,
  )
}
