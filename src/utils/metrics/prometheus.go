// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HEP packets received counter by protocol
	HEPPacketsReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "homer_hep_packets_received_total",
			Help: "Total number of HEP packets received by protocol",
		},
		[]string{"protocol"}, // tcp, udp, tls
	)

	// HEP packets processed counter
	HEPPacketsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "homer_hep_packets_processed_total",
			Help: "Total number of HEP packets successfully processed",
		},
		[]string{"protocol"},
	)

	// HEP packets failed counter
	HEPPacketsFailed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "homer_hep_packets_failed_total",
			Help: "Total number of HEP packets that failed processing",
		},
		[]string{"protocol", "reason"}, // reason: invalid, decode_error, etc.
	)

	// HEP packet size histogram
	HEPPacketSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "homer_hep_packet_size_bytes",
			Help:    "HEP packet size distribution in bytes",
			Buckets: prometheus.ExponentialBuckets(64, 2, 12), // 64 bytes to 256KB
		},
		[]string{"protocol"},
	)

	// Active connections gauge
	ActiveConnections = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "homer_active_connections",
			Help: "Number of active connections by protocol",
		},
		[]string{"protocol"},
	)

	// Bytes received counter
	BytesReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "homer_bytes_received_total",
			Help: "Total number of bytes received by protocol",
		},
		[]string{"protocol"},
	)

	// Bytes sent counter
	BytesSent = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "homer_bytes_sent_total",
			Help: "Total number of bytes sent by protocol",
		},
		[]string{"protocol"},
	)

	// Processing duration histogram
	ProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "homer_packet_processing_duration_seconds",
			Help:    "HEP packet processing duration in seconds",
			Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0},
		},
		[]string{"protocol"},
	)

	// Pipeline stage duration histogram
	PipelineStageDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "homer_packet_pipeline_stage_duration_seconds",
			Help:    "HEP packet processing duration by pipeline stage in seconds",
			Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0},
		},
		[]string{"protocol", "stage"},
	)

	// Pipeline stage error counter
	PipelineStageErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "homer_packet_pipeline_stage_errors_total",
			Help: "HEP packet errors by pipeline stage and reason",
		},
		[]string{"protocol", "stage", "reason"},
	)

	// Queue wait duration histogram
	PipelineQueueWaitDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "homer_packet_pipeline_queue_wait_duration_seconds",
			Help:    "Time packets spent waiting in the worker queue in seconds",
			Buckets: []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0},
		},
		[]string{"protocol"},
	)

	// DuckLake table flush duration histogram
	DucklakeTableFlushDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "homer_ducklake_table_flush_duration_seconds",
			Help:    "DuckLake table flush duration in seconds",
			Buckets: []float64{0.001, 0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10},
		},
		[]string{"table"},
	)

	// DuckLake flushed row counter
	DucklakeTableFlushedRows = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "homer_ducklake_table_flushed_rows_total",
			Help: "Total number of rows flushed from DuckLake in-memory buffers to disk tables",
		},
		[]string{"table"},
	)

	// Worker queue depth gauge
	WorkerQueueDepth = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "homer_worker_queue_depth",
			Help: "Current depth of the worker queue",
		},
	)

	// Worker queue capacity gauge
	WorkerQueueCapacity = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "homer_worker_queue_capacity",
			Help: "Maximum capacity of the worker queue",
		},
	)

	// OTLPRequestsReceived counts incoming OTLP export requests by signal
	// (traces|metrics|logs) and transport (grpc|http_proto|http_json).
	// One request can carry many records — see OTLPRecordsReceived for
	// the per-record fan-out.
	OTLPRequestsReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "homer_otlp_requests_received_total",
			Help: "Total number of OTLP export requests received by signal and transport",
		},
		[]string{"signal", "transport"},
	)

	// OTLPRequestsFailed counts OTLP export requests rejected before
	// the sink is invoked (decode_error, body_too_large, unsupported_media_type, ...).
	OTLPRequestsFailed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "homer_otlp_requests_failed_total",
			Help: "Total number of OTLP export requests that failed (decoding, validation, transport)",
		},
		[]string{"signal", "transport", "reason"},
	)

	// OTLPRecordsReceived counts the per-record fan-out within accepted
	// OTLP export requests: spans for traces, metric data points for
	// metrics, log records for logs.
	OTLPRecordsReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "homer_otlp_records_received_total",
			Help: "Total number of OTLP records received (spans, metric points, log records)",
		},
		[]string{"signal"},
	)

	// OTLPSinkErrors counts failures from downstream sinks (e.g.
	// ducklake_traces, ducklake_metrics, hep_logs). These are
	// "delivered to receiver, lost downstream" and are scored
	// separately from request-level failures so operators can tell the
	// two failure modes apart on a single dashboard.
	OTLPSinkErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "homer_otlp_sink_errors_total",
			Help: "Total number of OTLP sink errors by signal and sink",
		},
		[]string{"signal", "sink"},
	)

	// OTLPAsyncEnqueue counts async-queue enqueue outcomes (ok, queue_full,
	// shutdown) for the budget OTLP path in homer-core.
	OTLPAsyncEnqueue = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "homer_otlp_async_enqueue_total",
			Help: "OTLP async sink enqueue results by signal and outcome",
		},
		[]string{"signal", "outcome"},
	)

	// OTLPAsyncWorkerErrors counts failures from the inner sink observed
	// by the async OTLP worker (after the HTTP/gRPC handler already returned).
	OTLPAsyncWorkerErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "homer_otlp_async_worker_errors_total",
			Help: "OTLP async worker inner-sink failures by signal",
		},
		[]string{"signal"},
	)

	// LineProtoRequestsReceived counts incoming InfluxDB Line Protocol
	// HTTP write requests by transport endpoint (v1 / v2 / v3) and
	// outcome ("ok" or short error reason).
	LineProtoRequestsReceived = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "homer_lp_requests_received_total",
			Help: "Total number of InfluxDB Line Protocol HTTP requests received",
		},
		[]string{"endpoint", "outcome"},
	)

	// LineProtoPointsIngested counts the per-point fan-out of accepted
	// Line Protocol requests (one LP "line" → one point).
	LineProtoPointsIngested = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "homer_lp_points_ingested_total",
			Help: "Total number of Line Protocol points successfully written to DuckLake",
		},
		[]string{"db"},
	)

	// LineProtoWriteErrors counts ingest-side failures (DDL / INSERT)
	// after a body was successfully parsed. Separate from
	// LineProtoRequestsReceived{outcome="parse_error"} so operators can
	// tell parse failures from storage failures on a dashboard.
	LineProtoWriteErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "homer_lp_write_errors_total",
			Help: "Total number of Line Protocol write failures by stage",
		},
		[]string{"stage"},
	)
)

// RecordHEPPacketReceived increments the counter for received HEP packets
func RecordHEPPacketReceived(protocol string) {
	HEPPacketsReceived.WithLabelValues(protocol).Inc()
}

// RecordHEPPacketProcessed increments the counter for processed HEP packets
func RecordHEPPacketProcessed(protocol string) {
	HEPPacketsProcessed.WithLabelValues(protocol).Inc()
}

// RecordHEPPacketFailed increments the counter for failed HEP packets
func RecordHEPPacketFailed(protocol, reason string) {
	HEPPacketsFailed.WithLabelValues(protocol, reason).Inc()
}

// RecordHEPPacketSize records the size of a HEP packet
func RecordHEPPacketSize(protocol string, size int) {
	HEPPacketSize.WithLabelValues(protocol).Observe(float64(size))
}

// SetActiveConnections sets the number of active connections
func SetActiveConnections(protocol string, count float64) {
	ActiveConnections.WithLabelValues(protocol).Set(count)
}

// IncrementActiveConnections increments the active connections counter
func IncrementActiveConnections(protocol string) {
	ActiveConnections.WithLabelValues(protocol).Inc()
}

// DecrementActiveConnections decrements the active connections counter
func DecrementActiveConnections(protocol string) {
	ActiveConnections.WithLabelValues(protocol).Dec()
}

// RecordBytesReceived adds bytes to the received counter
func RecordBytesReceived(protocol string, bytes int64) {
	BytesReceived.WithLabelValues(protocol).Add(float64(bytes))
}

// RecordBytesSent adds bytes to the sent counter
func RecordBytesSent(protocol string, bytes int64) {
	BytesSent.WithLabelValues(protocol).Add(float64(bytes))
}

// RecordProcessingDuration records the processing duration
func RecordProcessingDuration(protocol string, duration float64) {
	ProcessingDuration.WithLabelValues(protocol).Observe(duration)
}

// RecordPipelineStageDuration records stage-specific processing time
func RecordPipelineStageDuration(protocol, stage string, duration float64) {
	PipelineStageDuration.WithLabelValues(protocol, stage).Observe(duration)
}

// RecordPipelineStageError records stage-specific processing errors
func RecordPipelineStageError(protocol, stage, reason string) {
	PipelineStageErrors.WithLabelValues(protocol, stage, reason).Inc()
}

// RecordPipelineQueueWaitDuration records queue wait duration
func RecordPipelineQueueWaitDuration(protocol string, duration float64) {
	PipelineQueueWaitDuration.WithLabelValues(protocol).Observe(duration)
}

// RecordDucklakeTableFlushDuration records table flush duration
func RecordDucklakeTableFlushDuration(table string, duration float64) {
	DucklakeTableFlushDuration.WithLabelValues(table).Observe(duration)
}

// RecordDucklakeTableFlushedRows records number of rows flushed for a table
func RecordDucklakeTableFlushedRows(table string, rows int64) {
	DucklakeTableFlushedRows.WithLabelValues(table).Add(float64(rows))
}

// SetWorkerQueueDepth sets the current worker queue depth
func SetWorkerQueueDepth(depth float64) {
	WorkerQueueDepth.Set(depth)
}

// SetWorkerQueueCapacity sets the worker queue capacity
func SetWorkerQueueCapacity(capacity float64) {
	WorkerQueueCapacity.Set(capacity)
}

// RecordOTLPRequest counts one accepted OTLP export request.
// signal: "traces" | "metrics" | "logs"
// transport: "grpc" | "http_proto" | "http_json"
func RecordOTLPRequest(signal, transport string) {
	OTLPRequestsReceived.WithLabelValues(signal, transport).Inc()
}

// RecordOTLPFailure counts one rejected OTLP export request.
// reason is a short, low-cardinality token (e.g. "decode_error",
// "unsupported_media_type", "body_too_large").
func RecordOTLPFailure(signal, transport, reason string) {
	OTLPRequestsFailed.WithLabelValues(signal, transport, reason).Inc()
}

// AddOTLPRecords adds n records to the per-signal record counter.
// Spans for traces, data points for metrics, log records for logs.
func AddOTLPRecords(signal string, n int) {
	if n <= 0 {
		return
	}
	OTLPRecordsReceived.WithLabelValues(signal).Add(float64(n))
}

// RecordOTLPSinkError counts one failure from a downstream OTLP sink.
// sink is a short, low-cardinality identifier (e.g. "hep_logs",
// "ducklake_traces", "ducklake_metrics").
func RecordOTLPSinkError(signal, sink string) {
	OTLPSinkErrors.WithLabelValues(signal, sink).Inc()
}

// RecordOTLPAsyncEnqueue records one async-queue enqueue attempt.
// outcome: "ok" | "queue_full" | "shutdown"
func RecordOTLPAsyncEnqueue(signal, outcome string) {
	OTLPAsyncEnqueue.WithLabelValues(signal, outcome).Inc()
}

// RecordOTLPAsyncWorkerError records one failed inner-sink write from the async worker.
func RecordOTLPAsyncWorkerError(signal string) {
	OTLPAsyncWorkerErrors.WithLabelValues(signal).Inc()
}

// RecordLineProtoRequest counts one Line Protocol HTTP request.
// endpoint: "v1" | "v2" | "v3" | "ping" | "health"
// outcome: "ok" | "parse_error" | "write_error" | "body_too_large" |
// "method_not_allowed" | "bad_request"
func RecordLineProtoRequest(endpoint, outcome string) {
	LineProtoRequestsReceived.WithLabelValues(endpoint, outcome).Inc()
}

// AddLineProtoPoints adds n successfully-ingested points to the per-db
// counter. db is the resolved logical database/bucket (or "default").
func AddLineProtoPoints(db string, n int) {
	if n <= 0 {
		return
	}
	if db == "" {
		db = "default"
	}
	LineProtoPointsIngested.WithLabelValues(db).Add(float64(n))
}

// RecordLineProtoWriteError counts one ingest-side failure. stage is a
// short identifier of the failing step ("ddl_create", "ddl_alter",
// "insert", …).
func RecordLineProtoWriteError(stage string) {
	LineProtoWriteErrors.WithLabelValues(stage).Inc()
}

