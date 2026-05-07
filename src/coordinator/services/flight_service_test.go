package services

import (
	"context"
	"strings"
	"testing"

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
