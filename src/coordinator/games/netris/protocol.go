// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package netris implements the lobby + relay for the two-player
// SIPetris-themed Tetris ("Netris") played from the Homer dashboard.
//
// The server is a thin relay (matchmaking + garbage translation +
// disconnect detection). Each client crunches its own board; the
// authoritative bit on the server is the n-1 garbage rule and the
// shared "hole column" so both clients render the same incoming
// garbage row.
package netris

import (
	"encoding/json"
	"math/rand"
	"strings"
)

// BoardCols is the playfield width clients use; must stay in sync
// with the UI side (tetrisCore.ts COLS constant).
const BoardCols = 10

// Message types — keep the strings stable: they cross the network and
// changing them is a breaking protocol change.
const (
	// Client -> Server
	MsgHello      = "hello"
	MsgReady      = "ready"
	MsgLineClear  = "line_clear"
	MsgBoard      = "board"
	MsgScore      = "score"
	MsgToppedOut  = "topped_out"
	MsgChat       = "chat"

	// Server -> Client
	MsgMatched         = "matched"
	MsgStart           = "start"
	MsgGarbageIn       = "garbage_in"
	MsgOpponentBoard   = "opponent_board"
	MsgOpponentScore   = "opponent_score"
	MsgOpponentLeft    = "opponent_left"
	MsgWin             = "win"
	MsgLose            = "lose"
	MsgWaitingTimeout  = "waiting_timeout"
	MsgError           = "error"
)

// Envelope is the on-the-wire container — every WS frame is exactly
// one Envelope JSON object. Unknown fields are tolerated so future
// protocol additions on either side don't immediately break older
// clients/servers.
type Envelope struct {
	Type string `json:"type"`

	// Hello / Matched / Chat
	DisplayName string `json:"display_name,omitempty"`
	Room        string `json:"room,omitempty"`
	You         string `json:"you,omitempty"`
	Opponent    string `json:"opponent,omitempty"`
	From        string `json:"from,omitempty"`
	Text        string `json:"text,omitempty"`

	// LineClear
	Cleared int `json:"cleared,omitempty"`

	// GarbageIn
	Lines int `json:"lines,omitempty"`
	Hole  int `json:"hole"` // intentionally not omitempty — 0 is a valid column

	// Board snapshot for opponent_board (opaque base64-bitmap, server
	// just relays it).
	Cells string `json:"cells,omitempty"`

	// Score frames
	Score int `json:"score,omitempty"`
	Level int `json:"level,omitempty"`
	// Lines doubles as score lines counter — see Lines in score path.

	// Win/Lose/Error
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// Encode serialises an envelope to its JSON wire form. The error
// branch is unreachable for the fields we use but kept for clarity at
// the call site.
func Encode(e Envelope) ([]byte, error) {
	return json.Marshal(e)
}

// Decode parses a wire frame, rejecting empty / non-typed envelopes
// early so the room loop doesn't have to second-guess.
func Decode(buf []byte) (Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(buf, &e); err != nil {
		return e, err
	}
	e.Type = strings.TrimSpace(e.Type)
	return e, nil
}

// GarbageLinesFor implements the classic "n-1" Tetris garbage rule
// with a Tetris bonus: a 4-line clear sends 4 garbage rows (instead
// of the bare-bones 3) so going for tetrises is rewarded.
func GarbageLinesFor(cleared int) int {
	switch {
	case cleared <= 1:
		return 0
	case cleared == 2:
		return 1
	case cleared == 3:
		return 2
	default:
		// 4 (Tetris) or any larger fluke (shouldn't happen with a
		// vanilla Tetris client, but defend against junk frames).
		return 4
	}
}

// RandomHoleColumn picks a column index in [0, BoardCols) using the
// hub's RNG so both players see the same hole position for a given
// garbage burst (the hub picks once and broadcasts the same `hole`
// to the recipient).
func RandomHoleColumn(rng *rand.Rand) int {
	if rng == nil {
		return 0
	}
	return rng.Intn(BoardCols)
}

// SanitiseDisplayName trims and caps the override display name a
// client can send via Hello. The JWT-derived username is still the
// source of truth in matchmaking — this is a label only.
func SanitiseDisplayName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Drop control chars and clamp to 32 runes so we don't ship an
	// abuse vector to the opponent's UI.
	out := make([]rune, 0, 32)
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			continue
		}
		out = append(out, r)
		if len(out) >= 32 {
			break
		}
	}
	return string(out)
}
