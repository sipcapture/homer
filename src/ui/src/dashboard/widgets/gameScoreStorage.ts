/** localStorage keys for dashboard mini-games best scores */

const PREFIX = 'homer_game_best_'

export function readBestScore(gameId: string): number {
  try {
    const v = localStorage.getItem(PREFIX + gameId)
    if (v == null || v === '') return 0
    const n = parseInt(v, 10)
    return Number.isFinite(n) && n >= 0 ? n : 0
  } catch {
    return 0
  }
}

/** Persists if score beats stored best; returns the new best (max of previous and score). */
export function writeBestScoreIfHigher(gameId: string, score: number): number {
  const prev = readBestScore(gameId)
  const next = Math.max(prev, Math.floor(score))
  if (next > prev) {
    try {
      localStorage.setItem(PREFIX + gameId, String(next))
    } catch {
      /* ignore quota / private mode */
    }
  }
  return next
}
