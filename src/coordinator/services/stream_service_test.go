// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/stream/hepstream"
)

// fakeSource is a channel-backed eventSource we feed into the service
// through dialerOverride. It satisfies the unexported `eventSource`
// interface so runNodeForwarder treats it like a real WebSocket.
type fakeSource struct {
	events chan hepstream.Event
	mu     sync.Mutex
	closed bool
}

func newFakeSource(size int) *fakeSource { return &fakeSource{events: make(chan hepstream.Event, size)} }

func (f *fakeSource) Next(ctx context.Context) (hepstream.Event, error) {
	select {
	case evt, ok := <-f.events:
		if !ok {
			return hepstream.Event{}, errors.New("source drained")
		}
		return evt, nil
	case <-ctx.Done():
		return hepstream.Event{}, ctx.Err()
	}
}

func (f *fakeSource) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil
	}
	f.closed = true
	close(f.events)
	return nil
}

func TestStreamService_LocalBrokerShortcut(t *testing.T) {
	b := hepstream.NewBroker(hepstream.Config{
		Enable:         true,
		BufferSize:     50,
		MaxSubscribers: 4,
		PerSubQueueLen: 16,
	})
	svc := NewStreamService(nil, b, 500*time.Millisecond, 100)
	if !svc.Configured() {
		t.Fatal("expected local-broker service to be Configured()")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, stop, err := svc.Subscribe(ctx, hepstream.Filter{}, 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer stop()

	b.Publish(hepstream.Event{TsMilli: 42, Proto: 1})

	select {
	case evt := <-ch:
		if evt.TsMilli != 42 {
			t.Fatalf("expected ts=42, got %d", evt.TsMilli)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event via local broker")
	}
}

func TestStreamService_FanOutFromNodes(t *testing.T) {
	// Two fake nodes, each pushing its own event. We expect to see
	// both on the merged channel in any order.
	sources := []*fakeSource{newFakeSource(4), newFakeSource(4)}
	// Dialer is called from two goroutines concurrently (one per node),
	// so guard the index. The race detector will fail any version that
	// skips this.
	var dialMu sync.Mutex
	calls := 0
	override := func(urlStr string, timeout time.Duration) (eventSource, error) {
		dialMu.Lock()
		defer dialMu.Unlock()
		if calls >= len(sources) {
			return nil, fmt.Errorf("unexpected extra dial: %s", urlStr)
		}
		src := sources[calls]
		calls++
		return src, nil
	}

	nodes := []config.NodeEndpoint{
		{Name: "a", Host: "127.0.0.1", Port: 50051},
		{Name: "b", Host: "127.0.0.1", Port: 50052},
	}
	svc := NewStreamService(nodes, nil, 500*time.Millisecond, 100)
	svc.dialerOverride = override

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, stop, err := svc.Subscribe(ctx, hepstream.Filter{}, 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer stop()

	sources[0].events <- hepstream.Event{TsMilli: 1, Proto: 1}
	sources[1].events <- hepstream.Event{TsMilli: 2, Proto: 1}

	seen := map[int64]bool{}
	deadline := time.After(2 * time.Second)
	for len(seen) < 2 {
		select {
		case evt := <-ch:
			seen[evt.TsMilli] = true
		case <-deadline:
			t.Fatalf("fan-out timed out, got %v", seen)
		}
	}
	if !seen[1] || !seen[2] {
		t.Fatalf("expected both events, got %v", seen)
	}
}

func TestStreamService_NotConfigured(t *testing.T) {
	svc := NewStreamService(nil, nil, 500*time.Millisecond, 100)
	if svc.Configured() {
		t.Fatal("service with no broker/nodes must not be Configured()")
	}
	_, _, err := svc.Subscribe(context.Background(), hepstream.Filter{}, 0)
	if err == nil {
		t.Fatal("expected Subscribe to fail when unconfigured")
	}
}
