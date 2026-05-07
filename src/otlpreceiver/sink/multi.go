// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package sink

import (
	"context"
	"errors"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// Multi fans an OTLP export request out to N sinks. All sinks are
// invoked even when an earlier one returns an error so a slow / broken
// secondary destination cannot starve the primary one. The first
// non-nil error is returned; the rest are joined onto it via errors.Join
// so dashboards still see them.
type Multi struct {
	sinks []Sink
}

// NewMulti returns a Multi sink that delegates to the given sinks.
// nil entries are dropped; if no usable sink remains the Multi behaves
// like a Noop.
func NewMulti(sinks ...Sink) *Multi {
	out := make([]Sink, 0, len(sinks))
	for _, s := range sinks {
		if s == nil {
			continue
		}
		out = append(out, s)
	}
	return &Multi{sinks: out}
}

// PushTraces forwards to every wrapped sink.
func (m *Multi) PushTraces(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) error {
	if m == nil || len(m.sinks) == 0 {
		return nil
	}
	var errs []error
	for _, s := range m.sinks {
		if err := s.PushTraces(ctx, req); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// PushMetrics forwards to every wrapped sink.
func (m *Multi) PushMetrics(ctx context.Context, req *colmetricspb.ExportMetricsServiceRequest) error {
	if m == nil || len(m.sinks) == 0 {
		return nil
	}
	var errs []error
	for _, s := range m.sinks {
		if err := s.PushMetrics(ctx, req); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// PushLogs forwards to every wrapped sink.
func (m *Multi) PushLogs(ctx context.Context, req *collogspb.ExportLogsServiceRequest) error {
	if m == nil || len(m.sinks) == 0 {
		return nil
	}
	var errs []error
	for _, s := range m.sinks {
		if err := s.PushLogs(ctx, req); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
