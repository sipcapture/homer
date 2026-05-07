/**
 * Netris — two-player SIPetris over a coordinator-hosted WebSocket.
 *
 * Each side runs its own Tetris simulation locally (shapes/colours/
 * scoring identical to single-player SIPetris). The server is a thin
 * relay: it pairs two clients into a room (named via `?room=` or via
 * the quick-match queue), translates a player's `line_clear` into an
 * opponent-bound `garbage_in` using the classic `n-1` rule, and
 * broadcasts disconnect / topped_out events. The shared "hole column"
 * for each garbage burst is picked by the server so both clients
 * draw the same incoming row.
 *
 * Wire protocol matches `src/coordinator/games/netris/protocol.go`
 * envelope. Auth is the same JWT used everywhere else in the dashboard
 * (see `openNetrisSocket` → `buildWsURL` in `api.ts`).
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { openNetrisSocket, type NetrisMessage, type NetrisSocket, type NetrisSocketState } from '@/api'
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
  pushGarbage,
  type ActivePiece,
  type Board,
  type BoardCell,
  type ShapeKey,
} from './tetrisCore'

const NETRIS_GAME_ID = 'netris'

/** Pixel chrome around the playfield grid (border + p-1 padding on
 *  both sides). Subtracted from the available wrapper size when
 *  computing how big a single cell can be. */
const ARENA_CHROME_PX = 10
/** Lower bound for an auto-fit cell — below this the board becomes
 *  unreadable. Users can still zoom in past it explicitly. */
const CELL_MIN_PX = 12
/** Upper bound for the cell size so the playfield doesn't overwhelm
 *  the dashboard tile on huge displays. */
const CELL_MAX_PX = 48
/** Fixed sidebar width for opponent / next / clears. Pinning it makes
 *  the auto-fit math non-circular (mini cell size depends on cellPx,
 *  but cellPx no longer depends on the mini panel width). */
const SIDEBAR_PX = 200
/** Mini cell clamps for the opponent board snapshot. */
const MINI_CELL_MIN_PX = 6
const MINI_CELL_MAX_PX = 18

/** Frequency of opponent_board snapshots — ~4 Hz keeps the relay light. */
const BOARD_SNAPSHOT_INTERVAL_MS = 250
/** Minimum time between score updates so a soft-drop doesn't flood the relay. */
const SCORE_UPDATE_INTERVAL_MS = 250

type ConnState =
  | 'idle'         // not connected; lobby controls visible
  | 'connecting'   // socket open in flight
  | 'waiting'      // matched message received with no opponent yet
  | 'matched'      // opponent present; waiting for both to send `ready`
  | 'playing'      // game loop running
  | 'won'          // opponent topped out / left mid-game
  | 'lost'         // we topped out
  | 'closed'       // socket closed without game finishing

interface ClearedLog {
  id: number
  cleared: number
  methods: string[]
  gain: number
}

let _logUid = 0

/** Spawn-zone columns (centre 4 cells of the 10-wide board). Every
 *  shape's initial cells (see `ROT0`) fall inside these columns when
 *  `spawnPiece` places them at x=3, so if any of them are filled in
 *  rows 0..1 the next piece literally has no room to appear. This is
 *  the canonical "block-out" top-out condition for guideline Tetris. */
const SPAWN_ZONE_COL_START = 3
const SPAWN_ZONE_COL_END = 7
const SPAWN_ZONE_ROW_END = 2

/** Returns true if the rising stack has covered the spawn zone. Used
 *  to decide whether incoming garbage should end the game: only here
 *  is "no room at the top" actually true. */
function isSpawnZoneBuried(b: Board): boolean {
  for (let r = 0; r < SPAWN_ZONE_ROW_END; r++) {
    const row = b[r]
    if (!row) continue
    for (let c = SPAWN_ZONE_COL_START; c < SPAWN_ZONE_COL_END; c++) {
      if (row[c]) return true
    }
  }
  return false
}

/** Generate a short, human-friendly random room code. Uses an
 *  unambiguous alphabet (no 0/O/1/I/L) so a code can be read out
 *  loud and typed in by the second player without confusion. */
function generateRoomCode(): string {
  const alphabet = 'ABCDEFGHJKMNPQRSTUVWXYZ23456789'
  let out = ''
  for (let i = 0; i < 6; i++) {
    out += alphabet.charAt(Math.floor(Math.random() * alphabet.length))
  }
  return out
}

/** Encode a Board into a flat 200-char string for the `cells` field
 *  of an `opponent_board` snapshot. `_` = empty so the wire form has
 *  no whitespace to confuse JSON parsers. */
function encodeBoard(b: Board): string {
  const out: string[] = []
  for (let r = 0; r < ROWS; r++) {
    const row = b[r]
    for (let c = 0; c < COLS; c++) {
      out.push(row?.[c] || '_')
    }
  }
  return out.join('')
}

/** Inverse of `encodeBoard`. Anything we don't recognise turns into
 *  empty so a malformed frame can never crash the renderer. */
function decodeBoard(s: string): Board {
  const out: Board = Array.from({ length: ROWS }, () => Array<BoardCell>(COLS).fill(''))
  if (typeof s !== 'string' || s.length < ROWS * COLS) {
    return out
  }
  for (let r = 0; r < ROWS; r++) {
    for (let c = 0; c < COLS; c++) {
      const ch = s.charAt(r * COLS + c)
      switch (ch) {
        case 'I':
        case 'O':
        case 'T':
        case 'L':
        case 'J':
        case 'S':
        case 'Z':
          out[r]![c] = ch as ShapeKey
          break
        case 'G':
          out[r]![c] = 'G'
          break
        default:
          out[r]![c] = ''
      }
    }
  }
  return out
}

/** Compose own active piece into the playfield for rendering / snapshotting. */
function compose(board: Board, piece: ActivePiece | null): Board {
  if (!piece) return board
  const out = board.map((row) => [...row])
  for (let r = 0; r < piece.cells.length; r++) {
    for (let c = 0; c < piece.cells[r]!.length; c++) {
      if (!piece.cells[r]![c]) continue
      const x = piece.x + c
      const y = piece.y + r
      if (y < 0 || y >= ROWS || x < 0 || x >= COLS) continue
      out[y]![x] = piece.shape
    }
  }
  return out
}

export default function NetrisPanel() {
  // --- Game state (own side) ---
  const [board, setBoard] = useState<Board>(() => emptyBoard())
  const [piece, setPiece] = useState<ActivePiece | null>(null)
  const [nextQueue, setNextQueue] = useState<ShapeKey[]>(() => makeBag())
  const [bagBuffer, setBagBuffer] = useState<ShapeKey[]>(() => makeBag())
  const [score, setScore] = useState(0)
  const [lines, setLines] = useState(0)
  const [level, setLevel] = useState(1)
  const [bestScore, setBestScore] = useState(0)
  const [log, setLog] = useState<ClearedLog[]>([])
  const [banner, setBanner] = useState('')

  // --- Connection / opponent state ---
  const [conn, setConnRaw] = useState<ConnState>('idle')
  const [activeRoom, setActiveRoom] = useState('')
  const [sockState, setSockState] = useState<NetrisSocketState>('closed')
  const [mode, setMode] = useState<'quick' | 'room'>('quick')
  const [roomCode, setRoomCode] = useState('')
  const [opponentName, setOpponentName] = useState('')
  const [youName, setYouName] = useState('')
  const [opponentBoard, setOpponentBoard] = useState<Board | null>(null)
  const [opponentScore, setOpponentScore] = useState({ score: 0, lines: 0, level: 1 })
  const [endReason, setEndReason] = useState('')
  // Tracks "we already pressed Ready in this matched session" so the
  // button can flip to a disabled "waiting" affordance instead of
  // pretending the click didn't register. The server is happy to
  // swallow duplicate ready frames, but the UI should make it
  // obvious that no further click is needed. Reset whenever the
  // session logically restarts (begin game, leave, opponent leaves
  // back to waiting, room reverts to no-opponent matched).
  const [selfReady, setSelfReady] = useState(false)

  // --- Playfield sizing (auto-fit + per-user zoom) ----------------------
  const [zoom, setZoom] = useState<number>(() => readZoom(NETRIS_GAME_ID))
  const arenaWrapRef = useRef<HTMLDivElement | null>(null)
  const { cellPx } = useArenaCellSize(ROWS, COLS, {
    containerRef: arenaWrapRef,
    reserveWidth: ARENA_CHROME_PX,
    reserveHeight: ARENA_CHROME_PX,
    min: CELL_MIN_PX,
    max: CELL_MAX_PX,
    zoom,
  })
  const miniCellPx = Math.min(MINI_CELL_MAX_PX, Math.max(MINI_CELL_MIN_PX, Math.floor(cellPx / 2)))
  const handleZoomChange = useCallback((next: number) => {
    setZoom(writeZoom(NETRIS_GAME_ID, next))
  }, [])

  // refs (mirrors of state for non-React handlers)
  const arenaRef = useRef<HTMLDivElement | null>(null)
  const sockRef = useRef<NetrisSocket | null>(null)
  const connRef = useRef<ConnState>('idle')
  const boardRef = useRef(board)
  const pieceRef = useRef(piece)
  const nextRef = useRef(nextQueue)
  const bagRef = useRef(bagBuffer)
  const scoreRef = useRef(score)
  const linesRef = useRef(lines)
  const levelRef = useRef(level)
  const lastDropRef = useRef(0)
  const lastBoardSentRef = useRef(0)
  const lastScoreSentRef = useRef(0)
  const rafRef = useRef<number>(0)
  const bannerTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => { boardRef.current = board }, [board])
  useEffect(() => { pieceRef.current = piece }, [piece])
  useEffect(() => { nextRef.current = nextQueue }, [nextQueue])
  useEffect(() => { bagRef.current = bagBuffer }, [bagBuffer])
  useEffect(() => { scoreRef.current = score }, [score])
  useEffect(() => { linesRef.current = lines }, [lines])
  useEffect(() => { levelRef.current = level }, [level])
  useEffect(() => { connRef.current = conn }, [conn])

  // setConn writes the connRef synchronously alongside the React
  // state setter so refs-gated callbacks (especially the
  // requestAnimationFrame tick) observe the new state on the very
  // next callback rather than waiting for React to re-render and
  // fire the connRef-mirror useEffect. The race this avoids:
  // beginGame() schedules tick → tick fires before the useEffect →
  // connRef.current is still 'matched' → tick bails out and never
  // reschedules → pieces stop falling on whichever client happens
  // to render last (i.e. the opponent's pieces appear frozen).
  const setConn = useCallback((next: ConnState) => {
    connRef.current = next
    setConnRaw(next)
  }, [])

  useEffect(() => setBestScore(readBestScore(NETRIS_GAME_ID)), [])

  const showBanner = useCallback((msg: string) => {
    setBanner(msg)
    if (bannerTimer.current) clearTimeout(bannerTimer.current)
    bannerTimer.current = setTimeout(() => setBanner(''), 1400)
  }, [])

  // --- Piece bag --------------------------------------------------------
  const drawNext = useCallback((): ShapeKey => {
    let q = nextRef.current
    let buf = bagRef.current
    if (q.length === 0) {
      q = buf
      buf = makeBag()
    }
    const head = q[0]!
    const newQ = q.slice(1)
    if (newQ.length < 3) {
      newQ.push(...buf)
      buf = makeBag()
    }
    nextRef.current = newQ
    bagRef.current = buf
    setNextQueue(newQ)
    setBagBuffer(buf)
    return head
  }, [])

  const sendIfOpen = useCallback((msg: NetrisMessage) => {
    sockRef.current?.send(msg)
  }, [])

  const sendBoardSnapshotThrottled = useCallback(() => {
    const now = performance.now()
    if (now - lastBoardSentRef.current < BOARD_SNAPSHOT_INTERVAL_MS) return
    lastBoardSentRef.current = now
    const composed = compose(boardRef.current, pieceRef.current)
    sendIfOpen({ type: 'board', cells: encodeBoard(composed) })
  }, [sendIfOpen])

  const sendScoreThrottled = useCallback((force = false) => {
    const now = performance.now()
    if (!force && now - lastScoreSentRef.current < SCORE_UPDATE_INTERVAL_MS) return
    lastScoreSentRef.current = now
    sendIfOpen({
      type: 'score',
      score: scoreRef.current,
      lines: linesRef.current,
      level: levelRef.current,
    })
  }, [sendIfOpen])

  // --- Game over (we top out) ------------------------------------------
  const handleSelfTopOut = useCallback(() => {
    if (connRef.current !== 'playing') return
    setConn('lost')
    setEndReason('topped_out')
    setBestScore(writeBestScoreIfHigher(NETRIS_GAME_ID, scoreRef.current))
    sendIfOpen({ type: 'topped_out' })
  }, [sendIfOpen])

  // --- Lock + line clear with PvP send ---------------------------------
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
        const friendly = methods.length === 1 ? methods[0]! : methods.join(' + ')
        showBanner(
          `${lineCount === 4 ? 'TETRIS' : `${lineCount} line${lineCount === 1 ? '' : 's'}`} · ${friendly} · +${gainedScore}`,
        )
        setLog((prev) => {
          const next: ClearedLog = { id: ++_logUid, cleared: lineCount, methods, gain: gainedScore }
          return [next, ...prev].slice(0, 8)
        })
        // Server applies the n-1 rule and chooses the hole column.
        sendIfOpen({ type: 'line_clear', cleared: lineCount })
        sendScoreThrottled(true)
      }
      boardRef.current = cleared
      setBoard(cleared)
      const nextShape = drawNext()
      const newPiece = spawnPiece(nextShape)
      if (collides(cleared, newPiece)) {
        pieceRef.current = null
        setPiece(null)
        handleSelfTopOut()
        return
      }
      pieceRef.current = newPiece
      setPiece(newPiece)
      sendBoardSnapshotThrottled()
    },
    [drawNext, handleSelfTopOut, sendBoardSnapshotThrottled, sendIfOpen, sendScoreThrottled, showBanner],
  )

  // --- Movement helpers (same shape as SIPetris) ------------------------
  const tryMove = useCallback((dx: number, dy: number) => {
    const cur = pieceRef.current
    if (!cur || connRef.current !== 'playing') return false
    const moved: ActivePiece = { ...cur, x: cur.x + dx, y: cur.y + dy }
    if (collides(boardRef.current, moved)) return false
    pieceRef.current = moved
    setPiece(moved)
    return true
  }, [])

  const tryRotate = useCallback(() => {
    const cur = pieceRef.current
    if (!cur || connRef.current !== 'playing') return
    if (cur.shape === 'O') return
    const rotated: ActivePiece = { ...cur, cells: rotateCW(cur.cells) }
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
    if (!cur || connRef.current !== 'playing') return
    if (tryMove(0, 1)) {
      scoreRef.current += 1
      setScore(scoreRef.current)
      lastDropRef.current = performance.now()
    } else {
      lockAndAdvance(cur)
    }
  }, [lockAndAdvance, tryMove])

  const hardDrop = useCallback(() => {
    let cur = pieceRef.current
    if (!cur || connRef.current !== 'playing') return
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

  // --- Game tick --------------------------------------------------------
  const tick = useCallback(
    (now: number) => {
      if (connRef.current !== 'playing') {
        rafRef.current = 0
        return
      }
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
      sendBoardSnapshotThrottled()
      rafRef.current = requestAnimationFrame(tick)
    },
    [lockAndAdvance, sendBoardSnapshotThrottled],
  )

  // --- Game start (after server `start`) --------------------------------
  const beginGame = useCallback(() => {
    if (bannerTimer.current) clearTimeout(bannerTimer.current)
    setBanner('')
    const fresh = emptyBoard()
    boardRef.current = fresh
    setBoard(fresh)
    setScore(0); scoreRef.current = 0
    setLines(0); linesRef.current = 0
    setLevel(1); levelRef.current = 1
    setLog([])
    setEndReason('')
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
    lastBoardSentRef.current = 0
    lastScoreSentRef.current = 0
    setSelfReady(false)
    setConn('playing')
    cancelAnimationFrame(rafRef.current)
    rafRef.current = requestAnimationFrame(tick)
    setTimeout(() => arenaRef.current?.focus(), 0)
  }, [drawNext, tick])

  // --- Network: connect / disconnect / message handler -----------------
  const closeSocket = useCallback(() => {
    sockRef.current?.close()
    sockRef.current = null
    setSockState('closed')
  }, [])

  const startSession = useCallback(() => {
    closeSocket()
    setConn('connecting')
    setOpponentName('')
    setYouName('')
    setActiveRoom(mode === 'room' ? roomCode.trim() : '')
    setOpponentBoard(null)
    setOpponentScore({ score: 0, lines: 0, level: 1 })
    setEndReason('')
    setSelfReady(false)
    cancelAnimationFrame(rafRef.current)

    const sock = openNetrisSocket(
      mode === 'room'
        ? { room: roomCode.trim() }
        : { quick: true },
      {
        onOpen: () => {
          setSockState('open')
        },
        onClose: () => {
          setSockState('closed')
          // If the server cut us before play started, drop back to idle
          // so the user can pick a different mode without confusion.
          if (connRef.current === 'connecting' || connRef.current === 'waiting' || connRef.current === 'matched') {
            setConn('closed')
          }
        },
        onError: () => { /* surfaced via state */ },
        onMessage: (msg) => handleServerMessage(msg),
      },
    )
    sockRef.current = sock
    setSockState('connecting')
  }, [closeSocket, mode, roomCode])

  const handleServerMessage = useCallback(
    (msg: NetrisMessage) => {
      switch (msg.type) {
        case 'matched': {
          setYouName(msg.you ?? '')
          setOpponentName(msg.opponent ?? '')
          if (msg.room) setActiveRoom(msg.room)
          // No opponent in this matched frame means we've been
          // bounced back to lone-waiter state; clear the local
          // ready latch so the next pairing has to re-arm it.
          if (!msg.opponent) setSelfReady(false)
          setConn(msg.opponent ? 'matched' : 'waiting')
          break
        }
        case 'start': {
          beginGame()
          break
        }
        case 'garbage_in': {
          const linesIn = Math.max(0, Math.floor(msg.lines ?? 0))
          if (linesIn <= 0) return
          const hole = Math.max(0, Math.min(COLS - 1, Math.floor(msg.hole ?? 0)))
          const next = pushGarbage(boardRef.current, linesIn, hole)
          boardRef.current = next
          setBoard(next)
          // Lift the active piece so it stays above the new stack
          // height (mirrors the way tspin / Tetris guideline games
          // handle garbage rising). This is NOT a top-out: a top-out
          // by garbage only happens when the stack has reached the
          // spawn zone — i.e. there's no room *for the next piece*.
          const cur = pieceRef.current
          if (cur) {
            let lifted: ActivePiece = cur
            for (let dy = 0; dy <= linesIn + 4; dy++) {
              const candidate: ActivePiece = { ...cur, y: cur.y - dy }
              if (!collides(next, candidate)) {
                lifted = candidate
                break
              }
            }
            if (lifted !== cur) {
              pieceRef.current = lifted
              setPiece(lifted)
            }
          }
          // True garbage top-out (block-out): the rising stack has
          // covered the spawn zone (rows 0..1 in the central columns
          // where every shape spawns), so the next piece cannot
          // appear. Only here do we declare the game over — the
          // opponent's clears alone never end the match.
          if (isSpawnZoneBuried(next)) {
            pieceRef.current = null
            setPiece(null)
            handleSelfTopOut()
            return
          }
          showBanner(`Garbage in: ${linesIn} line${linesIn === 1 ? '' : 's'}`)
          break
        }
        case 'opponent_board': {
          if (typeof msg.cells === 'string') {
            setOpponentBoard(decodeBoard(msg.cells))
          }
          break
        }
        case 'opponent_score': {
          setOpponentScore({
            score: msg.score ?? 0,
            lines: msg.lines ?? 0,
            level: msg.level ?? 1,
          })
          break
        }
        case 'win': {
          setConn('won')
          setEndReason(msg.reason ?? 'opponent_topped_out')
          setBestScore(writeBestScoreIfHigher(NETRIS_GAME_ID, scoreRef.current))
          break
        }
        case 'lose': {
          setConn('lost')
          setEndReason(msg.reason ?? 'topped_out')
          break
        }
        case 'opponent_left': {
          if (connRef.current !== 'playing' && connRef.current !== 'matched') return
          // For a matched-but-not-started room, opponent_left just
          // drops us back to "waiting" so we can sit tight for someone
          // else (server keeps the room alive while we're in it).
          if (connRef.current === 'matched') {
            setOpponentName('')
            // Server forgot our readiness when we lost the partner;
            // clear the local latch so the Ready button re-enables
            // for the next pairing.
            setSelfReady(false)
            setConn('waiting')
          } else {
            // win frame should have been sent right after; if it
            // wasn't (e.g. relay hiccup), still mark as won.
            setConn('won')
            setEndReason('opponent_left')
            setBestScore(writeBestScoreIfHigher(NETRIS_GAME_ID, scoreRef.current))
          }
          break
        }
        case 'waiting_timeout': {
          setConn('closed')
          setEndReason('waiting_timeout')
          break
        }
        case 'error': {
          setConn('closed')
          setEndReason(msg.message || 'error')
          break
        }
        default:
          break
      }
    },
    [beginGame, handleSelfTopOut, showBanner],
  )

  const sendReady = useCallback(() => {
    if (selfReady) return
    sendIfOpen({ type: 'ready' })
    setSelfReady(true)
    showBanner('Ready — waiting for opponent…')
  }, [selfReady, sendIfOpen, showBanner])

  const leaveSession = useCallback(() => {
    cancelAnimationFrame(rafRef.current)
    closeSocket()
    setConn('idle')
    setOpponentName('')
    setYouName('')
    setActiveRoom('')
    setOpponentBoard(null)
    setEndReason('')
    setBoard(emptyBoard())
    setPiece(null)
    setScore(0); scoreRef.current = 0
    setLines(0); linesRef.current = 0
    setLevel(1); levelRef.current = 1
    setSelfReady(false)
  }, [closeSocket])

  useEffect(() => () => {
    cancelAnimationFrame(rafRef.current)
    if (bannerTimer.current) clearTimeout(bannerTimer.current)
    sockRef.current?.close()
    sockRef.current = null
  }, [])

  // --- Keyboard ---------------------------------------------------------
  const handleKey = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      if (connRef.current !== 'playing') return
      switch (e.key) {
        case 'ArrowLeft':
          e.preventDefault(); tryMove(-1, 0); break
        case 'ArrowRight':
          e.preventDefault(); tryMove(1, 0); break
        case 'ArrowDown':
          e.preventDefault(); softDrop(); break
        case 'ArrowUp':
        case 'x':
        case 'X':
          e.preventDefault(); tryRotate(); break
        case ' ':
        case 'Spacebar':
          e.preventDefault(); hardDrop(); break
        default:
          break
      }
    },
    [hardDrop, softDrop, tryMove, tryRotate],
  )

  // --- Rendered own board ----------------------------------------------
  const renderedBoard = useMemo<BoardCell[][]>(() => compose(board, piece), [board, piece])
  const upcoming = nextQueue.slice(0, 3)

  // --- Status pill -------------------------------------------------------
  const statusPill = useMemo(() => {
    switch (conn) {
      case 'idle':
        return { label: 'Idle', tone: 'muted' as const }
      case 'connecting':
        return { label: `Connecting…`, tone: 'muted' as const }
      case 'waiting':
        return { label: 'Waiting for opponent…', tone: 'muted' as const }
      case 'matched':
        return { label: `Matched · vs ${opponentName || '???'}`, tone: 'primary' as const }
      case 'playing':
        return { label: `vs ${opponentName || '???'}`, tone: 'primary' as const }
      case 'won':
        return { label: `Won · ${endReason || ''}`, tone: 'success' as const }
      case 'lost':
        return { label: `Lost · ${endReason || ''}`, tone: 'danger' as const }
      case 'closed':
        return { label: endReason ? `Disconnected · ${endReason}` : 'Disconnected', tone: 'muted' as const }
      default:
        return { label: conn, tone: 'muted' as const }
    }
  }, [conn, opponentName, endReason])

  return (
    <div className="flex h-full min-h-0 select-none flex-col font-mono text-xs text-foreground">
      {/* Header: stats + lobby controls */}
      <div className="mb-1 flex flex-wrap items-center gap-2 gap-y-1 border-b border-border/60 pb-1">
        <div className="rounded-md border border-primary/40 bg-primary/5 px-2.5 py-1">
          <div className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground">Score</div>
          <div className="text-lg font-bold leading-tight tabular-nums text-primary">{score}</div>
        </div>
        <Stat label="Best" value={String(bestScore)} />
        <Stat label="Lines" value={String(lines)} />
        <Stat label="Level" value={String(level)} />
        <div className="min-w-[4px] flex-1" />

        <ZoomControls
          zoom={zoom}
          onZoomChange={handleZoomChange}
          title="Playfield zoom — auto-fits the widget; click % to reset"
        />

        <span
          className={cn(
            'rounded-md border px-2 py-0.5 text-[10px] font-semibold',
            statusPill.tone === 'primary' && 'border-primary/60 bg-primary/10 text-primary',
            statusPill.tone === 'success' && 'border-emerald-500/60 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400',
            statusPill.tone === 'danger' && 'border-rose-500/60 bg-rose-500/10 text-rose-600 dark:text-rose-400',
            statusPill.tone === 'muted' && 'border-border bg-card/60 text-muted-foreground',
          )}
        >
          {statusPill.label}
        </span>

        {activeRoom && (conn === 'waiting' || conn === 'matched' || conn === 'playing') && (
          <button
            type="button"
            title="Click to copy the room code — share it so a friend can join"
            onClick={() => {
              try {
                navigator.clipboard?.writeText(activeRoom)
                showBanner(`Room code copied: ${activeRoom}`)
              } catch {
                /* clipboard may be unavailable; banner with code is enough */
                showBanner(`Room code: ${activeRoom}`)
              }
            }}
            className="rounded-md border border-border bg-card/60 px-2 py-0.5 text-[10px] font-mono uppercase tracking-wider text-foreground hover:bg-card"
          >
            Room: <span className="font-semibold">{activeRoom}</span>
          </button>
        )}

        {conn === 'idle' || conn === 'closed' ? (
          <div className="flex flex-wrap items-center gap-1">
            <Button
              type="button"
              variant={mode === 'quick' ? 'default' : 'outline'}
              size="sm"
              className="h-8 px-2 text-xs"
              onClick={() => setMode('quick')}
              title="Auto-pair with the next available player"
            >
              Quick
            </Button>
            <Button
              type="button"
              variant={mode === 'room' ? 'default' : 'outline'}
              size="sm"
              className="h-8 px-2 text-xs"
              onClick={() => setMode('room')}
              title="Pair through a shared room code (creates one if it doesn't exist)"
            >
              Room
            </Button>
            {mode === 'room' && (
              <>
                <Input
                  value={roomCode}
                  onChange={(e) => setRoomCode(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && roomCode.trim() !== '') {
                      e.preventDefault()
                      startSession()
                    }
                  }}
                  placeholder="room code (any text)"
                  className="h-8 w-40 font-mono"
                  spellCheck={false}
                  autoComplete="off"
                />
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="h-8 px-2 text-xs"
                  onClick={() => setRoomCode(generateRoomCode())}
                  title="Generate a fresh random code"
                >
                  Random
                </Button>
              </>
            )}
            <Button
              type="button"
              variant="secondary"
              size="sm"
              className="h-8 px-3 text-xs"
              onClick={startSession}
              disabled={mode === 'room' && roomCode.trim() === ''}
              title={mode === 'quick' ? 'Join the quick-match queue' : 'Open this room — creates it if nobody else is in it'}
            >
              {mode === 'quick' ? 'Find match' : 'Create / Join'}
            </Button>
          </div>
        ) : (
          <div className="flex items-center gap-1">
            {conn === 'matched' && (
              <Button
                type="button"
                variant={selfReady ? 'outline' : 'default'}
                size="sm"
                className="h-8 px-3 text-xs"
                onClick={sendReady}
                disabled={selfReady}
                title={
                  selfReady
                    ? 'Waiting for opponent to ready up'
                    : 'Tell the server you are ready to start'
                }
              >
                {selfReady ? 'Ready · waiting' : 'Ready'}
              </Button>
            )}
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-8 px-3 text-xs"
              onClick={leaveSession}
            >
              {conn === 'won' || conn === 'lost' ? 'Leave' : 'Cancel'}
            </Button>
          </div>
        )}
      </div>

      {/* Main: own board + opponent board */}
      <div className="flex min-h-0 flex-1 items-stretch gap-3 overflow-hidden">
        {/* Own board — wrapper takes the available space and is what
            ResizeObserver measures; the arena div sizes itself to the
            cellPx-derived grid so the playfield grows with the tile. */}
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
          aria-label="Netris own board"
        >
          {(conn === 'idle' || conn === 'connecting' || conn === 'waiting' || conn === 'matched' || conn === 'closed') && (
            <div className="pointer-events-none absolute inset-0 z-20 flex items-center justify-center rounded-lg bg-background/55 px-3 text-center text-[11px] text-muted-foreground">
              <div>
                <div className="font-semibold text-foreground">Netris</div>
                <div className="mt-1">Two-player SIPetris.</div>
                {conn === 'idle' && (
                  <>
                    <div className="mt-1">Pick <b>Quick</b> for auto-pairing, or <b>Room</b> + a code.</div>
                    <div className="mt-1">Same code on both sides pairs you — first one in creates the room.</div>
                  </>
                )}
                {conn === 'connecting' && <div className="mt-1">Connecting to coordinator…</div>}
                {conn === 'waiting' && (
                  <div className="mt-1">
                    Waiting for opponent…{activeRoom ? <> Share code <b className="font-mono text-foreground">{activeRoom}</b>.</> : null}
                  </div>
                )}
                {conn === 'matched' && (
                  <div className="mt-1">
                    Matched with <b className="text-foreground">{opponentName || '???'}</b>.{' '}
                    {selfReady ? (
                      <>You're ready — waiting for <b className="text-foreground">{opponentName || 'opponent'}</b>…</>
                    ) : (
                      <>Press <b>Ready</b> when you're set.</>
                    )}
                  </div>
                )}
                {conn === 'closed' && (
                  <div className="mt-1">{endReason || 'Disconnected'}.</div>
                )}
              </div>
            </div>
          )}
          {banner && (
            <div className="pointer-events-none absolute left-1/2 top-1.5 z-20 -translate-x-1/2 whitespace-nowrap rounded-md border border-border bg-card px-3 py-1 text-[11px] font-medium shadow-sm">
              {banner}
            </div>
          )}
          {(conn === 'won' || conn === 'lost') && (
            <div className="absolute inset-0 z-30 flex items-center justify-center rounded-lg bg-background/70 backdrop-blur-[2px]">
              <div className="max-w-xs rounded-lg border border-border bg-card p-5 text-center shadow-lg">
                <div className="text-base font-semibold">
                  {conn === 'won' ? 'You win' : 'Stack collapsed'}
                </div>
                <div className="mt-1 text-muted-foreground">
                  {conn === 'won' ? '200 OK' : '503 Service Unavailable'}
                </div>
                <div className="mt-3 text-sm">
                  Score: <b className="tabular-nums">{score}</b>
                  {bestScore > 0 ? (
                    <span className="text-muted-foreground">
                      {' '}· Best: <b className="tabular-nums text-foreground">{bestScore}</b>
                    </span>
                  ) : null}
                </div>
                <div className="mt-1 text-muted-foreground">{endReason || ''}</div>
                <Button type="button" className="mt-4" size="sm" onClick={leaveSession}>
                  Back to lobby
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
                      cell === 'G' ? 'bg-muted-foreground/40' : '',
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
                    aria-label={def ? def.method : cell === 'G' ? 'garbage' : undefined}
                  />
                )
              }),
            )}
          </div>
        </div>
        </div>

        {/* Opponent + side panel */}
        <div
          className="flex min-h-0 shrink-0 flex-col gap-2 overflow-hidden"
          style={{ width: SIDEBAR_PX }}
        >
          <div className="rounded-lg border border-border bg-card/60 p-1.5">
            <div className="mb-1 flex items-center justify-between text-[10px] uppercase tracking-wider text-muted-foreground">
              <span className="font-semibold">{opponentName || 'Opponent'}</span>
              <span className="tabular-nums text-foreground">
                {opponentScore.score}
              </span>
            </div>
            <div
              className="grid rounded-sm bg-muted/40"
              style={{
                gridTemplateColumns: `repeat(${COLS}, ${miniCellPx}px)`,
                gridTemplateRows: `repeat(${ROWS}, ${miniCellPx}px)`,
                gap: 0,
              }}
            >
              {(opponentBoard ?? emptyBoard()).map((row, ry) =>
                row.map((cell, cx) => {
                  const def = cell && cell !== 'G' ? PIECES[cell] : null
                  return (
                    <div
                      key={`${ry}-${cx}`}
                      className={cn(
                        'border border-black/5 dark:border-white/5',
                        cell ? '' : 'bg-card/40',
                        cell === 'G' ? 'bg-muted-foreground/40' : '',
                      )}
                      style={def ? { background: def.color, borderColor: def.border } : undefined}
                    />
                  )
                }),
              )}
            </div>
            <div className="mt-1 text-[9px] text-muted-foreground">
              Lines {opponentScore.lines} · Level {opponentScore.level}
            </div>
          </div>

          <div className="shrink-0 rounded-lg border border-border bg-card/60 p-1.5">
            <div className="mb-0.5 text-[9px] font-semibold uppercase tracking-wider text-muted-foreground">
              Next
            </div>
            <div className="flex flex-col gap-1">
              {upcoming.map((sh, idx) => (
                <NextPreview key={`${sh}-${idx}`} shape={sh} />
              ))}
            </div>
          </div>

          <div className="flex min-h-0 flex-1 flex-col rounded-lg border border-border bg-card/60 p-1.5">
            <div className="mb-0.5 text-[9px] font-semibold uppercase tracking-wider text-muted-foreground">
              Last clears
            </div>
            <div className="min-h-0 flex-1 overflow-auto pr-0.5">
              {log.length === 0 ? (
                <div className="text-[9px] italic text-muted-foreground">
                  No transactions completed yet.
                </div>
              ) : (
                <ul className="space-y-1 text-[9px]">
                  {log.map((entry) => (
                    <li
                      key={entry.id}
                      className="flex items-start justify-between gap-2 rounded border border-border/40 bg-background/40 px-1.5 py-1"
                    >
                      <span className="truncate">
                        <b
                          className={
                            entry.cleared === 4
                              ? 'text-amber-600 dark:text-amber-400'
                              : 'text-foreground'
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
        </div>
      </div>

      <div className="mt-1 flex items-center justify-between text-[10px] text-muted-foreground">
        <span>
          {youName ? <>You: <b className="text-foreground">{youName}</b> · </> : null}
          Click your board, then arrow keys · Space hard drop · ↑ rotate
        </span>
        <span>
          {sockState === 'open' ? 'WS ●' : sockState === 'connecting' ? 'WS …' : 'WS ○'}
        </span>
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
        style={{ gridTemplateColumns: `repeat(4, 8px)`, gridTemplateRows: `repeat(2, 8px)`, gap: 1 }}
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
        <div className="text-[10px] font-semibold leading-none">{def.method}</div>
      </div>
    </div>
  )
}
