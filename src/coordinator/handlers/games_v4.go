// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
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
}

func NewGamesHandler() *GamesHandler {
	return &GamesHandler{}
}

// SetNetrisHub injects the running hub. Pass nil to keep the endpoint
// stubbed out (useful while the feature is rolling out, or when the
// operator explicitly disables games).
func (h *GamesHandler) SetNetrisHub(hub *netris.Hub) {
	h.netrisHub = hub
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
