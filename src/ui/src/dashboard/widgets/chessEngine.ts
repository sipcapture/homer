/**
 * chessEngine — minimax + alpha-beta search with a small evaluator.
 *
 * Synchronous, pure-function entrypoint (`search`). The Web-Worker
 * wrapper lives in `chessEngine.worker.ts`; this module has no DOM
 * or worker dependencies so it can be unit-tested directly.
 *
 * Evaluator: material + Michniewski piece-square tables (public-
 * domain, originally from chessprogramming.org). White-relative
 * centipawns; the score is negated for black's turn at the search
 * root so we can call it from either side.
 *
 * Search:
 *   - alpha-beta negamax with move ordering by MVV-LVA
 *   - depth 1..4 from the UI slider; the worker can also pass a
 *     deadline so the search aborts early
 *   - "level 0" returns a random legal move (for the easiest setting)
 *   - on mate, score is sign(depth_to_mate) * (MATE_SCORE - distance)
 *     so shorter mates beat longer ones
 *
 * Not designed to be strong — strong enough to be a credible opponent
 * to a casual player at the deepest level.
 */

import { Chess } from 'chess.js'
import {
  type ChessColor,
  type ChessPieceType,
  type ChessSquare,
  type SanMove,
  newGame,
  tryMove,
} from './chessCore'

export interface SearchOptions {
  /** Half-move plies. 0 = random legal, 1..4 = minimax depth. */
  depth: number
  /** Deterministic mode for tests — fixes the tie-break ordering. */
  seed?: number
  /** If set, the search aborts and returns the best move found so
   *  far when this signal fires. The worker uses it for cancellation. */
  signal?: AbortSignal
}

export interface SearchResult {
  /** UCI of the best move, or null if the position has no legal moves
   *  (mate / stalemate — caller should react accordingly). */
  uci: string | null
  /** Evaluation from the perspective of the side to move at the root,
   *  in centipawns. Positive = good for the side to move. */
  evalCp: number
  /** Principal variation in UCI strings (best line found). May be
   *  shorter than `depth` when the line is forced or pruned. */
  pv: string[]
  /** Number of nodes searched — useful for diagnostics. */
  nodes: number
  /** True if the search was aborted via `signal` and returned a
   *  best-so-far rather than the depth-complete result. */
  aborted: boolean
}

const PIECE_VALUE: Record<ChessPieceType, number> = {
  p: 100,
  n: 320,
  b: 330,
  r: 500,
  q: 900,
  k: 20000,
}

/** Michniewski piece-square tables, indexed [rank0=8th_rank..rank7=1st_rank][file_a..file_h]
 *  from White's perspective. Black squares are mirrored at lookup. */
const PST: Record<ChessPieceType, number[][]> = {
  p: [
    [ 0,  0,  0,  0,  0,  0,  0,  0],
    [50, 50, 50, 50, 50, 50, 50, 50],
    [10, 10, 20, 30, 30, 20, 10, 10],
    [ 5,  5, 10, 25, 25, 10,  5,  5],
    [ 0,  0,  0, 20, 20,  0,  0,  0],
    [ 5, -5,-10,  0,  0,-10, -5,  5],
    [ 5, 10, 10,-20,-20, 10, 10,  5],
    [ 0,  0,  0,  0,  0,  0,  0,  0],
  ],
  n: [
    [-50,-40,-30,-30,-30,-30,-40,-50],
    [-40,-20,  0,  0,  0,  0,-20,-40],
    [-30,  0, 10, 15, 15, 10,  0,-30],
    [-30,  5, 15, 20, 20, 15,  5,-30],
    [-30,  0, 15, 20, 20, 15,  0,-30],
    [-30,  5, 10, 15, 15, 10,  5,-30],
    [-40,-20,  0,  5,  5,  0,-20,-40],
    [-50,-40,-30,-30,-30,-30,-40,-50],
  ],
  b: [
    [-20,-10,-10,-10,-10,-10,-10,-20],
    [-10,  0,  0,  0,  0,  0,  0,-10],
    [-10,  0,  5, 10, 10,  5,  0,-10],
    [-10,  5,  5, 10, 10,  5,  5,-10],
    [-10,  0, 10, 10, 10, 10,  0,-10],
    [-10, 10, 10, 10, 10, 10, 10,-10],
    [-10,  5,  0,  0,  0,  0,  5,-10],
    [-20,-10,-10,-10,-10,-10,-10,-20],
  ],
  r: [
    [ 0,  0,  0,  0,  0,  0,  0,  0],
    [ 5, 10, 10, 10, 10, 10, 10,  5],
    [-5,  0,  0,  0,  0,  0,  0, -5],
    [-5,  0,  0,  0,  0,  0,  0, -5],
    [-5,  0,  0,  0,  0,  0,  0, -5],
    [-5,  0,  0,  0,  0,  0,  0, -5],
    [-5,  0,  0,  0,  0,  0,  0, -5],
    [ 0,  0,  0,  5,  5,  0,  0,  0],
  ],
  q: [
    [-20,-10,-10, -5, -5,-10,-10,-20],
    [-10,  0,  0,  0,  0,  0,  0,-10],
    [-10,  0,  5,  5,  5,  5,  0,-10],
    [ -5,  0,  5,  5,  5,  5,  0, -5],
    [  0,  0,  5,  5,  5,  5,  0, -5],
    [-10,  5,  5,  5,  5,  5,  0,-10],
    [-10,  0,  5,  0,  0,  0,  0,-10],
    [-20,-10,-10, -5, -5,-10,-10,-20],
  ],
  // Middlegame king table; we don't bother switching to an endgame
  // table — strength loss is small for a 4-ply search.
  k: [
    [-30,-40,-40,-50,-50,-40,-40,-30],
    [-30,-40,-40,-50,-50,-40,-40,-30],
    [-30,-40,-40,-50,-50,-40,-40,-30],
    [-30,-40,-40,-50,-50,-40,-40,-30],
    [-20,-30,-30,-40,-40,-30,-30,-20],
    [-10,-20,-20,-20,-20,-20,-20,-10],
    [ 20, 20,  0,  0,  0,  0, 20, 20],
    [ 20, 30, 10,  0,  0, 10, 30, 20],
  ],
}

const MATE_SCORE = 100_000

interface AbortChecker {
  hit: boolean
}

class AbortedError extends Error {
  constructor() { super('search aborted') }
}

function squareToRC(sq: ChessSquare): { rank: number; file: number } {
  // chess.js squares are file-letter + rank-digit, e.g. "e2".
  // We index PSTs as [rank_from_top_for_white][file] where
  // file 'a' = 0..'h' = 7 and rank '8' = 0..'1' = 7.
  const file = sq.charCodeAt(0) - 97
  const rankDigit = sq.charCodeAt(1) - 48
  const rank = 8 - rankDigit
  return { rank, file }
}

/** Static evaluation in centipawns from White's perspective.
 *  Positive = White is better. Side-relative scoring is applied at
 *  the search root by the caller of `evaluate`. */
function evaluate(game: Chess): number {
  // Terminal positions first — we want big positive/negative numbers
  // so the search prefers (or avoids) them irrespective of material.
  if (game.isCheckmate()) {
    // Side to move is checkmated → bad for that side. The leaf-side
    // is the *to-move* side, so return negative for the side to move,
    // which in white-relative terms is:
    //   - if it's white's turn and white is mated, return -MATE
    //   - if it's black's turn and black is mated, return +MATE
    return game.turn() === 'w' ? -MATE_SCORE : MATE_SCORE
  }
  if (
    game.isStalemate() ||
    game.isInsufficientMaterial() ||
    game.isThreefoldRepetition() ||
    game.isDrawByFiftyMoves()
  ) {
    return 0
  }

  let score = 0
  const board = game.board()
  for (let r = 0; r < 8; r++) {
    const row = board[r]
    if (!row) continue
    for (let f = 0; f < 8; f++) {
      const cell = row[f]
      if (!cell) continue
      const v = PIECE_VALUE[cell.type as ChessPieceType]
      // PST index: for white pieces, use [r][f]; for black, mirror rank.
      const pst = PST[cell.type as ChessPieceType]
      const pstVal = cell.color === 'w' ? pst[r]![f]! : pst[7 - r]![f]!
      const total = v + pstVal
      score += cell.color === 'w' ? total : -total
    }
  }
  return score
}

/** Compose a verbose Move into a sortable key for move ordering.
 *  Captures by least-valued attacker on most-valued victim go first;
 *  promotions go before plain quiet moves. */
function moveOrderKey(m: {
  captured?: ChessPieceType
  piece: ChessPieceType
  promotion?: ChessPieceType
}): number {
  let score = 0
  if (m.captured) {
    score += 10 * PIECE_VALUE[m.captured] - PIECE_VALUE[m.piece]
  }
  if (m.promotion) {
    score += PIECE_VALUE[m.promotion]
  }
  return score
}

interface OrderedMove {
  from: ChessSquare
  to: ChessSquare
  promotion?: 'q' | 'r' | 'b' | 'n'
  key: number
}

function orderedLegalMoves(game: Chess): OrderedMove[] {
  // chess.js v1.x: moves({verbose:true}) on a position with no legal
  // moves returns []. The caller is responsible for treating that as
  // a terminal position.
  const verbose = game.moves({ verbose: true })
  const out: OrderedMove[] = verbose.map((m) => ({
    from: m.from as ChessSquare,
    to: m.to as ChessSquare,
    promotion: m.promotion as 'q' | 'r' | 'b' | 'n' | undefined,
    key: moveOrderKey({
      captured: m.captured as ChessPieceType | undefined,
      piece: m.piece as ChessPieceType,
      promotion: m.promotion as ChessPieceType | undefined,
    }),
  }))
  out.sort((a, b) => b.key - a.key)
  return out
}

/** Negamax with alpha-beta. Returns side-to-move-relative cp. */
function negamax(
  game: Chess,
  depth: number,
  alpha: number,
  beta: number,
  ply: number,
  abort: AbortChecker,
): { score: number; pv: OrderedMove[] } {
  if (abort.hit) throw new AbortedError()

  if (depth <= 0 || game.isGameOver()) {
    // Evaluate from the perspective of the side to move.
    const whiteRel = evaluate(game)
    const side = game.turn() === 'w' ? 1 : -1
    // Adjust mate scores by ply so shallower mates win.
    if (whiteRel >= MATE_SCORE / 2) {
      return { score: side * (whiteRel - ply), pv: [] }
    }
    if (whiteRel <= -MATE_SCORE / 2) {
      return { score: side * (whiteRel + ply), pv: [] }
    }
    return { score: side * whiteRel, pv: [] }
  }

  const moves = orderedLegalMoves(game)
  if (moves.length === 0) {
    // No legal moves: either mate or stalemate. evaluate() captures it.
    const whiteRel = evaluate(game)
    const side = game.turn() === 'w' ? 1 : -1
    return { score: side * whiteRel, pv: [] }
  }

  let bestScore = -Infinity
  let bestPv: OrderedMove[] = []
  let bestMove: OrderedMove | null = null

  for (const m of moves) {
    game.move({ from: m.from, to: m.to, promotion: m.promotion })
    const child = negamax(game, depth - 1, -beta, -alpha, ply + 1, abort)
    game.undo()
    const score = -child.score
    if (score > bestScore) {
      bestScore = score
      bestMove = m
      bestPv = [m, ...child.pv]
    }
    if (score > alpha) alpha = score
    if (alpha >= beta) break
  }

  if (!bestMove) {
    // Shouldn't be reachable — moves was non-empty.
    return { score: 0, pv: [] }
  }
  return { score: bestScore, pv: bestPv }
}

/** Best legal move for the current side to move on `fen`. */
export function search(fen: string, opts: SearchOptions): SearchResult {
  const game = newGame(fen)
  const moves = orderedLegalMoves(game)
  if (moves.length === 0) {
    return { uci: null, evalCp: 0, pv: [], nodes: 0, aborted: false }
  }
  // Level 0 → random legal move (intended for absolute-beginner mode).
  if (opts.depth <= 0) {
    const pick = pickDeterministic(moves, opts.seed)
    return { uci: moveUci(pick), evalCp: 0, pv: [moveUci(pick)], nodes: 1, aborted: false }
  }
  const abort: AbortChecker = { hit: false }
  // Honour a signal that was already aborted by the time we got
  // called (e.g. immediate cancellation right before the call).
  if (opts.signal?.aborted) {
    abort.hit = true
  }
  const listener = () => { abort.hit = true }
  opts.signal?.addEventListener('abort', listener)
  try {
    const { score, pv } = negamax(game, opts.depth, -Infinity, Infinity, 0, abort)
    return {
      uci: pv[0] ? moveUci(pv[0]) : null,
      evalCp: score,
      pv: pv.map(moveUci),
      nodes: 0, // node counting kept for future; not needed for tests yet
      aborted: false,
    }
  } catch (e) {
    if (e instanceof AbortedError) {
      // Return a shallow best-move at the root by re-searching depth=1.
      const shallow = negamax(newGame(fen), 1, -Infinity, Infinity, 0, { hit: false })
      return {
        uci: shallow.pv[0] ? moveUci(shallow.pv[0]) : moveUci(moves[0]!),
        evalCp: shallow.score,
        pv: shallow.pv.map(moveUci),
        nodes: 0,
        aborted: true,
      }
    }
    throw e
  } finally {
    opts.signal?.removeEventListener('abort', listener)
  }
}

function moveUci(m: { from: ChessSquare; to: ChessSquare; promotion?: string }): string {
  return `${m.from}${m.to}${m.promotion ?? ''}`
}

/** Deterministic pick from a small array. The "deterministic" bit
 *  matters only for tests; in production seed is undefined and we use
 *  `Math.random()`. */
function pickDeterministic<T>(arr: T[], seed?: number): T {
  if (arr.length === 0) throw new Error('pickDeterministic: empty array')
  if (seed === undefined) return arr[Math.floor(Math.random() * arr.length)]!
  // Tiny xorshift just so seed actually changes the pick.
  let x = (seed | 0) || 1
  x ^= x << 13
  x ^= x >>> 17
  x ^= x << 5
  const idx = Math.abs(x) % arr.length
  return arr[idx]!
}

/** Surface side-by-side helpers for tests / UI hints. */
export const __internal = {
  evaluate,
  PIECE_VALUE,
  MATE_SCORE,
  squareToRC,
}

/** Play one engine move on `game` and return the descriptor — used by
 *  unit tests and (optionally) by the panel when running the engine
 *  synchronously. The panel normally goes through the worker. */
export function playEngineMove(
  game: Chess,
  opts: SearchOptions,
): { result: SearchResult; move: SanMove | null } {
  const result = search(game.fen(), opts)
  if (!result.uci) return { result, move: null }
  const parsed = result.uci
  const from = parsed.slice(0, 2) as ChessSquare
  const to = parsed.slice(2, 4) as ChessSquare
  const promotion = parsed.length === 5
    ? (parsed.charAt(4) as 'q' | 'r' | 'b' | 'n')
    : undefined
  const move = tryMove(game, { from, to, promotion })
  return { result, move }
}

/** Convert a side-relative cp score to a White-relative number for UI
 *  display. Caller passes the colour that the score belongs to (the
 *  side to move at the position scored). */
export function whiteRelativeCp(side: ChessColor, sideToMoveScore: number): number {
  return side === 'w' ? sideToMoveScore : -sideToMoveScore
}
