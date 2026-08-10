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

func TestNodesForRangeSkipsNonOverlapping(t *testing.T) {
	s := NewFlightService(nil, time.Second, true)
	const fromNs int64 = 1_700_000_000_000_000_000
	const toNs int64 = 1_700_000_900_000_000_000
	slack := smartRoutingMaxSlackNs
	s.rangeCache = map[string]tsRange{
		"hot":    {min: fromNs - int64(time.Hour), max: fromNs + int64(time.Minute)}, // overlaps
		"cold":   {min: 0, max: fromNs - slack - 1},                                  // too old even with slack
		"near":   {min: 0, max: fromNs - slack/2},                                    // within slack -> keep
		"future": {min: toNs + 1, max: toNs + int64(time.Hour)},                      // too new
		"empty":  {min: 0, max: 0},                                                   // unknown -> keep
	}
	nodes := []config.NodeEndpoint{{Name: "hot"}, {Name: "cold"}, {Name: "near"}, {Name: "future"}, {Name: "empty"}, {Name: "uncached"}}
	got := s.nodesForRange(nodes, fromNs, toNs)
	names := map[string]bool{}
	for _, n := range got {
		names[n.Name] = true
	}
	if !names["hot"] || !names["near"] || !names["empty"] || !names["uncached"] {
		t.Fatalf("hot/near/empty/uncached must be kept, got %v", names)
	}
	if names["cold"] {
		t.Fatalf("cold (effectiveMax<from) must be skipped, got %v", names)
	}
	if names["future"] {
		t.Fatalf("future (min>to) must be skipped, got %v", names)
	}
}

func TestNodesForRangeNeverEmpties(t *testing.T) {
	s := NewFlightService(nil, time.Second, true)
	from := int64(1_700_000_000_000_000_000)
	s.rangeCache = map[string]tsRange{
		"a": {max: from - smartRoutingMaxSlackNs - 10},
		"b": {max: from - smartRoutingMaxSlackNs - 20},
	}
	nodes := []config.NodeEndpoint{{Name: "a"}, {Name: "b"}}
	got := s.nodesForRange(nodes, from, from+int64(time.Hour))
	if len(got) != 2 {
		t.Fatalf("filter must not empty the set; want 2 got %d", len(got))
	}
}

func TestNodesForRangeDisabledKeepsAll(t *testing.T) {
	s := NewFlightService(nil, time.Second, false) // smart routing off
	s.rangeCache = map[string]tsRange{"a": {max: 100}}
	nodes := []config.NodeEndpoint{{Name: "a"}}
	if len(s.nodesForRange(nodes, 1_000, 2_000)) != 1 {
		t.Fatalf("disabled must keep all")
	}
}

func TestNodesForRangeFlushSlackKeepsNearMax(t *testing.T) {
	s := NewFlightService(nil, time.Second, true)
	from := int64(1_700_000_000_000_000_000)
	// max is before from, but within slack — must keep.
	s.rangeCache = map[string]tsRange{"hot": {max: from - smartRoutingMaxSlackNs + 1}}
	got := s.nodesForRange([]config.NodeEndpoint{{Name: "hot"}}, from, from+int64(time.Minute))
	if len(got) != 1 {
		t.Fatalf("node within flush/health slack must be kept")
	}
}
