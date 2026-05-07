// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package otlpreceiver

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sipcapture/homer-core/src/config"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// captureSink keeps the last request seen for each signal so test
// assertions can read them back without leaking goroutine state.
type captureSink struct {
	traces  atomic.Pointer[coltracepb.ExportTraceServiceRequest]
	metrics atomic.Pointer[colmetricspb.ExportMetricsServiceRequest]
	logs    atomic.Pointer[collogspb.ExportLogsServiceRequest]
}

func (c *captureSink) PushTraces(_ context.Context, req *coltracepb.ExportTraceServiceRequest) error {
	c.traces.Store(req)
	return nil
}

func (c *captureSink) PushMetrics(_ context.Context, req *colmetricspb.ExportMetricsServiceRequest) error {
	c.metrics.Store(req)
	return nil
}

func (c *captureSink) PushLogs(_ context.Context, req *collogspb.ExportLogsServiceRequest) error {
	c.logs.Store(req)
	return nil
}

// newTestHTTPServer returns an httpServer wired to httptest, plus the
// underlying captureSink. The returned httptest.Server runs the same
// mux the production listener exposes.
func newTestHTTPServer(t *testing.T) (*httptest.Server, *captureSink) {
	t.Helper()
	cfg := &config.OTLPConfig{
		Enable:          true,
		MaxRecvMsgBytes: 4 * 1024 * 1024,
		HTTP:            config.OTLPHTTPConfig{Enable: true, Listen: ":0"},
	}
	sink := &captureSink{}
	hs, err := newHTTPServer(cfg, sink)
	if err != nil {
		t.Fatalf("newHTTPServer: %v", err)
	}
	srv := httptest.NewServer(hs.server.Handler)
	t.Cleanup(srv.Close)
	return srv, sink
}

// sampleTracesPB returns a minimal ExportTraceServiceRequest.
func sampleTracesPB() *coltracepb.ExportTraceServiceRequest {
	return &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{{
				Key:   "service.name",
				Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "svc"}},
			}}},
			ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{Name: "op"}}}},
		}},
	}
}

func sampleMetricsPB() *colmetricspb.ExportMetricsServiceRequest {
	return &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricpb.ResourceMetrics{{
			ScopeMetrics: []*metricpb.ScopeMetrics{{
				Metrics: []*metricpb.Metric{{
					Name: "rps",
					Data: &metricpb.Metric_Gauge{Gauge: &metricpb.Gauge{
						DataPoints: []*metricpb.NumberDataPoint{{
							Value: &metricpb.NumberDataPoint_AsDouble{AsDouble: 1.0},
						}},
					}},
				}},
			}},
		}},
	}
}

func sampleLogsPB() *collogspb.ExportLogsServiceRequest {
	return &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{{
			ScopeLogs: []*logspb.ScopeLogs{{
				LogRecords: []*logspb.LogRecord{{
					SeverityText: "INFO",
					Body:         &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "hi"}},
				}},
			}},
		}},
	}
}

func TestHTTP_TracesProtobuf(t *testing.T) {
	srv, sink := newTestHTTPServer(t)
	body, err := proto.Marshal(sampleTracesPB())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	resp, err := http.Post(srv.URL+"/v1/traces", contentTypeProtobuf, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := sink.traces.Load(); got == nil || len(got.GetResourceSpans()) != 1 {
		t.Fatalf("sink not invoked or empty: %v", got)
	}
}

func TestHTTP_TracesJSON(t *testing.T) {
	srv, sink := newTestHTTPServer(t)
	body, err := protojson.Marshal(sampleTracesPB())
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}

	resp, err := http.Post(srv.URL+"/v1/traces", contentTypeJSON, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := sink.traces.Load(); got == nil {
		t.Fatalf("sink not invoked")
	}
}

func TestHTTP_TracesGzipProtobuf(t *testing.T) {
	srv, sink := newTestHTTPServer(t)
	raw, err := proto.Marshal(sampleTracesPB())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(raw); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	_ = gz.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/traces", &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", contentTypeProtobuf)
	req.Header.Set("Content-Encoding", "gzip")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := sink.traces.Load(); got == nil {
		t.Fatalf("sink not invoked")
	}
}

func TestHTTP_MetricsProtobuf(t *testing.T) {
	srv, sink := newTestHTTPServer(t)
	body, err := proto.Marshal(sampleMetricsPB())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(srv.URL+"/v1/metrics", contentTypeProtobuf, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := sink.metrics.Load(); got == nil {
		t.Fatalf("sink not invoked")
	}
}

func TestHTTP_LogsJSON(t *testing.T) {
	srv, sink := newTestHTTPServer(t)
	body, err := protojson.Marshal(sampleLogsPB())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(srv.URL+"/v1/logs", contentTypeJSON, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := sink.logs.Load(); got == nil {
		t.Fatalf("sink not invoked")
	}
}

func TestHTTP_UnsupportedMediaType(t *testing.T) {
	srv, _ := newTestHTTPServer(t)
	resp, err := http.Post(srv.URL+"/v1/traces", "text/plain", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", resp.StatusCode)
	}
}

func TestHTTP_BadProtobufReturns400(t *testing.T) {
	srv, _ := newTestHTTPServer(t)
	resp, err := http.Post(srv.URL+"/v1/traces", contentTypeProtobuf, strings.NewReader("not a protobuf"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHTTP_MethodNotAllowed(t *testing.T) {
	srv, _ := newTestHTTPServer(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/traces", nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}
