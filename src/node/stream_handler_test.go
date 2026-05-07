// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package node

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/sipcapture/homer-core/src/stream/hepstream"
)

// newTestNode returns a Node just good enough to exercise handleStream.
// We explicitly don't call node.New() because that wants a full Config
// + DuckDB attach path; the stream handler only touches n.mu and n.broker.
func newTestNode(b *hepstream.Broker) *Node {
	n := &Node{}
	n.SetBroker(b)
	return n
}

func TestHandleStream_NoBrokerConfigured(t *testing.T) {
	n := &Node{} // no broker
	srv := httptest.NewServer(http.HandlerFunc(n.handleStream))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/stream")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func TestHandleStream_LiveEventFlow(t *testing.T) {
	broker := hepstream.NewBroker(hepstream.Config{
		Enable:         true,
		BufferSize:     100,
		MaxSubscribers: 4,
		PerSubQueueLen: 16,
	})

	n := newTestNode(broker)
	srv := httptest.NewServer(http.HandlerFunc(n.handleStream))
	defer srv.Close()

	// Flip http://… into ws://… so the gorilla-compatible dialer
	// accepts it. httptest always returns http, so this is safe.
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/stream?proto=1"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Publish *after* the dial so we exercise the live path rather
	// than the history replay path. A small sleep covers the gap
	// between dial return and the handler's Subscribe call.
	time.Sleep(50 * time.Millisecond)
	broker.Publish(hepstream.Event{
		TsMilli: time.Now().UnixMilli(),
		Proto:   1,
		SrcIP:   "10.0.0.5",
		DstIP:   "10.0.0.1",
		SIP: &hepstream.SIPMeta{
			Method: "INVITE",
			CallID: "abc@test",
		},
	})

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, buf, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(buf, &got); err != nil {
		t.Fatalf("decode: %v (raw=%s)", err, buf)
	}
	if got["proto"].(float64) != 1 {
		t.Fatalf("expected proto=1, got %v", got["proto"])
	}
	sip, ok := got["sip"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected sip block, got %v", got)
	}
	if sip["method"] != "INVITE" {
		t.Fatalf("expected method INVITE, got %v", sip["method"])
	}
	if _, hasPayload := got["payload"]; hasPayload {
		t.Fatalf("payload must be omitted when include_payload is off")
	}
}

func TestHandleStream_HistoryReplay(t *testing.T) {
	broker := hepstream.NewBroker(hepstream.Config{
		Enable:         true,
		BufferSize:     100,
		MaxSubscribers: 4,
		PerSubQueueLen: 16,
	})
	for i := 0; i < 3; i++ {
		broker.Publish(hepstream.Event{
			TsMilli: time.Now().UnixMilli(),
			Proto:   1,
			SIP: &hepstream.SIPMeta{
				Method: "REGISTER",
				CallID: "hist",
			},
		})
	}

	n := newTestNode(broker)
	srv := httptest.NewServer(http.HandlerFunc(n.handleStream))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/stream?proto=1&history=10"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	seen := 0
	for seen < 3 {
		_, buf, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read after %d frames: %v", seen, err)
		}
		if !strings.Contains(string(buf), "REGISTER") {
			t.Fatalf("frame %d: expected REGISTER, got %s", seen, buf)
		}
		seen++
	}
}

func TestHandleStream_ProtoFilter(t *testing.T) {
	broker := hepstream.NewBroker(hepstream.Config{
		Enable:         true,
		BufferSize:     100,
		MaxSubscribers: 4,
		PerSubQueueLen: 16,
	})

	n := newTestNode(broker)
	srv := httptest.NewServer(http.HandlerFunc(n.handleStream))
	defer srv.Close()

	// Only proto=5 subscribes; proto=1 events should never arrive.
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/stream?proto=5"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)
	broker.Publish(hepstream.Event{TsMilli: 1, Proto: 1}) // filtered out
	broker.Publish(hepstream.Event{TsMilli: 2, Proto: 5}) // match

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, buf, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(buf), `"proto":5`) {
		t.Fatalf("expected proto=5 frame, got %s", buf)
	}
}
