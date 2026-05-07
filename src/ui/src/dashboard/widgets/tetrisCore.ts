/**
 * Tetris core — pure (no React) building blocks shared by SIPetrisPanel
 * and NetrisPanel.
 *
 * SIP-method theming (each tetromino is a SIP method) is kept here so
 * both single-player SIPetris and the PvP Netris widget use exactly
 * the same shapes, colours, and rotation/lock/clear logic.
 *
 * The Netris widget extends BoardCell with one extra value `'G'` for
 * "garbage" rows pushed in by the opponent — handled inline in the
 * widget; this module stays oblivious to garbage and only concerns
 * itself with the seven classic tetrominoes.
 */
export const COLS = 10
export const ROWS = 20

export type ShapeKey = 'I' | 'O' | 'T' | 'L' | 'J' | 'S' | 'Z'

export interface PieceDef {
  /** SIP method this tetromino represents. */
  method: string
  shape: ShapeKey
  color: string
  border: string
  blurb: string
}

export const PIECES: Record<ShapeKey, PieceDef> = {
  I: { method: 'INVITE', shape: 'I', color: '#22d3ee', border: '#0891b2', blurb: 'Setting up the call' },
  O: { method: 'ACK', shape: 'O', color: '#facc15', border: '#a16207', blurb: 'Final hop confirmed' },
  T: { method: 'REGISTER', shape: 'T', color: '#a78bfa', border: '#6d28d9', blurb: 'Binding contact' },
  L: { method: 'BYE', shape: 'L', color: '#fb923c', border: '#c2410c', blurb: 'Tearing down dialog' },
  J: { method: 'CANCEL', shape: 'J', color: '#60a5fa', border: '#1d4ed8', blurb: 'Early termination' },
  S: { method: 'OPTIONS', shape: 'S', color: '#34d399', border: '#047857', blurb: 'Capability probe' },
  Z: { method: 'PRACK', shape: 'Z', color: '#f472b6', border: '#9d174d', blurb: 'Provisional ack' },
}

/** SIP method → ShapeKey, or null when the method has no tetromino mapping. */
export function shapeForMethod(method: string | undefined): ShapeKey | null {
  if (!method) return null
  const m = method.toUpperCase()
  for (const k of Object.keys(PIECES) as ShapeKey[]) {
    if (PIECES[k].method === m) return k
  }
  return null
}

export type Grid = number[][]

export const ROT0: Record<ShapeKey, Grid> = {
  I: [
    [0, 0, 0, 0],
    [1, 1, 1, 1],
    [0, 0, 0, 0],
    [0, 0, 0, 0],
  ],
  O: [
    [0, 1, 1, 0],
    [0, 1, 1, 0],
    [0, 0, 0, 0],
    [0, 0, 0, 0],
  ],
  T: [
    [0, 1, 0, 0],
    [1, 1, 1, 0],
    [0, 0, 0, 0],
    [0, 0, 0, 0],
  ],
  L: [
    [0, 0, 1, 0],
    [1, 1, 1, 0],
    [0, 0, 0, 0],
    [0, 0, 0, 0],
  ],
  J: [
    [1, 0, 0, 0],
    [1, 1, 1, 0],
    [0, 0, 0, 0],
    [0, 0, 0, 0],
  ],
  S: [
    [0, 1, 1, 0],
    [1, 1, 0, 0],
    [0, 0, 0, 0],
    [0, 0, 0, 0],
  ],
  Z: [
    [1, 1, 0, 0],
    [0, 1, 1, 0],
    [0, 0, 0, 0],
    [0, 0, 0, 0],
  ],
}

export function rotateCW(grid: Grid): Grid {
  const n = grid.length
  const out: Grid = Array.from({ length: n }, () => Array(n).fill(0))
  for (let r = 0; r < n; r++) {
    for (let c = 0; c < n; c++) {
      out[c]![n - 1 - r] = grid[r]![c]!
    }
  }
  return out
}

export interface ActivePiece {
  shape: ShapeKey
  cells: Grid
  x: number
  y: number
}

/**
 * Cells stored on the board: a ShapeKey for a tetromino piece, '' for empty,
 * or 'G' for a garbage row in PvP mode (Netris) — kept as a separate value
 * so it renders in a neutral grey instead of any SIP method colour.
 */
export type BoardCell = ShapeKey | 'G' | ''
export type Board = BoardCell[][]

export function emptyBoard(): Board {
  return Array.from({ length: ROWS }, () => Array<BoardCell>(COLS).fill(''))
}

/**
 * 7-bag randomiser: each bag yields all 7 pieces in a random order, so
 * starvation never happens — the same fairness contract real Tetris
 * implementations use.
 */
export function makeBag(): ShapeKey[] {
  const keys: ShapeKey[] = ['I', 'O', 'T', 'L', 'J', 'S', 'Z']
  for (let i = keys.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1))
    ;[keys[i], keys[j]] = [keys[j]!, keys[i]!]
  }
  return keys
}

export function spawnPiece(shape: ShapeKey): ActivePiece {
  return {
    shape,
    cells: ROT0[shape].map((row) => [...row]),
    x: 3,
    y: shape === 'I' ? -1 : 0,
  }
}

/** True iff the piece (in its current cells/x/y) collides with walls or filled board cells. */
export function collides(board: Board, piece: ActivePiece): boolean {
  for (let r = 0; r < piece.cells.length; r++) {
    for (let c = 0; c < piece.cells[r]!.length; c++) {
      if (!piece.cells[r]![c]) continue
      const x = piece.x + c
      const y = piece.y + r
      if (x < 0 || x >= COLS || y >= ROWS) return true
      if (y >= 0 && board[y]![x] !== '') return true
    }
  }
  return false
}

export function lockPiece(board: Board, piece: ActivePiece): Board {
  const next = board.map((row) => [...row])
  for (let r = 0; r < piece.cells.length; r++) {
    for (let c = 0; c < piece.cells[r]!.length; c++) {
      if (!piece.cells[r]![c]) continue
      const x = piece.x + c
      const y = piece.y + r
      if (y < 0 || y >= ROWS || x < 0 || x >= COLS) continue
      next[y]![x] = piece.shape
    }
  }
  return next
}

/**
 * Returns the board with full rows removed and the SIP methods of cleared
 * lines (for the side log). Garbage rows ('G') count toward "full" if they
 * happen to fully fill horizontally, but the dominant-method picker
 * skips them so the side log keeps showing real SIP methods.
 */
export function clearLines(board: Board): { board: Board; cleared: number; methods: string[] } {
  const methods: string[] = []
  const kept: BoardCell[][] = []
  for (const row of board) {
    if (row.every((cell) => cell !== '')) {
      const counts = new Map<ShapeKey, number>()
      for (const cell of row) {
        if (cell === 'G') continue
        counts.set(cell, (counts.get(cell) ?? 0) + 1)
      }
      let bestShape: ShapeKey = 'I'
      let bestN = -1
      for (const [k, n] of counts.entries()) {
        if (n > bestN) {
          bestShape = k
          bestN = n
        }
      }
      methods.push(PIECES[bestShape].method)
    } else {
      kept.push(row)
    }
  }
  const cleared = ROWS - kept.length
  while (kept.length < ROWS) {
    kept.unshift(Array<BoardCell>(COLS).fill(''))
  }
  return { board: kept, cleared, methods }
}

/** Classic Tetris line scoring per drop (1/3/5/8 × level, ×100 base). */
export function scoreForClear(cleared: number, level: number): number {
  const base = cleared === 1 ? 100 : cleared === 2 ? 300 : cleared === 3 ? 500 : cleared === 4 ? 800 : 0
  return base * level
}

/** Drop interval in ms for a given level (1 = 800ms, accelerates). */
export function dropIntervalMs(level: number): number {
  return Math.max(80, Math.round(800 * Math.pow(0.85, Math.max(0, level - 1))))
}

/**
 * Push `lines` garbage rows from below: shifts the existing stack up
 * by `lines` and inserts that many rows of 'G' at the bottom, with one
 * empty cell at column `hole` per row so the receiver can still clear
 * them with a vertical I-piece. Cells pushed off the top are dropped
 * (top-out is the caller's job to detect via `collides` against the
 * active piece on the new board).
 */
export function pushGarbage(board: Board, lines: number, hole: number): Board {
  if (lines <= 0) return board
  const safeHole = ((hole % COLS) + COLS) % COLS
  const next: BoardCell[][] = []
  for (let i = lines; i < ROWS; i++) {
    next.push(board[i] ? [...board[i]!] : Array<BoardCell>(COLS).fill(''))
  }
  for (let i = 0; i < lines; i++) {
    const row: BoardCell[] = Array<BoardCell>(COLS).fill('G')
    row[safeHole] = ''
    next.push(row)
  }
  while (next.length < ROWS) {
    next.unshift(Array<BoardCell>(COLS).fill(''))
  }
  return next.slice(-ROWS)
}
