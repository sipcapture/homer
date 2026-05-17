/**
 * chessCore — thin typed wrapper around `chess.js`.
 *
 * One place that knows about chess rules on the UI side. Used by both
 * the single-player `ChessPanel` and the multi-player `NetChessPanel`,
 * by `chessEngine` (minimax), and by the worker. Re-exports the
 * `chess.js` types we actually consume so consumers only need this
 * file. Keep it framework-free (no React).
 *
 * On chess.js 1.x:
 *   - `new Chess(fen?)` throws on an invalid FEN; we wrap it.
 *   - `chess.move(...)` throws on an illegal move; we wrap it so the
 *     caller gets a typed `null` instead of having to litter try/catch.
 *   - `chess.history({ verbose: true })` returns `Move[]` (the verbose
 *     descriptor objects, not just SAN strings).
 */

import { Chess, type Color, type PieceSymbol, type Square, validateFen } from 'chess.js'

export type ChessColor = Color // 'w' | 'b'
export type ChessPieceType = PieceSymbol // 'p' | 'n' | 'b' | 'r' | 'q' | 'k'
export type ChessSquare = Square

export interface ChessPiece {
  color: ChessColor
  type: ChessPieceType
}

export type Promotion = 'q' | 'r' | 'b' | 'n'

export interface UciMove {
  from: ChessSquare
  to: ChessSquare
  promotion?: Promotion
}

/** Descriptor for a move that already happened on a board. */
export interface SanMove extends UciMove {
  san: string
  /** UCI long-algebraic notation: `e2e4`, `e7e8q`. */
  uci: string
  color: ChessColor
  piece: ChessPieceType
  captured?: ChessPieceType
  /** FEN after the move was applied. */
  fenAfter: string
  /** FEN before the move was applied. */
  fenBefore: string
  isCapture: boolean
  isCheck: boolean
  isCheckmate: boolean
  isCastleKingside: boolean
  isCastleQueenside: boolean
  isEnPassant: boolean
}

export type GameStatus =
  | 'ongoing'
  | 'checkmate'
  | 'stalemate'
  | 'draw_50'
  | 'draw_3fold'
  | 'draw_insufficient'
  | 'draw'

/** Standard chess starting position. */
export const STARTING_FEN = 'rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1'

/** Construct a new game. Throws if `fen` is provided but invalid so the
 *  caller can decide whether to fall back to the default. */
export function newGame(fen?: string): Chess {
  if (fen && fen !== STARTING_FEN) {
    const check = validateFen(fen)
    if (!check.ok) {
      throw new Error(`invalid fen: ${check.error ?? 'unknown'}`)
    }
  }
  return fen ? new Chess(fen) : new Chess()
}

/** Cheap FEN-only validation; useful when surface-level rejecting
 *  server frames before constructing a Chess instance. */
export function isValidFen(fen: string): boolean {
  return validateFen(fen).ok
}

/** UCI long-algebraic for a verbose Move: `from` + `to` + optional promotion letter. */
function moveToUci(m: { from: Square; to: Square; promotion?: PieceSymbol }): string {
  return `${m.from}${m.to}${m.promotion ?? ''}`
}

/** Parse a UCI string (`e2e4`, `e7e8q`) into our `UciMove`. Returns null
 *  on malformed input. We do NOT consult board state here — legality
 *  is checked by `tryMove`. */
export function parseUci(uci: string): UciMove | null {
  if (typeof uci !== 'string') return null
  const trimmed = uci.trim().toLowerCase()
  if (trimmed.length !== 4 && trimmed.length !== 5) return null
  const from = trimmed.slice(0, 2)
  const to = trimmed.slice(2, 4)
  const promo = trimmed.length === 5 ? trimmed.charAt(4) : undefined
  if (!/^[a-h][1-8]$/.test(from) || !/^[a-h][1-8]$/.test(to)) return null
  if (promo !== undefined && !/^[qrbn]$/.test(promo)) return null
  return {
    from: from as ChessSquare,
    to: to as ChessSquare,
    promotion: promo as Promotion | undefined,
  }
}

/** Build a fully-typed SanMove descriptor from a chess.js verbose Move
 *  and the FEN snapshots we captured around the call site. Kept here so
 *  every caller produces the same shape. */
function buildSan(
  m: {
    color: Color
    from: Square
    to: Square
    piece: PieceSymbol
    captured?: PieceSymbol
    promotion?: PieceSymbol
    san: string
    isCapture: () => boolean
    isPromotion: () => boolean
    isEnPassant: () => boolean
    isKingsideCastle: () => boolean
    isQueensideCastle: () => boolean
  },
  fenBefore: string,
  fenAfter: string,
  isCheck: boolean,
  isCheckmate: boolean,
): SanMove {
  return {
    from: m.from,
    to: m.to,
    promotion: m.promotion as Promotion | undefined,
    san: m.san,
    uci: moveToUci(m),
    color: m.color,
    piece: m.piece,
    captured: m.captured,
    fenBefore,
    fenAfter,
    isCapture: m.isCapture(),
    isCheck,
    isCheckmate,
    isCastleKingside: m.isKingsideCastle(),
    isCastleQueenside: m.isQueensideCastle(),
    isEnPassant: m.isEnPassant(),
  }
}

/** All legal moves from the given square (or from any square when
 *  `from` is undefined). The returned descriptors include SAN — handy
 *  for highlighting target squares and rendering hints. */
export function legalMoves(game: Chess, from?: ChessSquare): SanMove[] {
  const verbose = from
    ? game.moves({ verbose: true, square: from })
    : game.moves({ verbose: true })
  const fenBefore = game.fen()
  return verbose.map((m) => ({
    from: m.from,
    to: m.to,
    promotion: m.promotion as Promotion | undefined,
    san: m.san,
    uci: moveToUci(m),
    color: m.color,
    piece: m.piece,
    captured: m.captured,
    fenBefore,
    // We don't *play* the move here — fenAfter is m.after, which
    // chess.js v1.x populates on verbose moves.
    fenAfter: (m as unknown as { after?: string }).after ?? fenBefore,
    isCapture: m.isCapture(),
    isCheck: false, // computed only after the move is actually played
    isCheckmate: false,
    isCastleKingside: m.isKingsideCastle(),
    isCastleQueenside: m.isQueensideCastle(),
    isEnPassant: m.isEnPassant(),
  }))
}

/** Try to play `move` on `game`. Returns the resulting `SanMove`
 *  descriptor on success, or `null` if the move is illegal in the
 *  current position. Mutates `game`. */
export function tryMove(game: Chess, move: UciMove): SanMove | null {
  const fenBefore = game.fen()
  let m: ReturnType<Chess['move']> | null = null
  try {
    m = game.move({ from: move.from, to: move.to, promotion: move.promotion })
  } catch {
    return null
  }
  if (!m) return null
  const fenAfter = game.fen()
  return buildSan(m, fenBefore, fenAfter, game.isCheck(), game.isCheckmate())
}

/** Same as `tryMove` but parses UCI first. Convenience for handlers
 *  that receive UCI off the wire. */
export function tryUci(game: Chess, uci: string): SanMove | null {
  const parsed = parseUci(uci)
  if (!parsed) return null
  return tryMove(game, parsed)
}

/** Roll back the last half-move. Returns the popped descriptor or null
 *  if the history is empty. */
export function undoMove(game: Chess): SanMove | null {
  const before = game.fen()
  const undone = game.undo()
  if (!undone) return null
  const after = game.fen()
  // history's "before" is the position we just restored to — i.e.
  // chess.js's notion of `before` for the undone move. We flip the
  // FEN labelling so the descriptor still reads "fenBefore → fenAfter"
  // in the direction of game progression.
  return buildSan(undone, after, before, game.isCheck(), game.isCheckmate())
}

/** Inspect the current game status. `ongoing` if no terminal condition
 *  has triggered. */
export function status(game: Chess): GameStatus {
  if (game.isCheckmate()) return 'checkmate'
  if (game.isStalemate()) return 'stalemate'
  if (game.isInsufficientMaterial()) return 'draw_insufficient'
  if (game.isThreefoldRepetition()) return 'draw_3fold'
  if (game.isDrawByFiftyMoves()) return 'draw_50'
  if (game.isDraw()) return 'draw'
  return 'ongoing'
}

/** Whose turn it is — exposed so widgets don't have to import chess.js
 *  directly. */
export function turn(game: Chess): ChessColor {
  return game.turn()
}

/** Export the played-so-far game as PGN. `headers` lets the caller
 *  inject metadata (Event/Site/White/Black/Result) without touching
 *  chess.js internals. */
export function toPGN(game: Chess, headers?: Record<string, string>): string {
  if (headers) {
    for (const [k, v] of Object.entries(headers)) {
      game.header(k, v)
    }
  }
  return game.pgn()
}

/** Reconstruct a game from PGN. Returns the Chess instance plus the
 *  history as our SanMove descriptors. Throws if the PGN is invalid. */
export function fromPGN(pgn: string): { game: Chess; history: SanMove[] } {
  const game = new Chess()
  game.loadPgn(pgn)
  const verbose = game.history({ verbose: true })
  // chess.js verbose history items carry `before` / `after` FENs we
  // can lean on; we still recompute check/mate by replaying.
  const replay = new Chess()
  const history: SanMove[] = []
  for (const v of verbose) {
    const fenBefore = replay.fen()
    const m = replay.move({ from: v.from, to: v.to, promotion: v.promotion })
    if (!m) {
      // Defensive: PGN parsed but a move from history fails to replay.
      // Treat as corrupt input.
      throw new Error(`pgn replay failed at ${v.san}`)
    }
    const fenAfter = replay.fen()
    history.push(buildSan(m, fenBefore, fenAfter, replay.isCheck(), replay.isCheckmate()))
  }
  return { game, history }
}

/** Convenience: parse a UCI string against a position FEN and return
 *  the descriptor without mutating any persistent game. Returns null
 *  for malformed UCI or illegal moves. Used by the server-bound LLM
 *  validator path on the UI side and by tests. */
export function describeUciOnFen(fen: string, uci: string): SanMove | null {
  const parsed = parseUci(uci)
  if (!parsed) return null
  let game: Chess
  try {
    game = new Chess(fen)
  } catch {
    return null
  }
  return tryMove(game, parsed)
}

/** Cell of a board snapshot — same shape as chess.js's verbose board
 *  cells (`type`, `color`, `square`), with explicit nulls. */
export interface BoardCell {
  type: ChessPieceType
  color: ChessColor
  square: ChessSquare
}

/** 8x8 board snapshot — outer index 0 = rank 8 (top), inner index 0 =
 *  file a (left). Mirrors chess.js's `board()` shape. */
export type BoardSnapshot = (BoardCell | null)[][]

export function snapshot(game: Chess): BoardSnapshot {
  return game.board().map((row) => row.map((cell) => (cell ? { ...cell } : null)))
}
