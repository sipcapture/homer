// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/stream/hepstream"
)

// The node's /stream endpoint requires the node token, so the coordinator
// must send it when it dials.
func TestDialNode_SendsBearerToken(t *testing.T) {
	gotAuth := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth <- r.Header.Get("Authorization")
		http.Error(w, "stop", http.StatusUnauthorized)
	}))
	defer srv.Close()

	host, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)
	// nodeStreamURL dials Port+1, so step back one.
	node := config.NodeEndpoint{Host: host, Port: port - 1, Token: "node-secret"}
	svc := NewStreamService(nil, nil, 2*time.Second, 0)

	_, _ = svc.dialNode(context.Background(), node, hepstream.Filter{}, 0)
	select {
	case got := <-gotAuth:
		if got != "Bearer node-secret" {
			t.Fatalf("Authorization = %q, want Bearer node-secret", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("node was not dialed")
	}
}
