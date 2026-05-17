/**
 * NetChessPanel — two-player chess over a coordinator-hosted WebSocket.
 *
 * Architectural contrast with `NetrisPanel`:
 *   - Server is **authoritative** (`src/coordinator/games/netchess`):
 *     it holds the `*notnil/chess.Game`, validates every move,
 *     manages the clocks, and emits game-over frames.
 *   - The widget renders whatever FEN the server hands it. It does
 *     not run its own chess engine; the only client-side validation
 *     is via `chess.js` and is purely cosmetic (highlight legal
 *     destinations on click).
 *
 * Lifecycle states:
 *   idle    → lobby controls (Quick / Room / time control / colour)
 *   connecting → socket open in flight
 *   waiting → matched message received with no opponent
 *   matched → opponent present, both need to send `ready`
 *   playing → server broadcast `start`
 *   won/lost/draw → `game_over` received
 *   closed  → disconnect without finishing
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import {
  openNetChessSocket,
  type NetChessMessage,
  type NetChessSocket,
  type NetChessSocketState,
} from '@/api'
import { ChessBoard } from './ChessBoard'
import { ZoomControls } from './ZoomControls'
import { readZoom, writeZoom } from './gameZoomStorage'
import { useArenaCellSize } from './useArenaCellSize'
import {
  type ChessColor,
  type ChessSquare,
  type UciMove,
  STARTING_FEN,
  isValidFen,
  newGame,
  snapshot,
} from './chessCore'

const NETCHESS_GAME_ID = 'netchess'

const BOARD_CHROME_PX = 10
const CELL_MIN_PX = 22
const CELL_MAX_PX = 64
const SIDEBAR_PX = 220

type ConnState =
  | 'idle'
  | 'connecting'
  | 'waiting'
  | 'matched'
  | 'playing'
  | 'won'
  | 'lost'
  | 'draw'
  | 'closed'

interface TimeControlOpt {
  id: string
  label: string
  initialMs: number
  incrementMs: number
}

const TIME_CONTROLS: TimeControlOpt[] = [
  { id: 'bullet', label: 'Bullet 1+0', initialMs: 60_000, incrementMs: 0 },
  { id: 'blitz', label: 'Blitz 3+2', initialMs: 180_000, incrementMs: 2_000 },
  { id: 'rapid', label: 'Rapid 10+5', initialMs: 600_000, incrementMs: 5_000 },
  { id: 'classical', label: 'Classical 30+0', initialMs: 1_800_000, incrementMs: 0 },
]

function generateRoomCode(): string {
  const alphabet = 'ABCDEFGHJKMNPQRSTUVWXYZ23456789'
  let out = ''
  for (let i = 0; i < 6; i++) out += alphabet.charAt(Math.floor(Math.random() * alphabet.length))
  return out
}

function formatClock(ms: number): string {
  if (ms <= 0) return '0:00'
  const totalSec = Math.ceil(ms / 1000)
  const m = Math.floor(totalSec / 60)
  const s = totalSec % 60
  return `${m}:${s.toString().padStart(2, '0')}`
}

interface PgnEntry {
  ply: number
  san: string
}

export default function NetChessPanel() {
  // --- Lobby setup ------------------------------------------------------
  const [mode, setMode] = useState<'quick' | 'room' | 'spectate'>('quick')
  const [roomCode, setRoomCode] = useState('')
  const [colorPref, setColorPref] = useState<'white' | 'black' | 'random'>('random')
  const [tcId, setTcId] = useState<string>('blitz')
  const timeControl = useMemo(
    () => TIME_CONTROLS.find((t) => t.id === tcId) ?? TIME_CONTROLS[1]!,
    [tcId],
  )

  // --- Session state ----------------------------------------------------
  const [conn, setConnRaw] = useState<ConnState>('idle')
  const [activeRoom, setActiveRoom] = useState('')
  const [sockState, setSockState] = useState<NetChessSocketState>('closed')
  const [youName, setYouName] = useState('')
  const [oppName, setOppName] = useState('')
  const [myColor, setMyColor] = useState<ChessColor | null>(null)
  const [spectator, setSpectator] = useState(false)
  const [serverInitialMs, setServerInitialMs] = useState(0)
  // serverIncrementMs is sent in `matched` but we currently only use
  // the server's own clock numbers — keep a state slot for the future
  // (showing "+5s/move" in the lobby pill) but mark unused for now.
  const [, setServerIncrementMs] = useState(0)

  // --- Game state (server-authoritative) -------------------------------
  const [fen, setFen] = useState(STARTING_FEN)
  const [lastMove, setLastMove] = useState<{ from: ChessSquare; to: ChessSquare } | null>(null)
  const [moveList, setMoveList] = useState<PgnEntry[]>([])
  const [whiteMs, setWhiteMs] = useState(0)
  const [blackMs, setBlackMs] = useState(0)
  // Local ticking: the server is the source of truth and pushes
  // clock_sync on every move; between moves the client interpolates
  // from `lastSyncAt` so the clocks don't appear frozen.
  const lastSyncAt = useRef<number>(0)
  const [endReason, setEndReason] = useState('')
  const [endResult, setEndResult] = useState<'1-0' | '0-1' | '1/2-1/2' | ''>('')
  const [drawOffered, setDrawOffered] = useState<'them' | 'mine' | null>(null)
  const [takebackOffered, setTakebackOffered] = useState<'them' | 'mine' | null>(null)
  const [banner, setBanner] = useState('')
  const bannerTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  // refs for non-React handlers
  const sockRef = useRef<NetChessSocket | null>(null)
  const connRef = useRef<ConnState>('idle')
  const fenRef = useRef(fen)
  useEffect(() => { fenRef.current = fen }, [fen])
  useEffect(() => { connRef.current = conn }, [conn])
  const setConn = useCallback((s: ConnState) => {
    connRef.current = s
    setConnRaw(s)
  }, [])

  // --- Sizing -----------------------------------------------------------
  const wrapRef = useRef<HTMLDivElement | null>(null)
  const [zoom, setZoom] = useState<number>(() => readZoom(NETCHESS_GAME_ID))
  const { cellPx } = useArenaCellSize(8, 8, {
    containerRef: wrapRef,
    reserveWidth: BOARD_CHROME_PX,
    reserveHeight: BOARD_CHROME_PX,
    min: CELL_MIN_PX,
    max: CELL_MAX_PX,
    zoom,
  })
  const handleZoomChange = useCallback((next: number) => {
    setZoom(writeZoom(NETCHESS_GAME_ID, next))
  }, [])

  const showBanner = useCallback((msg: string) => {
    setBanner(msg)
    if (bannerTimer.current) clearTimeout(bannerTimer.current)
    bannerTimer.current = setTimeout(() => setBanner(''), 1800)
  }, [])

  // --- Side-to-move derived from FEN -----------------------------------
  const sideToMove = useMemo<ChessColor>(() => {
    if (!isValidFen(fen)) return 'w'
    try {
      const g = newGame(fen)
      return g.turn()
    } catch {
      return 'w'
    }
  }, [fen])

  const checkSquare = useMemo<ChessSquare | null>(() => {
    if (!isValidFen(fen)) return null
    try {
      const g = newGame(fen)
      if (!g.isCheck()) return null
      const side = g.turn()
      const board = snapshot(g)
      for (const row of board) {
        for (const cell of row) {
          if (cell && cell.type === 'k' && cell.color === side) return cell.square
        }
      }
    } catch { /* ignore */ }
    return null
  }, [fen])

  // --- Clock interpolation (between server syncs) ----------------------
  useEffect(() => {
    if (conn !== 'playing' || serverInitialMs <= 0) return
    const id = window.setInterval(() => {
      const now = Date.now()
      const elapsed = lastSyncAt.current === 0 ? 0 : now - lastSyncAt.current
      if (sideToMove === 'w') {
        setWhiteMs((ms) => Math.max(0, ms - elapsed))
      } else {
        setBlackMs((ms) => Math.max(0, ms - elapsed))
      }
      lastSyncAt.current = now
    }, 200)
    return () => window.clearInterval(id)
  }, [conn, serverInitialMs, sideToMove])

  // --- Server message handler ------------------------------------------
  const onServerMessage = useCallback(
    (msg: NetChessMessage) => {
      switch (msg.type) {
        case 'matched': {
          setYouName(msg.you ?? '')
          setOppName(msg.opponent ?? '')
          setActiveRoom(msg.room ?? '')
          setSpectator(!!msg.spectator)
          if (msg.color === 'white') setMyColor('w')
          else if (msg.color === 'black') setMyColor('b')
          else setMyColor(null)
          if (msg.initial_ms !== undefined) {
            setServerInitialMs(msg.initial_ms)
            setWhiteMs(msg.initial_ms)
            setBlackMs(msg.initial_ms)
          }
          if (msg.increment_ms !== undefined) setServerIncrementMs(msg.increment_ms)
          if (msg.fen && isValidFen(msg.fen)) setFen(msg.fen)
          if (msg.spectator) {
            // Spectators jump straight to "playing" (read-only view).
            setConn(msg.fen ? 'playing' : 'matched')
          } else if (!msg.opponent) {
            setConn('waiting')
          } else {
            setConn('matched')
          }
          break
        }
        case 'start': {
          if (msg.fen && isValidFen(msg.fen)) setFen(msg.fen)
          if (typeof msg.white_ms === 'number') setWhiteMs(msg.white_ms)
          if (typeof msg.black_ms === 'number') setBlackMs(msg.black_ms)
          lastSyncAt.current = Date.now()
          setConn('playing')
          setLastMove(null)
          setMoveList([])
          setEndReason('')
          setEndResult('')
          setDrawOffered(null)
          setTakebackOffered(null)
          break
        }
        case 'opponent_move': {
          if (msg.fen && isValidFen(msg.fen)) setFen(msg.fen)
          if (msg.uci && msg.uci.length >= 4) {
            setLastMove({
              from: msg.uci.slice(0, 2) as ChessSquare,
              to: msg.uci.slice(2, 4) as ChessSquare,
            })
          }
          if (msg.san) {
            setMoveList((prev) => [...prev, { ply: prev.length, san: msg.san! }])
          }
          if (typeof msg.white_ms === 'number') setWhiteMs(msg.white_ms)
          if (typeof msg.black_ms === 'number') setBlackMs(msg.black_ms)
          lastSyncAt.current = Date.now()
          break
        }
        case 'clock_sync': {
          if (msg.fen && isValidFen(msg.fen)) setFen(msg.fen)
          if (typeof msg.white_ms === 'number') setWhiteMs(msg.white_ms)
          if (typeof msg.black_ms === 'number') setBlackMs(msg.black_ms)
          lastSyncAt.current = Date.now()
          break
        }
        case 'game_over': {
          setEndResult((msg.result as '1-0' | '0-1' | '1/2-1/2') ?? '')
          setEndReason(msg.reason ?? '')
          if (typeof msg.white_ms === 'number') setWhiteMs(msg.white_ms)
          if (typeof msg.black_ms === 'number') setBlackMs(msg.black_ms)
          // Map result → won / lost / draw.
          if (spectator || myColor === null) {
            setConn('draw') // for spectator just leave the overlay on
          } else if (msg.result === '1/2-1/2') {
            setConn('draw')
          } else if (
            (msg.result === '1-0' && myColor === 'w') ||
            (msg.result === '0-1' && myColor === 'b')
          ) {
            setConn('won')
          } else {
            setConn('lost')
          }
          break
        }
        case 'draw_offered': {
          setDrawOffered('them')
          showBanner(`${msg.from || 'Opponent'} offered a draw`)
          break
        }
        case 'takeback_offered': {
          setTakebackOffered('them')
          showBanner(`${msg.from || 'Opponent'} requests a takeback`)
          break
        }
        case 'opponent_left': {
          if (connRef.current === 'matched') {
            setConn('waiting')
            setOppName('')
            showBanner('Opponent left — waiting for another')
          }
          break
        }
        case 'waiting_timeout': {
          setConn('closed')
          setEndReason('no_opponent')
          break
        }
        case 'error': {
          showBanner(msg.message || 'error')
          break
        }
        default:
          break
      }
    },
    [myColor, setConn, showBanner, spectator],
  )

  // --- Connect / disconnect --------------------------------------------
  const closeSocket = useCallback(() => {
    sockRef.current?.close()
    sockRef.current = null
    setSockState('closed')
  }, [])

  const startSession = useCallback(() => {
    closeSocket()
    setConn('connecting')
    setOppName('')
    setYouName('')
    setActiveRoom(mode === 'room' || mode === 'spectate' ? roomCode.trim() : '')
    setEndReason('')
    setEndResult('')
    setSpectator(false)
    setMoveList([])
    setLastMove(null)

    const opts = {
      quick: mode === 'quick',
      room: mode === 'room' || mode === 'spectate' ? roomCode.trim() : undefined,
      spectate: mode === 'spectate',
      color: colorPref,
      initialMs: timeControl.initialMs,
      incrementMs: timeControl.incrementMs,
    }
    const sock = openNetChessSocket(opts, {
      onOpen: () => setSockState('open'),
      onClose: () => {
        setSockState('closed')
        if (connRef.current === 'connecting' || connRef.current === 'waiting' || connRef.current === 'matched') {
          setConn('closed')
        }
      },
      onError: () => { /* surfaced via state */ },
      onMessage: onServerMessage,
    })
    sockRef.current = sock
    setSockState('connecting')
  }, [closeSocket, colorPref, mode, onServerMessage, roomCode, setConn, timeControl.incrementMs, timeControl.initialMs])

  const leaveSession = useCallback(() => {
    closeSocket()
    setConn('idle')
    setOppName('')
    setYouName('')
    setActiveRoom('')
    setMyColor(null)
    setSpectator(false)
    setFen(STARTING_FEN)
    setLastMove(null)
    setMoveList([])
    setWhiteMs(0)
    setBlackMs(0)
    setEndReason('')
    setEndResult('')
    setDrawOffered(null)
    setTakebackOffered(null)
  }, [closeSocket, setConn])

  // Cleanup on unmount.
  useEffect(() => () => {
    sockRef.current?.close()
    sockRef.current = null
    if (bannerTimer.current) clearTimeout(bannerTimer.current)
  }, [])

  // --- Send helpers ----------------------------------------------------
  const sendReady = useCallback(() => {
    sockRef.current?.send({ type: 'ready' })
    showBanner('Ready — waiting for opponent…')
  }, [showBanner])

  const onBoardMove = useCallback(
    (move: UciMove) => {
      if (conn !== 'playing' || spectator) return
      if (myColor !== sideToMove) return // not your turn
      const uci = `${move.from}${move.to}${move.promotion ?? ''}`
      sockRef.current?.send({ type: 'move', uci })
    },
    [conn, myColor, sideToMove, spectator],
  )

  const resign = useCallback(() => {
    sockRef.current?.send({ type: 'resign' })
  }, [])
  const offerDraw = useCallback(() => {
    sockRef.current?.send({ type: 'draw_offer' })
    setDrawOffered('mine')
    showBanner('Draw offered')
  }, [showBanner])
  const acceptDraw = useCallback(() => {
    sockRef.current?.send({ type: 'draw_accept' })
    setDrawOffered(null)
  }, [])
  const declineDraw = useCallback(() => {
    sockRef.current?.send({ type: 'draw_decline' })
    setDrawOffered(null)
  }, [])
  const requestTakeback = useCallback(() => {
    sockRef.current?.send({ type: 'takeback_request' })
    setTakebackOffered('mine')
    showBanner('Takeback requested')
  }, [showBanner])
  const acceptTakeback = useCallback(() => {
    sockRef.current?.send({ type: 'takeback_accept' })
    setTakebackOffered(null)
  }, [])
  const declineTakeback = useCallback(() => {
    sockRef.current?.send({ type: 'takeback_decline' })
    setTakebackOffered(null)
  }, [])

  // --- Derived display --------------------------------------------------
  const flipped = myColor === 'b'
  const youCanMove = conn === 'playing' && !spectator && myColor === sideToMove
  const statusLabel = useMemo(() => {
    switch (conn) {
      case 'idle': return 'Idle'
      case 'connecting': return 'Connecting…'
      case 'waiting': return activeRoom ? `Waiting · room ${activeRoom}` : 'Waiting for opponent…'
      case 'matched': return `Matched · vs ${oppName || '???'}`
      case 'playing': return spectator ? 'Spectating' : youCanMove ? 'Your move' : "Opponent's move"
      case 'won': return `Won · ${endReason}`
      case 'lost': return `Lost · ${endReason}`
      case 'draw': return `Draw · ${endReason}`
      case 'closed': return endReason ? `Disconnected · ${endReason}` : 'Disconnected'
      default: return conn
    }
  }, [activeRoom, conn, endReason, oppName, spectator, youCanMove])

  // Reformat last opponent's SAN history grouped 2-per-row.
  const pgnRows = useMemo(() => {
    const rows: { num: number; white?: string; black?: string }[] = []
    for (let i = 0; i < moveList.length; i += 2) {
      rows.push({
        num: i / 2 + 1,
        white: moveList[i]?.san,
        black: moveList[i + 1]?.san,
      })
    }
    return rows
  }, [moveList])

  return (
    <div className="flex h-full min-h-0 select-none flex-col font-mono text-xs text-foreground">
      {/* Top bar */}
      <div className="mb-1 flex flex-wrap items-center gap-2 gap-y-1 border-b border-border/60 pb-1">
        <span
          className={cn(
            'rounded-md border px-2 py-0.5 text-[10px] font-semibold',
            (conn === 'playing' || conn === 'matched') && 'border-primary/60 bg-primary/10 text-primary',
            conn === 'won' && 'border-emerald-500/60 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400',
            conn === 'lost' && 'border-rose-500/60 bg-rose-500/10 text-rose-600 dark:text-rose-400',
            (conn === 'idle' || conn === 'closed' || conn === 'connecting' || conn === 'waiting' || conn === 'draw') && 'border-border bg-card/60 text-muted-foreground',
          )}
        >
          {statusLabel}
        </span>

        {myColor && conn !== 'idle' && (
          <span className="rounded-md border border-border bg-card/60 px-2 py-0.5 text-[10px]">
            You: <b>{myColor === 'w' ? 'White' : 'Black'}</b>
          </span>
        )}

        {activeRoom && (conn === 'waiting' || conn === 'matched' || conn === 'playing') && (
          <button
            type="button"
            title="Click to copy room code"
            onClick={() => {
              try {
                navigator.clipboard?.writeText(activeRoom)
                showBanner(`Room code copied: ${activeRoom}`)
              } catch { showBanner(`Room code: ${activeRoom}`) }
            }}
            className="rounded-md border border-border bg-card/60 px-2 py-0.5 text-[10px] font-mono uppercase tracking-wider hover:bg-card"
          >
            Room: <b>{activeRoom}</b>
          </button>
        )}

        <div className="min-w-[4px] flex-1" />
        <ZoomControls zoom={zoom} onZoomChange={handleZoomChange} title="Board zoom" />

        {conn === 'idle' || conn === 'closed' ? (
          <LobbyControls
            mode={mode}
            setMode={setMode}
            roomCode={roomCode}
            setRoomCode={setRoomCode}
            colorPref={colorPref}
            setColorPref={setColorPref}
            tcId={tcId}
            setTcId={setTcId}
            onStart={startSession}
          />
        ) : (
          <div className="flex items-center gap-1">
            {conn === 'matched' && (
              <Button type="button" size="sm" className="h-8 px-3 text-xs" onClick={sendReady}>
                Ready
              </Button>
            )}
            {conn === 'playing' && !spectator && (
              <>
                <Button size="sm" variant="outline" className="h-8 px-2 text-xs" onClick={offerDraw} disabled={drawOffered === 'mine'}>
                  {drawOffered === 'mine' ? 'Draw offered' : 'Offer draw'}
                </Button>
                <Button size="sm" variant="outline" className="h-8 px-2 text-xs" onClick={requestTakeback} disabled={takebackOffered === 'mine'}>
                  Takeback
                </Button>
                <Button size="sm" variant="outline" className="h-8 px-2 text-xs" onClick={resign}>
                  Resign
                </Button>
              </>
            )}
            <Button size="sm" variant="outline" className="h-8 px-3 text-xs" onClick={leaveSession}>
              {conn === 'won' || conn === 'lost' || conn === 'draw' ? 'Leave' : 'Cancel'}
            </Button>
          </div>
        )}
      </div>

      {/* Main */}
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
              interactive={youCanMove}
              movableColor={myColor ?? undefined}
              lastMove={lastMove ?? undefined}
              checkSquare={checkSquare ?? undefined}
              onMove={onBoardMove}
            />
            {banner && (
              <div className="pointer-events-none absolute left-1/2 top-2 z-30 -translate-x-1/2 whitespace-nowrap rounded-md border border-border bg-card px-3 py-1 text-[11px] font-medium shadow-sm">
                {banner}
              </div>
            )}
            {(conn === 'idle' || conn === 'connecting' || conn === 'waiting' || conn === 'matched') && (
              <div className="pointer-events-none absolute inset-0 z-20 flex items-center justify-center rounded-md bg-background/55 px-3 text-center text-[11px] text-muted-foreground">
                <div>
                  <div className="text-sm font-semibold text-foreground">NetChess</div>
                  {conn === 'idle' && <div className="mt-1">Pick a mode in the toolbar to start.</div>}
                  {conn === 'connecting' && <div className="mt-1">Connecting…</div>}
                  {conn === 'waiting' && (
                    <div className="mt-1">
                      Waiting for opponent…{activeRoom ? <> Share code <b>{activeRoom}</b>.</> : null}
                    </div>
                  )}
                  {conn === 'matched' && (
                    <div className="mt-1">Matched with <b>{oppName || '???'}</b>. Press <b>Ready</b>.</div>
                  )}
                </div>
              </div>
            )}
            {(conn === 'won' || conn === 'lost' || conn === 'draw') && (
              <div className="absolute inset-0 z-30 flex items-center justify-center rounded-md bg-background/60">
                <div className="max-w-xs rounded-lg border border-border bg-card p-4 text-center shadow-lg">
                  <div className="text-base font-semibold">
                    {conn === 'won' ? 'You win' : conn === 'lost' ? 'You lose' : 'Draw'}
                  </div>
                  <div className="mt-1 text-muted-foreground">{endResult}{endReason ? ` · ${endReason}` : ''}</div>
                  <Button size="sm" className="mt-3" onClick={leaveSession}>Back to lobby</Button>
                </div>
              </div>
            )}
          </div>
        </div>

        {/* Sidebar */}
        <div
          className="flex min-h-0 shrink-0 flex-col gap-2 overflow-hidden"
          style={{ width: SIDEBAR_PX }}
        >
          <div className="flex gap-2">
            <ClockCard label="White" ms={whiteMs} active={conn === 'playing' && sideToMove === 'w'} timed={serverInitialMs > 0} />
            <ClockCard label="Black" ms={blackMs} active={conn === 'playing' && sideToMove === 'b'} timed={serverInitialMs > 0} />
          </div>

          {(drawOffered === 'them' || takebackOffered === 'them') && (
            <div className="rounded-lg border border-amber-500/40 bg-amber-500/10 p-2 text-[11px]">
              <div className="mb-1 font-semibold">
                {drawOffered === 'them' ? 'Draw offered' : 'Takeback requested'}
              </div>
              <div className="flex gap-1">
                <Button size="sm" className="h-7 px-2 text-xs" onClick={drawOffered === 'them' ? acceptDraw : acceptTakeback}>
                  Accept
                </Button>
                <Button size="sm" variant="outline" className="h-7 px-2 text-xs" onClick={drawOffered === 'them' ? declineDraw : declineTakeback}>
                  Decline
                </Button>
              </div>
            </div>
          )}

          <div className="flex min-h-0 flex-1 flex-col rounded-lg border border-border bg-card/60 p-1.5">
            <div className="mb-0.5 text-[9px] font-semibold uppercase tracking-wider text-muted-foreground">
              Moves
            </div>
            <div className="min-h-0 flex-1 overflow-auto pr-0.5">
              {pgnRows.length === 0 ? (
                <div className="text-[10px] italic text-muted-foreground">No moves yet.</div>
              ) : (
                <ol className="space-y-0.5 text-[11px]">
                  {pgnRows.map((r) => (
                    <li key={r.num} className="flex gap-1 tabular-nums">
                      <span className="w-6 shrink-0 text-muted-foreground">{r.num}.</span>
                      <span className="w-14 shrink-0">{r.white ?? ''}</span>
                      <span className="w-14 shrink-0">{r.black ?? ''}</span>
                    </li>
                  ))}
                </ol>
              )}
            </div>
          </div>
        </div>
      </div>

      <div className="mt-1 flex items-center justify-between text-[10px] text-muted-foreground">
        <span>
          {youName ? <>You: <b className="text-foreground">{youName}</b> · </> : null}
          {spectator ? 'Spectator view' : 'Click your piece, then a destination · drag-and-drop coming soon'}
        </span>
        <span>{sockState === 'open' ? 'WS ●' : sockState === 'connecting' ? 'WS …' : 'WS ○'}</span>
      </div>
    </div>
  )
}

function LobbyControls(props: {
  mode: 'quick' | 'room' | 'spectate'
  setMode: (m: 'quick' | 'room' | 'spectate') => void
  roomCode: string
  setRoomCode: (s: string) => void
  colorPref: 'white' | 'black' | 'random'
  setColorPref: (c: 'white' | 'black' | 'random') => void
  tcId: string
  setTcId: (s: string) => void
  onStart: () => void
}) {
  const { mode, setMode, roomCode, setRoomCode, colorPref, setColorPref, tcId, setTcId, onStart } = props
  return (
    <div className="flex flex-wrap items-center gap-1">
      <Button type="button" size="sm" variant={mode === 'quick' ? 'default' : 'outline'} className="h-8 px-2 text-xs" onClick={() => setMode('quick')}>
        Quick
      </Button>
      <Button type="button" size="sm" variant={mode === 'room' ? 'default' : 'outline'} className="h-8 px-2 text-xs" onClick={() => setMode('room')}>
        Room
      </Button>
      <Button type="button" size="sm" variant={mode === 'spectate' ? 'default' : 'outline'} className="h-8 px-2 text-xs" onClick={() => setMode('spectate')}>
        Spectate
      </Button>
      {(mode === 'room' || mode === 'spectate') && (
        <>
          <Input
            value={roomCode}
            onChange={(e) => setRoomCode(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter' && roomCode.trim() !== '') onStart() }}
            placeholder="room code"
            className="h-8 w-32 font-mono"
            spellCheck={false}
            autoComplete="off"
          />
          {mode === 'room' && (
            <Button type="button" size="sm" variant="outline" className="h-8 px-2 text-xs" onClick={() => setRoomCode(generateRoomCode())}>
              Random
            </Button>
          )}
        </>
      )}
      {mode !== 'spectate' && (
        <>
          <select
            aria-label="Time control"
            value={tcId}
            onChange={(e) => setTcId(e.target.value)}
            className="h-8 rounded border border-border bg-card/60 px-1 text-xs"
          >
            {TIME_CONTROLS.map((t) => (
              <option key={t.id} value={t.id}>{t.label}</option>
            ))}
          </select>
          <select
            aria-label="Colour"
            value={colorPref}
            onChange={(e) => setColorPref(e.target.value as 'white' | 'black' | 'random')}
            className="h-8 rounded border border-border bg-card/60 px-1 text-xs"
          >
            <option value="random">Random</option>
            <option value="white">White</option>
            <option value="black">Black</option>
          </select>
        </>
      )}
      <Button
        type="button"
        size="sm"
        variant="secondary"
        className="h-8 px-3 text-xs"
        onClick={onStart}
        disabled={(mode === 'room' || mode === 'spectate') && roomCode.trim() === ''}
      >
        {mode === 'quick' ? 'Find match' : mode === 'spectate' ? 'Watch' : 'Create / Join'}
      </Button>
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
