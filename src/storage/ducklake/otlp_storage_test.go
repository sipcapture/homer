// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// newOTLPTestStorage returns an OTLPStorage backed by an in-memory
// DuckDB instance. The lake name is a regular DuckDB schema so we can
// exercise the same INSERT/SELECT path without spinning up the full
// DuckLake stack.
func newOTLPTestStorage(t *testing.T) (*OTLPStorage, *sql.DB) {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open duckdb: %v", err)
	}
	if _, err := db.Exec(`CREATE SCHEMA otlp_test;`); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	s := &OTLPStorage{db: db, lakeName: "otlp_test"}
	if err := s.EnsureOTLPSchema(context.Background()); err != nil {
		t.Fatalf("EnsureOTLPSchema: %v", err)
	}
	return s, db
}

func TestOTLPStorage_WriteOTLPTraces(t *testing.T) {
	s, db := newOTLPTestStorage(t)
	defer db.Close()

	now := time.Now().UnixNano()
	req := []*tracepb.ResourceSpans{
		{
			Resource: &resourcepb.Resource{
				Attributes: []*commonpb.KeyValue{
					strKV("service.name", "checkout"),
					strKV("env", "prod"),
				},
			},
			ScopeSpans: []*tracepb.ScopeSpans{
				{
					Scope: &commonpb.InstrumentationScope{Name: "checkout/http", Version: "1.2.3"},
					Spans: []*tracepb.Span{
						{
							TraceId:           []byte("abcdef1234567890"),
							SpanId:            []byte("11223344"),
							ParentSpanId:      []byte("99887766"),
							Name:              "POST /cart/checkout",
							Kind:              tracepb.Span_SPAN_KIND_SERVER,
							StartTimeUnixNano: uint64(now),
							EndTimeUnixNano:   uint64(now + int64(150*time.Millisecond)),
							Attributes: []*commonpb.KeyValue{
								strKV("http.method", "POST"),
								intKV("http.status_code", 200),
							},
							Status: &tracepb.Status{Code: tracepb.Status_STATUS_CODE_OK},
						},
					},
				},
			},
		},
	}

	n, err := s.WriteOTLPTraces(context.Background(), req)
	if err != nil {
		t.Fatalf("WriteOTLPTraces: %v", err)
	}
	if n != 1 {
		t.Fatalf("written = %d, want 1", n)
	}

	var (
		count        int
		serviceName  string
		traceID      string
		name         string
		statusCode   int
		durationNs   int64
	)
	row := db.QueryRow(`SELECT COUNT(*), MAX(service_name), MAX(trace_id), MAX(name), MAX(status_code), MAX(duration_ns) FROM otlp_test.otlp_traces;`)
	if err := row.Scan(&count, &serviceName, &traceID, &name, &statusCode, &durationNs); err != nil {
		t.Fatalf("select: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
	if serviceName != "checkout" {
		t.Errorf("service_name = %q, want checkout", serviceName)
	}
	if name != "POST /cart/checkout" {
		t.Errorf("name = %q, want POST /cart/checkout", name)
	}
	if statusCode != int(tracepb.Status_STATUS_CODE_OK) {
		t.Errorf("status_code = %d, want %d", statusCode, tracepb.Status_STATUS_CODE_OK)
	}
	if durationNs <= 0 {
		t.Errorf("duration_ns = %d, want > 0", durationNs)
	}
	if traceID == "" {
		t.Errorf("trace_id is empty")
	}
}

func TestOTLPStorage_WriteOTLPMetrics(t *testing.T) {
	s, db := newOTLPTestStorage(t)
	defer db.Close()

	ts := uint64(time.Now().UnixNano())
	req := []*metricpb.ResourceMetrics{
		{
			Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{strKV("service.name", "billing")}},
			ScopeMetrics: []*metricpb.ScopeMetrics{
				{
					Scope: &commonpb.InstrumentationScope{Name: "billing/payments"},
					Metrics: []*metricpb.Metric{
						{
							Name: "payments.processed",
							Unit: "1",
							Data: &metricpb.Metric_Sum{
								Sum: &metricpb.Sum{
									DataPoints: []*metricpb.NumberDataPoint{
										{
											TimeUnixNano: ts,
											Value:        &metricpb.NumberDataPoint_AsInt{AsInt: 7},
											Attributes:   []*commonpb.KeyValue{strKV("currency", "EUR")},
										},
									},
								},
							},
						},
						{
							Name: "queue.depth",
							Data: &metricpb.Metric_Gauge{
								Gauge: &metricpb.Gauge{
									DataPoints: []*metricpb.NumberDataPoint{
										{
											TimeUnixNano: ts,
											Value:        &metricpb.NumberDataPoint_AsDouble{AsDouble: 12.5},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	n, err := s.WriteOTLPMetrics(context.Background(), req)
	if err != nil {
		t.Fatalf("WriteOTLPMetrics: %v", err)
	}
	if n != 2 {
		t.Fatalf("written = %d, want 2", n)
	}

	var sumValue int64
	if err := db.QueryRow(`SELECT value_int FROM otlp_test.otlp_metrics WHERE name = 'payments.processed';`).Scan(&sumValue); err != nil {
		t.Fatalf("select sum: %v", err)
	}
	if sumValue != 7 {
		t.Errorf("sum value_int = %d, want 7", sumValue)
	}

	var gaugeValue float64
	if err := db.QueryRow(`SELECT value_double FROM otlp_test.otlp_metrics WHERE name = 'queue.depth';`).Scan(&gaugeValue); err != nil {
		t.Fatalf("select gauge: %v", err)
	}
	if gaugeValue != 12.5 {
		t.Errorf("gauge value_double = %v, want 12.5", gaugeValue)
	}
}

func TestOTLPStorage_WriteOTLPLogs(t *testing.T) {
	s, db := newOTLPTestStorage(t)
	defer db.Close()

	ts := uint64(time.Now().UnixNano())
	req := []*logspb.ResourceLogs{
		{
			Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{strKV("service.name", "auth")}},
			ScopeLogs: []*logspb.ScopeLogs{
				{
					Scope: &commonpb.InstrumentationScope{Name: "auth/main"},
					LogRecords: []*logspb.LogRecord{
						{
							TimeUnixNano:   ts,
							SeverityNumber: logspb.SeverityNumber_SEVERITY_NUMBER_ERROR,
							SeverityText:   "ERROR",
							Body:           &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "login failed"}},
							TraceId:        []byte("trace-id-bytes-1"),
							SpanId:         []byte("spanid12"),
							Attributes:     []*commonpb.KeyValue{strKV("user.id", "alice")},
						},
						{
							TimeUnixNano:   ts + 1,
							SeverityNumber: logspb.SeverityNumber_SEVERITY_NUMBER_INFO,
							Body: &commonpb.AnyValue{Value: &commonpb.AnyValue_KvlistValue{
								KvlistValue: &commonpb.KeyValueList{Values: []*commonpb.KeyValue{
									strKV("event", "login_success"),
									strKV("user.id", "bob"),
								}},
							}},
						},
					},
				},
			},
		},
	}

	n, err := s.WriteOTLPLogs(context.Background(), req)
	if err != nil {
		t.Fatalf("WriteOTLPLogs: %v", err)
	}
	if n != 2 {
		t.Fatalf("written = %d, want 2", n)
	}

	var (
		errBody     string
		errSeverity int
	)
	if err := db.QueryRow(`SELECT body, severity_number FROM otlp_test.otlp_logs WHERE severity_text = 'ERROR';`).Scan(&errBody, &errSeverity); err != nil {
		t.Fatalf("select error log: %v", err)
	}
	if errBody != "login failed" {
		t.Errorf("body = %q, want 'login failed'", errBody)
	}
	if errSeverity != int(logspb.SeverityNumber_SEVERITY_NUMBER_ERROR) {
		t.Errorf("severity_number = %d, want %d", errSeverity, logspb.SeverityNumber_SEVERITY_NUMBER_ERROR)
	}

	// Structured body must land in body_json (string body must be empty).
	var (
		jsonBody string
		strBody  string
	)
	if err := db.QueryRow(`SELECT CAST(body_json AS VARCHAR), body FROM otlp_test.otlp_logs WHERE severity_number = ?;`,
		int32(logspb.SeverityNumber_SEVERITY_NUMBER_INFO)).Scan(&jsonBody, &strBody); err != nil {
		t.Fatalf("select structured log: %v", err)
	}
	if strBody != "" {
		t.Errorf("structured log: body = %q, want empty", strBody)
	}
	if jsonBody == "" || jsonBody == "null" {
		t.Errorf("structured log: body_json empty (%q)", jsonBody)
	}
}

func TestOTLPStorage_EmptyRequestsAreNoOp(t *testing.T) {
	s, db := newOTLPTestStorage(t)
	defer db.Close()

	if n, err := s.WriteOTLPTraces(context.Background(), nil); err != nil || n != 0 {
		t.Fatalf("traces nil: n=%d err=%v", n, err)
	}
	if n, err := s.WriteOTLPMetrics(context.Background(), nil); err != nil || n != 0 {
		t.Fatalf("metrics nil: n=%d err=%v", n, err)
	}
	if n, err := s.WriteOTLPLogs(context.Background(), nil); err != nil || n != 0 {
		t.Fatalf("logs nil: n=%d err=%v", n, err)
	}
}

func strKV(k, v string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}}}
}

func intKV(k string, v int64) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: v}}}
}
