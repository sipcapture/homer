// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package node

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/sipcapture/homer-core/src/stream/hepstream"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// streamUpgrader upgrades node-side HTTP to WebSocket. We reuse the
// gorilla-compatible fork already pulled in by src/server and allow any
// origin: the node HTTP port sits behind the coordinator on a private
// network and is not meant for direct browser access (same trust
// boundary as /query).
var streamUpgrader = websocket.Upgrader{
	ReadBufferSize:  16 * 1024,
	WriteBufferSize: 16 * 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

const (
	// streamWriteTimeout bounds how long the handler waits to push one
	// frame down the socket. Slow coordinators get disconnected rather
	// than back-pressuring the broker's per-sub queue.
	streamWriteTimeout = 5 * time.Second

	// streamPingInterval keeps idle connections alive through typical
	// load balancers / HTTP proxies (they often time out silent WS
	// sessions after 60s).
	streamPingInterval = 30 * time.Second

	// streamMaxHistory caps the initial burst of buffered events a new
	// subscriber receives. 10k is the ring-buffer's own upper bound;
	// 2k is a safer default so one reconnect doesn't spray megabytes.
	streamMaxHistory = 2000
)

// handleStream upgrades the request to a WebSocket, subscribes to the
// live broker with a filter derived from query parameters, and writes
// JSON frames until the client or broker goes away.
//
// Query parameters match hepstream.FilterFromQuery:
//
//	proto=1&proto=5            repeated OR-filter on ProtoType
//	method=INVITE&method=BYE   repeated OR-filter on SIP method
//	only_requests=1            drop SIP responses
//	include_payload=1          include raw HEP payload in each frame
//	history=500                replay up to N buffered events on connect
//
// There is no auth on this endpoint — it sits on the same HTTP port as
// /query and /health. Operators are expected to firewall it (documented
// in docs/HEP_STREAM.md).
func (n *Node) handleStream(w http.ResponseWriter, r *http.Request) {
	n.mu.RLock()
	broker := n.broker
	n.mu.RUnlock()

	if broker == nil {
		http.Error(w, "hep stream not configured", http.StatusServiceUnavailable)
		return
	}

	q := r.URL.Query()
	filter := hepstream.FilterFromQuery(q)

	// Parse history limit here (not in Filter) because it is a handler
	// concern, not a matching concern. Negative / oversized values are
	// clamped silently.
	history := 0
	if raw := q.Get("history"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			history = n
		}
	}
	if history > streamMaxHistory {
		history = streamMaxHistory
	}

	ch, cancel, err := broker.Subscribe(filter)
	if err != nil {
		// 503 instead of 429 because this error is almost always
		// caused by MaxSubscribers — a capacity issue, not a
		// per-client rate problem.
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer cancel()

	conn, err := streamUpgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade already wrote the error response.
		logger.Debug(fmt.Sprintf("stream: upgrade failed: %v", err))
		return
	}
	defer conn.Close()

	// Read pump: drain incoming frames (pings/pongs, close messages)
	// on a separate goroutine so the writer side can notice the peer
	// going away. gorilla-style: if the peer disconnects, the read
	// pump returns with an error and we close the connection from
	// here, which in turn unblocks the writer.
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		conn.SetReadLimit(4096) // we don't expect large client messages
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// Replay history before going live. We deliberately do this after
	// Subscribe so events that landed between Subscribe and the history
	// snapshot aren't dropped on the floor; duplicates are tolerated by
	// downstream consumers (they key on callid+ts).
	if history > 0 {
		past := broker.History(filter, history)
		for _, evt := range past {
			if err := writeFrame(conn, evt, filter.IncludePayload); err != nil {
				return
			}
		}
	}

	ticker := time.NewTicker(streamPingInterval)
	defer ticker.Stop()

	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				// broker closed our channel (broker.Stop or cancel
				// race); tell the peer and exit.
				_ = conn.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseGoingAway, "broker closed"),
					time.Now().Add(streamWriteTimeout))
				return
			}
			if err := writeFrame(conn, evt, filter.IncludePayload); err != nil {
				return
			}
		case <-ticker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(streamWriteTimeout)); err != nil {
				return
			}
		case <-readerDone:
			return
		case <-r.Context().Done():
			return
		}
	}
}

// writeFrame marshals evt with the subscriber's payload preference and
// pushes it down the socket. Returns any fatal error so the caller can
// tear the connection down.
func writeFrame(conn *websocket.Conn, evt hepstream.Event, includePayload bool) error {
	buf, err := evt.MarshalFor(includePayload)
	if err != nil {
		// A single marshal error should not drop the whole stream;
		// skip this event and keep going.
		logger.Debug(fmt.Sprintf("stream: marshal failed: %v", err))
		return nil
	}
	if err := conn.SetWriteDeadline(time.Now().Add(streamWriteTimeout)); err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, buf)
}
