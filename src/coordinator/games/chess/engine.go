// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package chess hosts the server-side helpers for the dashboard chess
// widgets: a minimal greedy engine (only used as a fallback when the
// LLM bridge fails) and the LLM call wrapper used by the
// `/games/chess/llm-move` HTTP endpoint.
//
// We intentionally keep the server engine simple. Strong play happens
// on the client (depth-4 minimax in a Web Worker). The server engine
// exists *only* so the panel doesn't break when LLM mode is enabled
// and the model returns garbage; it just needs to produce a legal,
// non-suicidal move within a few milliseconds.
package chess

import (
	"fmt"
	"math"
	"math/rand"
	"strings"

	notnil "github.com/notnil/chess"
)

// Material values in centipawns. The king's "value" is large so the
// evaluator never thinks losing it is acceptable — terminal positions
// short-circuit before this matters in practice, but defensive.
var pieceValue = map[notnil.PieceType]int{
	notnil.Pawn:   100,
	notnil.Knight: 320,
	notnil.Bishop: 330,
	notnil.Rook:   500,
	notnil.Queen:  900,
	notnil.King:   20000,
}

// materialBalance returns White-relative centipawns for the supplied
// position.  Positive = White is ahead.
func materialBalance(pos *notnil.Position) int {
	board := pos.Board()
	balance := 0
	for sq := notnil.A1; sq <= notnil.H8; sq++ {
		p := board.Piece(sq)
		if p == notnil.NoPiece {
			continue
		}
		v := pieceValue[p.Type()]
		if p.Color() == notnil.White {
			balance += v
		} else {
			balance -= v
		}
	}
	return balance
}

// evaluate returns a score from the perspective of the side to move
// in `pos`. Mate-in-this-position is the worst possible outcome for
// the side to move; we map it to a large negative number so a search
// avoids walking into it.
func evaluate(pos *notnil.Position) int {
	switch pos.Status() {
	case notnil.Checkmate:
		return -1_000_000 // side to move is mated
	case notnil.Stalemate,
		notnil.InsufficientMaterial,
		notnil.FiftyMoveRule,
		notnil.SeventyFiveMoveRule,
		notnil.ThreefoldRepetition,
		notnil.FivefoldRepetition,
		notnil.DrawOffer:
		return 0
	}
	bal := materialBalance(pos)
	if pos.Turn() == notnil.White {
		return bal
	}
	return -bal
}

// PickMove returns the best legal move on `game.Position()` using a
// shallow (depth-1) greedy search: for each candidate, play it and
// pick the one that minimises the opponent's reply evaluation. Ties
// are broken deterministically via `rng` (pass a seeded *rand.Rand
// in tests, or nil for non-deterministic). Returns "" for terminal
// positions (mate/stalemate/etc).
func PickMove(game *notnil.Game, rng *rand.Rand) string {
	pos := game.Position()
	moves := game.ValidMoves()
	if len(moves) == 0 {
		return ""
	}
	bestScore := math.MinInt
	type cand struct {
		move  *notnil.Move
		score int
	}
	bests := make([]cand, 0, 4)
	for _, mv := range moves {
		// Apply the move on a cloned game so we don't disturb the
		// caller's history.
		clone := game.Clone()
		if err := clone.Move(mv); err != nil {
			continue
		}
		// Score = -opponent_eval after our move. evaluate() returns a
		// side-to-move-relative number, and after our move the side
		// to move is the opponent, so we negate.
		score := -evaluate(clone.Position())
		// Tiny capture bonus to break ties towards taking material
		// when otherwise equivalent.
		if isCapture(mv, pos) {
			score += 5
		}
		if score > bestScore {
			bestScore = score
			bests = bests[:0]
			bests = append(bests, cand{move: mv, score: score})
		} else if score == bestScore {
			bests = append(bests, cand{move: mv, score: score})
		}
	}
	if len(bests) == 0 {
		return ""
	}
	pick := bests[0]
	if len(bests) > 1 {
		if rng != nil {
			pick = bests[rng.Intn(len(bests))]
		} else {
			pick = bests[0]
		}
	}
	return moveUCI(pick.move)
}

// isCapture reports whether playing `mv` from `pos` removes an
// opponent piece (regular capture or en passant). notnil/chess
// exposes this through the move tags; we recompute to stay
// independent of internal tag bits.
func isCapture(mv *notnil.Move, pos *notnil.Position) bool {
	target := pos.Board().Piece(mv.S2())
	if target != notnil.NoPiece {
		return true
	}
	return mv.HasTag(notnil.EnPassant)
}

// moveUCI returns the long-algebraic UCI string for the move:
// `e2e4`, `e7e8q`. Castling is encoded as `e1g1`/`e1c1` (king-only),
// matching what chess.js produces on the UI side.
func moveUCI(mv *notnil.Move) string {
	from := mv.S1().String()
	to := mv.S2().String()
	promo := ""
	switch mv.Promo() {
	case notnil.Queen:
		promo = "q"
	case notnil.Rook:
		promo = "r"
	case notnil.Bishop:
		promo = "b"
	case notnil.Knight:
		promo = "n"
	}
	return strings.ToLower(from + to + promo)
}

// ValidateUCI parses `uci` against the position in `game` and returns
// the matching notnil/chess move pointer. Used by both the LLM-move
// path (to validate what the model emitted) and the netchess hub.
// Returns a descriptive error so the caller can log "why" the move
// was rejected.
func ValidateUCI(game *notnil.Game, uci string) (*notnil.Move, error) {
	uci = strings.ToLower(strings.TrimSpace(uci))
	if len(uci) != 4 && len(uci) != 5 {
		return nil, fmt.Errorf("invalid uci length: %q", uci)
	}
	for _, mv := range game.ValidMoves() {
		if strings.EqualFold(moveUCI(mv), uci) {
			return mv, nil
		}
	}
	return nil, fmt.Errorf("illegal move: %s", uci)
}
