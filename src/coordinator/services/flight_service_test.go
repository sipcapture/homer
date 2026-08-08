package services

import (
	"context"
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
	svc := NewFlightService(nil, 0)

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
	}, 0)

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
	}, time.Second)

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
