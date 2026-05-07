// Synthetic OTLP HTTP exporter: builds a rich Export*ServiceRequest for traces,
// metrics, and logs to exercise an OTLP receiver (e.g. homer-core on :4318).
package main

import (
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func main() {
	baseURL := flag.String("url", "http://127.0.0.1:4318", "OTLP HTTP base URL (no trailing slash)")
	format := flag.String("format", "json", "payload encoding: json | proto")
	dryRun := flag.Bool("dry-run", false, "marshal only, print payload sizes; do not send HTTP")
	flag.Parse()

	*baseURL = strings.TrimRight(strings.TrimSpace(*baseURL), "/")
	now := time.Now()
	nano := uint64(now.UnixNano())
	startNano := nano - uint64(50*time.Millisecond)

	// Spread OTLP metric data points across a window so ingested rows have
	// different timestamps + values (charts are not flat horizontal lines).
	const (
		metricPoints = 48
		metricStep   = 20 * time.Second // 48*20s ≈ 16 min span
	)
	stepNano := uint64(metricStep)
	firstMetricNano := nano - stepNano*uint64(metricPoints-1)

	traceID := mustHex("aabbccdd0011223344556677889900ff", 16)
	spanRoot := mustHex("0102030405060708", 8)
	spanChild := mustHex("0f0e0d0c0b0a0908", 8)
	linkTrace := mustHex("feedfacecafebabe0123456789abcdef", 16)
	linkSpan := mustHex("1122334455667788", 8)

	resource := &resourcepb.Resource{
		Attributes: []*commonpb.KeyValue{
			kvStr("service.name", "otlp-synthetic"),
			kvStr("service.version", "0.0.1"),
			kvStr("deployment.environment", "dev"),
			kvBool("synthetic", true),
			kvInt("pid", 4242),
			kvDouble("load", 0.42),
			kvBytes("blob", []byte{0xde, 0xad}),
			kvAny("nested", nestedAnyValue()),
		},
		DroppedAttributesCount: 0,
	}

	traces := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{
			{
				Resource:  resource,
				SchemaUrl: "https://opentelemetry.io/schemas/1.24.0",
				ScopeSpans: []*tracepb.ScopeSpans{
					{
						SchemaUrl: "https://example.com/trace-scope/1.0.0",
						Scope: &commonpb.InstrumentationScope{
							Name:                   "synthetic.tracer",
							Version:                "1.0.0",
							Attributes:             []*commonpb.KeyValue{kvStr("scope.attr", "x")},
							DroppedAttributesCount: 0,
						},
						Spans: []*tracepb.Span{
							{
								TraceId:           traceID,
								SpanId:            spanRoot,
								TraceState:        "k=v",
								ParentSpanId:      nil,
								Flags:             0x100, // has remote context bit area non-zero example
								Name:              "root.server",
								Kind:              tracepb.Span_SPAN_KIND_SERVER,
								StartTimeUnixNano: startNano,
								EndTimeUnixNano:   nano,
								Attributes: []*commonpb.KeyValue{
									kvStr("http.method", "POST"),
									kvStr("http.route", "/invoke"),
								},
								DroppedAttributesCount: 0,
								Events: []*tracepb.Span_Event{
									{
										TimeUnixNano:           startNano + 1e6,
										Name:                   "checkpoint",
										Attributes:             []*commonpb.KeyValue{kvStr("stage", "after-auth")},
										DroppedAttributesCount: 0,
									},
								},
								DroppedEventsCount: 0,
								Links: []*tracepb.Span_Link{
									{
										TraceId:                linkTrace,
										SpanId:                 linkSpan,
										TraceState:             "x=y",
										Attributes:             []*commonpb.KeyValue{kvStr("link.kind", "follows_from")},
										DroppedAttributesCount: 0,
										Flags:                  0,
									},
								},
								DroppedLinksCount: 0,
								Status: &tracepb.Status{
									Code:    tracepb.Status_STATUS_CODE_OK,
									Message: "ok",
								},
							},
							{
								TraceId:           traceID,
								SpanId:            spanChild,
								TraceState:        "",
								ParentSpanId:      spanRoot,
								Name:              "child.client",
								Kind:              tracepb.Span_SPAN_KIND_CLIENT,
								StartTimeUnixNano: startNano + 2e6,
								EndTimeUnixNano:   nano - 1e6,
								Attributes:        []*commonpb.KeyValue{kvStr("peer.service", "downstream")},
								Status: &tracepb.Status{
									Code:    tracepb.Status_STATUS_CODE_ERROR,
									Message: "timeout",
								},
							},
							{
								TraceId:           traceID,
								SpanId:            mustHex("9988776655443322", 8),
								ParentSpanId:      spanRoot,
								Name:              "internal",
								Kind:              tracepb.Span_SPAN_KIND_INTERNAL,
								StartTimeUnixNano: startNano,
								EndTimeUnixNano:   nano,
							},
							{
								TraceId:           traceID,
								SpanId:            mustHex("8877665544332211", 8),
								ParentSpanId:      spanRoot,
								Name:              "producer",
								Kind:              tracepb.Span_SPAN_KIND_PRODUCER,
								StartTimeUnixNano: startNano,
								EndTimeUnixNano:   nano,
							},
							{
								TraceId:           traceID,
								SpanId:            mustHex("7766554433221100", 8),
								ParentSpanId:      spanRoot,
								Name:              "consumer",
								Kind:              tracepb.Span_SPAN_KIND_CONSUMER,
								StartTimeUnixNano: startNano,
								EndTimeUnixNano:   nano,
							},
						},
					},
				},
			},
		},
	}

	gaugeDPs := make([]*metricspb.NumberDataPoint, metricPoints)
	sumIntDPs := make([]*metricspb.NumberDataPoint, metricPoints)
	sumDeltaDPs := make([]*metricspb.NumberDataPoint, metricPoints)
	histDPs := make([]*metricspb.HistogramDataPoint, metricPoints)
	expHistDPs := make([]*metricspb.ExponentialHistogramDataPoint, metricPoints)
	summaryDPs := make([]*metricspb.SummaryDataPoint, metricPoints)
	nf := float64(metricPoints)
	for i := 0; i < metricPoints; i++ {
		ts := firstMetricNano + uint64(i)*stepNano
		t0 := ts - stepNano
		angle := float64(i) * 2 * math.Pi / nf

		// Gauge: slow sine + harmonic ripple + drift — visibly non-linear on charts.
		gVal := 48.0 + 32.0*math.Sin(angle) + 6.0*math.Sin(4*angle) + float64(i)*0.35
		gaugeDPs[i] = &metricspb.NumberDataPoint{
			Attributes:        []*commonpb.KeyValue{kvStr("color", "blue")},
			StartTimeUnixNano: t0,
			TimeUnixNano:      ts,
			Value:             &metricspb.NumberDataPoint_AsDouble{AsDouble: gVal},
		}
		if i == metricPoints-1 {
			gaugeDPs[i].Exemplars = []*metricspb.Exemplar{
				{
					FilteredAttributes: []*commonpb.KeyValue{kvStr("ex", "1")},
					TimeUnixNano:       ts - 1000,
					Value:              &metricspb.Exemplar_AsDouble{AsDouble: gVal - 0.5},
					SpanId:             spanChild,
					TraceId:            traceID,
				},
			}
		}

		sumIntDPs[i] = &metricspb.NumberDataPoint{
			StartTimeUnixNano: t0,
			TimeUnixNano:      ts,
			Value:             &metricspb.NumberDataPoint_AsInt{AsInt: int64(500 + i*37 + (i%5)*11)},
		}

		deltaV := 0.08 + 0.65*math.Pow(0.5+0.5*math.Sin(angle+1.2), 2) + float64(i%9)*0.03
		sumDeltaDPs[i] = &metricspb.NumberDataPoint{
			StartTimeUnixNano: t0,
			TimeUnixNano:      ts,
			Value:             &metricspb.NumberDataPoint_AsDouble{AsDouble: deltaV},
		}

		sumHi := 8.0 + float64(i)*0.55 + 3.0*math.Sin(angle*2)
		cnt := uint64(5 + (i % 17))
		histDPs[i] = &metricspb.HistogramDataPoint{
			Attributes:        []*commonpb.KeyValue{kvStr("le", "ok")},
			StartTimeUnixNano: t0,
			TimeUnixNano:      ts,
			Count:             cnt,
			Sum:               ptrF64(sumHi),
			BucketCounts: []uint64{
				uint64(1 + i%3),
				uint64(2 + i%4),
				uint64(1 + i%5),
				uint64(i % 4),
				uint64(i % 3),
			},
			ExplicitBounds: []float64{1, 5, 10, 25},
			Min:            ptrF64(0.05 + float64(i%10)*0.02),
			Max:            ptrF64(5 + float64(i%20) + math.Abs(math.Sin(angle))*12),
		}
		if i == metricPoints-1 {
			histDPs[i].Exemplars = []*metricspb.Exemplar{
				{TimeUnixNano: ts - 500, Value: &metricspb.Exemplar_AsDouble{AsDouble: sumHi * 0.9}},
			}
		}

		expHistDPs[i] = &metricspb.ExponentialHistogramDataPoint{
			StartTimeUnixNano: t0,
			TimeUnixNano:      ts,
			Count:             uint64(3 + i%12),
			Sum:               ptrF64(12 + float64(i)*0.9 + 4*math.Sin(angle)),
			Scale:             3,
			ZeroCount:         uint64(i % 2),
			Positive: &metricspb.ExponentialHistogramDataPoint_Buckets{
				Offset:       int32(i % 3),
				BucketCounts: []uint64{uint64(1 + i%5), uint64(2 + i%6)},
			},
			Negative: &metricspb.ExponentialHistogramDataPoint_Buckets{
				Offset:       -1,
				BucketCounts: []uint64{uint64(i % 2)},
			},
			Min:           ptrF64(0.01 + float64(i%5)*0.01),
			Max:           ptrF64(2 + float64(i%15)),
			ZeroThreshold: 0.001,
		}

		summaryDPs[i] = &metricspb.SummaryDataPoint{
			StartTimeUnixNano: t0,
			TimeUnixNano:      ts,
			Count:             uint64(100 + i*17),
			Sum:               500 + float64(i)*123.4 + 50*math.Sin(angle),
			QuantileValues: []*metricspb.SummaryDataPoint_ValueAtQuantile{
				{Quantile: 0.5, Value: 10 + float64(i%50)},
				{Quantile: 0.99, Value: 80 + float64(i%200)},
			},
		}
	}

	metrics := &colmetricspb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricspb.ResourceMetrics{
			{
				Resource:  resource,
				SchemaUrl: "https://opentelemetry.io/schemas/1.24.0",
				ScopeMetrics: []*metricspb.ScopeMetrics{
					{
						SchemaUrl: "https://example.com/metrics-scope/1.0.0",
						Scope: &commonpb.InstrumentationScope{
							Name:    "synthetic.meter",
							Version: "1.0.0",
						},
						Metrics: []*metricspb.Metric{
							{
								Name:        "synthetic.gauge",
								Description: "gauge with double + exemplar",
								Unit:        "1",
								Metadata:    []*commonpb.KeyValue{kvStr("metric.meta", "gauge")},
								Data: &metricspb.Metric_Gauge{
									Gauge: &metricspb.Gauge{
										DataPoints: gaugeDPs,
									},
								},
							},
							{
								Name:        "synthetic.sum.int",
								Description: "sum cumulative monotonic int",
								Unit:        "requests",
								Data: &metricspb.Metric_Sum{
									Sum: &metricspb.Sum{
										AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
										IsMonotonic:            true,
										DataPoints: sumIntDPs,
									},
								},
							},
							{
								Name:        "synthetic.sum.delta",
								Description: "sum delta non-monotonic double",
								Unit:        "s",
								Data: &metricspb.Metric_Sum{
									Sum: &metricspb.Sum{
										AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
										IsMonotonic:            false,
										DataPoints: sumDeltaDPs,
									},
								},
							},
							{
								Name:        "synthetic.histogram",
								Description: "explicit bounds histogram",
								Unit:        "ms",
								Data: &metricspb.Metric_Histogram{
									Histogram: &metricspb.Histogram{
										AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE,
										DataPoints: histDPs,
									},
								},
							},
							{
								Name:        "synthetic.exp_histogram",
								Description: "exponential histogram",
								Unit:        "1",
								Data: &metricspb.Metric_ExponentialHistogram{
									ExponentialHistogram: &metricspb.ExponentialHistogram{
										AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
										DataPoints: expHistDPs,
									},
								},
							},
							{
								Name:        "synthetic.summary",
								Description: "legacy summary",
								Unit:        "1",
								Data: &metricspb.Metric_Summary{
									Summary: &metricspb.Summary{
										DataPoints: summaryDPs,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	logs := &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logspb.ResourceLogs{
			{
				Resource:  resource,
				SchemaUrl: "https://opentelemetry.io/schemas/1.24.0",
				ScopeLogs: []*logspb.ScopeLogs{
					{
						SchemaUrl: "https://example.com/logs-scope/1.0.0",
						Scope: &commonpb.InstrumentationScope{
							Name:    "synthetic.logger",
							Version: "1.0.0",
						},
						LogRecords: []*logspb.LogRecord{
							{
								TimeUnixNano:         nano,
								ObservedTimeUnixNano: nano,
								SeverityNumber:       logspb.SeverityNumber_SEVERITY_NUMBER_INFO,
								SeverityText:         "INFO",
								Body:                 strAny("plain text body"),
								Attributes:           []*commonpb.KeyValue{kvStr("log.attr", "a")},
								TraceId:              traceID,
								SpanId:               spanRoot,
								Flags:                0,
							},
							{
								TimeUnixNano:         nano,
								ObservedTimeUnixNano: nano + 1,
								SeverityNumber:       logspb.SeverityNumber_SEVERITY_NUMBER_ERROR,
								SeverityText:         "ERROR",
								Body:                 nestedAnyValue(),
								EventName:            "com.example.BusinessEvent",
							},
							{
								TimeUnixNano:         nano,
								ObservedTimeUnixNano: nano,
								SeverityNumber:       logspb.SeverityNumber_SEVERITY_NUMBER_DEBUG,
								SeverityText:         "DEBUG",
								Body: &commonpb.AnyValue{
									Value: &commonpb.AnyValue_ArrayValue{
										ArrayValue: &commonpb.ArrayValue{
											Values: []*commonpb.AnyValue{
												strAny("one"),
												{Value: &commonpb.AnyValue_IntValue{IntValue: 2}},
											},
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

	var marshal func(proto.Message) ([]byte, error)
	var contentType string
	switch strings.ToLower(*format) {
	case "json":
		marshal = func(m proto.Message) ([]byte, error) {
			return protojson.MarshalOptions{UseProtoNames: true}.Marshal(m)
		}
		contentType = "application/json"
	case "proto":
		marshal = proto.Marshal
		contentType = "application/x-protobuf"
	default:
		log.Fatalf("unknown -format=%q (expected json or proto)", *format)
	}

	payloads := []struct {
		name string
		path string
		msg  proto.Message
	}{
		{"traces", "/v1/traces", traces},
		{"metrics", "/v1/metrics", metrics},
		{"logs", "/v1/logs", logs},
	}

	for _, p := range payloads {
		body, err := marshal(p.msg)
		if err != nil {
			log.Fatalf("%s: marshal: %v", p.name, err)
		}
		if *dryRun {
			fmt.Fprintf(os.Stderr, "%s: %d bytes (%s)\n", p.name, len(body), *format)
			continue
		}
		url := *baseURL + p.path
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			log.Fatalf("%s: request: %v", p.name, err)
		}
		req.Header.Set("Content-Type", contentType)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Fatalf("%s: %v", p.name, err)
		}
		slurp, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			log.Fatalf("%s: HTTP %s: %s", p.name, resp.Status, strings.TrimSpace(string(slurp)))
		}
		fmt.Fprintf(os.Stderr, "%s: %s (%d bytes)\n", p.name, resp.Status, len(body))
	}
}

func mustHex(s string, want int) []byte {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != want {
		log.Fatalf("hex %q: want %d bytes, got %v", s, want, err)
	}
	return b
}

func ptrF64(v float64) *float64 { return &v }

func kvStr(k, v string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: strAny(v)}
}

func kvBool(k string, v bool) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: v}}}
}

func kvInt(k string, v int64) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: v}}}
}

func kvDouble(k string, v float64) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: v}}}
}

func kvBytes(k string, b []byte) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_BytesValue{BytesValue: b}}}
}

func kvAny(k string, av *commonpb.AnyValue) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: av}
}

func strAny(s string) *commonpb.AnyValue {
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: s}}
}

func nestedAnyValue() *commonpb.AnyValue {
	return &commonpb.AnyValue{
		Value: &commonpb.AnyValue_KvlistValue{
			KvlistValue: &commonpb.KeyValueList{
				Values: []*commonpb.KeyValue{
					kvStr("k", "v"),
					kvAny("arr", &commonpb.AnyValue{
						Value: &commonpb.AnyValue_ArrayValue{
							ArrayValue: &commonpb.ArrayValue{
								Values: []*commonpb.AnyValue{
									strAny("a"),
									{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: 2.5}},
								},
							},
						},
					}),
				},
			},
		},
	}
}
