import { describe, expect, it } from 'vitest'
import {
  STARTING_FEN,
  describeUciOnFen,
  fromPGN,
  isValidFen,
  legalMoves,
  newGame,
  parseUci,
  snapshot,
  status,
  toPGN,
  tryMove,
  tryUci,
  turn,
  undoMove,
} from './chessCore'

describe('parseUci', () => {
  it('parses 4-letter coordinates', () => {
    expect(parseUci('e2e4')).toEqual({ from: 'e2', to: 'e4' })
  })
  it('parses promotion suffix', () => {
    expect(parseUci('e7e8q')).toEqual({ from: 'e7', to: 'e8', promotion: 'q' })
  })
  it('rejects bad inputs', () => {
    expect(parseUci('')).toBeNull()
    expect(parseUci('e2')).toBeNull()
    expect(parseUci('z9z1')).toBeNull()
    expect(parseUci('e2e4x')).toBeNull()
    expect(parseUci('e7e8k')).toBeNull() // king promotion is not legal in chess
  })
  it('is case-insensitive and trims', () => {
    expect(parseUci('  E2E4  ')).toEqual({ from: 'e2', to: 'e4' })
  })
})

describe('newGame / isValidFen', () => {
  it('starts at the standard position by default', () => {
    const g = newGame()
    expect(g.fen()).toBe(STARTING_FEN)
    expect(turn(g)).toBe('w')
  })
  it('accepts a valid mid-game FEN', () => {
    const fen = 'rnbqkbnr/pppp1ppp/8/4p3/4P3/8/PPPP1PPP/RNBQKBNR w KQkq - 0 2'
    const g = newGame(fen)
    expect(g.fen()).toBe(fen)
  })
  it('rejects a malformed FEN', () => {
    expect(() => newGame('not a fen at all')).toThrow()
    expect(isValidFen('not a fen at all')).toBe(false)
    expect(isValidFen(STARTING_FEN)).toBe(true)
  })
})

describe('legalMoves', () => {
  it('returns 20 moves from the starting position', () => {
    const g = newGame()
    expect(legalMoves(g)).toHaveLength(20)
  })
  it('filters by from-square', () => {
    const g = newGame()
    const fromE2 = legalMoves(g, 'e2')
    expect(fromE2.map((m) => m.to).sort()).toEqual(['e3', 'e4'])
  })
  it('returns empty for an empty square', () => {
    const g = newGame()
    expect(legalMoves(g, 'e4')).toEqual([])
  })
})

describe('tryMove', () => {
  it('plays a legal move and returns a descriptor', () => {
    const g = newGame()
    const m = tryMove(g, { from: 'e2', to: 'e4' })
    expect(m).not.toBeNull()
    expect(m!.san).toBe('e4')
    expect(m!.uci).toBe('e2e4')
    expect(m!.piece).toBe('p')
    expect(m!.color).toBe('w')
    expect(turn(g)).toBe('b')
  })
  it('rejects an illegal move without mutating the game', () => {
    const g = newGame()
    const before = g.fen()
    const m = tryMove(g, { from: 'e2', to: 'e5' })
    expect(m).toBeNull()
    expect(g.fen()).toBe(before)
  })
  it('handles promotions', () => {
    const fen = '8/4P3/8/8/8/8/k7/4K3 w - - 0 1'
    const g = newGame(fen)
    const m = tryMove(g, { from: 'e7', to: 'e8', promotion: 'q' })
    expect(m).not.toBeNull()
    expect(m!.promotion).toBe('q')
    expect(m!.san).toMatch(/^e8=Q/)
  })
})

describe('tryUci', () => {
  it('parses and plays in one shot', () => {
    const g = newGame()
    const m = tryUci(g, 'e2e4')
    expect(m?.uci).toBe('e2e4')
  })
  it('returns null on garbage UCI', () => {
    const g = newGame()
    expect(tryUci(g, 'zzzz')).toBeNull()
  })
})

describe('undoMove', () => {
  it('reverts the last move and restores turn', () => {
    const g = newGame()
    tryMove(g, { from: 'e2', to: 'e4' })
    expect(turn(g)).toBe('b')
    const u = undoMove(g)
    expect(u).not.toBeNull()
    expect(u!.uci).toBe('e2e4')
    expect(g.fen()).toBe(STARTING_FEN)
    expect(turn(g)).toBe('w')
  })
  it('returns null on an empty history', () => {
    const g = newGame()
    expect(undoMove(g)).toBeNull()
  })
})

describe('status', () => {
  it('reports ongoing for a fresh game', () => {
    expect(status(newGame())).toBe('ongoing')
  })
  it('reports checkmate after Fool\'s Mate', () => {
    const g = newGame()
    tryMove(g, { from: 'f2', to: 'f3' })
    tryMove(g, { from: 'e7', to: 'e5' })
    tryMove(g, { from: 'g2', to: 'g4' })
    tryMove(g, { from: 'd8', to: 'h4' })
    expect(status(g)).toBe('checkmate')
  })
  it('reports stalemate', () => {
    // Classic stalemate position: black king h8, white king f7, white queen g6, black to move.
    const stalemateFen = '7k/5K2/6Q1/8/8/8/8/8 b - - 0 1'
    const g = newGame(stalemateFen)
    expect(status(g)).toBe('stalemate')
  })
  it('reports insufficient material with K vs K', () => {
    const kvk = '8/8/8/8/4k3/8/8/4K3 w - - 0 1'
    const g = newGame(kvk)
    expect(status(g)).toBe('draw_insufficient')
  })
})

describe('PGN round-trip', () => {
  it('exports a played game to PGN and reloads it', () => {
    const g = newGame()
    tryMove(g, { from: 'e2', to: 'e4' })
    tryMove(g, { from: 'e7', to: 'e5' })
    tryMove(g, { from: 'g1', to: 'f3' })
    const pgn = toPGN(g, { White: 'Player', Black: 'Bot', Event: 'Test' })
    expect(pgn).toContain('1. e4 e5 2. Nf3')

    const restored = fromPGN(pgn)
    expect(restored.game.fen()).toBe(g.fen())
    expect(restored.history.map((m) => m.san)).toEqual(['e4', 'e5', 'Nf3'])
  })
  it('throws on invalid PGN input', () => {
    expect(() => fromPGN('this is not pgn at all')).toThrow()
    expect(() => fromPGN('1. e9 e5')).toThrow()
  })
  it('accepts an empty PGN as a fresh game', () => {
    const { game, history } = fromPGN('')
    expect(game.fen()).toBe(STARTING_FEN)
    expect(history).toEqual([])
  })
})

describe('describeUciOnFen', () => {
  it('describes a move without mutating any persistent game', () => {
    const m = describeUciOnFen(STARTING_FEN, 'e2e4')
    expect(m?.san).toBe('e4')
  })
  it('returns null for illegal moves on the given FEN', () => {
    expect(describeUciOnFen(STARTING_FEN, 'e2e5')).toBeNull()
  })
  it('returns null for malformed FEN', () => {
    expect(describeUciOnFen('not a fen', 'e2e4')).toBeNull()
  })
})

describe('snapshot', () => {
  it('returns an 8x8 grid with starting pieces', () => {
    const board = snapshot(newGame())
    expect(board).toHaveLength(8)
    expect(board[0]).toHaveLength(8)
    expect(board[0]![0]).toEqual({ type: 'r', color: 'b', square: 'a8' })
    expect(board[7]![4]).toEqual({ type: 'k', color: 'w', square: 'e1' })
    expect(board[3]![3]).toBeNull()
  })
})
