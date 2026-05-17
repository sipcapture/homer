/// <reference lib="webworker" />
/**
 * chessEngine.worker — Web Worker around `chessEngine.search`.
 *
 * Wire format:
 *   in  → { id: number, fen: string, depth: number }
 *   out ← { id: number, uci: string|null, evalCp: number, pv: string[] }
 *
 * The worker is single-threaded by definition and we don't pre-empt
 * a running search — the panel cancels by terminating the worker and
 * spawning a fresh one (cheap; the minimax has no warm-up state).
 *
 * Loaded via the Vite `new Worker(new URL(...), {type:"module"})`
 * pattern from `ChessPanel.tsx`.
 */

import { search } from './chessEngine'

interface InMsg {
  id: number
  fen: string
  depth: number
}

const ctx: DedicatedWorkerGlobalScope = self as unknown as DedicatedWorkerGlobalScope

ctx.onmessage = (e: MessageEvent<InMsg>) => {
  const { id, fen, depth } = e.data
  try {
    const r = search(fen, { depth })
    ctx.postMessage({
      id,
      uci: r.uci,
      evalCp: r.evalCp,
      pv: r.pv,
    })
  } catch (err) {
    ctx.postMessage({
      id,
      uci: null,
      evalCp: 0,
      pv: [],
      error: err instanceof Error ? err.message : 'engine error',
    })
  }
}
