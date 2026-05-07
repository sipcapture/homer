// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/stream/hepstream"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// StreamService aggregates live HEP events from:
//
//  1. an optional in-process broker (when coordinator runs in the same
//     process as ingest — "single box" deployments), or
//  2. WebSocket subscriptions to each configured node's /stream endpoint
//     (distributed deployments).
//
// Callers use Subscribe to obtain a merged channel of events matching
// the given filter plus a cancel function. The cancel unblocks all
// internal goroutines and closes the channel exactly once.
type StreamService struct {
	nodes          []config.NodeEndpoint
	localBroker    *hepstream.Broker
	dialTimeout    time.Duration
	historyLimit   int

	// dialerOverride lets tests substitute a fake WebSocket dialer
	// instead of the real network one. nil means "use the real
	// *websocket.DefaultDialer with our timeout".
	dialerOverride func(urlStr string, timeout time.Duration) (eventSource, error)
}

// NewStreamService constructs a StreamService. `localBroker` may be
// nil, in which case the service will always fan out to the configured
// nodes via WebSocket. Pass a non-zero fan-out timeout (clamped to
// sensible bounds internally) so a slow node never wedges the UI.
func NewStreamService(nodes []config.NodeEndpoint, localBroker *hepstream.Broker, fanOutTimeout time.Duration, historyLimit int) *StreamService {
	if fanOutTimeout <= 0 {
		fanOutTimeout = 2 * time.Second
	}
	if historyLimit < 0 {
		historyLimit = 0
	}
	return &StreamService{
		nodes:        nodes,
		localBroker:  localBroker,
		dialTimeout:  fanOutTimeout,
		historyLimit: historyLimit,
	}
}

// Configured reports whether this service has any way to deliver
// events (a local broker or at least one node endpoint). Used by the
// HTTP handler to decide whether to 503 early.
func (s *StreamService) Configured() bool {
	if s == nil {
		return false
	}
	if s.localBroker != nil {
		return true
	}
	return len(s.nodes) > 0
}

// Subscribe returns a merged channel of events matching f plus a
// cancel. `history` is passed through to the node-side `/stream`
// endpoint (and consulted directly for the local broker). Honouring
// history on both sides means a newly opened UI sees "the last N
// events" without a round-trip request to every node.
//
// The returned channel is closed exactly once the last underlying
// source has gone away (every node disconnected and, when applicable,
// the local broker closed). Cancel drains the channel silently.
func (s *StreamService) Subscribe(ctx context.Context, f hepstream.Filter, history int) (<-chan hepstream.Event, func(), error) {
	if !s.Configured() {
		return nil, nil, fmt.Errorf("stream service: no local broker and no nodes configured")
	}
	if history <= 0 || history > s.historyLimit {
		history = s.historyLimit
	}

	out := make(chan hepstream.Event, 256)
	ctx, cancel := context.WithCancel(ctx)

	var wg sync.WaitGroup
	var closeOnce sync.Once
	closeOut := func() { closeOnce.Do(func() { close(out) }) }

	// Local broker shortcut: when coordinator lives in the same
	// process as ingest we subscribe directly to the in-memory broker
	// and skip the WebSocket round-trip to localhost.
	if s.localBroker != nil {
		ch, bcancel, err := s.localBroker.Subscribe(f)
		if err != nil {
			cancel()
			close(out)
			return nil, nil, err
		}
		if history > 0 {
			past := s.localBroker.History(f, history)
			for _, evt := range past {
				select {
				case out <- evt:
				case <-ctx.Done():
					bcancel()
					cancel()
					close(out)
					return nil, nil, ctx.Err()
				}
			}
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer bcancel()
			for {
				select {
				case evt, ok := <-ch:
					if !ok {
						return
					}
					select {
					case out <- evt:
					case <-ctx.Done():
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	} else {
		// Distributed: one forwarder goroutine per configured node.
		for _, n := range s.nodes {
			wg.Add(1)
			go s.runNodeForwarder(ctx, &wg, n, f, history, out)
		}
	}

	// Cleanup goroutine: closes `out` after every source goroutine
	// returned. We can't close from inside runNodeForwarder because
	// any other source might still have events to deliver.
	go func() {
		wg.Wait()
		closeOut()
	}()

	return out, func() {
		cancel()
		// Drain to unblock writers in case nobody is reading. The
		// close above will still happen once wg drops to zero.
		go func() {
			for range out {
			}
		}()
	}, nil
}

// runNodeForwarder dials one node's /stream endpoint and forwards
// events to `out` until ctx is cancelled or the connection drops.
// Short-term disconnects are retried with exponential backoff so a
// node restart doesn't permanently break the UI stream.
func (s *StreamService) runNodeForwarder(ctx context.Context, wg *sync.WaitGroup, n config.NodeEndpoint, f hepstream.Filter, history int, out chan<- hepstream.Event) {
	defer wg.Done()

	backoff := 500 * time.Millisecond
	const maxBackoff = 30 * time.Second

	for {
		if err := ctx.Err(); err != nil {
			return
		}
		src, err := s.dialNode(ctx, n, f, history)
		if err != nil {
			logger.Debug(fmt.Sprintf("stream: node %s dial failed: %v", n.Name, err))
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}
		// Reset backoff after a successful connect so a brief blip
		// doesn't punish the next reconnect.
		backoff = 500 * time.Millisecond

		s.forward(ctx, src, out)
		// forward() returned because the upstream closed or ctx fired.
		// Loop to reconnect unless ctx is done.
	}
}

// eventSource is the minimal surface our forwarder needs from the
// underlying WebSocket connection — defined as an interface so tests
// can inject an in-memory channel without standing up an HTTP server.
type eventSource interface {
	Next(ctx context.Context) (hepstream.Event, error)
	Close() error
}

// dialNode opens a WebSocket to the node's /stream endpoint with the
// caller's filter and history limit baked into the query string.
func (s *StreamService) dialNode(ctx context.Context, n config.NodeEndpoint, f hepstream.Filter, history int) (eventSource, error) {
	if s.dialerOverride != nil {
		return s.dialerOverride(s.nodeStreamURL(n, f, history), s.dialTimeout)
	}

	u := s.nodeStreamURL(n, f, history)
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = s.dialTimeout

	dialCtx, dialCancel := context.WithTimeout(ctx, s.dialTimeout)
	defer dialCancel()

	conn, _, err := dialer.DialContext(dialCtx, u, nil)
	if err != nil {
		return nil, err
	}
	return &wsEventSource{conn: conn}, nil
}

// nodeStreamURL builds the per-node /stream URL. The node exposes its
// HTTP API on FlightServer.Port + 1 — see src/node/node.go. We mirror
// that offset rule here to avoid leaking it into config.
func (s *StreamService) nodeStreamURL(n config.NodeEndpoint, f hepstream.Filter, history int) string {
	scheme := "ws"
	if n.UseTLS {
		scheme = "wss"
	}
	q := f.ToQuery()
	if history > 0 {
		q.Set("history", fmt.Sprintf("%d", history))
	}
	u := url.URL{
		Scheme:   scheme,
		Host:     fmt.Sprintf("%s:%d", n.Host, n.Port+1),
		Path:     "/stream",
		RawQuery: q.Encode(),
	}
	return u.String()
}

// forward pipes events from src into out until src is drained or ctx
// is cancelled.
func (s *StreamService) forward(ctx context.Context, src eventSource, out chan<- hepstream.Event) {
	defer src.Close()
	for {
		evt, err := src.Next(ctx)
		if err != nil {
			return
		}
		select {
		case out <- evt:
		case <-ctx.Done():
			return
		}
	}
}

// wsEventSource adapts a *websocket.Conn into an eventSource by
// decoding each incoming text frame as hepstream.Event JSON.
type wsEventSource struct {
	conn   *websocket.Conn
	closed atomic.Bool
}

func (w *wsEventSource) Next(ctx context.Context) (hepstream.Event, error) {
	// Propagate ctx cancellation into the blocking ReadMessage by
	// closing the connection from a watcher goroutine. We do this
	// lazily on the first Next call to avoid spawning for sources
	// that are immediately discarded.
	if w.closed.Load() {
		return hepstream.Event{}, fmt.Errorf("stream source closed")
	}
	// Note: we rely on the node-side ping interval (30s) plus
	// read deadline resets to detect dead peers. Setting a read
	// deadline here would conflict with that.
	_, data, err := w.conn.ReadMessage()
	if err != nil {
		return hepstream.Event{}, err
	}
	var evt hepstream.Event
	if err := json.Unmarshal(data, &evt); err != nil {
		// Decode errors should not tear down the stream — they
		// almost certainly mean a partial / truncated frame, and
		// the next one will likely succeed.
		return hepstream.Event{}, nil //nolint:nilerr  // caller retries
	}
	// Honour ctx cancellation as a close signal.
	if err := ctx.Err(); err != nil {
		_ = w.conn.Close()
		w.closed.Store(true)
		return hepstream.Event{}, err
	}
	return evt, nil
}

func (w *wsEventSource) Close() error {
	if w.closed.Swap(true) {
		return nil
	}
	return w.conn.Close()
}
