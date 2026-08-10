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
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sipcapture/homer-core/src/config"
)

func TestSmartRoutingSkipsColdNode(t *testing.T) {
	var hotHit, warmHit, coldHit int32
	mk := func(minNs, maxNs int64, hit *int32) *httptest.Server {
		mux := http.NewServeMux()
		mux.HandleFunc("/metadata/stats", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]int64{"min_ts": minNs, "max_ts": maxNs})
		})
		mux.HandleFunc("/query", func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(hit, 1)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true, "count": 1,
				"data": []map[string]interface{}{{"n": 1}},
			})
		})
		return httptest.NewServer(mux)
	}
	now := time.Now().UnixNano()
	hour := int64(time.Hour)
	minute := int64(time.Minute)
	// hot and warm both overlap the last-15-minutes window [now-15m, now]:
	// hot's max is now, warm's max is now-5m (still inside the window). cold's
	// max is ~80 days ago, so it is skipped as "too old" by the full-overlap rule.
	hot := mk(now-2*hour, now, &hotHit)
	warm := mk(now-6*hour, now-5*minute, &warmHit)
	cold := mk(now-90*24*hour, now-80*24*hour, &coldHit)
	defer hot.Close()
	defer warm.Close()
	defer cold.Close()

	ep := func(name string, srv *httptest.Server) config.NodeEndpoint {
		host, portStr, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
		port, _ := strconv.Atoi(portStr)
		return config.NodeEndpoint{Name: name, Host: host, Port: port - 1} // node HTTP = Port+1
	}
	nodes := []config.NodeEndpoint{ep("hot", hot), ep("warm", warm), ep("cold", cold)}
	s := NewFlightService(nodes, 2*time.Second, true)
	s.mu.Lock()
	for _, n := range nodes {
		s.connected[n.Name] = true
		rng, err := s.fetchNodeRange(n)
		if err != nil {
			s.mu.Unlock()
			t.Fatalf("fetch %s: %v", n.Name, err)
		}
		s.rangeCache[n.Name] = rng
	}
	s.mu.Unlock()

	from := now - 15*int64(time.Minute) // last 15 minutes
	rows, err := s.QueryWithRange(context.Background(), "SELECT 1", from, now)
	if err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&coldHit) != 0 {
		t.Fatalf("cold node must not be queried, hits=%d", coldHit)
	}
	if atomic.LoadInt32(&hotHit) != 1 || atomic.LoadInt32(&warmHit) != 1 {
		t.Fatalf("hot/warm must each be queried once, got hot=%d warm=%d", hotHit, warmHit)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 merged rows (hot+warm), got %d", len(rows))
	}
}
