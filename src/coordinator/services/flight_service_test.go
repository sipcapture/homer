package services

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sipcapture/homer-core/src/config"
)

func TestFlightServiceCloseAllIsIdempotent(t *testing.T) {
	svc := NewFlightService(nil, 0, false)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("CloseAll should not panic on repeated calls, got panic: %v", r)
		}
	}()

	svc.CloseAll()
	svc.CloseAll()
}

func TestFlightServiceQueryNodeNotFound(t *testing.T) {
	svc := NewFlightService([]config.NodeEndpoint{
		{Name: "node-a", Host: "127.0.0.1", Port: 30000},
	}, 0, false)

	_, err := svc.QueryNode(context.Background(), "missing-node", "SELECT 1")
	if err == nil {
		t.Fatal("expected error for missing node, got nil")
	}
	if !strings.Contains(err.Error(), "missing-node") {
		t.Fatalf("expected error to include node name, got: %v", err)
	}
}

func TestFlightServiceQuerySendsBearerToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":[{"one":1}],"count":1}`))
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	host := u.Hostname()
	port, _ := strconv.Atoi(u.Port())
	// queryNode uses Port+1 for HTTP; point Port at serverPort-1.
	svc := NewFlightService([]config.NodeEndpoint{
		{Name: "local", Host: host, Port: port - 1, Token: "node-secret"},
	}, time.Second, false)

	rows, err := svc.QueryNode(context.Background(), "local", "SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%v", rows)
	}
	if gotAuth != "Bearer node-secret" {
		t.Fatalf("Authorization=%q", gotAuth)
	}
}

func TestFetchNodeRangeParsesStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metadata/stats" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"min_ts": int64(100), "max_ts": int64(200),
		})
	}))
	defer srv.Close()
	host, portStr, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	port, _ := strconv.Atoi(portStr)
	// Node HTTP API is at Port+1, so configure Port = port-1.
	s := NewFlightService([]config.NodeEndpoint{{Name: "n1", Host: host, Port: port - 1}}, time.Second, true)

	rng, err := s.fetchNodeRange(config.NodeEndpoint{Name: "n1", Host: host, Port: port - 1})
	if err != nil {
		t.Fatal(err)
	}
	if rng.min != 100 || rng.max != 200 {
		t.Fatalf("got %+v, want {100 200}", rng)
	}
}
