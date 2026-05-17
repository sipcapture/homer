/**
 * ChessBoard — presentational 8x8 board.
 *
 * No game state of its own. The owner (`ChessPanel`, `NetChessPanel`)
 * passes a FEN; the board renders it, accepts clicks, and reports
 * a candidate move back via `onMove`. Pieces are Unicode glyphs so
 * we don't need a separate asset pipeline; that's good enough at
 * widget cell sizes (24–80 px).
 *
 * Interaction model — click-to-select then click-to-move:
 *   1. Click your own piece → it gets a "selected" outline + legal
 *      destination dots appear on every reachable square.
 *   2. Click a destination dot → emit onMove({from, to}). If the move
 *      is a pawn promotion, a small popover asks Q/R/B/N first.
 *   3. Click the selected piece again, or anywhere illegal, deselects.
 *
 * The component never validates moves authoritatively — the parent
 * decides what to do with `onMove`. We compute `legalMoves` only so
 * the destination dots are accurate (and to know whether a promotion
 * picker is needed). If `interactive=false`, the click handlers are
 * no-ops and the destination dots are not shown.
 */

import { useMemo, useState } from 'react'
import { cn } from '@/lib/utils'
import {
  type BoardSnapshot,
  type ChessColor,
  type ChessPieceType,
  type ChessSquare,
  type Promotion,
  type UciMove,
  legalMoves,
  newGame,
  snapshot,
  turn,
} from './chessCore'

const FILES = ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h'] as const
const RANKS = ['8', '7', '6', '5', '4', '3', '2', '1'] as const

const PIECE_GLYPH: Record<ChessColor, Record<ChessPieceType, string>> = {
  w: { k: '♔', q: '♕', r: '♖', b: '♗', n: '♘', p: '♙' },
  b: { k: '♚', q: '♛', r: '♜', b: '♝', n: '♞', p: '♟' },
}

/** Square is "light" if (file + rank) is even, "dark" otherwise. */
function squareShade(square: ChessSquare): 'light' | 'dark' {
  const fileIdx = FILES.indexOf(square.charAt(0) as (typeof FILES)[number])
  const rankIdx = parseInt(square.charAt(1), 10) - 1
  return (fileIdx + rankIdx) % 2 === 0 ? 'dark' : 'light'
}

export interface ChessBoardProps {
  fen: string
  cellPx: number
  /** Flip the board so black is at the bottom. */
  flipped?: boolean
  /** Disable user input (used for review mode, spectator, opponent's turn). */
  interactive?: boolean
  /** Restrict pickable pieces to this colour (e.g. you only move white). */
  movableColor?: ChessColor
  /** Highlight from/to of the last move. */
  lastMove?: { from: ChessSquare; to: ChessSquare }
  /** Outline this square in red (king in check). */
  checkSquare?: ChessSquare | null
  /** Emitted when the user completes a move (including promotion choice). */
  onMove?: (move: UciMove) => void
}

interface PendingPromotion {
  from: ChessSquare
  to: ChessSquare
}

export function ChessBoard(props: ChessBoardProps) {
  const {
    fen,
    cellPx,
    flipped = false,
    interactive = true,
    movableColor,
    lastMove,
    checkSquare,
    onMove,
  } = props

  const board = useMemo<BoardSnapshot>(() => {
    try {
      return snapshot(newGame(fen))
    } catch {
      return snapshot(newGame())
    }
  }, [fen])

  const sideToMove = useMemo<ChessColor>(() => {
    try {
      return turn(newGame(fen))
    } catch {
      return 'w'
    }
  }, [fen])

  const [selected, setSelected] = useState<ChessSquare | null>(null)
  const [pendingPromo, setPendingPromo] = useState<PendingPromotion | null>(null)
  // React's "reset state on prop change" pattern: track the last FEN
  // in state and clear the selection synchronously when the position
  // changes (the new FEN may not even contain the previously-selected
  // piece). Calling setState during render is fine when guarded by an
  // equality check — React converges on the next render and no extra
  // effect is needed.
  const [lastFen, setLastFen] = useState(fen)
  if (lastFen !== fen) {
    setLastFen(fen)
    setSelected(null)
    setPendingPromo(null)
  }

  const legalForSelected = useMemo(() => {
    if (!selected) return [] as ReturnType<typeof legalMoves>
    try {
      return legalMoves(newGame(fen), selected)
    } catch {
      return []
    }
  }, [fen, selected])

  const legalDestSet = useMemo(() => new Set(legalForSelected.map((m) => m.to)), [legalForSelected])
  const promotionRequired = (from: ChessSquare, to: ChessSquare): boolean => {
    return legalForSelected.some(
      (m) => m.from === from && m.to === to && m.promotion !== undefined,
    )
  }

  const handleSquareClick = (sq: ChessSquare) => {
    if (!interactive) return
    const cellPiece = pieceAt(board, sq)
    // Click a candidate destination?
    if (selected && legalDestSet.has(sq)) {
      if (promotionRequired(selected, sq)) {
        setPendingPromo({ from: selected, to: sq })
        return
      }
      onMove?.({ from: selected, to: sq })
      setSelected(null)
      return
    }
    // Click your own piece → (re-)select.
    if (cellPiece && cellPiece.color === sideToMove && (!movableColor || cellPiece.color === movableColor)) {
      setSelected(sq === selected ? null : sq)
      return
    }
    // Anything else clears.
    setSelected(null)
  }

  const handlePromotionChoice = (choice: Promotion) => {
    if (!pendingPromo) return
    onMove?.({ from: pendingPromo.from, to: pendingPromo.to, promotion: choice })
    setPendingPromo(null)
    setSelected(null)
  }

  const orderedRanks = flipped ? [...RANKS].reverse() : RANKS
  const orderedFiles = flipped ? [...FILES].reverse() : FILES

  return (
    <div
      className="relative inline-block select-none rounded-md border border-border bg-card/40 p-1 shadow-sm"
      style={{ width: cellPx * 8 + 8, height: cellPx * 8 + 8 }}
      aria-label="Chess board"
    >
      <div
        className="grid"
        style={{
          gridTemplateColumns: `repeat(8, ${cellPx}px)`,
          gridTemplateRows: `repeat(8, ${cellPx}px)`,
        }}
      >
        {orderedRanks.map((rank) =>
          orderedFiles.map((file) => {
            const sq = `${file}${rank}` as ChessSquare
            const shade = squareShade(sq)
            const piece = pieceAtFenIndex(board, file, rank)
            const isSelected = selected === sq
            const isLegalDest = !!selected && legalDestSet.has(sq)
            const isLastFrom = lastMove?.from === sq
            const isLastTo = lastMove?.to === sq
            const isCheck = checkSquare === sq
            return (
              <button
                key={sq}
                type="button"
                aria-label={sq}
                onClick={() => handleSquareClick(sq)}
                disabled={!interactive}
                className={cn(
                  'relative flex items-center justify-center font-semibold leading-none transition-colors',
                  shade === 'light'
                    ? 'bg-[#eed7b4] dark:bg-[#b58863]'
                    : 'bg-[#b58863] dark:bg-[#6b4f3a]',
                  (isLastFrom || isLastTo) && 'ring-2 ring-yellow-400/70 ring-inset',
                  isSelected && 'ring-2 ring-primary ring-inset',
                  isCheck && 'ring-2 ring-rose-500 ring-inset',
                  !interactive && 'cursor-default',
                )}
                style={{ width: cellPx, height: cellPx, fontSize: Math.floor(cellPx * 0.78) }}
              >
                {piece && (
                  <span
                    aria-hidden
                    className={cn(
                      'pointer-events-none drop-shadow-sm',
                      piece.color === 'w' ? 'text-white' : 'text-black',
                    )}
                    style={{ textShadow: piece.color === 'w' ? '0 0 2px rgba(0,0,0,0.7)' : '0 0 2px rgba(255,255,255,0.5)' }}
                  >
                    {PIECE_GLYPH[piece.color][piece.type]}
                  </span>
                )}
                {isLegalDest && !piece && (
                  <span
                    aria-hidden
                    className="pointer-events-none absolute h-1/3 w-1/3 rounded-full bg-black/30 dark:bg-white/35"
                  />
                )}
                {isLegalDest && piece && (
                  <span
                    aria-hidden
                    className="pointer-events-none absolute inset-1 rounded-md ring-2 ring-rose-500/70"
                  />
                )}
                {/* Rank coordinate (a-file column for the side at the bottom) */}
                {file === orderedFiles[0] && (
                  <span
                    aria-hidden
                    className="pointer-events-none absolute left-0.5 top-0.5 text-[9px] font-bold opacity-60"
                  >
                    {rank}
                  </span>
                )}
                {/* File coordinate (bottom row) */}
                {rank === orderedRanks[orderedRanks.length - 1] && (
                  <span
                    aria-hidden
                    className="pointer-events-none absolute bottom-0.5 right-1 text-[9px] font-bold opacity-60"
                  >
                    {file}
                  </span>
                )}
              </button>
            )
          }),
        )}
      </div>

      {pendingPromo && (
        <PromotionPicker
          color={sideToMove}
          onChoose={handlePromotionChoice}
          onCancel={() => setPendingPromo(null)}
        />
      )}
    </div>
  )
}

function PromotionPicker({
  color,
  onChoose,
  onCancel,
}: {
  color: ChessColor
  onChoose: (p: Promotion) => void
  onCancel: () => void
}) {
  const choices: Promotion[] = ['q', 'r', 'b', 'n']
  return (
    <div
      className="absolute inset-0 z-30 flex items-center justify-center bg-background/70"
      onClick={onCancel}
    >
      <div
        className="flex gap-1 rounded-md border border-border bg-card p-2 shadow-lg"
        onClick={(e) => e.stopPropagation()}
      >
        {choices.map((c) => (
          <button
            key={c}
            type="button"
            onClick={() => onChoose(c)}
            className="flex h-12 w-12 items-center justify-center rounded border border-border bg-background text-3xl hover:bg-muted"
            aria-label={`Promote to ${c.toUpperCase()}`}
          >
            <span className={color === 'w' ? 'text-white drop-shadow-sm' : 'text-black drop-shadow-sm'}>
              {PIECE_GLYPH[color][c]}
            </span>
          </button>
        ))}
      </div>
    </div>
  )
}

function pieceAt(board: BoardSnapshot, sq: ChessSquare) {
  const file = sq.charCodeAt(0) - 97
  const rank = 8 - parseInt(sq.charAt(1), 10)
  return board[rank]?.[file] ?? null
}

function pieceAtFenIndex(
  board: BoardSnapshot,
  file: (typeof FILES)[number],
  rank: (typeof RANKS)[number],
) {
  return pieceAt(board, `${file}${rank}` as ChessSquare)
}
