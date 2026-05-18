import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const here = dirname(fileURLToPath(import.meta.url))

// Game-widget source files that must NOT touch the live HEP packet
// stream. Keep this list aligned with the dashboard widget registry.
const GAME_FILES = [
  'ChessPanel.tsx',
  'ChessBoard.tsx',
  'NetChessPanel.tsx',
  'NetrisPanel.tsx',
  'chessCore.ts',
  'chessEngine.ts',
  'chessEngine.worker.ts',
]

// Token deny-list applied to the raw source. We match on identifiers
// and URL fragments rather than imports so the test still bites if
// someone bypasses ES module imports via `await import(…)` or string
// concatenation.
const FORBIDDEN = [
  'openHepStream',
  '/api/v4/stream',
  'hepstream',
]

describe('games isolation: chess / netchess / netris widgets do not touch the live packet stream', () => {
  for (const file of GAME_FILES) {
    it(`${file} must not reference any HEP packet stream symbol`, () => {
      const src = readFileSync(join(here, file), 'utf8')
      for (const needle of FORBIDDEN) {
        expect(
          src,
          `${file} contains forbidden token "${needle}". Game widgets must not subscribe to the live HEP stream (/api/v4/stream/hep). If you need packet data for a new minigame, wire it through a dedicated panel like SIPetrisPanel / PacketDefenderPanel instead.`,
        ).not.toContain(needle)
      }
    })
  }

  it('deny-list still bites itself (meta-test)', () => {
    expect(FORBIDDEN.some((needle) => 'openHepStream("/api/v4/stream/hep")'.includes(needle))).toBe(
      true,
    )
  })
})
