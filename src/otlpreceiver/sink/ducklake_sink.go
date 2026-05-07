// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package sink

import (
	"context"
	"fmt"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
	"github.com/sipcapture/homer-core/src/utils/metrics"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// DuckLake is the canonical OTLP sink: every signal lands in its own
// DuckLake table on the primary shard (otlp_traces, otlp_metrics,
// otlp_logs). Per-signal storage can be disabled in config — those
// PushXxx calls then short-circuit without touching the database.
type DuckLake struct {
	storage *ducklake.OTLPStorage
	cfg     config.OTLPSinksConfig
}

// NewDuckLake wires a DuckLake sink. The storage handle is acquired
// from the manager (typically Manager.OTLPStorage()). Returns an error
// if the storage handle is nil — the receiver should then fall back to
// a Noop sink rather than start in a half-broken state.
func NewDuckLake(storage *ducklake.OTLPStorage, cfg config.OTLPSinksConfig) (*DuckLake, error) {
	if storage == nil {
		return nil, fmt.Errorf("otlp ducklake sink: nil OTLPStorage handle")
	}
	return &DuckLake{storage: storage, cfg: cfg}, nil
}

// EnsureSchema runs the idempotent CREATE TABLE / partitioning DDL.
// Safe to call from the receiver Start path before the first request
// arrives; safe to call repeatedly (subsequent calls are no-ops).
func (d *DuckLake) EnsureSchema(ctx context.Context) error {
	if d == nil || d.storage == nil {
		return nil
	}
	return d.storage.EnsureOTLPSchema(ctx)
}

// PushTraces persists incoming spans into otlp_traces.
func (d *DuckLake) PushTraces(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) error {
	if d == nil || !d.cfg.StoreTraces || req == nil {
		return nil
	}
	n, err := d.storage.WriteOTLPTraces(ctx, req.GetResourceSpans())
	if err != nil {
		// WriteOTLPTraces already incremented the sink-error metric.
		return err
	}
	metrics.AddOTLPRecords("traces", n)
	return nil
}

// PushMetrics persists incoming data points into otlp_metrics.
func (d *DuckLake) PushMetrics(ctx context.Context, req *colmetricspb.ExportMetricsServiceRequest) error {
	if d == nil || !d.cfg.StoreMetrics || req == nil {
		return nil
	}
	n, err := d.storage.WriteOTLPMetrics(ctx, req.GetResourceMetrics())
	if err != nil {
		return err
	}
	metrics.AddOTLPRecords("metrics", n)
	return nil
}

// PushLogs persists incoming log records into otlp_logs.
func (d *DuckLake) PushLogs(ctx context.Context, req *collogspb.ExportLogsServiceRequest) error {
	if d == nil || !d.cfg.StoreLogs || req == nil {
		return nil
	}
	n, err := d.storage.WriteOTLPLogs(ctx, req.GetResourceLogs())
	if err != nil {
		return err
	}
	metrics.AddOTLPRecords("logs", n)
	return nil
}
