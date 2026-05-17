import { describe, expect, it } from 'vitest'
import { newGame, tryMove } from './chessCore'
import { playEngineMove, search, whiteRelativeCp } from './chessEngine'

describe('engine: starting position is sane', () => {
  it('returns a legal first move for white at depth 2', () => {
    const r = search(newGame().fen(), { depth: 2 })
    expect(r.uci).toBeTruthy()
    expect(r.uci!.length).toBeGreaterThanOrEqual(4)
  })
  it('never reports a mate score in the opening', () => {
    const r = search(newGame().fen(), { depth: 2 })
    expect(Math.abs(r.evalCp)).toBeLessThan(50_000)
  })
})

describe('engine: tactics', () => {
  it('finds mate-in-1: classic back-rank', () => {
    // Black to move; ...Rxe1 is mate.  Setup: White king on g1, no escape;
    // Black rook coming to e1 mates because c1 is blocked by the white knight.
    // Position: rnbqkbnr/pppp1ppp/8/8/8/8/PPPPPPPP/RNBQKBNR — replaced by a
    // hand-crafted FEN that guarantees mate-in-1.
    //   Pieces: white K g1, R a1, N c1, P f2 g2 h2; black R e8 a8, K g8.
    //   Black to move, ...Re1#.
    const fen = 'r3r1k1/8/8/8/8/8/5PPP/R1N3K1 b - - 0 1'
    const r = search(fen, { depth: 2 })
    expect(r.uci).toBe('e8e1')
  })

  it('does not give away a piece for nothing at depth 2', () => {
    // After 1.e4 e5 2.Nf3 Nc6 3.Bc4 Bc5, the engine as White should not
    // play 4.Nxe5? because Nxe5 loses a piece to ...Nxe5 (no fork available).
    const g = newGame()
    tryMove(g, { from: 'e2', to: 'e4' })
    tryMove(g, { from: 'e7', to: 'e5' })
    tryMove(g, { from: 'g1', to: 'f3' })
    tryMove(g, { from: 'b8', to: 'c6' })
    tryMove(g, { from: 'f1', to: 'c4' })
    tryMove(g, { from: 'f8', to: 'c5' })
    const r = search(g.fen(), { depth: 3 })
    expect(r.uci).not.toBe('f3e5')
  })

  it('takes a free queen when offered', () => {
    // Black queen sits on d4, undefended (back-rank bishops blocked
    // by pawns, no pawn on c5/e5). White knight on b3 attacks d4 and
    // can capture for free. White to move.
    const fen = 'rnb1kbnr/pppppppp/8/8/3q4/1N6/PPPPPPPP/R1BQKBNR w KQkq - 0 1'
    const r = search(fen, { depth: 2 })
    expect(r.uci).toBe('b3d4')
  })
})

describe('engine: terminal positions', () => {
  it('returns null UCI when there are no legal moves (stalemate)', () => {
    // 7k/5K2/6Q1/8/8/8/8/8 b - - 0 1 is stalemate for black to move.
    const r = search('7k/5K2/6Q1/8/8/8/8/8 b - - 0 1', { depth: 2 })
    expect(r.uci).toBeNull()
  })
})

describe('engine: depth 0 is random-legal', () => {
  it('returns *some* legal move with seed reproducibility', () => {
    const r1 = search(newGame().fen(), { depth: 0, seed: 42 })
    const r2 = search(newGame().fen(), { depth: 0, seed: 42 })
    expect(r1.uci).toBe(r2.uci)
    expect(r1.uci).toBeTruthy()
  })
})

describe('engine: playEngineMove', () => {
  it('applies the chosen move to the supplied game', () => {
    const g = newGame()
    const { result, move } = playEngineMove(g, { depth: 1 })
    expect(result.uci).toBeTruthy()
    expect(move).not.toBeNull()
    expect(g.fen()).not.toBe(newGame().fen())
  })
})

describe('whiteRelativeCp', () => {
  it('flips sign for black', () => {
    expect(whiteRelativeCp('w', 120)).toBe(120)
    expect(whiteRelativeCp('b', 120)).toBe(-120)
  })
})

describe('engine: cancellation', () => {
  it('honours an aborted signal and still returns a legal move', () => {
    const ctrl = new AbortController()
    // Abort immediately to force the abort path.
    ctrl.abort()
    const r = search(newGame().fen(), { depth: 4, signal: ctrl.signal })
    expect(r.uci).toBeTruthy()
    expect(r.aborted).toBe(true)
  })
})
