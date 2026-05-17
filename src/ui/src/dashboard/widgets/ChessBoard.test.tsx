import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ChessBoard } from './ChessBoard'
import { STARTING_FEN } from './chessCore'

describe('ChessBoard', () => {
  it('renders 64 square buttons', () => {
    render(<ChessBoard fen={STARTING_FEN} cellPx={32} />)
    expect(screen.getAllByRole('button')).toHaveLength(64)
  })

  it('renders pieces using filled Unicode glyphs in the starting position', () => {
    const { container } = render(<ChessBoard fen={STARTING_FEN} cellPx={32} />)
    // Both colours share the filled silhouettes — colour is carried by
    // the text-white / text-black class, not by a different glyph.
    expect(container.textContent).toContain('♚')
    expect(container.textContent).toContain('♟')
    expect(container.querySelectorAll('span.text-white').length).toBeGreaterThan(0)
    expect(container.querySelectorAll('span.text-black').length).toBeGreaterThan(0)
  })

  it('click-select-then-click-move emits onMove with from/to', () => {
    const onMove = vi.fn()
    render(<ChessBoard fen={STARTING_FEN} cellPx={32} onMove={onMove} />)
    fireEvent.click(screen.getByLabelText('e2'))
    fireEvent.click(screen.getByLabelText('e4'))
    expect(onMove).toHaveBeenCalledWith({ from: 'e2', to: 'e4' })
  })

  it('ignores illegal destinations', () => {
    const onMove = vi.fn()
    render(<ChessBoard fen={STARTING_FEN} cellPx={32} onMove={onMove} />)
    fireEvent.click(screen.getByLabelText('e2'))
    fireEvent.click(screen.getByLabelText('e5')) // 3-square pawn move
    expect(onMove).not.toHaveBeenCalled()
  })

  it('does not emit when interactive is false', () => {
    const onMove = vi.fn()
    render(<ChessBoard fen={STARTING_FEN} cellPx={32} interactive={false} onMove={onMove} />)
    fireEvent.click(screen.getByLabelText('e2'))
    fireEvent.click(screen.getByLabelText('e4'))
    expect(onMove).not.toHaveBeenCalled()
  })

  it('shows promotion picker on pawn promotion and emits chosen piece', () => {
    const onMove = vi.fn()
    // Position: white pawn on e7, white king g1, black king e1; white to move.
    const fen = '8/4P3/8/8/8/8/8/4k1K1 w - - 0 1'
    render(<ChessBoard fen={fen} cellPx={32} onMove={onMove} />)
    fireEvent.click(screen.getByLabelText('e7'))
    fireEvent.click(screen.getByLabelText('e8'))
    // No move emitted yet — picker is up.
    expect(onMove).not.toHaveBeenCalled()
    // Pick a rook.
    fireEvent.click(screen.getByLabelText('Promote to R'))
    expect(onMove).toHaveBeenCalledWith({ from: 'e7', to: 'e8', promotion: 'r' })
  })

  it('flips the board so that black is on the bottom when flipped=true', () => {
    const { container } = render(
      <ChessBoard fen={STARTING_FEN} cellPx={32} flipped />,
    )
    // First grid child should now be the a1 (white rook on the back rank)
    // because flipped puts rank 1 at the top.
    const grid = container.querySelector('.grid')
    const firstSquare = grid?.firstElementChild as HTMLElement
    expect(firstSquare.getAttribute('aria-label')).toBe('h1')
  })
})
