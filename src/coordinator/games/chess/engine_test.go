// Copyright (C) 2025 Homer Server Contributors

package chess

import (
	"context"
	"encoding/json"
	"math/rand"
	"strings"
	"testing"

	notnil "github.com/notnil/chess"
)

// startingFEN keeps the tests independent of how notnil/chess names
// its starting-position constant in any specific version.
var startingFEN = notnil.NewGame().FEN()

func TestPickMove_StartingPosition(t *testing.T) {
	game := notnil.NewGame()
	uci := PickMove(game, rand.New(rand.NewSource(42)))
	if uci == "" {
		t.Fatal("expected a legal move at the starting position, got empty string")
	}
	if _, err := ValidateUCI(game, uci); err != nil {
		t.Fatalf("PickMove returned an illegal UCI %q: %v", uci, err)
	}
}

func TestPickMove_TakesFreeMaterial(t *testing.T) {
	// White knight on b3 attacks an undefended black queen on d4.
	// PickMove (depth-1 greedy) must take it.
	fen := "rnb1kbnr/pppppppp/8/8/3q4/1N6/PPPPPPPP/R1BQKBNR w KQkq - 0 1"
	opt, err := notnil.FEN(fen)
	if err != nil {
		t.Fatalf("FEN parse: %v", err)
	}
	game := notnil.NewGame(opt)
	uci := PickMove(game, rand.New(rand.NewSource(1)))
	if uci != "b3d4" {
		t.Fatalf("expected b3d4 (free queen capture), got %q", uci)
	}
}

func TestPickMove_TerminalPosition(t *testing.T) {
	// Stalemate: black to move, no legal moves.
	fen := "7k/5K2/6Q1/8/8/8/8/8 b - - 0 1"
	opt, err := notnil.FEN(fen)
	if err != nil {
		t.Fatalf("FEN parse: %v", err)
	}
	game := notnil.NewGame(opt)
	if uci := PickMove(game, nil); uci != "" {
		t.Fatalf("expected empty UCI on stalemate, got %q", uci)
	}
}

func TestValidateUCI(t *testing.T) {
	game := notnil.NewGame()
	if _, err := ValidateUCI(game, "e2e4"); err != nil {
		t.Fatalf("e2e4 should be legal: %v", err)
	}
	if _, err := ValidateUCI(game, "e2e5"); err == nil {
		t.Fatal("e2e5 should be illegal from starting position")
	}
	if _, err := ValidateUCI(game, "bogus"); err == nil {
		t.Fatal("bogus UCI should be rejected on length")
	}
}

// stubLLM satisfies LLMChatter and returns a canned response. The
// `respond` function lets each test customise what the model "says".
type stubLLM struct {
	enabled bool
	model   string
	respond func(systemPrompt, userPrompt string) (string, error)
}

func (s *stubLLM) Enabled() bool { return s.enabled }
func (s *stubLLM) Model() string { return s.model }
func (s *stubLLM) ChatJSON(_ context.Context, sys, usr string, out any) (string, int64, error) {
	if s.respond == nil {
		return s.model, 1, nil
	}
	body, err := s.respond(sys, usr)
	if err != nil {
		return s.model, 1, err
	}
	// Same JSON-shape contract as the real LLMClient.ChatJSON: caller
	// supplies a pointer to a struct, we unmarshal into it.
	if out != nil {
		if err := json.Unmarshal([]byte(body), out); err != nil {
			return s.model, 1, err
		}
	}
	return s.model, 1, nil
}

func TestGetLLMMove_HappyPath(t *testing.T) {
	llm := &stubLLM{
		enabled: true,
		model:   "test-1",
		respond: func(_, _ string) (string, error) {
			return `{"uci":"e2e4"}`, nil
		},
	}
	resp := GetLLMMove(context.Background(), llm, MoveRequest{
		FEN: startingFEN,
	}, nil)
	if resp.Source != SourceLLM {
		t.Fatalf("expected LLM source, got %s (%s)", resp.Source, resp.FallbackReason)
	}
	if resp.UCI != "e2e4" {
		t.Fatalf("expected e2e4, got %q", resp.UCI)
	}
}

func TestGetLLMMove_FallbackOnInvalidMove(t *testing.T) {
	llm := &stubLLM{
		enabled: true,
		model:   "test-2",
		respond: func(_, _ string) (string, error) {
			// Illegal — 3-square pawn move.
			return `{"uci":"e2e5"}`, nil
		},
	}
	resp := GetLLMMove(context.Background(), llm, MoveRequest{
		FEN: startingFEN,
	}, rand.New(rand.NewSource(7)))
	if resp.Source != SourceFallback {
		t.Fatalf("expected fallback, got %s", resp.Source)
	}
	if resp.UCI == "" {
		t.Fatal("fallback must still produce a legal move")
	}
	if !strings.Contains(resp.FallbackReason, "invalid") {
		t.Fatalf("expected fallback reason to mention invalid move, got %q", resp.FallbackReason)
	}
}

func TestGetLLMMove_FallbackOnLLMError(t *testing.T) {
	llm := &stubLLM{
		enabled: true,
		respond: func(_, _ string) (string, error) {
			return "", errSimulated{}
		},
	}
	resp := GetLLMMove(context.Background(), llm, MoveRequest{
		FEN: startingFEN,
	}, rand.New(rand.NewSource(7)))
	if resp.Source != SourceFallback {
		t.Fatalf("expected fallback, got %s", resp.Source)
	}
	if resp.UCI == "" {
		t.Fatal("fallback must produce a legal move on LLM error")
	}
}

func TestGetLLMMove_NilClientReturnsFallback(t *testing.T) {
	resp := GetLLMMove(context.Background(), nil, MoveRequest{
		FEN: startingFEN,
	}, rand.New(rand.NewSource(7)))
	if resp.Source != SourceFallback {
		t.Fatalf("expected fallback, got %s", resp.Source)
	}
	if resp.FallbackReason != "llm disabled" {
		t.Fatalf("expected reason 'llm disabled', got %q", resp.FallbackReason)
	}
}

func TestGetLLMMove_InvalidFen(t *testing.T) {
	llm := &stubLLM{enabled: true, respond: func(_, _ string) (string, error) {
		return `{"uci":"e2e4"}`, nil
	}}
	resp := GetLLMMove(context.Background(), llm, MoveRequest{FEN: "not a fen"}, nil)
	if resp.Source != SourceFallback || resp.UCI != "" {
		t.Fatalf("expected empty fallback on invalid FEN, got %+v", resp)
	}
}

type errSimulated struct{}

func (errSimulated) Error() string { return "simulated llm error" }
