/**
 * ChessPanel — single-player chess widget.
 *
 * You vs. one of two opponents:
 *   - "bot": built-in minimax (depth from the level slider, 1..4),
 *     runs in a Web Worker so the UI stays responsive.
 *   - "llm": delegates each bot move to `POST /api/v4/games/chess/llm-move`,
 *     which routes through `src/mcp/llm.go` and falls back to a
 *     server-side minimax if the LLM returns garbage. The toggle is
 *     shown only when `fetchChessLLMStatus()` reports `{enabled: true}`.
 *
 * Features (MVP):
 *   - Time controls: Untimed / Bullet 1+0 / Blitz 3+2 / Rapid 10+5.
 *     Server-grade Fischer increment, ticked from the client (single
 *     player only — no anti-cheat needed here).
 *   - Side choice (white / black / random), board auto-flips for black.
 *   - Level slider (1..4) — used by bot mode and as the fallback depth
 *     hint in LLM mode.
 *   - Takeback: in bot mode pops two plies (your move + bot's reply);
 *     in LLM mode same logic.
 *   - PGN export to clipboard; localStorage persistence of an
 *     in-progress game across reloads.
 *   - End-of-game overlay (mate / stalemate / 3fold / 50-move /
 *     insufficient material / resigned / flag).
 *
 * Out of scope for this widget — see `NetChessPanel` for 2-player.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import {
  fetchChessLLMStatus,
  postChessLLMMove,
  type ChessLLMMoveResponse,
} from '@/api'
import { ChessBoard } from './ChessBoard'
import { ZoomControls } from './ZoomControls'
import { readZoom, writeZoom } from './gameZoomStorage'
import { useArenaCellSize } from './useArenaCellSize'
import {
  type ChessColor,
  type ChessSquare,
  type GameStatus,
  type SanMove,
  type UciMove,
  STARTING_FEN,
  fromPGN,
  newGame,
  parseUci,
  snapshot,
  status,
  toPGN,
  tryMove,
  turn,
  undoMove,
} from './chessCore'

const CHESS_GAME_ID = 'chess'
const PERSIST_KEY = 'homer_chess_state_v1'

const BOARD_CHROME_PX = 10
// 8x8 boards reach the auto-fit ceiling far earlier than the 10x20
// Tetris-style arenas this hook was designed for. Keep the floor low
// enough to honour ZOOM_MIN=0.5 even on tiny widgets, and the ceiling
// high enough that ZOOM_MAX=2.0 on a typical widget produces a
// visibly larger board (the wrapper scrolls when the result exceeds
// the container).
const CELL_MIN_PX = 16
const CELL_MAX_PX = 160
const SIDEBAR_PX = 220

type TimeControlId = 'untimed' | 'bullet' | 'blitz' | 'rapid'

interface TimeControl {
  id: TimeControlId
  label: string
  initialMs: number
  incrementMs: number
}

const TIME_CONTROLS: Record<TimeControlId, TimeControl> = {
  untimed: { id: 'untimed', label: 'Untimed', initialMs: 0, incrementMs: 0 },
  bullet:  { id: 'bullet',  label: 'Bullet 1+0', initialMs: 60_000,   incrementMs: 0 },
  blitz:   { id: 'blitz',   label: 'Blitz 3+2',  initialMs: 180_000,  incrementMs: 2_000 },
  rapid:   { id: 'rapid',   label: 'Rapid 10+5', initialMs: 600_000,  incrementMs: 5_000 },
}

type Mode = 'bot' | 'llm'
type SidePref = 'white' | 'black' | 'random'

interface PersistedState {
  pgn: string
  playerColor: ChessColor
  timeControlId: TimeControlId
  whiteMs: number
  blackMs: number
  level: number
  mode: Mode
}

function readPersisted(): PersistedState | null {
  if (typeof window === 'undefined') return null
  try {
    const raw = localStorage.getItem(PERSIST_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<PersistedState>
    if (typeof parsed.pgn !== 'string') return null
    return {
      pgn: parsed.pgn,
      playerColor: parsed.playerColor === 'b' ? 'b' : 'w',
      timeControlId:
        parsed.timeControlId && (TIME_CONTROLS as Record<string, TimeControl>)[parsed.timeControlId]
          ? (parsed.timeControlId as TimeControlId)
          : 'untimed',
      whiteMs: typeof parsed.whiteMs === 'number' ? parsed.whiteMs : 0,
      blackMs: typeof parsed.blackMs === 'number' ? parsed.blackMs : 0,
      level: typeof parsed.level === 'number' ? clampLevel(parsed.level) : 3,
      mode: parsed.mode === 'llm' ? 'llm' : 'bot',
    }
  } catch {
    return null
  }
}

function writePersisted(s: PersistedState) {
  if (typeof window === 'undefined') return
  try { localStorage.setItem(PERSIST_KEY, JSON.stringify(s)) } catch { /* ignore */ }
}

function clearPersisted() {
  if (typeof window === 'undefined') return
  try { localStorage.removeItem(PERSIST_KEY) } catch { /* ignore */ }
}

function clampLevel(n: number): number {
  return Math.min(4, Math.max(1, Math.floor(n)))
}

function formatClock(ms: number): string {
  if (ms <= 0) return '0:00'
  const totalSec = Math.ceil(ms / 1000)
  const m = Math.floor(totalSec / 60)
  const s = totalSec % 60
  return `${m}:${s.toString().padStart(2, '0')}`
}

interface EngineMessageIn {
  id: number
  fen: string
  depth: number
}
interface EngineMessageOut {
  id: number
  uci: string | null
  evalCp?: number
  pv?: string[]
  error?: string
}

export default function ChessPanel() {
  // --- Persisted setup --------------------------------------------------
  const initialPersisted = useMemo(readPersisted, [])

  const [timeControlId, setTimeControlId] = useState<TimeControlId>(
    initialPersisted?.timeControlId ?? 'untimed',
  )
  const timeControl = TIME_CONTROLS[timeControlId]

  const [playerColor, setPlayerColor] = useState<ChessColor>(
    initialPersisted?.playerColor ?? 'w',
  )
  const [sidePref, setSidePref] = useState<SidePref>('white')
  const [level, setLevel] = useState<number>(initialPersisted?.level ?? 3)
  const [mode, setMode] = useState<Mode>(initialPersisted?.mode ?? 'bot')

  // --- LLM availability (polled once on mount) --------------------------
  const [llmAvailable, setLlmAvailable] = useState(false)
  const [llmModel, setLlmModel] = useState<string>('')
  const [lastMoveSource, setLastMoveSource] = useState<'player' | 'bot' | 'llm' | 'fallback' | null>(null)
  useEffect(() => {
    let cancelled = false
    fetchChessLLMStatus()
      .then((s) => {
        if (cancelled) return
        setLlmAvailable(!!s.enabled)
        setLlmModel(s.model ?? '')
        // If we restored persisted mode=llm but the server says LLM
        // is now disabled, drop back to bot so the widget keeps
        // making moves.
        if (!s.enabled) setMode('bot')
      })
      .catch(() => { /* treated as disabled */ })
    return () => { cancelled = true }
  }, [])

  // --- Game state -------------------------------------------------------
  const gameRef = useRef(newGame())
  const [fen, setFen] = useState(STARTING_FEN)
  const [history, setHistory] = useState<SanMove[]>([])
  const [gameStatus, setGameStatus] = useState<GameStatus>('ongoing')
  const [lastMove, setLastMove] = useState<{ from: ChessSquare; to: ChessSquare } | null>(null)
  const [resigned, setResigned] = useState<'white' | 'black' | null>(null)
  const [flagged, setFlagged] = useState<'white' | 'black' | null>(null)
  const [thinking, setThinking] = useState(false)
  const [banner, setBanner] = useState('')
  const bannerTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  // --- Clocks -----------------------------------------------------------
  const [whiteMs, setWhiteMs] = useState<number>(initialPersisted?.whiteMs ?? timeControl.initialMs)
  const [blackMs, setBlackMs] = useState<number>(initialPersisted?.blackMs ?? timeControl.initialMs)
  const whiteRef = useRef(whiteMs)
  const blackRef = useRef(blackMs)
  useEffect(() => { whiteRef.current = whiteMs }, [whiteMs])
  useEffect(() => { blackRef.current = blackMs }, [blackMs])

  // Game-active flag derives from status — clocks tick only while ongoing
  // and history is non-empty (so the first move starts the clock).
  const gameOver = gameStatus !== 'ongoing' || resigned !== null || flagged !== null

  // --- Hydrate from persisted PGN once on mount -------------------------
  useEffect(() => {
    if (!initialPersisted) return
    try {
      const { game, history: h } = fromPGN(initialPersisted.pgn)
      gameRef.current = game
      setFen(game.fen())
      setHistory(h)
      const last = h[h.length - 1]
      if (last) setLastMove({ from: last.from, to: last.to })
      setGameStatus(status(game))
    } catch {
      // Corrupt persisted state — wipe.
      clearPersisted()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // --- Worker management ------------------------------------------------
  const workerRef = useRef<Worker | null>(null)
  const pendingReqId = useRef(0)
  const ensureWorker = useCallback(() => {
    if (workerRef.current) return workerRef.current
    try {
      const w: Worker = new Worker(new URL('./chessEngine.worker.ts', import.meta.url), { type: 'module' })
      workerRef.current = w
      return w
    } catch {
      workerRef.current = null
      return null
    }
  }, [])
  const terminateWorker = useCallback(() => {
    if (workerRef.current) {
      workerRef.current.terminate()
      workerRef.current = null
    }
  }, [])
  useEffect(() => () => terminateWorker(), [terminateWorker])

  // --- Sizing -----------------------------------------------------------
  const wrapRef = useRef<HTMLDivElement | null>(null)
  const [zoom, setZoom] = useState<number>(() => readZoom(CHESS_GAME_ID))
  const { cellPx } = useArenaCellSize(8, 8, {
    containerRef: wrapRef,
    reserveWidth: BOARD_CHROME_PX,
    reserveHeight: BOARD_CHROME_PX,
    min: CELL_MIN_PX,
    max: CELL_MAX_PX,
    zoom,
  })

  // --- Banner -----------------------------------------------------------
  const showBanner = useCallback((msg: string) => {
    setBanner(msg)
    if (bannerTimer.current) clearTimeout(bannerTimer.current)
    bannerTimer.current = setTimeout(() => setBanner(''), 1600)
  }, [])

  // --- Persist on every history / clock change --------------------------
  useEffect(() => {
    if (history.length === 0 && !resigned && !flagged) {
      clearPersisted()
      return
    }
    writePersisted({
      pgn: toPGN(gameRef.current),
      playerColor,
      timeControlId,
      whiteMs: whiteRef.current,
      blackMs: blackRef.current,
      level,
      mode,
    })
  }, [history, playerColor, timeControlId, level, mode, resigned, flagged])

  // --- Apply a move (player or engine) ----------------------------------
  const applyMove = useCallback(
    (move: UciMove, source: 'player' | 'engine'): SanMove | null => {
      const result = tryMove(gameRef.current, move)
      if (!result) return null
      setFen(gameRef.current.fen())
      setHistory((prev) => [...prev, result])
      setLastMove({ from: result.from, to: result.to })
      const newStatus = status(gameRef.current)
      setGameStatus(newStatus)
      // Fischer increment after each completed half-move.
      if (timeControl.incrementMs > 0) {
        if (result.color === 'w') setWhiteMs((v) => v + timeControl.incrementMs)
        else setBlackMs((v) => v + timeControl.incrementMs)
      }
      if (source === 'player') setLastMoveSource('player')
      else if (mode === 'bot') setLastMoveSource('bot')
      // 'llm' / 'fallback' are set in requestEngineMove based on the
      // server's source bit — leave whatever was set there.
      if (newStatus === 'checkmate') {
        showBanner(source === 'player' ? 'Checkmate — you win' : 'Checkmate — bot wins')
      } else if (newStatus !== 'ongoing') {
        showBanner(`Game over — ${humanStatus(newStatus)}`)
      } else if (result.isCheck) {
        showBanner('Check')
      }
      return result
    },
    [timeControl.incrementMs, showBanner, mode],
  )

  // --- Engine turn ------------------------------------------------------
  const requestEngineMove = useCallback(() => {
    if (gameOver) return
    if (turn(gameRef.current) === playerColor) return // not the engine's turn
    setThinking(true)
    const id = ++pendingReqId.current
    if (mode === 'bot') {
      const worker = ensureWorker()
      if (!worker) {
        // No worker available — fall back to synchronous engine.
        import('./chessEngine').then(({ search }) => {
          const r = search(gameRef.current.fen(), { depth: clampLevel(level) })
          if (id !== pendingReqId.current) return
          if (r.uci) {
            const parsed = parseUci(r.uci)
            if (parsed) applyMove(parsed, 'engine')
          }
          setThinking(false)
        })
        return
      }
      const msg: EngineMessageIn = { id, fen: gameRef.current.fen(), depth: clampLevel(level) }
      worker.onmessage = (e: MessageEvent<EngineMessageOut>) => {
        if (e.data.id !== id) return
        setThinking(false)
        if (!e.data.uci) return
        const parsed = parseUci(e.data.uci)
        if (parsed) applyMove(parsed, 'engine')
      }
      worker.postMessage(msg)
    } else {
      // LLM mode: ask the server. The server validates the model's
      // answer and falls back to its own greedy picker on failure,
      // so the response always carries a legal UCI for non-terminal
      // positions. We surface the `source` indicator in the status
      // pill so the user knows whether the LLM or the fallback
      // moved this turn.
      postChessLLMMove({
        fen: gameRef.current.fen(),
        history_pgn: toPGN(gameRef.current),
        level: clampLevel(level),
      })
        .then((resp: ChessLLMMoveResponse) => {
          if (id !== pendingReqId.current) return
          setThinking(false)
          if (!resp.uci) return
          const parsed = parseUci(resp.uci)
          if (!parsed) return
          setLastMoveSource(resp.source === 'llm' ? 'llm' : 'fallback')
          applyMove(parsed, 'engine')
        })
        .catch(() => {
          if (id !== pendingReqId.current) return
          // Network / 5xx — fall back to the local worker so the
          // game can keep going.
          setThinking(false)
          setLastMoveSource('fallback')
          const worker = ensureWorker()
          if (!worker) return
          const msg: EngineMessageIn = { id, fen: gameRef.current.fen(), depth: clampLevel(level) }
          worker.onmessage = (e: MessageEvent<EngineMessageOut>) => {
            if (e.data.id !== id) return
            if (!e.data.uci) return
            const parsed = parseUci(e.data.uci)
            if (parsed) applyMove(parsed, 'engine')
          }
          worker.postMessage(msg)
        })
    }
  }, [applyMove, ensureWorker, gameOver, level, mode, playerColor])

  // After every history change, ask the engine if it's now its turn.
  useEffect(() => {
    if (gameOver) return
    if (turn(gameRef.current) !== playerColor) {
      // small delay so the user perceives that the engine "thinks"
      const t = setTimeout(() => requestEngineMove(), 120)
      return () => clearTimeout(t)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [history, playerColor, gameOver])

  // --- Clock tick -------------------------------------------------------
  useEffect(() => {
    if (timeControl.initialMs === 0) return // untimed
    if (gameOver) return
    if (history.length === 0) return // clock starts on the first move
    const interval = setInterval(() => {
      const side = turn(gameRef.current)
      if (side === 'w') {
        const next = Math.max(0, whiteRef.current - 100)
        whiteRef.current = next
        setWhiteMs(next)
        if (next <= 0) setFlagged('white')
      } else {
        const next = Math.max(0, blackRef.current - 100)
        blackRef.current = next
        setBlackMs(next)
        if (next <= 0) setFlagged('black')
      }
    }, 100)
    return () => clearInterval(interval)
  }, [timeControl.initialMs, gameOver, history.length])

  // --- Player move ------------------------------------------------------
  const onBoardMove = useCallback(
    (move: UciMove) => {
      if (gameOver) return
      if (turn(gameRef.current) !== playerColor) return
      applyMove(move, 'player')
    },
    [applyMove, gameOver, playerColor],
  )

  // --- New game --------------------------------------------------------
  const newSession = useCallback(() => {
    pendingReqId.current++ // invalidate any in-flight engine result
    terminateWorker()
    gameRef.current = newGame()
    setFen(STARTING_FEN)
    setHistory([])
    setGameStatus('ongoing')
    setLastMove(null)
    setResigned(null)
    setFlagged(null)
    setThinking(false)
    let color: ChessColor
    if (sidePref === 'white') color = 'w'
    else if (sidePref === 'black') color = 'b'
    else color = Math.random() < 0.5 ? 'w' : 'b'
    setPlayerColor(color)
    const fresh = TIME_CONTROLS[timeControlId]
    setWhiteMs(fresh.initialMs)
    setBlackMs(fresh.initialMs)
    whiteRef.current = fresh.initialMs
    blackRef.current = fresh.initialMs
    clearPersisted()
  }, [sidePref, terminateWorker, timeControlId])

  // --- Resign -----------------------------------------------------------
  const resign = useCallback(() => {
    if (gameOver) return
    pendingReqId.current++
    setResigned(playerColor === 'w' ? 'white' : 'black')
    showBanner('You resigned')
  }, [gameOver, playerColor, showBanner])

  // --- Takeback ---------------------------------------------------------
  const takeback = useCallback(() => {
    if (history.length === 0) return
    pendingReqId.current++ // cancel any in-flight engine reply
    // Pop one ply, and if it's now the engine's turn, pop another so
    // the player is on move again.
    let popped = 0
    if (undoMove(gameRef.current)) popped++
    if (popped === 1 && !gameOver && turn(gameRef.current) !== playerColor) {
      if (undoMove(gameRef.current)) popped++
    }
    if (popped === 0) return
    setFen(gameRef.current.fen())
    setHistory((prev) => prev.slice(0, Math.max(0, prev.length - popped)))
    const last = gameRef.current.history({ verbose: true }).slice(-1)[0]
    setLastMove(last ? { from: last.from as ChessSquare, to: last.to as ChessSquare } : null)
    setGameStatus(status(gameRef.current))
    setResigned(null)
    setFlagged(null)
    showBanner(`Takeback (${popped} ${popped === 1 ? 'ply' : 'plies'})`)
  }, [gameOver, history.length, playerColor, showBanner])

  // --- PGN export -------------------------------------------------------
  const exportPgn = useCallback(async () => {
    const pgn = toPGN(gameRef.current, {
      Event: 'Homer dashboard chess',
      White: playerColor === 'w' ? 'Player' : (mode === 'llm' ? 'LLM' : 'Bot'),
      Black: playerColor === 'b' ? 'Player' : (mode === 'llm' ? 'LLM' : 'Bot'),
      Result:
        gameStatus === 'checkmate'
          ? turn(gameRef.current) === 'w' ? '0-1' : '1-0'
          : resigned
          ? resigned === 'white' ? '0-1' : '1-0'
          : flagged
          ? flagged === 'white' ? '0-1' : '1-0'
          : gameStatus === 'ongoing' ? '*' : '1/2-1/2',
    })
    try {
      await navigator.clipboard?.writeText(pgn)
      showBanner('PGN copied to clipboard')
    } catch {
      // Fallback — open a textarea-style banner. For now just keep it short.
      showBanner('PGN ready (clipboard blocked)')
    }
  }, [flagged, gameStatus, mode, playerColor, resigned, showBanner])

  const handleZoomChange = useCallback((next: number) => {
    setZoom(writeZoom(CHESS_GAME_ID, next))
  }, [])

  // --- Derived display --------------------------------------------------
  const flipped = playerColor === 'b'
  // checkSquare is derived from `fen` so React invalidates the memo on
  // every position change; we parse `fen` again here (rather than
  // touching `gameRef.current`) so the dependency list and the code
  // line up — keeps the react-hooks/exhaustive-deps rule happy and
  // makes the function safe even if `gameRef` drifts.
  const checkSquare = useMemo<ChessSquare | null>(() => {
    try {
      const g = newGame(fen)
      if (!g.isCheck()) return null
      const side = turn(g)
      const board = snapshot(g)
      for (const row of board) {
        for (const cell of row) {
          if (cell && cell.type === 'k' && cell.color === side) return cell.square
        }
      }
    } catch { /* ignore unparseable fens */ }
    return null
  }, [fen])

  const sideToMove = turn(gameRef.current)
  const movableColor = playerColor

  const endLabel = useMemo(() => {
    if (resigned) return `${resigned === 'white' ? 'White' : 'Black'} resigned`
    if (flagged) return `${flagged === 'white' ? 'White' : 'Black'} flagged on time`
    if (gameStatus === 'ongoing') return ''
    return humanStatus(gameStatus)
  }, [flagged, gameStatus, resigned])

  return (
    <div className="flex h-full min-h-0 select-none flex-col font-mono text-xs text-foreground">
      {/* Top bar */}
      <div className="mb-1 flex flex-wrap items-center gap-2 gap-y-1 border-b border-border/60 pb-1">
        <Pill
          label="Status"
          value={
            gameOver
              ? endLabel || 'Game over'
              : sideToMove === playerColor
              ? 'Your move'
              : thinking
              ? (mode === 'llm' ? `${llmModel || 'LLM'} thinking…` : 'Bot thinking…')
              : (mode === 'llm' ? 'LLM to move' : 'Bot to move')
          }
        />
        <Pill label="You" value={playerColor === 'w' ? 'White' : 'Black'} />
        <Pill label="Moves" value={String(Math.ceil(history.length / 2))} />
        {lastMoveSource === 'fallback' && mode === 'llm' && (
          <span
            className="rounded-md border border-amber-500/60 bg-amber-500/10 px-2 py-0.5 text-[10px] font-semibold text-amber-700 dark:text-amber-400"
            title="LLM returned an invalid move — server fallback engine played this turn"
          >
            fallback
          </span>
        )}

        <div className="min-w-[4px] flex-1" />

        <select
          aria-label="Time control"
          className="h-8 rounded border border-border bg-card/60 px-1 text-xs"
          value={timeControlId}
          onChange={(e) => setTimeControlId(e.target.value as TimeControlId)}
          disabled={history.length > 0 && !gameOver}
          title="Time control — new game required to apply"
        >
          {Object.values(TIME_CONTROLS).map((tc) => (
            <option key={tc.id} value={tc.id}>{tc.label}</option>
          ))}
        </select>

        <select
          aria-label="Side"
          className="h-8 rounded border border-border bg-card/60 px-1 text-xs"
          value={sidePref}
          onChange={(e) => setSidePref(e.target.value as SidePref)}
          title="Side for the next new game"
        >
          <option value="white">White</option>
          <option value="black">Black</option>
          <option value="random">Random</option>
        </select>

        <label className="flex items-center gap-1 text-[10px] text-muted-foreground" title="Engine search depth">
          Level
          <input
            type="range"
            min={1}
            max={4}
            value={level}
            onChange={(e) => setLevel(clampLevel(Number(e.target.value)))}
            className="h-1 w-16"
          />
          <span className="tabular-nums text-foreground">{level}</span>
        </label>

        {llmAvailable && (
          <div className="flex items-center gap-1" title="Opponent source">
            <Button
              size="sm"
              variant={mode === 'bot' ? 'default' : 'outline'}
              className="h-8 px-2 text-xs"
              onClick={() => setMode('bot')}
            >
              Bot
            </Button>
            <Button
              size="sm"
              variant={mode === 'llm' ? 'default' : 'outline'}
              className="h-8 px-2 text-xs"
              onClick={() => setMode('llm')}
            >
              LLM
            </Button>
          </div>
        )}

        <ZoomControls zoom={zoom} onZoomChange={handleZoomChange} title="Board zoom" />

        <Button size="sm" variant="secondary" className="h-8 px-2 text-xs" onClick={newSession}>
          New game
        </Button>
        <Button size="sm" variant="outline" className="h-8 px-2 text-xs" onClick={takeback} disabled={history.length === 0}>
          Takeback
        </Button>
        <Button size="sm" variant="outline" className="h-8 px-2 text-xs" onClick={resign} disabled={gameOver || history.length === 0}>
          Resign
        </Button>
        <Button size="sm" variant="outline" className="h-8 px-2 text-xs" onClick={exportPgn} disabled={history.length === 0}>
          Export PGN
        </Button>
      </div>

      {/* Main: board + sidebar */}
      <div className="flex min-h-0 flex-1 items-stretch gap-3 overflow-hidden">
        <div
          ref={wrapRef}
          className="flex min-h-0 min-w-0 flex-1 items-center justify-center overflow-auto"
        >
          <div className="relative">
            <ChessBoard
              fen={fen}
              cellPx={cellPx}
              flipped={flipped}
              interactive={!gameOver && sideToMove === playerColor}
              movableColor={movableColor}
              lastMove={lastMove ?? undefined}
              checkSquare={checkSquare ?? undefined}
              onMove={onBoardMove}
            />
            {banner && (
              <div className="pointer-events-none absolute left-1/2 top-2 z-30 -translate-x-1/2 whitespace-nowrap rounded-md border border-border bg-card px-3 py-1 text-[11px] font-medium shadow-sm">
                {banner}
              </div>
            )}
            {gameOver && (
              <div className="absolute inset-0 z-40 flex items-center justify-center rounded-md bg-background/60">
                <div className="max-w-xs rounded-lg border border-border bg-card p-4 text-center shadow-lg">
                  <div className="text-base font-semibold">Game over</div>
                  <div className="mt-1 text-muted-foreground">{endLabel}</div>
                  <Button size="sm" className="mt-3" onClick={newSession}>New game</Button>
                </div>
              </div>
            )}
          </div>
        </div>

        <div
          className="flex min-h-0 shrink-0 flex-col gap-2 overflow-hidden"
          style={{ width: SIDEBAR_PX }}
        >
          <div className="flex gap-2">
            <ClockCard label="White" ms={whiteMs} active={!gameOver && sideToMove === 'w' && history.length > 0} timed={timeControl.initialMs > 0} />
            <ClockCard label="Black" ms={blackMs} active={!gameOver && sideToMove === 'b' && history.length > 0} timed={timeControl.initialMs > 0} />
          </div>

          <div className="flex min-h-0 flex-1 flex-col rounded-lg border border-border bg-card/60 p-1.5">
            <div className="mb-0.5 text-[9px] font-semibold uppercase tracking-wider text-muted-foreground">
              Move list
            </div>
            <div className="min-h-0 flex-1 overflow-auto pr-0.5">
              {history.length === 0 ? (
                <div className="text-[10px] italic text-muted-foreground">No moves yet.</div>
              ) : (
                <ol className="space-y-0.5 text-[11px]">
                  {pairMoves(history).map((pair, i) => (
                    <li key={i} className="flex gap-1 tabular-nums">
                      <span className="w-6 shrink-0 text-muted-foreground">{i + 1}.</span>
                      <span className="w-14 shrink-0">{pair[0]?.san ?? ''}</span>
                      <span className="w-14 shrink-0">{pair[1]?.san ?? ''}</span>
                    </li>
                  ))}
                </ol>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

function Pill({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-border bg-card/60 px-2 py-0.5">
      <div className="text-[9px] uppercase tracking-wide text-muted-foreground">{label}</div>
      <div className="text-xs font-semibold leading-tight">{value}</div>
    </div>
  )
}

function ClockCard({ label, ms, active, timed }: { label: string; ms: number; active: boolean; timed: boolean }) {
  return (
    <div
      className={cn(
        'flex-1 rounded-md border px-2 py-1 text-center',
        active ? 'border-primary/70 bg-primary/10' : 'border-border bg-card/60',
      )}
    >
      <div className="text-[9px] uppercase tracking-wider text-muted-foreground">{label}</div>
      <div className="text-lg font-bold tabular-nums leading-tight">
        {timed ? formatClock(ms) : '∞'}
      </div>
    </div>
  )
}

function pairMoves(h: SanMove[]): [SanMove | undefined, SanMove | undefined][] {
  const out: [SanMove | undefined, SanMove | undefined][] = []
  for (let i = 0; i < h.length; i += 2) {
    out.push([h[i], h[i + 1]])
  }
  return out
}

function humanStatus(s: GameStatus): string {
  switch (s) {
    case 'ongoing': return 'in progress'
    case 'checkmate': return 'checkmate'
    case 'stalemate': return 'stalemate'
    case 'draw_50': return 'draw by 50-move rule'
    case 'draw_3fold': return 'draw by threefold repetition'
    case 'draw_insufficient': return 'draw by insufficient material'
    case 'draw': return 'draw'
    default: return s
  }
}
