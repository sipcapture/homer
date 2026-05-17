// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package chess

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strings"

	notnil "github.com/notnil/chess"
)

// LLMChatter is the slice of `mcp.LLMClient` we actually need. Pulling
// it out as an interface lets the handler tests inject a fake client
// without dragging the OpenAI plumbing along, and matches the
// `MCPSearcher`-style seams already used in this codebase.
type LLMChatter interface {
	Enabled() bool
	Model() string
	ChatJSON(ctx context.Context, systemPrompt, userPrompt string, out any) (model string, latencyMS int64, err error)
}

// MoveSource describes who produced a move returned by GetLLMMove.
type MoveSource string

const (
	SourceLLM      MoveSource = "llm"
	SourceFallback MoveSource = "fallback"
)

// MoveResponse is the return shape of GetLLMMove. Latency / Model are
// passed through for the UI to display.
type MoveResponse struct {
	UCI       string
	Source    MoveSource
	Model     string
	LatencyMS int64
	// FallbackReason is populated when Source == SourceFallback. The
	// HTTP handler surfaces it under `meta` for debugging.
	FallbackReason string
}

// MoveRequest is the input to GetLLMMove. FEN is required; History is
// the move list in PGN-style SAN for context (optional).
type MoveRequest struct {
	FEN     string
	History string
	// Level hints the LLM at the desired strength (1..4). Mostly
	// flavour today, but passed through so prompt tuning can
	// differentiate skill levels later.
	Level int
}

const llmChessSystemPrompt = `You are a chess engine. The user will give you a chess position in FEN.
Reply with ONLY a single JSON object of the form:
  {"uci": "<move-in-uci>"}
where <move-in-uci> is the long-algebraic representation of YOUR best legal move
for the side to move, e.g. "e2e4", "g1f3", "e7e8q" for promotion.
Rules:
- The move MUST be legal in the supplied position.
- Use lowercase letters for squares and promotion piece.
- Do NOT output any commentary, prose, or markdown. JSON object only.`

// uciRegexp pre-screens raw model output before we even ask
// `notnil/chess` to validate. It's tolerant: a/h files, 1/8 ranks,
// optional q/r/b/n promotion.
var uciRegexp = regexp.MustCompile(`(?i)\b([a-h][1-8][a-h][1-8][qrbn]?)\b`)

// GetLLMMove asks the LLM for a move on the given position, validates
// the answer against `notnil/chess`, and falls back to the engine's
// greedy picker on any failure. It always returns a legal move when
// one exists; the only way `MoveResponse.UCI == ""` is if the position
// itself is terminal (mate / stalemate / draw).
//
// The function never panics on bad input — invalid FENs are mapped to
// fallback behaviour with a descriptive reason.
func GetLLMMove(ctx context.Context, llm LLMChatter, req MoveRequest, rng *rand.Rand) MoveResponse {
	// Parse the FEN once — both the LLM-validation path and the
	// fallback need a working game object.
	game, err := gameFromFEN(req.FEN)
	if err != nil {
		return MoveResponse{
			UCI:            "",
			Source:         SourceFallback,
			FallbackReason: fmt.Sprintf("invalid fen: %v", err),
		}
	}

	// Terminal position → no legal move; just return empty UCI so
	// the caller can render the game-over overlay.
	if game.Outcome() != notnil.NoOutcome {
		return MoveResponse{UCI: "", Source: SourceFallback, FallbackReason: "terminal position"}
	}

	// LLM disabled — straight to fallback.
	if llm == nil || !llm.Enabled() {
		uci := PickMove(game, rng)
		return MoveResponse{
			UCI:            uci,
			Source:         SourceFallback,
			FallbackReason: "llm disabled",
		}
	}

	// Build the user prompt: FEN is the source of truth; history is
	// just context. We cap the history length so an obscenely long
	// game doesn't blow up the model's input budget.
	hist := strings.TrimSpace(req.History)
	if len(hist) > 8000 {
		hist = hist[len(hist)-8000:]
	}
	userPrompt := fmt.Sprintf(
		"fen=%s\nhistory_pgn=%s\nlevel=%d\nIt is your move. Reply with a JSON object containing only the UCI of your move.",
		strings.TrimSpace(req.FEN), hist, clampLevel(req.Level),
	)

	var parsed struct {
		UCI string `json:"uci"`
	}
	model, latency, err := llm.ChatJSON(ctx, llmChessSystemPrompt, userPrompt, &parsed)
	if err != nil {
		uci := PickMove(game, rng)
		return MoveResponse{
			UCI:            uci,
			Source:         SourceFallback,
			Model:          model,
			LatencyMS:      latency,
			FallbackReason: fmt.Sprintf("llm error: %v", err),
		}
	}

	// Some models put the UCI elsewhere (raw string, "move" field, etc).
	candidate := strings.TrimSpace(strings.ToLower(parsed.UCI))
	if candidate == "" {
		// Try a last-resort regex on the raw answer — but we didn't
		// keep the raw answer here. Re-call would be wasteful; just
		// fall back.
		uci := PickMove(game, rng)
		return MoveResponse{
			UCI:            uci,
			Source:         SourceFallback,
			Model:          model,
			LatencyMS:      latency,
			FallbackReason: "llm returned no uci field",
		}
	}

	// Validate.
	if _, err := ValidateUCI(game, candidate); err != nil {
		// Sometimes the model fences a UCI inside other text — try
		// regex extraction from the candidate string before giving up.
		if m := uciRegexp.FindString(candidate); m != "" {
			if _, err2 := ValidateUCI(game, m); err2 == nil {
				return MoveResponse{
					UCI:       strings.ToLower(m),
					Source:    SourceLLM,
					Model:     model,
					LatencyMS: latency,
				}
			}
		}
		uci := PickMove(game, rng)
		return MoveResponse{
			UCI:            uci,
			Source:         SourceFallback,
			Model:          model,
			LatencyMS:      latency,
			FallbackReason: fmt.Sprintf("llm move invalid: %v", err),
		}
	}

	return MoveResponse{
		UCI:       candidate,
		Source:    SourceLLM,
		Model:     model,
		LatencyMS: latency,
	}
}

// gameFromFEN wraps notnil's FEN loader with a single-purpose error
// surface — callers do not need to know which option struct the
// library wants.
func gameFromFEN(fen string) (*notnil.Game, error) {
	if strings.TrimSpace(fen) == "" {
		return nil, fmt.Errorf("empty fen")
	}
	opt, err := notnil.FEN(fen)
	if err != nil {
		return nil, err
	}
	return notnil.NewGame(opt), nil
}

func clampLevel(n int) int {
	if n < 1 {
		return 1
	}
	if n > 4 {
		return 4
	}
	return n
}
