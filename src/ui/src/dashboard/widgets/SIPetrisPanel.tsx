/**
 * SIPetris — Tetris-style mini-game where each tetromino is a SIP method.
 *
 * Mapping (one piece per classic Tetris shape; chosen so the dropping
 * piece is recognisable from the SIP request line that "fits" it):
 *
 *   I  (4-in-a-row)        — INVITE   (the long call setup)
 *   O  (2x2 square)        — ACK      (small, square, terminal)
 *   T  (T-shape)           — REGISTER (binding hub)
 *   L  (L-shape)           — BYE      (call wraps up)
 *   J  (mirror L)          — CANCEL   (mirror of BYE — early teardown)
 *   S  (S-shape)           — OPTIONS  (sliding probe)
 *   Z  (Z-shape)           — PRACK    (mirror probe; provisional ack)
 *
 * Rules are vanilla Tetris — fall, soft-drop (↓), hard-drop (Space),
 * rotate (↑ / X), move (← / →) — but every cleared line shouts the SIP
 * method that filled it, and the side panel lists the SIP-style
 * "transactions" you've completed.
 *
 * Scoring follows classic Tetris (1/3/5/8 × level), level rises every
 * 10 cleared lines, drop interval shortens with level. localStorage
 * persists the best score per game id (homer_game_best_sipetris).
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { openHepStream, type HepStreamClient, type HepStreamEvent } from '@/api'
import { readBestScore, writeBestScoreIfHigher } from './gameScoreStorage'
import { readZoom, writeZoom } from './gameZoomStorage'
import { useArenaCellSize } from './useArenaCellSize'
import { ZoomControls } from './ZoomControls'
import {
  COLS,
  ROWS,
  PIECES,
  ROT0,
  rotateCW,
  collides,
  lockPiece,
  clearLines,
  scoreForClear,
  dropIntervalMs,
  makeBag,
  spawnPiece,
  emptyBoard,
  shapeForMethod,
  type ActivePiece,
  type Board,
  type BoardCell,
  type ShapeKey,
} from './tetrisCore'

const SIPETRIS_GAME_ID = 'sipetris'

/** Pixel chrome around the playfield (border + p-1 padding both sides). */
const ARENA_CHROME_PX = 10
/** Lower / upper bounds for the auto-fit cell size; the user's zoom
 *  factor is multiplied on top and re-clamped here. */
const CELL_MIN_PX = 14
const CELL_MAX_PX = 56

const SIDEBAR_LS_KEY = 'homer_sipetris_sidebar_w'
const SIDEBAR_W_MIN = 120
const SIDEBAR_W_MAX = 480
const SIDEBAR_W_DEFAULT = 180

function readSidebarWidth(): number {
  if (typeof window === 'undefined') return SIDEBAR_W_DEFAULT
  try {
    const n = Number(localStorage.getItem(SIDEBAR_LS_KEY))
    if (Number.isFinite(n)) {
      return Math.min(SIDEBAR_W_MAX, Math.max(SIDEBAR_W_MIN, Math.round(n)))
    }
  } catch {
    /* ignore */
  }
  return SIDEBAR_W_DEFAULT
}

interface ClearedLog {
  id: number
  cleared: number
  methods: string[]
  /** Score gained from this clear. */
  gain: number
}

let _logUid = 0

/** Cap of queued live SIP methods waiting to bias the next spawn. Keeps the
 *  buffer to roughly the visible Next preview; older events are dropped FIFO. */
const LIVE_METHOD_RING_CAP = 16

export default function SIPetrisPanel() {
  const [board, setBoard] = useState<Board>(() => emptyBoard())
  const [piece, setPiece] = useState<ActivePiece | null>(null)
  const [nextQueue, setNextQueue] = useState<ShapeKey[]>(() => makeBag())
  const [bagBuffer, setBagBuffer] = useState<ShapeKey[]>(() => makeBag())
  const [score, setScore] = useState(0)
  const [lines, setLines] = useState(0)
  const [level, setLevel] = useState(1)
  const [running, setRunning] = useState(false)
  const [paused, setPaused] = useState(false)
  const [gameOver, setGameOver] = useState(false)
  const [bestScore, setBestScore] = useState(0)
  const [log, setLog] = useState<ClearedLog[]>([])
  const [banner, setBanner] = useState('')
  const [liveTraffic, setLiveTraffic] = useState(false)
  const [liveState, setLiveState] = useState<'off' | 'connecting' | 'open' | 'closed'>('off')
  const [sidebarWidth, setSidebarWidth] = useState(readSidebarWidth)
  const sidebarDragRef = useRef<{
    pointerId: number
    originX: number
    originW: number
  } | null>(null)
  const [liveMethodCount, setLiveMethodCount] = useState(0)
  const [zoom, setZoom] = useState<number>(() => readZoom(SIPETRIS_GAME_ID))
  const arenaRef = useRef<HTMLDivElement | null>(null)
  const arenaWrapRef = useRef<HTMLDivElement | null>(null)
  const { cellPx } = useArenaCellSize(ROWS, COLS, {
    containerRef: arenaWrapRef,
    reserveWidth: ARENA_CHROME_PX,
    reserveHeight: ARENA_CHROME_PX,
    min: CELL_MIN_PX,
    max: CELL_MAX_PX,
    zoom,
  })
  const handleZoomChange = useCallback((next: number) => {
    setZoom(writeZoom(SIPETRIS_GAME_ID, next))
  }, [])
  // FIFO ring of recent live SIP methods (mappable to a ShapeKey). drawNext
  // pops from this ring first, falling back to the 7-bag when empty so the
  // game keeps running even if the stream stalls.
  const liveMethodRingRef = useRef<ShapeKey[]>([])
  const liveClientRef = useRef<HepStreamClient | null>(null)

  // refs let the keyboard handler and rAF tick read fresh state without
  // re-binding listeners on every state change (keeps the focus scoped
  // to the arena div clean).
  const boardRef = useRef(board)
  const pieceRef = useRef(piece)
  const nextRef = useRef(nextQueue)
  const bagRef = useRef(bagBuffer)
  const runRef = useRef(running)
  const pauseRef = useRef(paused)
  const overRef = useRef(gameOver)
  const levelRef = useRef(level)
  const linesRef = useRef(lines)
  const scoreRef = useRef(score)
  const lastDropRef = useRef(0)
  const rafRef = useRef<number>(0)
  const bannerTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => { boardRef.current = board }, [board])
  useEffect(() => { pieceRef.current = piece }, [piece])
  useEffect(() => { nextRef.current = nextQueue }, [nextQueue])
  useEffect(() => { bagRef.current = bagBuffer }, [bagBuffer])
  useEffect(() => { runRef.current = running }, [running])
  useEffect(() => { pauseRef.current = paused }, [paused])
  useEffect(() => { overRef.current = gameOver }, [gameOver])
  useEffect(() => { levelRef.current = level }, [level])
  useEffect(() => { linesRef.current = lines }, [lines])
  useEffect(() => { scoreRef.current = score }, [score])

  useEffect(() => setBestScore(readBestScore(SIPETRIS_GAME_ID)), [])

  const showBanner = useCallback((msg: string) => {
    setBanner(msg)
    if (bannerTimer.current) clearTimeout(bannerTimer.current)
    bannerTimer.current = setTimeout(() => setBanner(''), 1400)
  }, [])

  const drawNext = useCallback((): ShapeKey => {
    // Live mode shortcut: if a real SIP method is queued, use it as the
    // next piece and bypass the bag entirely. The bag still ticks in the
    // background through the visible "Next" preview, so toggling Live off
    // lands smoothly on a fresh bag.
    const liveRing = liveMethodRingRef.current
    if (liveRing.length > 0) {
      const head = liveRing.shift()!
      setLiveMethodCount(liveRing.length)
      return head
    }

    let q = nextRef.current
    let buf = bagRef.current
    if (q.length === 0) {
      q = buf
      buf = makeBag()
    }
    const head = q[0]!
    const newQ = q.slice(1)
    if (newQ.length < 3) {
      // Always keep at least 3 visible upcoming pieces so the side
      // panel "Next" preview never blanks out between bags.
      newQ.push(...buf)
      buf = makeBag()
    }
    nextRef.current = newQ
    bagRef.current = buf
    setNextQueue(newQ)
    setBagBuffer(buf)
    return head
  }, [])

  const handleGameOver = useCallback(() => {
    runRef.current = false
    overRef.current = true
    setRunning(false)
    setGameOver(true)
    setBestScore(writeBestScoreIfHigher(SIPETRIS_GAME_ID, scoreRef.current))
  }, [])

  const lockAndAdvance = useCallback(
    (locked: ActivePiece) => {
      const baked = lockPiece(boardRef.current, locked)
      const { board: cleared, cleared: lineCount, methods } = clearLines(baked)
      let gainedScore = 0
      if (lineCount > 0) {
        gainedScore = scoreForClear(lineCount, levelRef.current)
        const newLines = linesRef.current + lineCount
        const newLevel = Math.max(1, Math.floor(newLines / 10) + 1)
        linesRef.current = newLines
        scoreRef.current = scoreRef.current + gainedScore
        levelRef.current = newLevel
        setLines(newLines)
        setLevel(newLevel)
        setScore(scoreRef.current)
        const friendly = methods.length === 1 ? methods[0]! : `${methods.join(' + ')}`
        showBanner(
          `${lineCount === 4 ? 'TETRIS' : `${lineCount} line${lineCount === 1 ? '' : 's'}`} · ${friendly} · +${gainedScore}`,
        )
        setLog((prev) => {
          const next: ClearedLog = { id: ++_logUid, cleared: lineCount, methods, gain: gainedScore }
          return [next, ...prev].slice(0, 8)
        })
      } else {
        // Soft "lock without clear" feedback in the banner so players
        // notice the SIP method they just landed.
        showBanner(`Locked ${PIECES[locked.shape].method}`)
      }
      boardRef.current = cleared
      setBoard(cleared)
      const nextShape = drawNext()
      const newPiece = spawnPiece(nextShape)
      if (collides(cleared, newPiece)) {
        pieceRef.current = null
        setPiece(null)
        handleGameOver()
        return
      }
      pieceRef.current = newPiece
      setPiece(newPiece)
    },
    [drawNext, handleGameOver, showBanner],
  )

  const tryMove = useCallback(
    (dx: number, dy: number) => {
      const cur = pieceRef.current
      if (!cur || !runRef.current || pauseRef.current || overRef.current) return false
      const moved: ActivePiece = { ...cur, x: cur.x + dx, y: cur.y + dy }
      if (collides(boardRef.current, moved)) return false
      pieceRef.current = moved
      setPiece(moved)
      return true
    },
    [],
  )

  const tryRotate = useCallback(() => {
    const cur = pieceRef.current
    if (!cur || !runRef.current || pauseRef.current || overRef.current) return
    if (cur.shape === 'O') return // O tetromino looks identical when rotated
    const rotated: ActivePiece = { ...cur, cells: rotateCW(cur.cells) }
    // Tiny wall-kick: try ±1 / ±2 horizontal nudges if rotation collides.
    for (const dx of [0, -1, 1, -2, 2]) {
      const test = { ...rotated, x: rotated.x + dx }
      if (!collides(boardRef.current, test)) {
        pieceRef.current = test
        setPiece(test)
        return
      }
    }
  }, [])

  const softDrop = useCallback(() => {
    const cur = pieceRef.current
    if (!cur || !runRef.current || pauseRef.current || overRef.current) return
    if (tryMove(0, 1)) {
      // Each soft-drop cell awards 1 point, like in classic NES Tetris.
      scoreRef.current += 1
      setScore(scoreRef.current)
      lastDropRef.current = performance.now()
    } else {
      lockAndAdvance(cur)
    }
  }, [lockAndAdvance, tryMove])

  const hardDrop = useCallback(() => {
    let cur = pieceRef.current
    if (!cur || !runRef.current || pauseRef.current || overRef.current) return
    let dropped = 0
    while (true) {
      const next: ActivePiece = { ...cur, y: cur.y + 1 }
      if (collides(boardRef.current, next)) break
      cur = next
      dropped++
    }
    scoreRef.current += dropped * 2
    setScore(scoreRef.current)
    pieceRef.current = cur
    setPiece(cur)
    lockAndAdvance(cur)
  }, [lockAndAdvance])

  const tick = useCallback(
    (now: number) => {
      if (!runRef.current || overRef.current) {
        rafRef.current = 0
        return
      }
      if (!pauseRef.current) {
        const interval = dropIntervalMs(levelRef.current)
        if (now - lastDropRef.current >= interval) {
          const cur = pieceRef.current
          if (cur) {
            const moved: ActivePiece = { ...cur, y: cur.y + 1 }
            if (collides(boardRef.current, moved)) {
              lockAndAdvance(cur)
            } else {
              pieceRef.current = moved
              setPiece(moved)
            }
            lastDropRef.current = now
          }
        }
      } else {
        // Reset the drop clock to "now" while paused so resuming
        // doesn't immediately drop the piece by an accumulated tick.
        lastDropRef.current = now
      }
      rafRef.current = requestAnimationFrame(tick)
    },
    [lockAndAdvance],
  )

  const startGame = useCallback(() => {
    if (bannerTimer.current) clearTimeout(bannerTimer.current)
    setBanner('')
    setBoard(emptyBoard())
    boardRef.current = emptyBoard()
    setScore(0)
    scoreRef.current = 0
    setLines(0)
    linesRef.current = 0
    setLevel(1)
    levelRef.current = 1
    setLog([])
    setGameOver(false)
    overRef.current = false
    setPaused(false)
    pauseRef.current = false
    setRunning(true)
    runRef.current = true
    const initBag = makeBag()
    const seedQ = initBag.concat(makeBag())
    nextRef.current = seedQ
    bagRef.current = makeBag()
    setNextQueue(seedQ)
    setBagBuffer(bagRef.current)
    const head = drawNext()
    const p0 = spawnPiece(head)
    pieceRef.current = p0
    setPiece(p0)
    lastDropRef.current = performance.now()
    cancelAnimationFrame(rafRef.current)
    rafRef.current = requestAnimationFrame(tick)
    // Re-focus the arena so keyboard input reaches it after Restart.
    setTimeout(() => arenaRef.current?.focus(), 0)
  }, [drawNext, tick])

  useEffect(() => () => {
    cancelAnimationFrame(rafRef.current)
    if (bannerTimer.current) clearTimeout(bannerTimer.current)
  }, [])

  // Live SIP stream → bias the next piece. Mirrors the contract used by
  // Packet Defender: the WebSocket is opened (with auto-reconnect inside
  // the client), only metadata frames are needed, and we only enqueue
  // methods we have a tetromino for. Non-mappable methods (UPDATE,
  // NOTIFY…) are ignored so the bag stays valid.
  useEffect(() => {
    if (!liveTraffic) {
      liveClientRef.current?.close()
      liveClientRef.current = null
      liveMethodRingRef.current = []
      setLiveMethodCount(0)
      setLiveState('off')
      return
    }
    setLiveState('connecting')
    const client = openHepStream(
      { proto: 1, history: 20 },
      {
        onOpen: () => setLiveState('open'),
        onClose: () => setLiveState('closed'),
        onError: () => {
          /* surfaced via state; backoff happens inside the client */
        },
        onEvent: (evt: HepStreamEvent) => {
          const sk = shapeForMethod(evt.sip?.method)
          if (!sk) return
          const ring = liveMethodRingRef.current
          ring.push(sk)
          if (ring.length > LIVE_METHOD_RING_CAP) {
            ring.splice(0, ring.length - LIVE_METHOD_RING_CAP)
          }
          setLiveMethodCount(ring.length)
        },
      },
    )
    liveClientRef.current = client
    return () => {
      client.close()
      liveClientRef.current = null
      liveMethodRingRef.current = []
      setLiveMethodCount(0)
    }
  }, [liveTraffic])

  const handleKey = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      if (!runRef.current || overRef.current) return
      // Always allow Pause toggle even when paused, but block movement.
      if (e.key === 'p' || e.key === 'P') {
        e.preventDefault()
        setPaused((p) => !p)
        return
      }
      if (pauseRef.current) return
      switch (e.key) {
        case 'ArrowLeft':
          e.preventDefault()
          tryMove(-1, 0)
          break
        case 'ArrowRight':
          e.preventDefault()
          tryMove(1, 0)
          break
        case 'ArrowDown':
          e.preventDefault()
          softDrop()
          break
        case 'ArrowUp':
        case 'x':
        case 'X':
          e.preventDefault()
          tryRotate()
          break
        case ' ':
        case 'Spacebar':
          e.preventDefault()
          hardDrop()
          break
        default:
          break
      }
    },
    [hardDrop, softDrop, tryMove, tryRotate],
  )

  // Composite render of board + active piece into a flat ShapeKey | '' grid
  // so the JSX stays a simple double map.
  const renderedBoard = useMemo<BoardCell[][]>(() => {
    const out = board.map((row) => [...row])
    if (piece) {
      for (let r = 0; r < piece.cells.length; r++) {
        for (let c = 0; c < piece.cells[r]!.length; c++) {
          if (!piece.cells[r]![c]) continue
          const x = piece.x + c
          const y = piece.y + r
          if (y < 0 || y >= ROWS || x < 0 || x >= COLS) continue
          out[y]![x] = piece.shape
        }
      }
    }
    return out
  }, [board, piece])

  const upcoming = nextQueue.slice(0, 3)

  const onSidebarResizePointerDown = useCallback(
    (e: React.PointerEvent<HTMLDivElement>) => {
      e.preventDefault()
      e.stopPropagation()
      e.currentTarget.setPointerCapture(e.pointerId)
      sidebarDragRef.current = {
        pointerId: e.pointerId,
        originX: e.clientX,
        originW: sidebarWidth,
      }
    },
    [sidebarWidth],
  )

  const onSidebarResizePointerMove = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    const d = sidebarDragRef.current
    if (!d || e.pointerId !== d.pointerId) return
    const w = d.originW + (e.clientX - d.originX)
    setSidebarWidth(Math.min(SIDEBAR_W_MAX, Math.max(SIDEBAR_W_MIN, w)))
  }, [])

  const onSidebarResizePointerUp = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    const d = sidebarDragRef.current
    if (!d || e.pointerId !== d.pointerId) return
    sidebarDragRef.current = null
    try {
      e.currentTarget.releasePointerCapture(e.pointerId)
    } catch {
      /* not captured */
    }
    setSidebarWidth((prev) => {
      const c = Math.min(SIDEBAR_W_MAX, Math.max(SIDEBAR_W_MIN, Math.round(prev)))
      try {
        localStorage.setItem(SIDEBAR_LS_KEY, String(c))
      } catch {
        /* ignore */
      }
      return c
    })
  }, [])

  const onSidebarResizeDoubleClick = useCallback(() => {
    setSidebarWidth(SIDEBAR_W_DEFAULT)
    try {
      localStorage.setItem(SIDEBAR_LS_KEY, String(SIDEBAR_W_DEFAULT))
    } catch {
      /* ignore */
    }
  }, [])

  return (
    <div className="flex h-full min-h-0 select-none flex-col font-mono text-xs text-foreground">
      <div className="mb-1 flex flex-wrap items-center gap-3 gap-y-1 border-b border-border/60 pb-1">
        <div className="flex items-end gap-2">
          <div className="rounded-md border border-primary/40 bg-primary/5 px-2.5 py-1">
            <div className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Score</div>
            <div className="text-lg font-bold leading-tight tabular-nums text-primary">{score}</div>
          </div>
          <Stat label="Best" value={String(bestScore)} />
        </div>
        <Stat label="Lines" value={String(lines)} />
        <Stat label="Level" value={String(level)} />
        <div className="min-w-[4px] flex-1" />
        <ZoomControls
          zoom={zoom}
          onZoomChange={handleZoomChange}
          title="Playfield zoom — auto-fits the widget; click % to reset"
        />
        <Button
          type="button"
          variant={liveTraffic ? 'default' : 'outline'}
          size="sm"
          className="h-8 shrink-0 text-xs"
          onClick={() => setLiveTraffic((v) => !v)}
          title={
            liveTraffic
              ? `Real SIP method events bias the next piece (stream ${liveState}; ${liveMethodCount} queued)`
              : 'Stream real SIP traffic from homer-core and let live methods choose the next tetromino'
          }
        >
          {liveTraffic
            ? liveState === 'open'
              ? `Live ● on${liveMethodCount > 0 ? ` (${liveMethodCount})` : ''}`
              : `Live ${liveState}`
            : 'Live ○ off'}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-8 shrink-0 text-xs"
          disabled={!running || gameOver}
          onClick={() => setPaused((p) => !p)}
        >
          {paused ? 'Resume' : 'Pause'}
        </Button>
        <Button
          type="button"
          variant="secondary"
          size="sm"
          className="h-8 shrink-0 text-xs"
          onClick={startGame}
        >
          {running ? 'Restart' : 'Start'}
        </Button>
      </div>

      <div className="flex min-h-0 flex-1 items-stretch overflow-hidden">
        {/* Arena wrapper takes the leftover horizontal space; the
            ResizeObserver attached to it drives `cellPx` so the
            playfield grows with the dashboard tile (and scrolls
            internally when the user zooms past the auto-fit). */}
        <div
          ref={arenaWrapRef}
          className="flex min-h-0 min-w-0 flex-1 items-center justify-center overflow-auto"
        >
          <div
            ref={arenaRef}
            tabIndex={0}
            onKeyDown={handleKey}
            className="relative flex-shrink-0 rounded-lg border border-border bg-muted/40 p-1 outline-none focus:ring-1 focus:ring-primary/50"
            style={{ width: COLS * cellPx + 8, height: ROWS * cellPx + 8 }}
            aria-label="SIPetris board"
          >
          {paused && running && !gameOver && (
            <div className="absolute inset-0 z-30 flex items-center justify-center rounded-lg bg-background/55 backdrop-blur-[1px]">
              <span className="rounded-md border border-border bg-card px-3 py-1.5 text-center text-xs font-semibold shadow-sm">
                Paused · Score {score}
              </span>
            </div>
          )}
          {!running && !gameOver && (
            <div className="pointer-events-none absolute inset-0 z-20 flex items-center justify-center rounded-lg bg-background/55 px-3 text-center text-[11px] text-muted-foreground">
              <div>
                <div className="font-semibold text-foreground">SIPetris</div>
                <div className="mt-1">Each tetromino is a SIP method.</div>
                <div className="mt-1">← → move · ↑/X rotate · ↓ soft drop · Space hard drop · P pause</div>
              </div>
            </div>
          )}
          {banner && (
            <div className="pointer-events-none absolute left-1/2 top-1.5 z-20 -translate-x-1/2 whitespace-nowrap rounded-md border border-border bg-card px-3 py-1 text-[11px] font-medium shadow-sm">
              {banner}
            </div>
          )}
          {gameOver && (
            <div className="absolute inset-0 z-30 flex items-center justify-center rounded-lg bg-background/70 backdrop-blur-[2px]">
              <div className="max-w-xs rounded-lg border border-border bg-card p-5 text-center shadow-lg">
                <div className="text-base font-semibold">Stack collapsed</div>
                <div className="mt-1 text-muted-foreground">503 Service Unavailable</div>
                <div className="mt-3 text-sm">
                  Score: <b className="tabular-nums">{score}</b>
                  {bestScore > 0 ? (
                    <span className="text-muted-foreground">
                      {' '}
                      · Best: <b className="tabular-nums text-foreground">{bestScore}</b>
                    </span>
                  ) : null}
                </div>
                <div className="mt-1 text-muted-foreground">
                  Level {level} · {lines} lines
                </div>
                <Button type="button" className="mt-4" size="sm" onClick={startGame}>
                  Play again
                </Button>
              </div>
            </div>
          )}

          <div
            className="grid"
            style={{
              gridTemplateColumns: `repeat(${COLS}, ${cellPx}px)`,
              gridTemplateRows: `repeat(${ROWS}, ${cellPx}px)`,
              gap: 0,
            }}
          >
            {renderedBoard.map((row, ry) =>
              row.map((cell, cx) => {
                const def = cell && cell !== 'G' ? PIECES[cell] : null
                return (
                  <div
                    key={`${ry}-${cx}`}
                    className={cn(
                      'border border-black/5 dark:border-white/5',
                      cell ? '' : 'bg-card/40',
                    )}
                    style={
                      def
                        ? {
                            background: def.color,
                            borderColor: def.border,
                            boxShadow: 'inset 0 0 0 1px rgba(255,255,255,0.25)',
                          }
                        : undefined
                    }
                    aria-label={def ? def.method : undefined}
                  />
                )
              }),
            )}
          </div>
          </div>
        </div>

        <div
          role="separator"
          aria-orientation="vertical"
          aria-label="Resize side panel"
          title="Drag to resize · double-click to reset"
          onPointerDown={onSidebarResizePointerDown}
          onPointerMove={onSidebarResizePointerMove}
          onPointerUp={onSidebarResizePointerUp}
          onPointerCancel={onSidebarResizePointerUp}
          onDoubleClick={onSidebarResizeDoubleClick}
          className="group relative z-10 w-3 shrink-0 cursor-col-resize touch-none select-none"
        >
          <div className="absolute inset-y-1 left-1/2 w-px -translate-x-1/2 bg-border group-hover:bg-primary/60 group-active:bg-primary" />
        </div>

        <div
          className="flex min-h-0 shrink-0 flex-col gap-2 overflow-hidden"
          style={{ width: sidebarWidth }}
        >
          <div className="shrink-0 rounded-lg border border-border bg-card/60 p-1.5 sm:p-2">
            <div className="mb-0.5 text-[9px] font-semibold uppercase tracking-wider text-muted-foreground sm:mb-1 sm:text-[10px]">
              Next
            </div>
            <div className="flex flex-col gap-1.5 sm:gap-2">
              {upcoming.map((sh, idx) => (
                <NextPreview key={`${sh}-${idx}`} shape={sh} />
              ))}
            </div>
          </div>

          <div className="flex min-h-0 flex-1 flex-col rounded-lg border border-border bg-card/60 p-1.5 sm:p-2">
            <div className="mb-0.5 text-[9px] font-semibold uppercase tracking-wider text-muted-foreground sm:mb-1 sm:text-[10px]">
              Last clears
            </div>
            <div className="min-h-0 flex-1 overflow-auto pr-0.5 sm:pr-1">
              {log.length === 0 ? (
                <div className="text-[9px] italic text-muted-foreground sm:text-[10px]">
                  No transactions completed yet.
                </div>
              ) : (
                <ul className="space-y-1 text-[9px] sm:text-[10px]">
                  {log.map((entry) => (
                    <li
                      key={entry.id}
                      className="flex items-start justify-between gap-2 rounded border border-border/40 bg-background/40 px-1.5 py-1"
                    >
                      <span className="truncate">
                        <b
                          className={
                            entry.cleared === 4 ? 'text-amber-600 dark:text-amber-400' : 'text-foreground'
                          }
                        >
                          {entry.cleared === 4 ? 'TETRIS' : `${entry.cleared} line${entry.cleared === 1 ? '' : 's'}`}
                        </b>
                        <span className="text-muted-foreground"> · {entry.methods.join(', ')}</span>
                      </span>
                      <span className="shrink-0 tabular-nums text-primary">+{entry.gain}</span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </div>

          <div className="shrink-0 rounded-lg border border-border/60 bg-card/30 p-1.5 sm:p-2">
            <div className="mb-0.5 text-[9px] font-semibold uppercase tracking-wider text-muted-foreground sm:mb-1 sm:text-[10px]">
              Method legend
            </div>
            <ul className="grid grid-cols-1 gap-0.5 text-[9px] sm:text-[10px]">
              {(Object.keys(PIECES) as ShapeKey[]).map((sh) => (
                <li key={sh} className="flex items-center gap-1.5">
                  <span
                    className="inline-block h-2.5 w-2.5 rounded-sm"
                    style={{ background: PIECES[sh].color, border: `1px solid ${PIECES[sh].border}` }}
                  />
                  <span className="font-semibold">{PIECES[sh].method}</span>
                  <span className="text-muted-foreground">({sh})</span>
                </li>
              ))}
            </ul>
          </div>
        </div>
      </div>

      <div className="mt-1 flex items-center justify-between text-[10px] text-muted-foreground">
        <span>Click the board to focus, then use arrow keys.</span>
        <span>Each cleared line is a SIP transaction · Tetris = 4-method burst.</span>
      </div>
    </div>
  )
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <div className="text-[10px] text-muted-foreground">{label}</div>
      <div className="text-base font-semibold">{value}</div>
    </div>
  )
}

function NextPreview({ shape }: { shape: ShapeKey }) {
  const def = PIECES[shape]
  const grid = ROT0[shape]
  return (
    <div className="flex items-center gap-2">
      <div
        className="grid shrink-0"
        style={{ gridTemplateColumns: `repeat(4, 10px)`, gridTemplateRows: `repeat(2, 10px)`, gap: 1 }}
      >
        {grid.slice(0, 2).map((row, ry) =>
          row.map((c, cx) => (
            <div
              key={`${ry}-${cx}`}
              className="rounded-[2px]"
              style={
                c
                  ? { background: def.color, border: `1px solid ${def.border}` }
                  : { background: 'transparent' }
              }
            />
          )),
        )}
      </div>
      <div className="min-w-0">
        <div className="text-[10px] font-semibold leading-none sm:text-[11px]">{def.method}</div>
        <div className="truncate text-[8px] text-muted-foreground sm:text-[9px]">{def.blurb}</div>
      </div>
    </div>
  )
}
