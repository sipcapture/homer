// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package sink defines the boundary between the OTLP receiver
// (HTTP+gRPC) and homer-core's storage subsystems. The receiver itself
// is signal-agnostic — it parses the wire format and hands a typed
// request to a Sink. Concrete sinks turn that into HEP packets,
// DuckLake INSERTs, etc.
package sink

import (
	"context"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// Sink is the destination for parsed OTLP export requests. Implementations
// must be safe for concurrent use by both the gRPC and HTTP receivers.
//
// PushXxx returns an error to signal "delivery failed" so the receiver
// can map it to a 5xx HTTP response or a non-OK gRPC status. A nil
// error means "we accepted responsibility for these records" — the
// sink is then on the hook for any retries / batching.
type Sink interface {
	PushTraces(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) error
	PushMetrics(ctx context.Context, req *colmetricspb.ExportMetricsServiceRequest) error
	PushLogs(ctx context.Context, req *collogspb.ExportLogsServiceRequest) error
}

// Noop is a Sink that accepts and discards every request. Useful for
// dry-run benchmarks and as the fallback when every concrete sink is
// disabled in config.
type Noop struct{}

// PushTraces does nothing.
func (Noop) PushTraces(_ context.Context, _ *coltracepb.ExportTraceServiceRequest) error {
	return nil
}

// PushMetrics does nothing.
func (Noop) PushMetrics(_ context.Context, _ *colmetricspb.ExportMetricsServiceRequest) error {
	return nil
}

// PushLogs does nothing.
func (Noop) PushLogs(_ context.Context, _ *collogspb.ExportLogsServiceRequest) error {
	return nil
}
