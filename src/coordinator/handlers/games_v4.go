// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	chessgame "github.com/sipcapture/homer-core/src/coordinator/games/chess"
	"github.com/sipcapture/homer-core/src/coordinator/games/netchess"
	"github.com/sipcapture/homer-core/src/coordinator/games/netris"
)

// GamesHandler hosts WebSocket endpoints for the dashboard mini-games
// that need server-side coordination. Today the only such game is
// Netris (PvP SIPetris); future games can hang their own routes off
// the same handler / hub plumbing.
//
// Construction leaves NetrisHub nil so the coordinator can opt-in to
// the feature: a nil hub causes the endpoint to return 503, mirroring
// how StreamHandler treats a nil StreamService.
type GamesHandler struct {
	netrisHub *netris.Hub
	// chessLLM is the LLM client used by the single-player Chess
	// widget's "LLM mode". Nil = LLM mode unavailable (the widget
	// will hide the toggle).
	chessLLM chessgame.LLMChatter
	// chessRNG is used by the chess LLM fallback to break ties
	// between equally-scored greedy moves. Separate RNG so tests can
	// inject a deterministic seed; nil falls back to non-deterministic.
	chessRNG *rand.Rand
	// netchessHub powers the PvP NetChess widget. Nil keeps the
	// endpoint stubbed (503) — same pattern as netris.
	netchessHub *netchess.Hub
}

func NewGamesHandler() *GamesHandler {
	return &GamesHandler{
		chessRNG: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// SetNetrisHub injects the running hub. Pass nil to keep the endpoint
// stubbed out (useful while the feature is rolling out, or when the
// operator explicitly disables games).
func (h *GamesHandler) SetNetrisHub(hub *netris.Hub) {
	h.netrisHub = hub
}

// SetChessLLM wires in the shared LLM client. Pass nil (or a client
// where Enabled() is false) to leave the Chess widget in bot-only
// mode; the dashboard reads this state via V4ChessLLMStatus.
func (h *GamesHandler) SetChessLLM(llm chessgame.LLMChatter) {
	h.chessLLM = llm
}

// SetNetChessHub injects the PvP NetChess hub. Pass nil to keep the
// endpoint stubbed out.
func (h *GamesHandler) SetNetChessHub(hub *netchess.Hub) {
	h.netchessHub = hub
}

const (
	netrisReadLimit       = 8 * 1024
	netrisPongWait        = 60 * time.Second
	netrisPingInterval    = 25 * time.Second
	netrisRoomCodeMax     = 64
	netrisDisplayNameMax  = 32
	netrisHandshakeWait   = 5 * time.Second
	netrisOutboxBuffer    = 64
)

// V4Netris upgrades the request to a WebSocket and routes the
// resulting client through the netris.Hub.
//
// Query parameters:
//
//	room=<code>          Join (or create) a named room. Capacity 2.
//	mode=quick           Auto-pair with the next unmatched player.
//	display=<name>       Optional display label (≤32 sane chars).
//
// Auth is the same JWTMiddlewareV4 that protects every other v4
// endpoint; for browsers that can't set Authorization on a WS
// handshake, the JWT comes via `?access_token=…` (handled by
// auth_v4_helpers.go::JWTMiddlewareV4 already).
func (h *GamesHandler) V4Netris(c echo.Context) error {
	if h.netrisHub == nil {
		return writeError(c, http.StatusServiceUnavailable,
			"Service Unavailable", "Netris is not configured")
	}

	username := usernameFromContext(c)
	if username == "" {
		return writeError(c, http.StatusUnauthorized,
			"Unauthorized", "JWT username claim is missing")
	}

	q := c.Request().URL.Query()
	room := strings.TrimSpace(q.Get("room"))
	if len(room) > netrisRoomCodeMax {
		room = room[:netrisRoomCodeMax]
	}
	mode := strings.ToLower(strings.TrimSpace(q.Get("mode")))
	display := q.Get("display")
	if len(display) > netrisDisplayNameMax {
		display = display[:netrisDisplayNameMax]
	}

	if room == "" && mode != "quick" {
		return writeError(c, http.StatusBadRequest,
			"Bad Request", "must specify either ?room=<code> or ?mode=quick")
	}

	conn, err := clientStreamUpgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		// Upgrade() already wrote an HTTP error.
		return nil
	}
	defer conn.Close()

	conn.SetReadLimit(netrisReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(netrisPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(netrisPongWait))
	})

	player := netris.NewPlayer(username, display, netrisOutboxBuffer)

	// Join now so the lone-room "matched (no opponent)" frame is in
	// the outbox before the writer goroutine starts. Doing it after
	// would race with the first iteration of the read pump.
	switch {
	case room != "":
		if _, err := h.netrisHub.JoinRoom(room, player); err != nil {
			// Room full — emit a one-shot error frame and close.
			buf, _ := netris.Encode(netris.Envelope{
				Type:    netris.MsgError,
				Message: err.Error(),
			})
			_ = conn.SetWriteDeadline(time.Now().Add(clientWriteTimeout))
			_ = conn.WriteMessage(websocket.TextMessage, buf)
			return nil
		}
	default:
		if _, err := h.netrisHub.JoinQuick(player); err != nil {
			buf, _ := netris.Encode(netris.Envelope{
				Type:    netris.MsgError,
				Message: err.Error(),
			})
			_ = conn.SetWriteDeadline(time.Now().Add(clientWriteTimeout))
			_ = conn.WriteMessage(websocket.TextMessage, buf)
			return nil
		}
	}
	defer h.netrisHub.Leave(player)

	// Writer pump: drains the player's outbox until the hub closes
	// it, then signals back via the writeDone channel so the read
	// loop can stop too.
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		ticker := time.NewTicker(netrisPingInterval)
		defer ticker.Stop()
		for {
			select {
			case buf, ok := <-player.Out:
				if !ok {
					_ = conn.SetWriteDeadline(time.Now().Add(clientWriteTimeout))
					_ = conn.WriteMessage(websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseGoingAway, "session closed"))
					return
				}
				if err := conn.SetWriteDeadline(time.Now().Add(clientWriteTimeout)); err != nil {
					return
				}
				if err := conn.WriteMessage(websocket.TextMessage, buf); err != nil {
					return
				}
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil,
					time.Now().Add(clientWriteTimeout)); err != nil {
					return
				}
			}
		}
	}()

	// Reader pump: parse and forward each frame to the hub. We
	// return on any read error (including normal close) so the
	// deferred Leave() runs and the writer pump unblocks.
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			break
		}
		env, err := netris.Decode(payload)
		if err != nil || env.Type == "" {
			// Bad frame — skip but stay connected; client may
			// recover after a hot-reload.
			continue
		}
		h.netrisHub.HandleFrame(player, env)
	}

	// Wait for the writer to finish so we don't double-close conn
	// while it's still calling WriteMessage.
	<-writeDone
	return nil
}

const (
	netchessReadLimit     = 8 * 1024
	netchessPongWait      = 60 * time.Second
	netchessPingInterval  = 25 * time.Second
	netchessOutboxBuffer  = 64
)

// V4NetChess upgrades the request to a WebSocket and routes the
// resulting client through the netchess.Hub. Same auth and write/read
// pump shape as V4Netris; the only structural differences are the
// extra query parameters (color, time control, spectate) and the
// authoritative server-side game state inside the hub.
//
// Query parameters:
//
//	room=<code>          Join (or create) a named room. Capacity 2 (+8 spectators).
//	mode=quick           Auto-pair with the next unmatched player.
//	color=white|black|random
//	initial_ms=<int>     Initial clock per side in ms (default 600000).
//	increment_ms=<int>   Fischer increment in ms (default 5000).
//	spectate=true        Join an existing room read-only (requires room=<code>).
//	display=<name>       Optional display label (≤ MaxDisplayNameLen sane chars).
func (h *GamesHandler) V4NetChess(c echo.Context) error {
	if h.netchessHub == nil {
		return writeError(c, http.StatusServiceUnavailable,
			"Service Unavailable", "NetChess is not configured")
	}

	username := usernameFromContext(c)
	if username == "" {
		return writeError(c, http.StatusUnauthorized,
			"Unauthorized", "JWT username claim is missing")
	}

	q := c.Request().URL.Query()
	room := strings.TrimSpace(q.Get("room"))
	if len(room) > netchess.MaxRoomCodeLen {
		room = room[:netchess.MaxRoomCodeLen]
	}
	mode := strings.ToLower(strings.TrimSpace(q.Get("mode")))
	display := q.Get("display")
	spectate := strings.EqualFold(strings.TrimSpace(q.Get("spectate")), "true")
	color := strings.ToLower(strings.TrimSpace(q.Get("color")))
	switch color {
	case netchess.ColorWhite, netchess.ColorBlack, "random", "":
		// ok
	default:
		return writeError(c, http.StatusBadRequest, "Bad Request",
			"color must be white, black, or random")
	}

	if spectate && room == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request",
			"spectate=true requires ?room=<code>")
	}
	if !spectate && room == "" && mode != "quick" {
		return writeError(c, http.StatusBadRequest, "Bad Request",
			"must specify either ?room=<code>, ?mode=quick, or ?spectate=true&room=<code>")
	}

	initialMS := parseInt64Param(q.Get("initial_ms"))
	incrementMS := parseInt64Param(q.Get("increment_ms"))

	conn, err := clientStreamUpgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return nil
	}
	defer conn.Close()

	conn.SetReadLimit(netchessReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(netchessPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(netchessPongWait))
	})

	player := netchess.NewPlayer(username, display, netchessOutboxBuffer)
	opts := netchess.JoinOptions{
		ColorPref:   color,
		InitialMS:   initialMS,
		IncrementMS: incrementMS,
	}

	switch {
	case spectate:
		if _, err := h.netchessHub.JoinSpectator(room, player); err != nil {
			buf, _ := netchess.Encode(netchess.Envelope{Type: netchess.MsgError, Message: err.Error()})
			_ = conn.SetWriteDeadline(time.Now().Add(clientWriteTimeout))
			_ = conn.WriteMessage(websocket.TextMessage, buf)
			return nil
		}
	case room != "":
		if _, err := h.netchessHub.JoinRoom(room, player, opts); err != nil {
			buf, _ := netchess.Encode(netchess.Envelope{Type: netchess.MsgError, Message: err.Error()})
			_ = conn.SetWriteDeadline(time.Now().Add(clientWriteTimeout))
			_ = conn.WriteMessage(websocket.TextMessage, buf)
			return nil
		}
	default:
		if _, err := h.netchessHub.JoinQuick(player, opts); err != nil {
			buf, _ := netchess.Encode(netchess.Envelope{Type: netchess.MsgError, Message: err.Error()})
			_ = conn.SetWriteDeadline(time.Now().Add(clientWriteTimeout))
			_ = conn.WriteMessage(websocket.TextMessage, buf)
			return nil
		}
	}
	defer h.netchessHub.Leave(player)

	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		ticker := time.NewTicker(netchessPingInterval)
		defer ticker.Stop()
		for {
			select {
			case buf, ok := <-player.Out:
				if !ok {
					_ = conn.SetWriteDeadline(time.Now().Add(clientWriteTimeout))
					_ = conn.WriteMessage(websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseGoingAway, "session closed"))
					return
				}
				if err := conn.SetWriteDeadline(time.Now().Add(clientWriteTimeout)); err != nil {
					return
				}
				if err := conn.WriteMessage(websocket.TextMessage, buf); err != nil {
					return
				}
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil,
					time.Now().Add(clientWriteTimeout)); err != nil {
					return
				}
			}
		}
	}()

	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			break
		}
		env, err := netchess.Decode(payload)
		if err != nil || env.Type == "" {
			continue
		}
		h.netchessHub.HandleFrame(player, env)
	}

	<-writeDone
	return nil
}

// parseInt64Param is a small forgiving int64 parser for query params:
// empty / unparseable → 0 so the hub falls back to its defaults.
func parseInt64Param(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	var v int64
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0
		}
		v = v*10 + int64(ch-'0')
		if v > 1<<40 {
			return 0
		}
	}
	return v
}

// chessLLMStatusResponse mirrors the JSON shape the Chess widget
// reads via fetchChessLLMStatus(). Stable: changing field names is a
// breaking client/server contract.
type chessLLMStatusResponse struct {
	Enabled bool   `json:"enabled"`
	Model   string `json:"model,omitempty"`
}

// V4ChessLLMStatus reports whether the dashboard Chess widget can
// offer LLM-opponent mode. Cheap, no body — the widget polls this on
// mount.
func (h *GamesHandler) V4ChessLLMStatus(c echo.Context) error {
	if h.chessLLM == nil || !h.chessLLM.Enabled() {
		return c.JSON(http.StatusOK, chessLLMStatusResponse{Enabled: false})
	}
	return c.JSON(http.StatusOK, chessLLMStatusResponse{
		Enabled: true,
		Model:   h.chessLLM.Model(),
	})
}

// chessLLMMoveRequest is the body the Chess widget sends when it's the
// LLM's turn. We keep the field names short and match the UI camelCase
// style for the response.
type chessLLMMoveRequest struct {
	FEN     string `json:"fen"`
	History string `json:"history_pgn,omitempty"`
	Level   int    `json:"level,omitempty"`
}

type chessLLMMoveResponse struct {
	UCI       string `json:"uci"`
	Source    string `json:"source"` // "llm" | "fallback"
	Model     string `json:"model,omitempty"`
	LatencyMS int64  `json:"latency_ms"`
	Reason    string `json:"reason,omitempty"`
}

// V4ChessLLMMove asks the configured LLM for a move on the supplied
// FEN. Validates the model output via notnil/chess; on any failure,
// falls back to a server-side greedy picker so the widget always
// gets *some* legal move back. The widget displays the `source`
// indicator so the user can see which path was taken.
func (h *GamesHandler) V4ChessLLMMove(c echo.Context) error {
	var req chessLLMMoveRequest
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "invalid JSON body")
	}
	if strings.TrimSpace(req.FEN) == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "fen is required")
	}
	resp := chessgame.GetLLMMove(c.Request().Context(), h.chessLLM, chessgame.MoveRequest{
		FEN:     req.FEN,
		History: req.History,
		Level:   req.Level,
	}, h.chessRNG)
	return c.JSON(http.StatusOK, chessLLMMoveResponse{
		UCI:       resp.UCI,
		Source:    string(resp.Source),
		Model:     resp.Model,
		LatencyMS: resp.LatencyMS,
		Reason:    resp.FallbackReason,
	})
}

// usernameFromContext extracts the JWT username claim placed by the
// v4 auth middleware. Returns "" when the claim is missing — caller
// must treat that as auth failure.
func usernameFromContext(c echo.Context) string {
	user := c.Get("user")
	if user == nil {
		return ""
	}
	token, ok := user.(*jwt.Token)
	if !ok || token == nil {
		return ""
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return ""
	}
	return claims.Username
}
