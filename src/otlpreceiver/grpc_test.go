// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package otlpreceiver

import (
	"context"
	"net"
	"testing"
	"time"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// newTestGRPC builds a bufconn-backed gRPC server with the three OTLP
// services registered against a captureSink, and returns a dialed
// client connection ready for Export calls.
func newTestGRPC(t *testing.T) (*grpc.ClientConn, *captureSink) {
	t.Helper()
	sink := &captureSink{}
	srv := grpc.NewServer()
	coltracepb.RegisterTraceServiceServer(srv, &grpcTraceService{sink: sink})
	colmetricspb.RegisterMetricsServiceServer(srv, &grpcMetricsService{sink: sink})
	collogspb.RegisterLogsServiceServer(srv, &grpcLogsService{sink: sink})

	lis := bufconn.Listen(1024 * 1024)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	dialer := func(_ context.Context, _ string) (net.Conn, error) { return lis.Dial() }
	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, sink
}

func TestGRPC_TracesExport(t *testing.T) {
	conn, sink := newTestGRPC(t)
	client := coltracepb.NewTraceServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.Export(ctx, sampleTracesPB()); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if got := sink.traces.Load(); got == nil || len(got.GetResourceSpans()) != 1 {
		t.Fatalf("sink not invoked or empty: %v", got)
	}
}

func TestGRPC_MetricsExport(t *testing.T) {
	conn, sink := newTestGRPC(t)
	client := colmetricspb.NewMetricsServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.Export(ctx, sampleMetricsPB()); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if got := sink.metrics.Load(); got == nil {
		t.Fatalf("sink not invoked")
	}
}

func TestGRPC_LogsExport(t *testing.T) {
	conn, sink := newTestGRPC(t)
	client := collogspb.NewLogsServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.Export(ctx, sampleLogsPB()); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if got := sink.logs.Load(); got == nil {
		t.Fatalf("sink not invoked")
	}
}
