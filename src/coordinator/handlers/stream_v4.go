// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/stream/hepstream"
)

// clientStreamUpgrader upgrades the UI's HTTP/1.1 request into a
// WebSocket. Unlike the node-side upgrader we accept *any* origin here
// because the UI is served from the same coordinator and behind the
// operator-configured reverse proxy; tightening origin checks is the
// deployment's job, not ours.
var clientStreamUpgrader = websocket.Upgrader{
	ReadBufferSize:  16 * 1024,
	WriteBufferSize: 16 * 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

const (
	clientWriteTimeout = 5 * time.Second
	clientPingInterval = 30 * time.Second
)

// V4HepStream exposes the live HEP event stream to UI clients.
//
// Query parameters:
//
//	proto=1&proto=5            repeated OR-filter on ProtoType
//	method=INVITE&method=BYE   repeated OR-filter on SIP method
//	only_requests=1            drop SIP responses
//	include_payload=1          include raw HEP payload (admin only,
//	                            and only when AllowPayload=true)
//	history=500                initial burst of buffered events
//
// The endpoint is registered under protectedV4 so the usual JWT auth
// applies. Browsers that cannot set Authorization headers on the WS
// handshake can pass the token via `?access_token=…` — handled
// transparently by parseBearerToken.
func (h *StreamHandler) V4HepStream(c echo.Context) error {
	if h.service == nil || !h.service.Configured() {
		return writeError(c, http.StatusServiceUnavailable,
			"Service Unavailable", "HEP stream is not configured")
	}

	req := c.Request()
	q := req.URL.Query()
	filter := hepstream.FilterFromQuery(q)

	// include_payload is an authorisation decision, not a matching
	// decision. Apply the admin+operator policy here; if the request
	// is denied, strip the flag rather than 403ing the whole stream
	// so a user-mode client still gets the metadata they asked for.
	if filter.IncludePayload {
		if !h.allowPayload || !isAdmin(c) {
			filter.IncludePayload = false
		}
	}

	history := h.historyLimit
	if raw := q.Get("history"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			history = n
		}
	}
	if h.historyLimit > 0 && history > h.historyLimit {
		history = h.historyLimit
	}

	// Subscribe BEFORE the upgrade so a 503 comes back as a plain
	// HTTP response (upgraded sockets can only return close frames).
	stream, cancel, err := h.service.Subscribe(req.Context(), filter, history)
	if err != nil {
		return writeError(c, http.StatusServiceUnavailable,
			"Service Unavailable", err.Error())
	}

	conn, err := clientStreamUpgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		cancel()
		// Upgrade() already wrote an HTTP error. Echo treats a nil
		// return here as "handler done" which matches reality.
		return nil
	}
	// From this point onward, cancel() is owned by the read/write
	// loop. Defer both so even a panic cleans up.
	defer conn.Close()
	defer cancel()

	// Read pump: discards everything from the client, just so the
	// writer side can notice a disconnect. Browsers never send
	// anything useful on this stream.
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		conn.SetReadLimit(4096)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(clientPingInterval)
	defer ticker.Stop()

	for {
		select {
		case evt, ok := <-stream:
			if !ok {
				_ = conn.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseGoingAway, "upstream closed"),
					time.Now().Add(clientWriteTimeout))
				return nil
			}
			if err := writeClientFrame(conn, evt, filter.IncludePayload); err != nil {
				return nil
			}
		case <-ticker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil,
				time.Now().Add(clientWriteTimeout)); err != nil {
				return nil
			}
		case <-readerDone:
			return nil
		case <-req.Context().Done():
			return nil
		}
	}
}

func writeClientFrame(conn *websocket.Conn, evt hepstream.Event, includePayload bool) error {
	buf, err := evt.MarshalFor(includePayload)
	if err != nil {
		// Skip malformed frames rather than tear the stream down.
		return nil
	}
	if err := conn.SetWriteDeadline(time.Now().Add(clientWriteTimeout)); err != nil {
		return err
	}
	if err := conn.WriteMessage(websocket.TextMessage, buf); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}
