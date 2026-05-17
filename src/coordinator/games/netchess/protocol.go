// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package netchess implements the lobby + authoritative game state for
// the two-player Chess ("NetChess") widget on the Homer dashboard.
//
// Unlike Netris (a thin relay where each client crunches its own
// board), NetChess is server-authoritative: the hub holds a
// `*notnil/chess.Game` per room, validates every move, runs the
// clocks, and detects mate / draw conditions. Clients render the
// position the server hands them, submit UCI strings, and never need
// to second-guess legality.
package netchess

import (
	"encoding/json"
	"strings"
)

// Message types — keep stable across versions; changing them is a
// breaking protocol change.
const (
	// Client → Server
	MsgHello          = "hello"
	MsgReady          = "ready"
	MsgMove           = "move"
	MsgResign         = "resign"
	MsgDrawOffer      = "draw_offer"
	MsgDrawAccept     = "draw_accept"
	MsgDrawDecline    = "draw_decline"
	MsgTakebackReq    = "takeback_request"
	MsgTakebackAccept = "takeback_accept"
	MsgTakebackDecl   = "takeback_decline"
	MsgChat           = "chat"

	// Server → Client
	MsgMatched        = "matched"
	MsgStart          = "start"
	MsgOpponentMove   = "opponent_move"
	MsgClockSync      = "clock_sync"
	MsgGameOver       = "game_over"
	MsgDrawOffered    = "draw_offered"
	MsgTakebackOffer  = "takeback_offered"
	MsgOpponentLeft   = "opponent_left"
	MsgWaitingTimeout = "waiting_timeout"
	MsgError          = "error"
)

// Game-over reasons. The set is closed: clients render different
// copy per reason.
const (
	ReasonCheckmate    = "checkmate"
	ReasonStalemate    = "stalemate"
	ReasonResignation  = "resignation"
	ReasonAgreement    = "agreement"
	ReasonFlag         = "flag"
	ReasonInsufficient = "insufficient_material"
	ReasonFiftyMove    = "fifty_move"
	ReasonThreefold    = "threefold_repetition"
	ReasonOpponentLeft = "opponent_disconnect"
)

// Time-control defaults — used when the client doesn't override via
// the URL query. Server enforces clocks; clients display.
const (
	DefaultInitialMS  = 600_000 // 10 minutes
	DefaultIncMS      = 5_000   // 5 second Fischer increment
	MaxRoomCodeLen    = 64
	MaxDisplayNameLen = 32
	MaxChatLen        = 200
	MaxSpectators     = 8
)

// Color constants on the wire. We don't reuse chess.Color values
// because we don't want the wire format coupled to a library type.
const (
	ColorWhite = "white"
	ColorBlack = "black"
)

// Envelope is the single on-the-wire JSON container. Unknown fields
// are tolerated on either side, so adding new optional fields does
// not require a coordinated rollout.
type Envelope struct {
	Type string `json:"type"`

	// Hello / Matched / Chat
	DisplayName string `json:"display_name,omitempty"`
	Room        string `json:"room,omitempty"`
	You         string `json:"you,omitempty"`
	Opponent    string `json:"opponent,omitempty"`
	Color       string `json:"color,omitempty"`    // your colour: "white"|"black"
	From        string `json:"from,omitempty"`     // chat sender username
	Text        string `json:"text,omitempty"`     // chat text
	Spectator   bool   `json:"spectator,omitempty"` // sent in matched to flag read-only mode

	// Time control (sent in matched once the pair forms)
	InitialMS   int64 `json:"initial_ms,omitempty"`
	IncrementMS int64 `json:"increment_ms,omitempty"`

	// Move / OpponentMove
	UCI string `json:"uci,omitempty"`
	SAN string `json:"san,omitempty"`
	FEN string `json:"fen,omitempty"`
	// Optional client-asserted clock — purely advisory; the server
	// trusts its own timer.
	ClockMS int64 `json:"clock_ms,omitempty"`

	// ClockSync / OpponentMove
	WhiteMS int64 `json:"white_ms,omitempty"`
	BlackMS int64 `json:"black_ms,omitempty"`

	// GameOver
	Result string `json:"result,omitempty"` // "1-0" | "0-1" | "1/2-1/2"
	Reason string `json:"reason,omitempty"` // see Reason* constants

	// Error frames
	Message string `json:"message,omitempty"`
}

// Encode marshals the envelope to JSON. The error branch is
// unreachable for the fields we ship; kept for clarity at call sites.
func Encode(e Envelope) ([]byte, error) {
	return json.Marshal(e)
}

// Decode parses a wire frame and trims the type field so the room
// loop doesn't have to second-guess. Rejects empty payloads.
func Decode(buf []byte) (Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(buf, &e); err != nil {
		return e, err
	}
	e.Type = strings.TrimSpace(e.Type)
	return e, nil
}

// SanitiseDisplayName trims / caps / strips control characters from a
// user-supplied display label. JWT-derived usernames remain the
// source of truth for matchmaking; this is a render-only label.
func SanitiseDisplayName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	out := make([]rune, 0, MaxDisplayNameLen)
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			continue
		}
		out = append(out, r)
		if len(out) >= MaxDisplayNameLen {
			break
		}
	}
	return string(out)
}

// SanitiseChat clamps a chat message to MaxChatLen runes and drops
// control characters so the broadcast can't ship raw escape codes to
// opposing clients.
func SanitiseChat(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	out := make([]rune, 0, MaxChatLen)
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			continue
		}
		out = append(out, r)
		if len(out) >= MaxChatLen {
			break
		}
	}
	return string(out)
}
