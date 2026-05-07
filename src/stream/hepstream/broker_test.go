// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package hepstream

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func mkEvent(proto uint32, method, callid string) Event {
	e := Event{
		TsMilli: time.Now().UnixMilli(),
		Proto:   proto,
		SrcIP:   "10.0.0.1",
		SrcPort: 5060,
		DstIP:   "10.0.0.2",
		DstPort: 5060,
		Payload: "payload-" + callid,
	}
	if proto == 1 {
		e.SIP = &SIPMeta{Method: method, CallID: callid}
	}
	return e
}

func TestBrokerDisabledReturnsNil(t *testing.T) {
	b := NewBroker(Config{Enable: false})
	if b != nil {
		t.Fatalf("disabled broker must be nil, got %v", b)
	}
	// All methods must be safe on nil receiver.
	b.Publish(mkEvent(1, "INVITE", "x"))
	if _, _, err := b.Subscribe(Filter{}); err == nil {
		t.Fatalf("Subscribe on nil broker must return an error")
	}
	if h := b.History(Filter{}, 10); h != nil {
		t.Fatalf("History on nil broker must return nil, got %v", h)
	}
	if s := b.Stats(); s != (Stats{}) {
		t.Fatalf("Stats on nil broker must be zero, got %+v", s)
	}
}

func TestBrokerPublishFanOut(t *testing.T) {
	b := NewBroker(Config{Enable: true, BufferSize: 8, MaxSubscribers: 4, PerSubQueueLen: 16})

	ch1, cancel1, err := b.Subscribe(Filter{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel1()
	ch2, cancel2, err := b.Subscribe(Filter{Protos: []uint32{1}, Methods: []string{"INVITE"}})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel2()

	b.Publish(mkEvent(1, "INVITE", "call-1"))
	b.Publish(mkEvent(1, "REGISTER", "reg-1"))
	b.Publish(mkEvent(5, "", "rtcp-1"))

	got1 := drain(ch1, 3, 200*time.Millisecond)
	if len(got1) != 3 {
		t.Fatalf("sub1 got %d events, want 3: %+v", len(got1), got1)
	}

	got2 := drain(ch2, 1, 200*time.Millisecond)
	if len(got2) != 1 {
		t.Fatalf("sub2 got %d events, want 1: %+v", len(got2), got2)
	}
	if got2[0].SIP == nil || got2[0].SIP.Method != "INVITE" {
		t.Fatalf("sub2 event not INVITE: %+v", got2[0])
	}
}

func TestBrokerBackpressureDropsOnSlowSub(t *testing.T) {
	b := NewBroker(Config{Enable: true, BufferSize: 4, MaxSubscribers: 2, PerSubQueueLen: 2})

	// Subscribe but never read — the broker should drop events on us,
	// not block the publisher.
	_, cancel, err := b.Subscribe(Filter{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	const N = 100
	for i := 0; i < N; i++ {
		b.Publish(mkEvent(1, "INVITE", "cid"))
	}
	stats := b.Stats()
	if stats.Published != N {
		t.Fatalf("Published=%d want %d", stats.Published, N)
	}
	if stats.DroppedBackpressure == 0 {
		t.Fatalf("expected DroppedBackpressure > 0, got %+v", stats)
	}
}

func TestBrokerRingBufferHistory(t *testing.T) {
	b := NewBroker(Config{Enable: true, BufferSize: 3, MaxSubscribers: 1, PerSubQueueLen: 1})
	b.Publish(mkEvent(1, "INVITE", "a"))
	b.Publish(mkEvent(1, "INVITE", "b"))
	b.Publish(mkEvent(1, "INVITE", "c"))
	b.Publish(mkEvent(1, "INVITE", "d")) // overwrites "a"

	hist := b.History(Filter{}, 0)
	if len(hist) != 3 {
		t.Fatalf("history len=%d want 3: %+v", len(hist), hist)
	}
	want := []string{"b", "c", "d"}
	for i, e := range hist {
		if e.SIP == nil || e.SIP.CallID != want[i] {
			t.Fatalf("history[%d]=%q want %q", i, callid(e), want[i])
		}
	}

	// With a filter + limit, History keeps the most recent matches.
	hist2 := b.History(Filter{Methods: []string{"INVITE"}}, 2)
	if len(hist2) != 2 {
		t.Fatalf("filtered history len=%d want 2", len(hist2))
	}
	if callid(hist2[0]) != "c" || callid(hist2[1]) != "d" {
		t.Fatalf("filtered history tail wrong: %v", []string{callid(hist2[0]), callid(hist2[1])})
	}
}

func TestBrokerMaxSubscribers(t *testing.T) {
	b := NewBroker(Config{Enable: true, BufferSize: 4, MaxSubscribers: 2, PerSubQueueLen: 4})
	_, c1, err := b.Subscribe(Filter{})
	if err != nil {
		t.Fatalf("s1: %v", err)
	}
	defer c1()
	_, c2, err := b.Subscribe(Filter{})
	if err != nil {
		t.Fatalf("s2: %v", err)
	}
	defer c2()
	if _, _, err := b.Subscribe(Filter{}); err != ErrTooManySubscribers {
		t.Fatalf("s3 want ErrTooManySubscribers, got %v", err)
	}

	c1()
	// After cancelling s1 a new subscriber must fit.
	_, c3, err := b.Subscribe(Filter{})
	if err != nil {
		t.Fatalf("s3 after cancel: %v", err)
	}
	defer c3()
}

func TestBrokerCancelIdempotent(t *testing.T) {
	b := NewBroker(Config{Enable: true, BufferSize: 4, MaxSubscribers: 2, PerSubQueueLen: 4})
	_, cancel, err := b.Subscribe(Filter{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	cancel()
	cancel() // must not panic, must not double-close the channel
}

func TestBrokerConcurrentPublishAndSubscribe(t *testing.T) {
	b := NewBroker(Config{Enable: true, BufferSize: 64, MaxSubscribers: 16, PerSubQueueLen: 64})

	var received atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, cancel, err := b.Subscribe(Filter{})
			if err != nil {
				t.Errorf("Subscribe: %v", err)
				return
			}
			defer cancel()
			timeout := time.After(200 * time.Millisecond)
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						return
					}
					received.Add(1)
				case <-timeout:
					return
				}
			}
		}()
	}

	time.Sleep(10 * time.Millisecond) // let subscribers register
	var pwg sync.WaitGroup
	for p := 0; p < 4; p++ {
		pwg.Add(1)
		go func() {
			defer pwg.Done()
			for i := 0; i < 200; i++ {
				b.Publish(mkEvent(1, "INVITE", "c"))
			}
		}()
	}
	pwg.Wait()
	wg.Wait()

	if received.Load() == 0 {
		t.Fatalf("no events received under concurrent load")
	}
}

func drain(ch <-chan Event, n int, timeout time.Duration) []Event {
	deadline := time.After(timeout)
	out := make([]Event, 0, n)
	for len(out) < n {
		select {
		case e, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, e)
		case <-deadline:
			return out
		}
	}
	return out
}

func callid(e Event) string {
	if e.SIP == nil {
		return ""
	}
	return e.SIP.CallID
}
