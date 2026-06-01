// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// otlp_storage.go: dedicated OTLP traces / metrics / logs tables in DuckLake.
//
// All three OTLP signals land in purpose-built lake tables on the
// primary shard (otlp_traces, otlp_metrics, otlp_logs). The schemas
// are intentionally minimal: the most queryable fields are first-class
// columns, and the rest of the OTLP envelope (instrumentation scope,
// resource attrs, span events/links, metric exemplars, log body
// structured value, …) is preserved as a JSON blob so we don't lose
// fidelity until a richer mapping is designed.
//
// HEP-side tables are deliberately untouched — OTLP is its own world
// and forcing logs into the HEP type 100 schema would lose attributes,
// trace_id/span_id linkage, and severity semantics.
package ducklake

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"github.com/sipcapture/homer-core/src/utils/metrics"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// otlpTracesTableSQL / otlpMetricsTableSQL are the schemas applied to
// DuckLake on first use. Kept in package scope so EnsureOTLPSchema and
// the test helper share the same source of truth.
const (
	otlpTracesTableSQL = `
		date           DATE,
		timestamp      TIMESTAMP,
		end_timestamp  TIMESTAMP,
		duration_ns    BIGINT,
		trace_id       VARCHAR,
		span_id        VARCHAR,
		parent_span_id VARCHAR,
		name           VARCHAR,
		kind           INTEGER,
		status_code    INTEGER,
		status_message VARCHAR,
		service_name   VARCHAR,
		scope_name     VARCHAR,
		scope_version  VARCHAR,
		resource_attrs JSON,
		span_attrs     JSON,
		events_count   INTEGER,
		links_count    INTEGER,
		raw            JSON
	`
	otlpMetricsTableSQL = `
		date           DATE,
		timestamp      TIMESTAMP,
		name           VARCHAR,
		description    VARCHAR,
		unit           VARCHAR,
		type           VARCHAR,
		value_double   DOUBLE,
		value_int      BIGINT,
		service_name   VARCHAR,
		scope_name     VARCHAR,
		scope_version  VARCHAR,
		attributes     JSON,
		resource_attrs JSON,
		raw            JSON
	`
	otlpLogsTableSQL = `
		date              DATE,
		timestamp         TIMESTAMP,
		observed_timestamp TIMESTAMP,
		severity_number   INTEGER,
		severity_text     VARCHAR,
		body              VARCHAR,
		body_json         JSON,
		trace_id          VARCHAR,
		span_id           VARCHAR,
		flags             INTEGER,
		service_name      VARCHAR,
		scope_name        VARCHAR,
		scope_version     VARCHAR,
		attributes        JSON,
		resource_attrs    JSON,
		raw               JSON
	`
)

// OTLPStorage owns the otlp_traces / otlp_metrics tables on the primary
// shard. EnsureOTLPSchema is idempotent and cheap to call repeatedly,
// but the typical lifecycle is "create once at module Start, write
// many".
type OTLPStorage struct {
	db       *sql.DB
	lakeName string

	// once protects EnsureOTLPSchema so the receiver Start path can
	// call it from multiple goroutines (gRPC + HTTP) without racing
	// to issue duplicate DDL.
	once   sync.Once
	ddlErr error
}

// OTLPStorage returns the OTLP-table accessor wired against the
// primary shard. Returns nil when the manager has no shards (test
// stubs), which the receiver treats as "DuckLake sink disabled".
func (m *Manager) OTLPStorage() *OTLPStorage {
	if m == nil || m.sharded == nil {
		return nil
	}
	primary := m.sharded.Primary()
	if primary == nil {
		return nil
	}
	return &OTLPStorage{
		db:       primary.GetDB(),
		lakeName: primary.GetLakeName(),
	}
}

// EnsureOTLPSchema creates otlp_traces / otlp_metrics inside the
// configured DuckLake catalog if they do not already exist. Safe to
// call concurrently — the first call wins and subsequent ones return
// the cached error (or nil).
func (s *OTLPStorage) EnsureOTLPSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("otlp storage: nil database handle")
	}
	s.once.Do(func() {
		tables := []struct {
			name      string
			createSQL string
		}{
			{"otlp_traces", otlpTracesTableSQL},
			{"otlp_metrics", otlpMetricsTableSQL},
			{"otlp_logs", otlpLogsTableSQL},
		}
		for _, t := range tables {
			fqn := fmt.Sprintf("%s.%s", s.lakeName, t.name)
			// Check existence BEFORE create: SET PARTITIONED BY / SET SORTED BY
			// bump schema_version on every run, and each bump leaks another
			// ducklake_inlined_data_* table (upstream duckdb/ducklake#1065).
			// Only configure freshly created tables, not on every restart.
			alreadyExisted := duckLakeTableExists(s.db, s.lakeName, t.name)

			if _, err := s.db.ExecContext(ctx,
				fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s);", fqn, t.createSQL)); err != nil {
				s.ddlErr = fmt.Errorf("otlp storage: %w", err)
				return
			}
			if alreadyExisted {
				continue
			}
			// Best-effort: time-partition + sort the OTLP tables the same way
			// HEP tables are. Failures are logged, not fatal — older DuckLake
			// builds without ALTER...PARTITIONED BY keep a single partition.
			if _, err := s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s SET PARTITIONED BY (date);", fqn)); err != nil {
				logger.Warn(fmt.Sprintf("otlp storage: failed to set partitioning for %s: %v", fqn, err))
			}
			if _, err := s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s SET SORTED BY (timestamp ASC);", fqn)); err != nil {
				logger.Warn(fmt.Sprintf("otlp storage: failed to set sort order for %s: %v", fqn, err))
			}
		}
		logger.Info("OTLP storage schema ready", "lake", s.lakeName, "tables", "otlp_traces, otlp_metrics, otlp_logs")
	})
	return s.ddlErr
}

// WriteOTLPTraces flattens an OTLP Resource/Scope/Span tree into
// otlp_traces rows and inserts them in a single multi-row VALUES
// statement. Returns the number of spans written and the first
// non-fatal error encountered.
func (s *OTLPStorage) WriteOTLPTraces(ctx context.Context, resourceSpans []*tracepb.ResourceSpans) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("otlp storage: nil database handle")
	}
	if len(resourceSpans) == 0 {
		return 0, nil
	}

	const colsPerRow = 19
	rows := make([]string, 0, 64)
	args := make([]any, 0, 64*colsPerRow)
	written := 0

	for _, rs := range resourceSpans {
		resAttrs := otlpAttrsToMap(rs.GetResource().GetAttributes())
		serviceName := stringFromAttrMap(resAttrs, "service.name")
		resAttrsJSON := mustJSON(resAttrs)
		rawResource := mustJSON(rs.GetResource())

		for _, ss := range rs.GetScopeSpans() {
			scope := ss.GetScope()
			scopeName := scope.GetName()
			scopeVer := scope.GetVersion()

			for _, span := range ss.GetSpans() {
				spanAttrs := otlpAttrsToMap(span.GetAttributes())
				spanAttrsJSON := mustJSON(spanAttrs)

				rawSpan := map[string]any{
					"resource":              json.RawMessage(rawResource),
					"scope":                 json.RawMessage(mustJSON(scope)),
					"span":                  json.RawMessage(mustJSON(span)),
					"resource_schema_url":   rs.GetSchemaUrl(),
					"scope_spans_schema":    ss.GetSchemaUrl(),
				}
				rawJSON := mustJSON(rawSpan)

				start := time.Unix(0, int64(span.GetStartTimeUnixNano())).UTC()
				end := time.Unix(0, int64(span.GetEndTimeUnixNano())).UTC()
				duration := int64(span.GetEndTimeUnixNano() - span.GetStartTimeUnixNano())

				rows = append(rows, "(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)")
				args = append(args,
					start.Format("2006-01-02"), // date
					start,                      // timestamp (start)
					end,                        // end_timestamp
					duration,                   // duration_ns
					hex.EncodeToString(span.GetTraceId()),
					hex.EncodeToString(span.GetSpanId()),
					hex.EncodeToString(span.GetParentSpanId()),
					span.GetName(),
					int32(span.GetKind()),
					int32(span.GetStatus().GetCode()),
					span.GetStatus().GetMessage(),
					serviceName,
					scopeName,
					scopeVer,
					resAttrsJSON,
					spanAttrsJSON,
					int32(len(span.GetEvents())),
					int32(len(span.GetLinks())),
					rawJSON,
				)
				written++
			}
		}
	}

	if written == 0 {
		return 0, nil
	}

	q := fmt.Sprintf("INSERT INTO %s.otlp_traces VALUES %s;", s.lakeName, strings.Join(rows, ","))
	if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
		metrics.RecordOTLPSinkError("traces", "ducklake_traces")
		return 0, fmt.Errorf("otlp storage: insert traces: %w", err)
	}
	return written, nil
}

// WriteOTLPMetrics flattens an OTLP Resource/Scope/Metric tree into
// otlp_metrics rows. Sums and Gauges produce one row per data point;
// Histograms / ExponentialHistograms / Summaries produce a single
// "summary" row per metric carrying counts/sums (the per-bucket data
// is preserved in the raw JSON).
func (s *OTLPStorage) WriteOTLPMetrics(ctx context.Context, resourceMetrics []*metricpb.ResourceMetrics) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("otlp storage: nil database handle")
	}
	if len(resourceMetrics) == 0 {
		return 0, nil
	}

	const colsPerRow = 14
	rows := make([]string, 0, 128)
	args := make([]any, 0, 128*colsPerRow)
	written := 0

	for _, rm := range resourceMetrics {
		resAttrs := otlpAttrsToMap(rm.GetResource().GetAttributes())
		serviceName := stringFromAttrMap(resAttrs, "service.name")
		resAttrsJSON := mustJSON(resAttrs)

		for _, sm := range rm.GetScopeMetrics() {
			scope := sm.GetScope()
			scopeName := scope.GetName()
			scopeVer := scope.GetVersion()

			for _, m := range sm.GetMetrics() {
				name := m.GetName()
				desc := m.GetDescription()
				unit := m.GetUnit()
				rawMetricBytes := mustJSON(m)

				appendDP := func(ts time.Time, kind string, vDouble float64, vInt int64, attrs []*commonpb.KeyValue) {
					attrMap := otlpAttrsToMap(attrs)
					attrsJSON := mustJSON(attrMap)
					rows = append(rows, "(?,?,?,?,?,?,?,?,?,?,?,?,?,?)")
					args = append(args,
						ts.Format("2006-01-02"),
						ts,
						name,
						desc,
						unit,
						kind,
						vDouble,
						vInt,
						serviceName,
						scopeName,
						scopeVer,
						attrsJSON,
						resAttrsJSON,
						rawMetricBytes,
					)
					written++
				}

				switch d := m.GetData().(type) {
				case *metricpb.Metric_Gauge:
					for _, dp := range d.Gauge.GetDataPoints() {
						ts := time.Unix(0, int64(dp.GetTimeUnixNano())).UTC()
						vd, vi := numberDPValue(dp)
						appendDP(ts, "gauge", vd, vi, dp.GetAttributes())
					}
				case *metricpb.Metric_Sum:
					for _, dp := range d.Sum.GetDataPoints() {
						ts := time.Unix(0, int64(dp.GetTimeUnixNano())).UTC()
						vd, vi := numberDPValue(dp)
						appendDP(ts, "sum", vd, vi, dp.GetAttributes())
					}
				case *metricpb.Metric_Histogram:
					for _, dp := range d.Histogram.GetDataPoints() {
						ts := time.Unix(0, int64(dp.GetTimeUnixNano())).UTC()
						appendDP(ts, "histogram", dp.GetSum(), int64(dp.GetCount()), dp.GetAttributes())
					}
				case *metricpb.Metric_ExponentialHistogram:
					for _, dp := range d.ExponentialHistogram.GetDataPoints() {
						ts := time.Unix(0, int64(dp.GetTimeUnixNano())).UTC()
						appendDP(ts, "exponential_histogram", dp.GetSum(), int64(dp.GetCount()), dp.GetAttributes())
					}
				case *metricpb.Metric_Summary:
					for _, dp := range d.Summary.GetDataPoints() {
						ts := time.Unix(0, int64(dp.GetTimeUnixNano())).UTC()
						appendDP(ts, "summary", dp.GetSum(), int64(dp.GetCount()), dp.GetAttributes())
					}
				default:
					// Unknown / empty payload — skip but still log
					// once so operators see it during onboarding.
					logger.Debug("otlp storage: skipping metric with unsupported data type", "metric", name)
				}
			}
		}
	}

	if written == 0 {
		return 0, nil
	}

	q := fmt.Sprintf("INSERT INTO %s.otlp_metrics VALUES %s;", s.lakeName, strings.Join(rows, ","))
	if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
		metrics.RecordOTLPSinkError("metrics", "ducklake_metrics")
		return 0, fmt.Errorf("otlp storage: insert metrics: %w", err)
	}
	return written, nil
}

// WriteOTLPLogs flattens an OTLP Resource/Scope/Log tree into
// otlp_logs rows. Each LogRecord becomes one row; bodies that are
// strings land in `body`, structured bodies (kvlist/array/map) are
// serialised into `body_json` and `body` is left empty.
func (s *OTLPStorage) WriteOTLPLogs(ctx context.Context, resourceLogs []*logspb.ResourceLogs) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("otlp storage: nil database handle")
	}
	if len(resourceLogs) == 0 {
		return 0, nil
	}

	const colsPerRow = 16
	rows := make([]string, 0, 128)
	args := make([]any, 0, 128*colsPerRow)
	written := 0

	for _, rl := range resourceLogs {
		resAttrs := otlpAttrsToMap(rl.GetResource().GetAttributes())
		serviceName := stringFromAttrMap(resAttrs, "service.name")
		resAttrsJSON := mustJSON(resAttrs)
		rawResource := mustJSON(rl.GetResource())

		for _, sl := range rl.GetScopeLogs() {
			scope := sl.GetScope()
			scopeName := scope.GetName()
			scopeVer := scope.GetVersion()

			for _, lr := range sl.GetLogRecords() {
				attrs := otlpAttrsToMap(lr.GetAttributes())
				attrsJSON := mustJSON(attrs)

				var bodyText string
				var bodyJSON string
				switch v := lr.GetBody().GetValue().(type) {
				case *commonpb.AnyValue_StringValue:
					bodyText = v.StringValue
				case nil:
					// no body
				default:
					bodyJSON = mustJSON(anyValueToGo(lr.GetBody()))
				}
				if bodyJSON == "" {
					bodyJSON = "null"
				}

				ts := time.Unix(0, int64(lr.GetTimeUnixNano())).UTC()
				if lr.GetTimeUnixNano() == 0 {
					ts = time.Unix(0, int64(lr.GetObservedTimeUnixNano())).UTC()
				}
				obs := time.Unix(0, int64(lr.GetObservedTimeUnixNano())).UTC()

				rawRecord := map[string]any{
					"resource":            json.RawMessage(rawResource),
					"scope":               json.RawMessage(mustJSON(scope)),
					"log_record":          json.RawMessage(mustJSON(lr)),
					"resource_schema_url": rl.GetSchemaUrl(),
					"scope_logs_schema":   sl.GetSchemaUrl(),
				}
				rawJSON := mustJSON(rawRecord)

				rows = append(rows, "(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)")
				args = append(args,
					ts.Format("2006-01-02"),
					ts,
					obs,
					int32(lr.GetSeverityNumber()),
					lr.GetSeverityText(),
					bodyText,
					bodyJSON,
					hex.EncodeToString(lr.GetTraceId()),
					hex.EncodeToString(lr.GetSpanId()),
					int32(lr.GetFlags()),
					serviceName,
					scopeName,
					scopeVer,
					attrsJSON,
					resAttrsJSON,
					rawJSON,
				)
				written++
			}
		}
	}

	if written == 0 {
		return 0, nil
	}

	q := fmt.Sprintf("INSERT INTO %s.otlp_logs VALUES %s;", s.lakeName, strings.Join(rows, ","))
	if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
		metrics.RecordOTLPSinkError("logs", "ducklake_logs")
		return 0, fmt.Errorf("otlp storage: insert logs: %w", err)
	}
	return written, nil
}

// numberDPValue extracts the typed value from a NumberDataPoint. OTLP
// allows either a double or an int; we surface both columns and let
// the consumer pick. Missing value defaults to 0/0.
func numberDPValue(dp *metricpb.NumberDataPoint) (float64, int64) {
	if dp == nil {
		return 0, 0
	}
	switch v := dp.GetValue().(type) {
	case *metricpb.NumberDataPoint_AsDouble:
		return v.AsDouble, 0
	case *metricpb.NumberDataPoint_AsInt:
		return float64(v.AsInt), v.AsInt
	}
	return 0, 0
}

// otlpAttrsToMap flattens an OTLP attribute slice into a string-keyed
// map of native Go values for JSON encoding. Nested arrays / kvlists
// are walked recursively.
func otlpAttrsToMap(attrs []*commonpb.KeyValue) map[string]any {
	if len(attrs) == 0 {
		return nil
	}
	out := make(map[string]any, len(attrs))
	for _, kv := range attrs {
		if kv == nil {
			continue
		}
		out[kv.GetKey()] = anyValueToGo(kv.GetValue())
	}
	return out
}

// anyValueToGo unwraps an OTLP AnyValue into a JSON-friendly Go value.
func anyValueToGo(v *commonpb.AnyValue) any {
	if v == nil {
		return nil
	}
	switch x := v.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return x.StringValue
	case *commonpb.AnyValue_BoolValue:
		return x.BoolValue
	case *commonpb.AnyValue_IntValue:
		return x.IntValue
	case *commonpb.AnyValue_DoubleValue:
		return x.DoubleValue
	case *commonpb.AnyValue_ArrayValue:
		vals := x.ArrayValue.GetValues()
		out := make([]any, 0, len(vals))
		for _, vv := range vals {
			out = append(out, anyValueToGo(vv))
		}
		return out
	case *commonpb.AnyValue_KvlistValue:
		return otlpAttrsToMap(x.KvlistValue.GetValues())
	case *commonpb.AnyValue_BytesValue:
		return x.BytesValue
	}
	return nil
}

// stringFromAttrMap is a small helper for surfacing standard OTel
// resource attributes (service.name, host.name, …) as first-class
// columns without re-decoding the JSON blob.
func stringFromAttrMap(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// mustJSON encodes v with encoding/json. Returns "null" on failure so
// the caller can pass the result straight to a JSON column without
// branching on errors (the metric attribute encoder cannot fail in
// practice — it is map[string]any of primitives).
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

// resourceAttrsHash returns a stable digest over an OTLP Resource. Not
// used in the hot path today; kept here for future correlation work
// (e.g. tagging traces with a synthetic resource_id once we want to
// join them against external catalogs).
func resourceAttrsHash(r *resourcepb.Resource) string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("%x", mustJSON(otlpAttrsToMap(r.GetAttributes())))
}
