// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package hepstream

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrTooManySubscribers is returned by Subscribe when the broker is at
// the MaxSubscribers cap. Callers (i.e. the /stream HTTP handler) should
// translate this into HTTP 503.
var ErrTooManySubscribers = errors.New("hepstream: too many subscribers")

// Stats is a point-in-time snapshot of broker counters. Useful for the
// `/modules` endpoint and for tests.
type Stats struct {
	Published          uint64
	DroppedRate        uint64 // dropped because per-sub rate limit hit
	DroppedBackpressure uint64 // dropped because per-sub queue full
	DroppedFilter      uint64 // filtered out (not actually "dropped", but counted for visibility)
	Subscribers        int64
	BufferSize         int
	BufferHead         int
}

// subscriber is the private record the broker keeps for each live
// subscription. The caller sees only the receive-only channel and a
// cancel func.
type subscriber struct {
	id     uint64
	ch     chan Event
	filter Filter

	// rate limiting. tokens is a simple token-bucket counter kept in
	// the same sub mutex as lastTick. We re-fill up to RatePerSubPPS
	// every second; fractional refills aren't worth the cost for
	// this use case.
	mu       sync.Mutex
	tokens   int
	lastTick time.Time
}

// Broker is a small in-memory pub-sub over Event values. Safe for
// concurrent use.
type Broker struct {
	cfg Config

	mu          sync.RWMutex
	subs        map[uint64]*subscriber
	nextSubID   uint64

	// ring buffer of the most recent events, for history replay on
	// Subscribe. Fixed-size; overwrite-oldest policy.
	ring     []Event
	ringHead int    // next write position
	ringLen  int    // logical size (≤ cap(ring))
	ringMu   sync.Mutex // serialises appends to the ring buffer

	// counters. Atomic so Publish can report metrics without
	// contending on mu.
	publishedN          atomic.Uint64
	droppedRateN        atomic.Uint64
	droppedBackpressureN atomic.Uint64
	droppedFilterN      atomic.Uint64
}

// NewBroker returns a Broker configured from cfg. If cfg.Enable is
// false, NewBroker returns nil — callers must treat nil as "no broker
// configured" and skip any Publish/Subscribe entirely (there are
// nil-guards on both methods as a belt-and-braces fallback).
func NewBroker(cfg Config) *Broker {
	if !cfg.Enable {
		return nil
	}
	cfg = cfg.applyFallbacks()

	b := &Broker{
		cfg:  cfg,
		subs: make(map[uint64]*subscriber, cfg.MaxSubscribers),
	}
	if cfg.BufferSize > 0 {
		b.ring = make([]Event, cfg.BufferSize)
	}
	return b
}

// Publish distributes evt to every matching subscriber. Safe to call
// with a nil receiver — that is how the ingest hot path ignores the
// feature when disabled ("if b == nil { return }" below).
//
// Publish never blocks: a slow subscriber's channel is skipped (and
// counted in DroppedBackpressure) so one greedy consumer cannot
// back-pressure the ingest worker. The only lock held is the read lock
// on the subscriber map.
func (b *Broker) Publish(evt Event) {
	if b == nil {
		return
	}
	b.publishedN.Add(1)
	b.appendToRing(evt)

	b.mu.RLock()
	// Take a snapshot of subscribers so we can release the read lock
	// quickly; the downstream fan-out will take per-subscriber locks.
	subs := make([]*subscriber, 0, len(b.subs))
	for _, s := range b.subs {
		subs = append(subs, s)
	}
	b.mu.RUnlock()

	for _, s := range subs {
		if !s.filter.Match(evt) {
			b.droppedFilterN.Add(1)
			continue
		}
		if !s.takeToken(b.cfg.RatePerSubPPS) {
			b.droppedRateN.Add(1)
			continue
		}
		select {
		case s.ch <- evt:
		default:
			b.droppedBackpressureN.Add(1)
		}
	}
}

func (b *Broker) appendToRing(evt Event) {
	if b.cfg.BufferSize <= 0 {
		return
	}
	b.ringMu.Lock()
	b.ring[b.ringHead] = evt
	b.ringHead = (b.ringHead + 1) % b.cfg.BufferSize
	if b.ringLen < b.cfg.BufferSize {
		b.ringLen++
	}
	b.ringMu.Unlock()
}

// Subscribe registers a new consumer. The returned channel yields
// events that match f; the cancel func unregisters the subscriber and
// closes the channel exactly once.
//
// If the broker is at MaxSubscribers, Subscribe returns
// (nil, nil, ErrTooManySubscribers). Cancelling a nil cancel is a
// programming error; callers should defer cancel() immediately after
// checking the error.
func (b *Broker) Subscribe(f Filter) (<-chan Event, func(), error) {
	if b == nil {
		return nil, nil, errors.New("hepstream: broker is nil (feature disabled)")
	}
	b.mu.Lock()
	if len(b.subs) >= b.cfg.MaxSubscribers {
		b.mu.Unlock()
		return nil, nil, ErrTooManySubscribers
	}
	b.nextSubID++
	id := b.nextSubID
	s := &subscriber{
		id:       id,
		ch:       make(chan Event, b.cfg.PerSubQueueLen),
		filter:   f,
		tokens:   b.cfg.RatePerSubPPS,
		lastTick: time.Now(),
	}
	b.subs[id] = s
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs, id)
			b.mu.Unlock()
			close(s.ch)
		})
	}
	return s.ch, cancel, nil
}

// History returns up to limit most recent events that match f, in
// oldest-first order. Intended to be called right after Subscribe so
// the consumer can replay what it missed before going live. Passing
// limit <= 0 returns everything currently in the buffer.
func (b *Broker) History(f Filter, limit int) []Event {
	if b == nil || b.cfg.BufferSize <= 0 {
		return nil
	}
	b.ringMu.Lock()
	defer b.ringMu.Unlock()

	if b.ringLen == 0 {
		return nil
	}
	out := make([]Event, 0, b.ringLen)
	start := 0
	if b.ringLen == b.cfg.BufferSize {
		start = b.ringHead // oldest sits at head when buffer is full
	}
	for i := 0; i < b.ringLen; i++ {
		idx := (start + i) % b.cfg.BufferSize
		evt := b.ring[idx]
		if f.Match(evt) {
			out = append(out, evt)
		}
	}
	if limit > 0 && len(out) > limit {
		// Keep the most recent `limit` entries.
		out = out[len(out)-limit:]
	}
	return out
}

// Stats returns a snapshot of broker counters.
func (b *Broker) Stats() Stats {
	if b == nil {
		return Stats{}
	}
	b.mu.RLock()
	nsubs := int64(len(b.subs))
	b.mu.RUnlock()

	b.ringMu.Lock()
	bufLen := b.ringLen
	bufHead := b.ringHead
	b.ringMu.Unlock()

	return Stats{
		Published:           b.publishedN.Load(),
		DroppedRate:         b.droppedRateN.Load(),
		DroppedBackpressure: b.droppedBackpressureN.Load(),
		DroppedFilter:       b.droppedFilterN.Load(),
		Subscribers:         nsubs,
		BufferSize:          bufLen,
		BufferHead:          bufHead,
	}
}

// takeToken consumes one token from the per-subscriber bucket. Returns
// false when the bucket is empty, meaning the event should be dropped
// on this subscriber. Rate 0 means unlimited.
func (s *subscriber) takeToken(ratePPS int) bool {
	if ratePPS <= 0 {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// Refill: we give back `ratePPS` tokens every whole second since
	// the last refill. Using time.Since here (not wall-clock arithmetic
	// on lastTick) keeps the bucket monotonic.
	if elapsed := time.Since(s.lastTick); elapsed >= time.Second {
		// Integer-seconds refill keeps the math simple and avoids the
		// trap of awarding fractional tokens that never reach 1.
		secs := int(elapsed / time.Second)
		refill := secs * ratePPS
		if refill > ratePPS {
			// Cap the bucket at one second's worth so a long quiet
			// period doesn't let a burst through.
			refill = ratePPS
		}
		s.tokens += refill
		if s.tokens > ratePPS {
			s.tokens = ratePPS
		}
		s.lastTick = s.lastTick.Add(time.Duration(secs) * time.Second)
	}
	if s.tokens <= 0 {
		return false
	}
	s.tokens--
	return true
}
