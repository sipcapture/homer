// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package config provides unified configuration for Homer Server modules.
// Homer Server can run in three modes (any combination):
//   - Writer: Receives HEP packets and writes to DuckLake
//   - Node: Serves data from DuckLake via FlightSQL
//   - Coordinator: API gateway that queries nodes and serves JSON to UI
package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/mcuadros/go-defaults"
	"github.com/sipcapture/homer-core/src/passwordhash"
	"github.com/spf13/viper"
)

// HashPassword returns a bcrypt hash for a new password.
func HashPassword(password string) (string, error) {
	return passwordhash.Hash(password)
}

// Config is the main configuration structure for Homer Server
type Config struct {
	Ingest      IngestConfig      `json:"ingest" mapstructure:"ingest"`
	Storage     StorageConfig     `json:"storage" mapstructure:"storage"`
	Node        NodeConfig        `json:"node" mapstructure:"node"`
	Coordinator CoordinatorConfig `json:"coordinator" mapstructure:"coordinator"`
	MCP         MCPConfig         `json:"mcp" mapstructure:"mcp"`

	// Global settings
	Log           LogConfig        `json:"log" mapstructure:"log"`
	Prometheus    PrometheusConfig `json:"prometheus" mapstructure:"prometheus"`
	RemoteLogging RemoteLogConfig  `json:"remote_logging" mapstructure:"remote_logging"`
}

// RemoteLogConfig configures remote logging backends
type RemoteLogConfig struct {
	Loki          LokiLogConfig          `json:"loki" mapstructure:"loki"`
	Elasticsearch ElasticsearchLogConfig `json:"elasticsearch" mapstructure:"elasticsearch"`
	LineProto     LineProtoLogConfig     `json:"lineproto" mapstructure:"lineproto"`
}

// LokiLogConfig configures Grafana Loki logging
type LokiLogConfig struct {
	Enable       bool   `json:"enable" mapstructure:"enable" default:"false"`
	URL          string `json:"url" mapstructure:"url" default:"http://localhost:3100/loki/api/v1/push"`
	Bulk         int    `json:"bulk" mapstructure:"bulk" default:"400"`
	Timer        int    `json:"timer" mapstructure:"timer" default:"4"`
	Buffer       int    `json:"buffer" mapstructure:"buffer" default:"100000"`
	HEPFilter    []int  `json:"hep_filter" mapstructure:"hep_filter"`
	IPPortLabels bool   `json:"ip_port_labels" mapstructure:"ip_port_labels" default:"false"`
	// LokiCustomLabels: keys allowed for Lua SetLokiLabel (empty = custom labels disabled).
	LokiCustomLabels []string `json:"loki_custom_labels" mapstructure:"loki_custom_labels"`
}

// ElasticsearchLogConfig configures Elasticsearch logging
type ElasticsearchLogConfig struct {
	Enable     bool   `json:"enable" mapstructure:"enable" default:"false"`
	Addr       string `json:"addr" mapstructure:"addr" default:"http://localhost:9200"`
	User       string `json:"user" mapstructure:"user"`
	Pass       string `json:"pass" mapstructure:"pass"`
	Discovery  bool   `json:"discovery" mapstructure:"discovery" default:"true"`
	IndexDaily bool   `json:"index_daily" mapstructure:"index_daily" default:"true"`
	IndexName  string `json:"index_name" mapstructure:"index_name" default:"hep"`
	HEPFilter  []int  `json:"hep_filter" mapstructure:"hep_filter"`
}

// LineProtoLogConfig configures InfluxDB Line Protocol logging
type LineProtoLogConfig struct {
	Enable    bool   `json:"enable" mapstructure:"enable" default:"false"`
	URL       string `json:"url" mapstructure:"url" default:"http://localhost:8086/write?db=hep"`
	Bulk      int    `json:"bulk" mapstructure:"bulk" default:"400"`
	Timer     int    `json:"timer" mapstructure:"timer" default:"4"`
	Buffer    int    `json:"buffer" mapstructure:"buffer" default:"100000"`
	HEPFilter []int  `json:"hep_filter" mapstructure:"hep_filter"`
}

// IngestConfig configures the HEP ingest module (receiving and parsing)
type IngestConfig struct {
	Enable bool `json:"enable" mapstructure:"enable" default:"true"`

	// Ingest load control (0 = auto-detect NumCPU/2, capped at 4)
	WorkerCount int `json:"worker_count" mapstructure:"worker_count" default:"0"`
	QueueSize   int `json:"queue_size" mapstructure:"queue_size" default:"200000"`
	// WorkerMetricsFlushPackets controls how many packets each writer worker
	// handles before flushing batched Prometheus counters (received/processed/bytes).
	// Larger values reduce metric-update overhead at the cost of coarser time series.
	// 0 means use the built-in default (128).
	WorkerMetricsFlushPackets int `json:"worker_metrics_flush_packets" mapstructure:"worker_metrics_flush_packets" default:"0"`

	// HEP receivers
	UDP   UDPServerConfig   `json:"udp" mapstructure:"udp"`
	TCP   TCPServerConfig   `json:"tcp" mapstructure:"tcp"`
	TLS   TLSServerConfig   `json:"tls" mapstructure:"tls"`
	HTTP  HTTPServerConfig  `json:"http" mapstructure:"http"`
	HTTPS HTTPSServerConfig `json:"https" mapstructure:"https"`

	// HEP processing settings
	HEP HEPConfig `json:"hep" mapstructure:"hep"`
	SIP SIPConfig `json:"sip" mapstructure:"sip"`

	// Scripting engine
	Scripting ScriptingConfig `json:"scripting" mapstructure:"scripting"`

	// HepStream configures the in-process broker that fans decoded HEP
	// events out over /stream (on the node) and, upstream of that, to
	// /api/v4/stream/hep (on the coordinator). Disabled by default —
	// the publisher-side worker stays a no-op when Enable is false.
	HepStream IngestHepStreamConfig `json:"hep_stream" mapstructure:"hep_stream"`

	// OTLP configures the OpenTelemetry receiver (gRPC + HTTP/protobuf
	// + HTTP/JSON) for traces, metrics and logs. Disabled by default;
	// when enabled, logs land in the existing HEP pipeline as proto
	// type 100 and traces/metrics go into dedicated DuckLake tables.
	OTLP OTLPConfig `json:"otlp" mapstructure:"otlp"`

	// LineProto configures the InfluxDB Line Protocol HTTP receiver.
	// Each measurement materialises into its own custom DuckLake table
	// (created lazily and extended via ALTER TABLE ADD COLUMN as new
	// fields appear). Disabled by default — operators opt in by
	// setting ingest.line_protocol.enable=true.
	LineProto LineProtoConfig `json:"line_protocol" mapstructure:"line_protocol"`

	// Siprec configures the in-process SIPREC SRS (signaling-only).
	Siprec SiprecConfig `json:"siprec" mapstructure:"siprec"`

	// Vqrtcp configures the dedicated SIP listener for VQ-RTCPXR QoS reports.
	Vqrtcp VqrtcpConfig `json:"vqrtcp" mapstructure:"vqrtcp"`

	// Force HEP payload storage for specific types (e.g., [34, 35, 38])
	ForceHEPPayload []int `json:"force_hep_payload" mapstructure:"force_hep_payload"`
}

// SiprecConfig configures the Homer SIPREC signaling server (RFC 7865/7866).
type SiprecConfig struct {
	Enable        bool     `json:"enable" mapstructure:"enable" default:"false"`
	BindIP        string   `json:"bind_ip" mapstructure:"bind_ip" default:"0.0.0.0"`
	AdvertiseIP   string   `json:"advertise_ip" mapstructure:"advertise_ip" default:""`
	SIPPort       int      `json:"sip_port" mapstructure:"sip_port" default:"5062"`
	Transports    []string `json:"transports" mapstructure:"transports"`
	RequireSiprec bool     `json:"require_siprec" mapstructure:"require_siprec" default:"true"`
	UserAgent     string   `json:"user_agent" mapstructure:"user_agent" default:"homer-siprec-srs"`
	NodeID        string   `json:"node_id" mapstructure:"node_id" default:"homer"`
}

// VqrtcpConfig configures the VQ-RTCPXR SIP QoS report receiver.
type VqrtcpConfig struct {
	Enable          bool     `json:"enable" mapstructure:"enable" default:"false"`
	BindIP          string   `json:"bind_ip" mapstructure:"bind_ip" default:"0.0.0.0"`
	SIPPort         int      `json:"sip_port" mapstructure:"sip_port" default:"5063"`
	Transports      []string `json:"transports" mapstructure:"transports"`
	Methods         []string `json:"methods" mapstructure:"methods"`
	Reply200        bool     `json:"reply_200" mapstructure:"reply_200" default:"true"`
	AsyncQueueDepth int      `json:"async_queue_depth" mapstructure:"async_queue_depth" default:"1024"`
	NodeID          string   `json:"node_id" mapstructure:"node_id" default:"homer"`
}

// LineProtoConfig configures the InfluxDB Line Protocol HTTP receiver.
// The receiver speaks the InfluxDB v1 (/write) and v2 (/api/v2/write)
// HTTP write endpoints, plus a gigapi-compatible alias
// (/api/v3/write_lp). Each measurement maps to a dynamically-created
// DuckLake table named `<table_prefix><measurement>` (default prefix is
// empty so the table matches the measurement; set `table_prefix` e.g. to
// `lp_` to namespace LP tables). An optional ?db= query parameter routes
// points into a per-database DuckDB schema.
type LineProtoConfig struct {
	// Enable toggles the receiver and its HTTP listener.
	Enable bool `json:"enable" mapstructure:"enable" default:"false"`

	// Listen is the HTTP bind address (default :8086 — InfluxDB v1's
	// canonical port; safe to change when colocating with another
	// InfluxDB-compatible service).
	Listen string `json:"listen" mapstructure:"listen" default:":8086"`

	// MaxBodyBytes caps the inbound request body size. Default 8 MiB
	// matches InfluxDB v2's `max-body-size`.
	MaxBodyBytes int64 `json:"max_body_bytes" mapstructure:"max_body_bytes" default:"8388608"`

	// DefaultPrecision is used when the client doesn't pass ?precision=
	// (or sends an unknown value). Valid: "ns" / "us" / "ms" / "s".
	DefaultPrecision string `json:"default_precision" mapstructure:"default_precision" default:"ns"`

	// DefaultDB is used when the client doesn't pass ?db=/?bucket=.
	// Empty means "land in the default schema".
	DefaultDB string `json:"default_db" mapstructure:"default_db" default:""`

	// TablePrefix is prepended to every measurement before it becomes
	// a DuckLake table name. Default is empty (measurement = table name);
	// set e.g. `lp_` to keep line-protocol tables separate from HEP.
	TablePrefix string `json:"table_prefix" mapstructure:"table_prefix" default:""`

	// AllowHepSipCall enables structured Line Protocol ingest into real HEP
	// DuckLake tables (measurement must equal the table name, e.g.
	// hep_proto_1_call, hep_proto_1_registration, hep_proto_5_default, …).
	// When false, measurements starting with hep_proto_ are rejected if
	// table_prefix is empty (see docs/LINE_PROTOCOL.md).
	AllowHepSipCall bool `json:"allow_hep_sip_call" mapstructure:"allow_hep_sip_call" default:"false"`

	// ReadTimeoutSec / WriteTimeoutSec apply to the HTTP server.
	ReadTimeoutSec  int `json:"read_timeout_sec" mapstructure:"read_timeout_sec" default:"30"`
	WriteTimeoutSec int `json:"write_timeout_sec" mapstructure:"write_timeout_sec" default:"30"`

	// TLS material; empty Cert/Key disables TLS.
	Cert   string `json:"cert" mapstructure:"cert" default:""`
	Key    string `json:"key" mapstructure:"key" default:""`
	CaCert string `json:"cacert" mapstructure:"cacert" default:""`
}

// OTLPConfig configures the OpenTelemetry Protocol receiver. The
// receiver exposes the canonical OTLP/gRPC port (4317) and OTLP/HTTP
// port (4318) and accepts protobuf and JSON bodies.
type OTLPConfig struct {
	Enable bool `json:"enable" mapstructure:"enable" default:"false"`

	// AsyncEnable, when true, wraps the DuckLake sink in a bounded in-process
	// queue: Export handlers return after enqueue (clone + channel send).
	// The worker persists asynchronously; on queue full the client gets an
	// error and may retry. Default true; set false for synchronous writes per request.
	AsyncEnable bool `json:"async_enable" mapstructure:"async_enable" default:"true"`
	// AsyncQueueDepth is the maximum number of pending OTLP export batches
	// (across traces, metrics, logs sharing one channel). Default 512.
	AsyncQueueDepth int `json:"async_queue_depth" mapstructure:"async_queue_depth" default:"512"`
	// AsyncEnqueueTimeoutMs is how long Push* waits for a free queue slot.
	// 0 means non-blocking: if the queue is full, return immediately with an error.
	AsyncEnqueueTimeoutMs int `json:"async_enqueue_timeout_ms" mapstructure:"async_enqueue_timeout_ms" default:"0"`

	// MaxRecvMsgBytes caps both the gRPC inbound message size and the
	// HTTP request body size. Default 4 MiB matches OTel SDK defaults.
	MaxRecvMsgBytes int `json:"max_recv_msg_bytes" mapstructure:"max_recv_msg_bytes" default:"4194304"`

	GRPC  OTLPGRPCConfig  `json:"grpc" mapstructure:"grpc"`
	HTTP  OTLPHTTPConfig  `json:"http" mapstructure:"http"`
	Sinks OTLPSinksConfig `json:"sinks" mapstructure:"sinks"`
}

// OTLPGRPCConfig configures the OTLP gRPC listener.
type OTLPGRPCConfig struct {
	Enable bool   `json:"enable" mapstructure:"enable" default:"true"`
	Listen string `json:"listen" mapstructure:"listen" default:":4317"`
	// TLS material; empty Cert/Key disables TLS.
	Cert   string `json:"cert" mapstructure:"cert" default:""`
	Key    string `json:"key" mapstructure:"key" default:""`
	CaCert string `json:"cacert" mapstructure:"cacert" default:""`
}

// OTLPHTTPConfig configures the OTLP HTTP listener.
// Routes are fixed: /v1/traces, /v1/metrics, /v1/logs.
type OTLPHTTPConfig struct {
	Enable bool   `json:"enable" mapstructure:"enable" default:"true"`
	Listen string `json:"listen" mapstructure:"listen" default:":4318"`
	// TLS material; empty Cert/Key disables TLS.
	Cert   string `json:"cert" mapstructure:"cert" default:""`
	Key    string `json:"key" mapstructure:"key" default:""`
	CaCert string `json:"cacert" mapstructure:"cacert" default:""`
	// ReadTimeoutSec / WriteTimeoutSec apply to the HTTP server.
	ReadTimeoutSec  int `json:"read_timeout_sec" mapstructure:"read_timeout_sec" default:"30"`
	WriteTimeoutSec int `json:"write_timeout_sec" mapstructure:"write_timeout_sec" default:"30"`
}

// OTLPSinksConfig governs which OTLP signals are persisted. All three
// land in dedicated DuckLake tables (otlp_traces / otlp_metrics /
// otlp_logs) on the primary shard — the HEP pipeline is intentionally
// not on the OTLP path so attributes, severity, and trace_id linkage
// are preserved at full fidelity.
type OTLPSinksConfig struct {
	// StoreTraces materialises spans into the otlp_traces DuckLake table.
	StoreTraces bool `json:"store_traces" mapstructure:"store_traces" default:"true"`
	// StoreMetrics materialises metric points into the otlp_metrics DuckLake table.
	StoreMetrics bool `json:"store_metrics" mapstructure:"store_metrics" default:"true"`
	// StoreLogs materialises log records into the otlp_logs DuckLake table.
	StoreLogs bool `json:"store_logs" mapstructure:"store_logs" default:"true"`
}

// IngestHepStreamConfig configures the publisher side of the live HEP
// broker (see src/stream/hepstream). Kept separate from the
// coordinator-side config because a box can run ingest without a
// coordinator (and vice versa).
type IngestHepStreamConfig struct {
	// Enable spins up an in-process hepstream.Broker; HEPInput.worker
	// publishes decoded events to it on its hot path. Zero-cost when
	// false (Broker stays nil, Publish short-circuits). Defaults to
	// true because the broker has negligible overhead without
	// subscribers (non-blocking select over an empty map) and the
	// UI's Live toggle and Packet Defender both expect the stream
	// to be ready out of the box.
	Enable bool `json:"enable" mapstructure:"enable" default:"true"`

	// BufferSize is the number of most-recent events the broker
	// retains for late-joining subscribers (history replay).
	BufferSize int `json:"buffer_size" mapstructure:"buffer_size" default:"10000"`

	// MaxSubscribers caps the number of concurrent /stream clients
	// the node will accept. Extra clients get HTTP 503.
	MaxSubscribers int `json:"max_subscribers" mapstructure:"max_subscribers" default:"32"`

	// PerSubQueueLen is the buffered channel length per subscriber;
	// events exceeding this on a slow client are dropped silently on
	// that client (counted in broker stats) rather than back-pressuring
	// ingest.
	PerSubQueueLen int `json:"per_sub_queue_len" mapstructure:"per_sub_queue_len" default:"256"`

	// RatePerSubPPS throttles individual subscribers. 0 disables
	// throttling; sane values are 100..1000 per second.
	RatePerSubPPS int `json:"rate_per_sub_pps" mapstructure:"rate_per_sub_pps" default:"500"`
}

// ScriptingConfig configures the Lua scripting engine
type ScriptingConfig struct {
	Enable    bool   `json:"enable" mapstructure:"enable" default:"false"`
	Engine    string `json:"engine" mapstructure:"engine" default:"lua"`
	Folder    string `json:"folder" mapstructure:"folder" default:""`
	HEPFilter []int  `json:"hep_filter" mapstructure:"hep_filter"` // Empty means process all
}

// StorageConfig configures the storage backend
type StorageConfig struct {
	Enable   bool           `json:"enable" mapstructure:"enable" default:"true"`
	DuckLake DuckLakeConfig `json:"ducklake" mapstructure:"ducklake"`
}

// NodeConfig configures the FlightSQL node module
type NodeConfig struct {
	Enable bool `json:"enable" mapstructure:"enable" default:"false"`

	// FlightSQL server settings
	FlightServer FlightServerConfig `json:"flight_server" mapstructure:"flight_server"`

	// FlightSQLServer is the standard Apache Arrow FlightSQL listener (Grafana, ADBC).
	FlightSQLServer FlightSQLServerConfig `json:"flightsql_server" mapstructure:"flightsql_server"`

	// DuckLake storage for reading
	DuckLake DuckLakeConfig `json:"ducklake" mapstructure:"ducklake"`
}

// SmartRoutingConfig controls time-range node pruning for UI search fan-out.
type SmartRoutingConfig struct {
	// Enable turns on skipping nodes whose newest data predates the query
	// window. Off by default; off = every connected node is queried.
	Enable bool `json:"enable" mapstructure:"enable" default:"false"`
}

// CoordinatorConfig configures the API coordinator module
type CoordinatorConfig struct {
	Enable bool `json:"enable" mapstructure:"enable" default:"false"`

	// SmartRouting: skip nodes with no data in the query time range (UI path).
	SmartRouting SmartRoutingConfig `json:"smart_routing" mapstructure:"smart_routing"`

	// HTTP API server
	HTTPServer CoordinatorHTTPServerConfig `json:"http_server" mapstructure:"http_server"`

	// Nodes to query via FlightSQL
	Nodes []NodeEndpoint `json:"nodes" mapstructure:"nodes"`

	// DuckDB file path for users/settings storage
	SettingsDBPath string `json:"settings_db_path" mapstructure:"settings_db_path" default:"/var/lib/homer/homer_settings.duckdb"`

	// LakeName is the DuckLake catalog name used in all SQL queries.
	// Must match storage.ducklake.lake_name / node.ducklake.lake_name.
	// Defaults to "homer_lake" for backward compatibility.
	// Populated automatically from Storage/Node config at startup — no need to set manually.
	LakeName string `json:"lake_name" mapstructure:"lake_name" default:"homer_lake"`

	// QueryTimeoutSec is the HTTP timeout for queries to nodes (default: 30).
	// Health checks use a separate 5s timeout.
	QueryTimeoutSec int `json:"query_timeout_sec" mapstructure:"query_timeout_sec" default:"30"`

	// TransactionViewMaxOpens is how many successful GET /export/view/:uuid opens are allowed per token
	// before it is exhausted (TTL via expires_at still applies). Default 3, min 1, max 1000.
	TransactionViewMaxOpens int `json:"transaction_view_max_opens" mapstructure:"transaction_view_max_opens" default:"3"`

	// LakeTopNStrategy selects how long-range timestamp-DESC top-N transaction
	// searches run: "stream" (default — newest-first time slices without ORDER BY,
	// re-sorted in Go, lowest memory), "chunked" (newest-first time slices with
	// per-window ORDER BY) or "full" (single ORDER BY over the whole range).
	// Auto-populated from storage.ducklake.search.lake_topn_strategy at startup.
	LakeTopNStrategy string `json:"lake_topn_strategy" mapstructure:"lake_topn_strategy" default:"stream"`

	// LakeChunkSec is the OUTER time-window width (seconds) the coordinator uses
	// to slice a long-range transaction search for the "chunked" strategy,
	// newest-first with early exit. Each window is sent to the node as one query
	// (the node further sub-slices it for memory safety, search.lake_chunk_sec
	// = 1h), so this knob controls coordinator round-trips, not peak memory.
	// Default 86400 (24h). Not used by the "stream" strategy, which runs a
	// single query over the whole range.
	LakeChunkSec int `json:"lake_chunk_sec" mapstructure:"lake_chunk_sec" default:"86400"`

	// LazyPayload runs transaction searches in two phases: a narrow search/sort
	// that omits the wide blob columns (payload, data_extra), then a bounded
	// by-uuid point-lookup that re-attaches them. Keeps memory flat on large
	// LIMITs over wide SIP rows while preserving the full-row response.
	// Auto-populated from storage.ducklake.search.lazy_payload at startup.
	LazyPayload bool `json:"lazy_payload" mapstructure:"lazy_payload" default:"true"`

	// IPAliasCacheTTLSec is how long the in-memory IP→alias LPM table is reused between
	// ListActive rebuilds (search/QoS/message enrichment). CRUD on aliases still invalidates
	// immediately. Default 30, min 5, max 86400 (24h). Zero means use the default.
	IPAliasCacheTTLSec int `json:"ip_alias_cache_ttl_sec" mapstructure:"ip_alias_cache_ttl_sec" default:"30"`

	// Authentication settings
	JWT  JWTConfig  `json:"jwt" mapstructure:"jwt"`
	Auth AuthConfig `json:"auth" mapstructure:"auth"`

	// OAuth2Provider is optional single OAuth2 IdP configuration (nil or disabled = no OAuth).
	OAuth2Provider *OAuthProviderConfig `json:"oauth2_provider,omitempty" mapstructure:"oauth2_provider"`

	// LDAP configures optional directory authentication (password login with type "ldap").
	LDAP LDAPConfig `json:"ldap" mapstructure:"ldap"`

	// APISettings enables Auth-Token header access (auth_token in DuckLake), same idea as
	// homer-app api_settings.enable_token_access + auth_token_header.
	APISettings APISettingsConfig `json:"api_settings" mapstructure:"api_settings"`

	// Correlation configures the Lua-based call-id correlation engine that runs
	// inside POST /api/v4/transactions/messages. Scripts are stored in the
	// settings DuckDB table `correlation_scripts` keyed by (hepid, profile, type='correlation').
	Correlation CorrelationScriptingConfig `json:"correlation" mapstructure:"correlation"`

	// HepStream configures the coordinator side of the live HEP WebSocket
	// stream exposed at GET /api/v4/stream/hep. The publisher side
	// (broker + ring buffer) lives in Ingest.HepStream; this section only
	// governs what the coordinator itself is willing to forward to the UI.
	HepStream CoordinatorHepStreamConfig `json:"hep_stream" mapstructure:"hep_stream"`

	// Widgets configures dashboard widget picker categories (see widgets.control).
	Widgets WidgetsConfig `json:"widgets" mapstructure:"widgets"`

	// FlightSQLServer exposes Arrow FlightSQL on the coordinator and proxies SQL
	// to node flightsql_port endpoints (Grafana single entrypoint).
	FlightSQLServer FlightSQLServerConfig `json:"flightsql_server" mapstructure:"flightsql_server"`

	// LineProtoTablePrefix mirrors ingest.line_protocol.table_prefix for
	// GET /api/v4/line_protocol/tables default ?prefix=; set at load from Ingest
	// (not read from JSON).
	LineProtoTablePrefix string `json:"-" mapstructure:"-"`

	// MCP runtime settings propagated from top-level config in modular startup.
	MCP MCPConfig `json:"-" mapstructure:"-"`
}

// CorrelationScriptingConfig configures the Lua correlation engine used by the
// coordinator.
type CorrelationScriptingConfig struct {
	// Enable turns the correlation engine on. When false no Lua VM is created
	// and /transactions/messages behaves exactly as before.
	Enable bool `json:"enable" mapstructure:"enable" default:"false"`

	// SQLTimeoutMS caps every executeSQL call from Lua.
	SQLTimeoutMS int `json:"sql_timeout_ms" mapstructure:"sql_timeout_ms" default:"5000"`

	// ScriptTimeoutMS caps total Lua execution time per correlation request.
	ScriptTimeoutMS int `json:"script_timeout_ms" mapstructure:"script_timeout_ms" default:"3000"`

	// MaxSQLCalls caps the number of executeSQL invocations per correlation run.
	MaxSQLCalls int `json:"max_sql_calls" mapstructure:"max_sql_calls" default:"16"`

	// MaxSQLRows caps the number of rows returned by each executeSQL call.
	MaxSQLRows int `json:"max_sql_rows" mapstructure:"max_sql_rows" default:"10000"`

	// SyncIntervalSec is the period between background reloads of correlation_scripts
	// into the in-memory ScriptMap cache. 0 disables periodic sync.
	SyncIntervalSec int `json:"sync_interval_sec" mapstructure:"sync_interval_sec" default:"30"`

	// SeedDefault seeds disabled (status=false) default correlation
	// templates for the `1_call` and `1_registration` keys when
	// correlation_scripts has no row yet for the corresponding (hepid, profile)
	// pair. Seeding runs independently of Enable so the
	// Settings → Scripts UI is never empty out of the box; the seeds are
	// inert until an operator reviews and enables them.
	SeedDefault bool `json:"seed_default" mapstructure:"seed_default" default:"true"`
}

// CoordinatorHepStreamConfig configures the coordinator-side WebSocket
// that forwards live HEP events to the UI. The actual broker lives in
// Ingest.HepStream; this struct only governs the coordinator's fan-out
// behaviour.
type CoordinatorHepStreamConfig struct {
	// Enable exposes GET /api/v4/stream/hep. When false the endpoint
	// returns 503 "stream not configured" even if the ingest-side
	// broker is running. Defaults to true so the v4 UI (Live toggle,
	// Packet Defender) works out of the box. Payload streaming stays
	// off by default (see AllowPayload) so enabling the endpoint by
	// itself exposes metadata only.
	Enable bool `json:"enable" mapstructure:"enable" default:"true"`

	// AllowPayload lets admin clients request raw SIP payload via
	// ?include_payload=1. Non-admin clients are rejected even when
	// this is true; when it is false, nobody gets payload, not even
	// admins. Payload defaults off to minimise accidental exposure of
	// live call headers in log scrapers.
	AllowPayload bool `json:"allow_payload" mapstructure:"allow_payload" default:"false"`

	// FanOutTimeoutMS is the per-node WebSocket dial + read timeout
	// used by the coordinator when subscribing to each configured
	// node's /stream endpoint. A slow node must not block the UI's
	// upgrade, so 2s is the initial ceiling.
	FanOutTimeoutMS int `json:"fan_out_timeout_ms" mapstructure:"fan_out_timeout_ms" default:"2000"`

	// HistoryLimit caps how many buffered events a new subscriber
	// receives as an initial burst (across all fanned-in nodes). The
	// node-side buffer_size is the upper bound, but the UI rarely
	// needs more than a few hundred events to feel "live".
	HistoryLimit int `json:"history_limit" mapstructure:"history_limit" default:"200"`
}

// MCPConfig configures MCP stdio server module.
type MCPConfig struct {
	Enable            bool         `json:"enable" mapstructure:"enable" default:"false"`
	Mode              string       `json:"mode" mapstructure:"mode" default:"hybrid"` // hybrid|structured|sql
	HomerBaseURL      string       `json:"homer_base_url" mapstructure:"homer_base_url" default:"http://127.0.0.1:8080"`
	HomerToken        string       `json:"homer_token" mapstructure:"homer_token" default:""`
	DefaultLimit      int          `json:"default_limit" mapstructure:"default_limit" default:"100"`
	SQLDefaultLimit   int          `json:"sql_default_limit" mapstructure:"sql_default_limit" default:"100"`
	RequestTimeoutSec int          `json:"request_timeout_sec" mapstructure:"request_timeout_sec" default:"30"`
	LLM               MCPLLMConfig `json:"llm" mapstructure:"llm"`
}

// MCPLLMConfig configures optional LLM-assisted NL parsing for MCP queries.
type MCPLLMConfig struct {
	Enable      bool    `json:"enable" mapstructure:"enable" default:"false"`
	Provider    string  `json:"provider" mapstructure:"provider" default:"openai"` // currently openai/openai-compatible
	BaseURL     string  `json:"base_url" mapstructure:"base_url" default:"https://api.openai.com/v1"`
	APIKey      string  `json:"api_key" mapstructure:"api_key" default:""`
	Model       string  `json:"model" mapstructure:"model" default:"gpt-4o-mini"`
	Temperature float64 `json:"temperature" mapstructure:"temperature" default:"0.1"`
	MaxTokens   int     `json:"max_tokens" mapstructure:"max_tokens" default:"400"`
	TimeoutSec  int     `json:"timeout_sec" mapstructure:"timeout_sec" default:"15"`
}

// UDPServerConfig configures the UDP HEP receiver
type UDPServerConfig struct {
	Enable           bool   `json:"enable" mapstructure:"enable" default:"true"`
	Host             string `json:"host" mapstructure:"host" default:"0.0.0.0"`
	Port             int    `json:"port" mapstructure:"port" default:"9060"`
	Multicore        bool   `json:"multicore" mapstructure:"multicore" default:"true"`
	SocketRecvBuffer int    `json:"socket_recv_buffer" mapstructure:"socket_recv_buffer" default:"8388608"`
	SocketSendBuffer int    `json:"socket_send_buffer" mapstructure:"socket_send_buffer" default:"1048576"`
	ReadBufferCap    int    `json:"read_buffer_cap" mapstructure:"read_buffer_cap" default:"131072"`
}

// TCPServerConfig configures the TCP HEP receiver
type TCPServerConfig struct {
	Enable         bool   `json:"enable" mapstructure:"enable" default:"false"`
	Host           string `json:"host" mapstructure:"host" default:"0.0.0.0"`
	Port           int    `json:"port" mapstructure:"port" default:"9060"`
	Multicore      bool   `json:"multicore" mapstructure:"multicore" default:"true"`
	MaxConnections int    `json:"max_connections" mapstructure:"max_connections" default:"0"`
	ReadTimeoutSec int    `json:"read_timeout_sec" mapstructure:"read_timeout_sec" default:"300"`
}

// TLSServerConfig configures the TLS HEP receiver
type TLSServerConfig struct {
	Enable             bool   `json:"enable" mapstructure:"enable" default:"false"`
	Host               string `json:"host" mapstructure:"host" default:"0.0.0.0"`
	Port               int    `json:"port" mapstructure:"port" default:"9061"`
	Cert               string `json:"cert" mapstructure:"cert" default:""`
	Key                string `json:"key" mapstructure:"key" default:""`
	CaCert             string `json:"cacert" mapstructure:"cacert" default:""`
	MinTLSVersion      string `json:"min_tls_version" mapstructure:"min_tls_version" default:"TLS1.2"`
	MaxTLSVersion      string `json:"max_tls_version" mapstructure:"max_tls_version" default:"TLS1.3"`
	MutualTLS          bool   `json:"mutual_tls" mapstructure:"mutual_tls" default:"false"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" default:"false"`
	MaxConnections     int    `json:"max_connections" mapstructure:"max_connections" default:"0"`
	ReadTimeoutSec     int    `json:"read_timeout_sec" mapstructure:"read_timeout_sec" default:"300"`
}

// HTTPServerConfig configures the HTTP HEP receiver
type HTTPServerConfig struct {
	Enable             bool   `json:"enable" mapstructure:"enable" default:"true"`
	Host               string `json:"host" mapstructure:"host" default:"0.0.0.0"`
	Port               int    `json:"port" mapstructure:"port" default:"9080"`
	ReadTimeout        int    `json:"read_timeout" mapstructure:"read_timeout" default:"30"`
	WriteTimeout       int    `json:"write_timeout" mapstructure:"write_timeout" default:"30"`
	MaxRequestBodySize int    `json:"max_request_body_size" mapstructure:"max_request_body_size" default:"67108864"`
	WebSocketEnable    bool   `json:"websocket_enable" mapstructure:"websocket_enable" default:"false"`
}

// HTTPSServerConfig configures the HTTPS HEP receiver
type HTTPSServerConfig struct {
	Enable             bool   `json:"enable" mapstructure:"enable" default:"false"`
	Host               string `json:"host" mapstructure:"host" default:"0.0.0.0"`
	Port               int    `json:"port" mapstructure:"port" default:"9443"`
	Cert               string `json:"cert" mapstructure:"cert" default:""`
	Key                string `json:"key" mapstructure:"key" default:""`
	CaCert             string `json:"cacert" mapstructure:"cacert" default:""`
	MinTLSVersion      string `json:"min_tls_version" mapstructure:"min_tls_version" default:"TLS1.2"`
	MaxTLSVersion      string `json:"max_tls_version" mapstructure:"max_tls_version" default:"TLS1.3"`
	MutualTLS          bool   `json:"mutual_tls" mapstructure:"mutual_tls" default:"false"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" default:"false"`
	WebSocketEnable    bool   `json:"websocket_enable" mapstructure:"websocket_enable" default:"false"`
}

// FlightServerConfig configures the FlightSQL server
type FlightServerConfig struct {
	Enable         bool   `json:"enable" mapstructure:"enable" default:"true"`
	Host           string `json:"host" mapstructure:"host" default:"0.0.0.0"`
	Port           int    `json:"port" mapstructure:"port" default:"50051"`
	AuthToken      string `json:"auth_token" mapstructure:"auth_token" default:""`
	MaxMessageSize int    `json:"max_message_size" mapstructure:"max_message_size" default:"16777216"`
}

// FlightSQLServerConfig configures the Apache Arrow FlightSQL gRPC server used by
// Grafana (InfluxDB datasource, FlightSQL query language) and other Arrow clients.
// It is separate from flight_server (DuckDB Airport / PATH descriptors).
type FlightSQLServerConfig struct {
	Enable                    bool   `json:"enable" mapstructure:"enable" default:"false"`
	Host                      string `json:"host" mapstructure:"host" default:"0.0.0.0"`
	Port                      int    `json:"port" mapstructure:"port" default:"50055"`
	AuthToken                 string `json:"auth_token" mapstructure:"auth_token" default:""`
	CatalogRefreshIntervalSec int    `json:"catalog_refresh_interval_sec" mapstructure:"catalog_refresh_interval_sec" default:"30"`
}

// DuckDBTuning configures DuckDB engine parameters such as the in-RAM
// memory cap, the worker thread pool, and where the engine spills to
// disk when the buffer pool is full.
//
// All three knobs map directly to DuckDB SET statements
// (`SET memory_limit = ...`, `SET threads = ...`, `SET temp_directory = ...`).
// When threads is 0 or memory_limit is empty, the writer and node apply
// sensible defaults (threads = min(NumCPU/2, 4), memory_limit = "2GB")
// to prevent DuckDB from oversubscribing the host. Set explicit values
// to override these defaults. Docker images set HOMER_*_TUNING_* ENV
// so the generic container does not stay on the 2GB floor.
type DuckDBTuning struct {
	// Threads sets the size of the DuckDB worker pool. 0 = auto
	// (writer/node default to min(NumCPU/2, 4) to leave headroom
	// for Go goroutines). Set explicitly to override.
	Threads int `json:"threads" mapstructure:"threads" default:"0"`
	// MemoryLimit caps the DuckDB buffer pool. Accepts the strings
	// DuckDB itself accepts ("8GB", "1.5 GiB", "8589934592 bytes"),
	// empty = writer/node default to "2GB". Without a limit DuckDB
	// claims ~80% of host RAM which causes swapping on shared nodes.
	MemoryLimit string `json:"memory_limit" mapstructure:"memory_limit" default:""`
	// TempDirectory is where DuckDB spills tuples that no longer fit
	// in the buffer pool. Empty = DuckDB default (`<cwd>/.tmp`).
	// Pin this to a fast SSD volume on busy nodes so spill traffic
	// does not contend with the catalog disk.
	TempDirectory string `json:"temp_directory" mapstructure:"temp_directory" default:""`
}

// DuckLakeConfig configures DuckLake storage
type DuckLakeConfig struct {
	CatalogType   string `json:"catalog_type" mapstructure:"catalog_type" default:"sqlite"` // DuckLake catalog — sqlite
	CatalogPath   string `json:"catalog_path" mapstructure:"catalog_path" default:"homer_catalog.sqlite"`
	DataPath      string `json:"data_path" mapstructure:"data_path" default:"/var/lib/homer/parquet"`
	LakeName      string `json:"lake_name" mapstructure:"lake_name" default:"homer_lake"`
	BatchSize     int    `json:"batch_size" mapstructure:"batch_size" default:"10000"`
	FlushInterval int    `json:"flush_interval_sec" mapstructure:"flush_interval_sec" default:"30"`
	SearchBuffer  bool   `json:"search_buffer" mapstructure:"search_buffer" default:"false"`
	ShardCount    int    `json:"shard_count" mapstructure:"shard_count" default:"1"`
	FlushQueue    *bool  `json:"flush_queue" mapstructure:"flush_queue"` // nil = auto (true for SQLite), explicit true/false overrides
	// AutoRepairCatalog runs a startup catalog autofix that removes duplicate
	// snapshot/table metadata rows causing "Corrupt DuckLake - multiple
	// snapshots returned from database". Lossless for data. nil = enabled
	// (default); set false to disable.
	AutoRepairCatalog *bool `json:"auto_repair_catalog" mapstructure:"auto_repair_catalog"`
	// DataInliningRowLimit controls the DuckLake data inlining threshold (DuckLake v1.0).
	// Writes of ≤N rows are stored directly in the catalog database instead of creating
	// small Parquet files.
	//
	// Default 0 = inlining DISABLED (every write goes to a Parquet file). This is
	// deliberate: DuckLake's own default inlines small writes into the catalog DB, and
	// under streaming ingest with many small writes (Line Protocol, OTLP, low-volume
	// HEP subtypes) that turns the catalog (sqlite) into the dominant memory + disk
	// consumer — an 800 MB catalog backing only a few dozen Parquet files, and multi-GB
	// RSS when DuckLake mirrors the catalog in memory. The CompactionService now also
	// flushes inlined data on each maintenance cycle as a safety net.
	//   *  0  -> disable inlining (recommended; always write Parquet)
	//   * >0  -> inline writes smaller than N rows (only for low write cardinality)
	//   * -1  -> leave DuckLake's own default (inlines ~10 rows) — avoid for streaming
	DataInliningRowLimit int                 `json:"data_inlining_row_limit" mapstructure:"data_inlining_row_limit" default:"0"`
	Tuning               DuckDBTuning        `json:"tuning" mapstructure:"tuning"`
	S3                   S3Config            `json:"s3" mapstructure:"s3"`
	Compaction           CompactionConfig    `json:"compaction" mapstructure:"compaction"`
	StoragePolicy        StoragePolicyConfig `json:"storage_policy" mapstructure:"storage_policy"`
	Volumes              []VolumeConfig      `json:"volumes" mapstructure:"volumes"` // Direct volumes config (alternative to storage_policy for read-only nodes)
	Search               SearchConfig        `json:"search" mapstructure:"search"`
}

// SearchConfig tunes how the read/query path executes long-range searches.
type SearchConfig struct {
	// LakeTopNStrategy selects how a long-range `ORDER BY timestamp DESC LIMIT N`
	// lake query is executed (it only applies when the split planner already
	// decided the query is a timestamp-DESC top-N over a range wider than one
	// hour):
	//   "stream" (default) — drop the ORDER BY and use a plain streaming LIMIT
	//        so DuckDB stops after N rows and memory stays flat (the lowest-memory,
	//        fastest option), then re-sort the N rows newest-first in Go before
	//        returning. Caveat: the N rows are an arbitrary scan-order sample of
	//        the range, so they are ordered correctly but not guaranteed to be the
	//        globally newest N. Use "chunked" when exact newest-N is required.
	//   "chunked" — scan descending time windows (LakeChunkSec wide), newest
	//        first, stopping once N rows are gathered. Exact newest-first result
	//        while bounding peak memory to one window. Avoids the OOM the
	//        whole-range scan hits on wide SIP rows.
	//   "full"    — original single ORDER BY scan over the whole range (can OOM
	//        on wide data under a small memory_limit).
	LakeTopNStrategy string `json:"lake_topn_strategy" mapstructure:"lake_topn_strategy" default:"stream"`
	// LakeChunkSec is the time-window width (seconds) for the "chunked" strategy.
	// Smaller windows bound memory tighter at the cost of more sub-queries when
	// data is sparse. Default 3600 (1 hour).
	LakeChunkSec int `json:"lake_chunk_sec" mapstructure:"lake_chunk_sec" default:"3600"`
	// LazyPayload runs transaction searches in two phases: a narrow search/sort
	// over all columns except the wide blobs (payload, data_extra), then a
	// bounded by-uuid point-lookup that re-attaches them. This avoids
	// decompressing the wide payload column for the whole scanned range — only
	// the <=LIMIT returned rows pay that cost — so large LIMITs stay memory-safe
	// without sacrificing the full-row response. Default true.
	LazyPayload bool `json:"lazy_payload" mapstructure:"lazy_payload" default:"true"`
}

// StoragePolicyConfig configures tiered storage with hot and cold volumes
type StoragePolicyConfig struct {
	Enable             bool           `json:"enable" mapstructure:"enable" default:"false"`
	Volumes            []VolumeConfig `json:"volumes" mapstructure:"volumes"`
	TTLMoveIntervalSec int            `json:"ttl_move_interval_sec" mapstructure:"ttl_move_interval_sec" default:"3600"` // How often to check for data to move
	MoveFactor         float64        `json:"move_factor" mapstructure:"move_factor" default:"0.8"`                      // Move when volume is X% full
	ConcurrentMoves    int            `json:"concurrent_moves" mapstructure:"concurrent_moves" default:"2"`              // Max concurrent partition moves
	MoveOnStartup      bool           `json:"move_on_startup" mapstructure:"move_on_startup" default:"false"`            // Run tiering on startup
}

// VolumeConfig configures a storage volume (hot or cold)
type VolumeConfig struct {
	Name           string `json:"name" mapstructure:"name"`                                       // Volume name (e.g., "hot", "cold")
	Type           string `json:"type" mapstructure:"type" default:"local"`                       // "local" or "s3"
	Path           string `json:"path" mapstructure:"path"`                                       // Data path: local path or S3 URL (s3://bucket/path/)
	CatalogType    string `json:"catalog_type" mapstructure:"catalog_type" default:"sqlite"`      // DuckLake catalog — sqlite
	CatalogPath    string `json:"catalog_path" mapstructure:"catalog_path"`                       // Catalog path for this volume
	Priority       int    `json:"priority" mapstructure:"priority" default:"0"`                   // Lower = higher priority (writes go to lowest)
	MaxDataAgeDays int    `json:"max_data_age_days" mapstructure:"max_data_age_days" default:"0"` // Move data older than N days to next volume (0 = no limit)
	MaxSizeGB      int    `json:"max_size_gb" mapstructure:"max_size_gb" default:"0"`             // Max size in GB before moving to next volume (0 = no limit)
	S3Region       string `json:"s3_region" mapstructure:"s3_region" default:""`
	S3AccessKeyID  string `json:"s3_access_key_id" mapstructure:"s3_access_key_id" default:""`
	S3SecretKey    string `json:"s3_secret_access_key" mapstructure:"s3_secret_access_key" default:""`
	S3Endpoint     string `json:"s3_endpoint" mapstructure:"s3_endpoint" default:""` // For S3-compatible (R2, MinIO)
	S3UseSSL       bool   `json:"s3_use_ssl" mapstructure:"s3_use_ssl" default:"true"`
	S3URLStyle     string `json:"s3_url_style" mapstructure:"s3_url_style" default:""` // Empty = path, set "vhost" for virtual-hosted-style
	// OverrideDataPath passes OVERRIDE_DATA_PATH TRUE to DuckLake ATTACH when the path
	// in config intentionally differs from DATA_PATH stored in an existing catalog
	// (e.g. bucket rename, or node path typo vs writer). Prefer matching paths first.
	OverrideDataPath bool `json:"override_data_path" mapstructure:"override_data_path" default:"false"`
}

// CompactionConfig configures automatic compaction and retention
type CompactionConfig struct {
	// Enable turns on periodic compaction on the writer DuckLake catalog.
	// On by default: small per-flush Parquet files and DuckLake snapshots
	// accumulate quickly, so without periodic merge/expire/cleanup the catalog
	// and file count grow unbounded. When storage_policy has multiple volumes
	// and at least one local volume, the writer forces compaction on regardless
	// of this flag (hot parquet). Set to false only to opt out explicitly.
	Enable           bool `json:"enable" mapstructure:"enable" default:"true"`
	CheckIntervalSec int  `json:"check_interval_sec" mapstructure:"check_interval_sec" default:"3600"` // 1 hour
	RetentionDays    int  `json:"retention_days" mapstructure:"retention_days" default:"0"`            // 0 = disabled
	// RetentionDaysByTable overrides RetentionDays for specific DuckLake tables
	// (keys are bare table names, e.g. "hep_proto_1_registration"). An override
	// of 0 disables TTL for that table. When RetentionDays is 0, positive
	// overrides still run retention for the listed tables only.
	RetentionDaysByTable map[string]int `json:"retention_days_by_table" mapstructure:"retention_days_by_table"`
	// RetentionUnit selects how RetentionDays / RetentionDaysByTable values
	// are interpreted: "days" (default) uses calendar-day arithmetic
	// (matches legacy behavior exactly); "hours" reinterprets the same
	// integer fields as hours. Applies globally to both the default and
	// every per-table override.
	RetentionUnit string `json:"retention_unit" mapstructure:"retention_unit" default:"days"`
	// SnapshotExpireIntervalSec controls how long to keep DuckLake snapshots (seconds).
	SnapshotExpireIntervalSec int `json:"snapshot_expire_interval_sec" mapstructure:"snapshot_expire_interval_sec" default:"3600"`
	// MinAgeSec leaves a partition alone until nothing has been written to it for
	// this long, measured from the snapshot that added its newest file. Ingest
	// targets one date partition at a time, and compacting it races the writer's
	// flush: the merged file is then discarded, so the work is repeated every
	// cycle. Enforced by the "native" engine only; ducklake_merge_adjacent_files
	// offers no equivalent control.
	//
	// The default is a few minutes rather than an hour so that a busy writer still
	// consolidates during the day: partitions are keyed by date, so an hour-long
	// quiet period never arrives under continuous ingest and today's files would
	// pile up until midnight.
	MinAgeSec int `json:"min_age_sec" mapstructure:"min_age_sec" default:"300"`
	// MinFileSizeBytes: minimum file size for merge (smaller files will be merged). 0 = no limit.
	MinFileSizeBytes int64 `json:"min_file_size_bytes" mapstructure:"min_file_size_bytes" default:"0"`
	// MaxFileSizeBytes: maximum size of merged file. 0 = no limit (DuckLake default).
	MaxFileSizeBytes int64 `json:"max_file_size_bytes" mapstructure:"max_file_size_bytes" default:"0"`
	// MaxCompactedFiles: maximum number of compaction operations (output files)
	// per table per cycle. 0 = writer default (32). This is not an input-file
	// cap; peak memory is bounded by max_file_size_bytes and merge threads=1.
	MaxCompactedFiles int `json:"max_compacted_files" mapstructure:"max_compacted_files" default:"0"`
	// Engine selects the compaction implementation:
	//   "duckdb" — DuckLake's ducklake_merge_adjacent_files (default). Loads a
	//              whole partition into memory, which can OOM on wide SIP data.
	//   "native" — Go compactor that concatenates a partition's parquet row
	//              groups itself (peak memory ≈ one row group), then swaps each
	//              partition in with one short DuckDB transaction: a DELETE over
	//              the partition column retires the old files and
	//              ducklake_add_data_files registers the merged output. DuckLake
	//              allocates every snapshot and file id, so the live writer's
	//              cache cannot go stale. Requires a local data_path and a
	//              DuckLake build providing ducklake_add_data_files; otherwise it
	//              falls back to the DuckDB merge automatically.
	Engine string `json:"engine" mapstructure:"engine" default:"duckdb"`
	// TargetFileSizeBytes caps each merged output file for the native engine.
	// 0 = engine default (512MB).
	TargetFileSizeBytes int64 `json:"target_file_size_bytes" mapstructure:"target_file_size_bytes" default:"0"`
	// MaxRowGroupBytes bounds the native engine's memory use. A parquet row group
	// is the unit the merge holds in memory, and row groups are sized in rows
	// regardless of how wide those rows are: files with 1.5GB row groups (10k rows
	// of SIP payload) have been measured needing ~10GB RSS to merge. Partitions
	// containing a row group above this budget are left alone instead. Expect peak
	// RSS several times the budget. 0 = engine default (256MB).
	MaxRowGroupBytes int64 `json:"max_row_group_bytes" mapstructure:"max_row_group_bytes" default:"0"`
}

// S3Config configures S3 storage for DuckLake
type S3Config struct {
	Region          string `json:"region" mapstructure:"region" default:""`
	AccessKeyID     string `json:"access_key_id" mapstructure:"access_key_id" default:""`
	SecretAccessKey string `json:"secret_access_key" mapstructure:"secret_access_key" default:""`
	Endpoint        string `json:"endpoint" mapstructure:"endpoint" default:""`
	UseSSL          bool   `json:"use_ssl" mapstructure:"use_ssl" default:"true"`
	URLStyle        string `json:"url_style" mapstructure:"url_style" default:""` // Empty = path, set "vhost" for virtual-hosted-style
}

// HEPConfig configures HEP protocol processing
type HEPConfig struct {
	HepV2Enable    bool `json:"hepv2_enable" mapstructure:"hepv2_enable" default:"true"`
	HepV3Enable    bool `json:"hepv3_enable" mapstructure:"hepv3_enable" default:"true"`
	ProtobufEnable bool `json:"protobuf_enable" mapstructure:"protobuf_enable" default:"true"`
	Deduplicate    bool `json:"deduplicate" mapstructure:"deduplicate" default:"false"`
}

// SIPConfig configures SIP processing
type SIPConfig struct {
	CensorMethods  []string `json:"censored_methods" mapstructure:"censored_methods"`
	DiscardMethods []string `json:"discard_methods" mapstructure:"discard_methods"`
	// AlegIDs lists SIP header names (case-insensitive). The first matching header in wire order sets XCallID; used for HEP CID when the chunk has no CID (see sipparser.ParseMsgZeroCopy).
	AlegIDs []string `json:"aleg_ids" mapstructure:"aleg_ids"`
	// CustomHeaders lists optional SIP header names copied into data_extra.custom_headers in storage.
	CustomHeaders []string `json:"custom_headers" mapstructure:"custom_headers"`
	ForceALegID   bool     `json:"force_aleg_id" mapstructure:"force_aleg_id" default:"false"`
}

// CoordinatorHTTPServerConfig configures the Coordinator API server
type CoordinatorHTTPServerConfig struct {
	Enable       bool   `json:"enable" mapstructure:"enable" default:"true"`
	Host         string `json:"host" mapstructure:"host" default:"0.0.0.0"`
	Port         int    `json:"port" mapstructure:"port" default:"8080"`
	ReadTimeout  int    `json:"read_timeout" mapstructure:"read_timeout" default:"30"`
	WriteTimeout int    `json:"write_timeout" mapstructure:"write_timeout" default:"30"`
	StaticPath   string `json:"static_path" mapstructure:"static_path" default:""`
	// GamedataDir is an on-disk directory served read-only at /gamedata/.
	// Used for large game assets (e.g. the Doom widget IWAD) that must not
	// be embedded into the binary via go:embed. The default matches the
	// package install prefix; a missing directory simply yields 404s.
	GamedataDir string `json:"gamedata_dir" mapstructure:"gamedata_dir" default:"/usr/local/homer-core/gamedata"`
}

// NodeEndpoint defines a FlightSQL node to query
type NodeEndpoint struct {
	Name string `json:"name" mapstructure:"name"`
	Host string `json:"host" mapstructure:"host"`
	Port int    `json:"port" mapstructure:"port" default:"50051"`
	// FlightSQLPort is the node's Apache Arrow FlightSQL gRPC port (Grafana). Zero = not used by coordinator FlightSQL proxy.
	FlightSQLPort int    `json:"flightsql_port" mapstructure:"flightsql_port" default:"0"`
	UseTLS        bool   `json:"use_tls" mapstructure:"use_tls" default:"false"`
	Token         string `json:"token" mapstructure:"token" default:""`
	Priority      int    `json:"priority" mapstructure:"priority" default:"0"`
}

// JWTConfig configures JWT authentication
type JWTConfig struct {
	Secret      string `json:"secret" mapstructure:"secret" default:""`
	ExpireHours int    `json:"expire_hours" mapstructure:"expire_hours" default:"24"`
	// CookieEnable issues an HttpOnly session cookie on login (default true).
	// The bundled UI sends credentials: include so the cookie is shared across tabs.
	CookieEnable *bool `json:"cookie_enable,omitempty" mapstructure:"cookie_enable"`
	// CookieName is the HttpOnly cookie name (default homer_session).
	CookieName string `json:"cookie_name,omitempty" mapstructure:"cookie_name"`
	// CookieSameSite is Lax (default), Strict, or None.
	CookieSameSite string `json:"cookie_same_site,omitempty" mapstructure:"cookie_same_site"`
	// CookieSecure forces the Secure flag; nil = auto (TLS or X-Forwarded-Proto: https).
	CookieSecure *bool `json:"cookie_secure,omitempty" mapstructure:"cookie_secure"`
}

// LegacySHA256SipcaptureHash is the SHA-256 hex digest of the historical
// default password "sipcapture". Used only in tests and legacy hash migration;
// production bootstrap no longer applies this value automatically.
const LegacySHA256SipcaptureHash = "883ffc1f37fd0fe542b0fb9740035c4383e7d976c411161d24e62edace280f90"

// AuthConfig configures authentication
type AuthConfig struct {
	// Type selects the primary password-auth mode: "internal" (DuckDB users +
	// optional bootstrap), "ldap", or "oauth". If empty or omitted on the
	// object, Load normalizes to "internal".
	Type              string `json:"type,omitempty" mapstructure:"type"`
	AdminUser         string `json:"admin_user,omitempty" mapstructure:"admin_user" default:"admin"`
	AdminPasswordHash string `json:"admin_password_hash,omitempty" mapstructure:"admin_password_hash" default:""`
	// FallbackAuthType, if set to "internal" or "ldap", is tried when password login
	// with the client-selected type fails (wrong credentials or backend unavailable).
	FallbackAuthType string `json:"fallback_auth_type,omitempty" mapstructure:"fallback_auth_type"`
	// DisablePasswordLogin hides internal/LDAP password login from discovery and rejects
	// POST /api/v4/auth/sessions (OAuth-only deployments).
	DisablePasswordLogin bool `json:"disable_password_login,omitempty" mapstructure:"disable_password_login" default:"false"`
	// AuthFromInternalString is true when coordinator.auth was the JSON string "internal",
	// or after normalization when type is "internal" (DuckDB bootstrap path).
	AuthFromInternalString bool `json:"-" mapstructure:"-"`
}

// MarshalJSON writes {"type":"internal", ...} for internal mode; otherwise
// an object with type (if set) plus admin_user / admin_password_hash when present.
func (a AuthConfig) MarshalJSON() ([]byte, error) {
	if a.AuthFromInternalString || strings.EqualFold(strings.TrimSpace(a.Type), "internal") {
		type out struct {
			Type                 string `json:"type"`
			AdminUser            string `json:"admin_user,omitempty"`
			AdminPasswordHash    string `json:"admin_password_hash,omitempty"`
			FallbackAuthType     string `json:"fallback_auth_type,omitempty"`
			DisablePasswordLogin bool   `json:"disable_password_login,omitempty"`
		}
		o := out{Type: "internal"}
		if u := strings.TrimSpace(a.AdminUser); u != "" && u != "admin" {
			o.AdminUser = a.AdminUser
		}
		if h := strings.TrimSpace(a.AdminPasswordHash); h != "" {
			o.AdminPasswordHash = a.AdminPasswordHash
		}
		if fb := strings.TrimSpace(a.FallbackAuthType); fb != "" {
			o.FallbackAuthType = fb
		}
		if a.DisablePasswordLogin {
			o.DisablePasswordLogin = true
		}
		return json.Marshal(o)
	}
	type out struct {
		Type                 string `json:"type,omitempty"`
		AdminUser            string `json:"admin_user,omitempty"`
		AdminPasswordHash    string `json:"admin_password_hash,omitempty"`
		FallbackAuthType     string `json:"fallback_auth_type,omitempty"`
		DisablePasswordLogin bool   `json:"disable_password_login,omitempty"`
	}
	return json.Marshal(out{
		Type:                 strings.TrimSpace(a.Type),
		AdminUser:            a.AdminUser,
		AdminPasswordHash:    a.AdminPasswordHash,
		FallbackAuthType:     strings.TrimSpace(a.FallbackAuthType),
		DisablePasswordLogin: a.DisablePasswordLogin,
	})
}

var authConfigReflectType = reflect.TypeOf(AuthConfig{})

func decodeCoordinatorAuthHook(from reflect.Type, to reflect.Type, data any) (any, error) {
	if to != authConfigReflectType {
		return data, nil
	}
	if from == nil || from.Kind() != reflect.String {
		return data, nil
	}
	s, ok := data.(string)
	if !ok {
		return data, nil
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("coordinator.auth: empty string is not allowed")
	}
	if strings.EqualFold(s, "internal") {
		return AuthConfig{
			Type:                   "internal",
			AdminUser:              "admin",
			AuthFromInternalString: true,
		}, nil
	}
	return nil, fmt.Errorf("coordinator.auth: unsupported string value %q (use \"internal\" or {\"type\":\"internal\"})", s)
}

// normalizeCoordinatorAuth applies coordinator.auth.type after Unmarshal (object
// {"type":"internal"} vs JSON string "internal" from decodeCoordinatorAuthHook).
// An empty type always becomes "internal".
func normalizeCoordinatorAuth(cfg *CoordinatorConfig) error {
	auth := &cfg.Auth
	auth.Type = strings.TrimSpace(strings.ToLower(auth.Type))
	if auth.Type == "" {
		auth.Type = "internal"
	}

	switch auth.Type {
	case "internal":
		auth.AuthFromInternalString = true
		if strings.TrimSpace(auth.AdminUser) == "" {
			auth.AdminUser = "admin"
		}
	case "ldap", "oauth":
		auth.AuthFromInternalString = false
	default:
		return fmt.Errorf("coordinator.auth.type: unsupported %q (allowed: internal, ldap, oauth, or omit)", auth.Type)
	}
	fb := strings.TrimSpace(strings.ToLower(auth.FallbackAuthType))
	if fb != "" {
		if fb != "internal" && fb != "ldap" {
			return fmt.Errorf("coordinator.auth.fallback_auth_type: unsupported %q (allowed: internal, ldap, or omit)", auth.FallbackAuthType)
		}
	}
	auth.FallbackAuthType = fb
	return nil
}

// LDAPConfig configures optional LDAP / Active Directory password authentication for the coordinator.
// Field names mirror homer-app ldap_config where possible.
type LDAPConfig struct {
	Enable              bool     `json:"enable" mapstructure:"enable" default:"false"`
	Host                string   `json:"host" mapstructure:"host"`
	Port                int      `json:"port" mapstructure:"port" default:"389"`
	UseSSL              bool     `json:"use_ssl" mapstructure:"use_ssl" default:"false"`
	SkipTLS             bool     `json:"skip_tls" mapstructure:"skip_tls" default:"true"`
	InsecureSkipVerify  bool     `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" default:"true"`
	ServerName          string   `json:"server_name" mapstructure:"server_name"`
	BindDN              string   `json:"bind_dn" mapstructure:"bind_dn"`
	BindPassword        string   `json:"bind_password" mapstructure:"bind_password"`
	Base                string   `json:"base" mapstructure:"base"`
	UserFilter          string   `json:"user_filter" mapstructure:"user_filter" default:"(uid=%s)"`
	Attributes          []string `json:"attributes" mapstructure:"attributes"`
	Anonymous           bool     `json:"anonymous" mapstructure:"anonymous" default:"false"`
	UserDN              string   `json:"user_dn" mapstructure:"user_dn"`
	AdminGroup          string   `json:"admin_group" mapstructure:"admin_group"`
	UserGroup           string   `json:"user_group" mapstructure:"user_group"`
	AdminMode           bool     `json:"admin_mode" mapstructure:"admin_mode" default:"false"`
	UserMode            bool     `json:"user_mode" mapstructure:"user_mode" default:"false"`
	GroupFilter         string   `json:"group_filter" mapstructure:"group_filter" default:"(memberUid=%s)"`
	GroupAttribute      []string `json:"group_attributes" mapstructure:"group_attributes"`
	UseDNForGroupSearch bool     `json:"use_dn_for_group_search" mapstructure:"use_dn_for_group_search" default:"false"`
}

// APISettingsConfig mirrors homer-app api_settings for static API tokens (auth_token table).
type APISettingsConfig struct {
	// EnableTokenAccess allows authenticating with the configured header (default Auth-Token)
	// containing a raw token row from the coordinator settings DuckDB auth_token table instead of a JWT.
	EnableTokenAccess bool `json:"enable_token_access" mapstructure:"enable_token_access" default:"false"`
	// AuthTokenHeader is the HTTP header name for the token (default "Auth-Token").
	AuthTokenHeader string `json:"auth_token_header" mapstructure:"auth_token_header" default:"Auth-Token"`
}

// OAuthProviderConfig defines an OAuth2 provider entry
type OAuthProviderConfig struct {
	Enable        bool   `json:"enable" mapstructure:"enable" default:"false"`
	Name          string `json:"name" mapstructure:"name"`
	Position      int    `json:"position" mapstructure:"position" default:"0"`
	Type          string `json:"type" mapstructure:"type" default:"oauth2"`
	ProviderImage string `json:"provider_image" mapstructure:"provider_image"`
	ProviderName  string `json:"provider_name" mapstructure:"provider_name"`
	// URL is legacy (pre–authorization-code flow). Ignored when authorization code fields below are set.
	URL          string `json:"url" mapstructure:"url"`
	AutoRedirect bool   `json:"auto_redirect" mapstructure:"auto_redirect" default:"false"`
	CallbackURL  string `json:"callback_url" mapstructure:"callback_url"`

	// Authorization code flow (RFC 6749). When ClientID, AuthURL, TokenURL, RedirectURL, and ProfileURL
	// are all non-empty, the coordinator runs the full flow (state + optional PKCE, server-side code exchange).
	ClientID     string   `json:"client_id" mapstructure:"client_id"`
	ClientSecret string   `json:"client_secret" mapstructure:"client_secret"`
	AuthURL      string   `json:"auth_url" mapstructure:"auth_url"`
	TokenURL     string   `json:"token_url" mapstructure:"token_url"`
	RedirectURL  string   `json:"redirect_url" mapstructure:"redirect_url"`
	ProfileURL   string   `json:"profile_url" mapstructure:"profile_url"`
	Scopes       []string `json:"scopes" mapstructure:"scopes"`
	UsePKCE      bool     `json:"use_pkce" mapstructure:"use_pkce"`
	// SkipAutoProvision when true: user must already exist in DuckDB (matched by username or email).
	SkipAutoProvision bool     `json:"skip_auto_provision" mapstructure:"skip_auto_provision"`
	AdminGroups       []string `json:"admin_groups" mapstructure:"admin_groups"`
	GroupClaim        string   `json:"group_claim" mapstructure:"group_claim"`
}

// OAuthAuthorizationCodeConfigured reports whether oauth2_provider is set up for the authorization code flow.
func OAuthAuthorizationCodeConfigured(p *OAuthProviderConfig) bool {
	if p == nil || !p.Enable {
		return false
	}
	if strings.TrimSpace(p.Name) == "" {
		return false
	}
	if strings.TrimSpace(p.ClientID) == "" || strings.TrimSpace(p.AuthURL) == "" ||
		strings.TrimSpace(p.TokenURL) == "" || strings.TrimSpace(p.RedirectURL) == "" ||
		strings.TrimSpace(p.ProfileURL) == "" {
		return false
	}
	if !p.UsePKCE && strings.TrimSpace(p.ClientSecret) == "" {
		return false
	}
	return true
}

// LogConfig configures logging
// LogConfig configures logging
type LogConfig struct {
	Level  string   `json:"level" mapstructure:"level" default:"info"`
	JSON   bool     `json:"json" mapstructure:"json" default:"false"`
	Output []string `json:"output" mapstructure:"output"` // file, stdout, syslog

	File   FileLogConfig   `json:"file" mapstructure:"file"`
	Stdout StdoutLogConfig `json:"stdout" mapstructure:"stdout"`
	Syslog SyslogLogConfig `json:"syslog" mapstructure:"syslog"`
}

// FileLogConfig configures file logging with rotation
type FileLogConfig struct {
	Path          string `json:"path" mapstructure:"path" default:"/var/log/homer"`
	Name          string `json:"name" mapstructure:"name" default:"homer.log"`
	MaxAgeDays    int    `json:"max_age_days" mapstructure:"max_age_days" default:"7"`
	RotationHours int    `json:"rotation_hours" mapstructure:"rotation_hours" default:"24"`
	MaxSizeMB     int    `json:"max_size_mb" mapstructure:"max_size_mb" default:"100"`
}

// StdoutLogConfig configures stdout logging
type StdoutLogConfig struct {
	Colored bool `json:"colored" mapstructure:"colored" default:"true"`
}

// SyslogLogConfig configures syslog logging
type SyslogLogConfig struct {
	URI      string `json:"uri" mapstructure:"uri" default:""`
	Facility string `json:"facility" mapstructure:"facility" default:"local0"`
	Level    string `json:"level" mapstructure:"level" default:"LOG_INFO"`
	Tag      string `json:"tag" mapstructure:"tag" default:"homer"`
}

// PrometheusConfig configures Prometheus metrics
type PrometheusConfig struct {
	Enable     bool   `json:"enable" mapstructure:"enable" default:"false"`
	Host       string `json:"host" mapstructure:"host" default:"0.0.0.0"`
	Port       int    `json:"port" mapstructure:"port" default:"9090"`
	Path       string `json:"path" mapstructure:"path" default:"/metrics"`
	TargetIP   string `json:"target_ip" mapstructure:"target_ip" default:""`     // Comma-separated IPs for target mapping
	TargetName string `json:"target_name" mapstructure:"target_name" default:""` // Comma-separated names for target mapping
	// AgentLabel selects which HEP capture-agent field fills the Prometheus
	// "node_id" label for SIP/RTCP/RTP metrics:
	//   "node_id"   — HEP chunk 0x000c / heplify -hi (numeric; default)
	//   "node_name" — HEP chunk 0x0013 / heplify -hn (hostname string; falls
	//                 back to node_id when the name chunk is absent)
	AgentLabel string `json:"agent_label" mapstructure:"agent_label" default:"node_id"`
}

const (
	PrometheusAgentLabelNodeID   = "node_id"
	PrometheusAgentLabelNodeName = "node_name"
)

// NormalizePrometheusAgentLabel returns a supported agent_label value.
// Empty or unknown values fall back to node_id.
func NormalizePrometheusAgentLabel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case PrometheusAgentLabelNodeName:
		return PrometheusAgentLabelNodeName
	default:
		return PrometheusAgentLabelNodeID
	}
}

// Global configuration instance
var MainConfig *Config

// Load loads configuration from file and environment
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Config file settings
	if configPath != "" {
		// Check if configPath is a directory or a file
		info, err := os.Stat(configPath)
		if err == nil && info.IsDir() {
			// It's a directory - search for config file inside it
			v.SetConfigName("homer-core")
			v.SetConfigType("json")
			v.AddConfigPath(configPath)
		} else {
			// It's a file path (or doesn't exist yet)
			v.SetConfigFile(configPath)
		}
	} else {
		// Search for config file by candidate names and directories.
		// Prefer "homer.json" (default deb/rpm install name) before "homer-core.json".
		if found := findConfigFile(v); found != "" {
			v.SetConfigFile(found)
		} else {
			// Fallback: let viper search with its own logic.
			v.SetConfigName("homer-core")
			v.SetConfigType("json")
			v.AddConfigPath(".")
			v.AddConfigPath("./config")
			v.AddConfigPath("/etc/homer")
			v.AddConfigPath("/etc/homer-core")
			v.AddConfigPath("/usr/local/homer-core/etc")
			v.AddConfigPath("$HOME/.homer-core")
		}
	}

	// Environment variables
	v.SetEnvPrefix("HOMER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// AutomaticEnv() only reads ENV for keys that viper already "knows"
	// (via SetDefault / config file). bindAllEnvs registers BindEnv for
	// every scalar leaf of Config{} via reflection — without this,
	// variables like HOMER_COORDINATOR_JWT_SECRET were silently ignored.
	bindAllEnvs(v, reflect.TypeOf(Config{}), nil)

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found, use defaults + env vars
	}

	// Pre-scan ENV for indexed slices of structs
	// (HOMER_*_VOLUMES_<idx>_*, HOMER_COORDINATOR_NODES_<idx>_*).
	// Runs AFTER ReadInConfig so an operator can override volumes from
	// homer.json via ENV in docker-compose.
	applySliceEnvOverrides(v)

	// Pre-scan ENV for indexed slices of primitives
	// (HOMER_INGEST_SIP_ALEG_IDS_<idx>, HOMER_INGEST_SIP_CUSTOM_HEADERS_<idx>,
	// HOMER_LOG_OUTPUT_<idx>, ...). The documented docker-compose convention
	// uses this indexed form, which viper's AutomaticEnv() does not handle for
	// []string fields on its own.
	applyPrimitiveSliceEnvOverrides(v)

	// Pre-populate the struct from `default:"..."` struct tags BEFORE
	// Unmarshal. mapstructure's default decoder runs with ZeroFields=false,
	// so it will only override fields that are explicitly present in the
	// config source (file or env). Fields absent from the source keep the
	// values seeded here from struct tags. Without this step, sections
	// that the operator omits from homer.json (e.g. ingest.remote_logging.loki,
	// ingest.scripting, ingest.remote_logging.elasticsearch, ...) stay
	// zero-valued and silently drop the URL / Bulk / Timer / Buffer / Engine
	// numbers that the struct tags advertise. This is the same class of
	// bug that bit homer-lake on the OTLP / write_queue sections; see the
	// comment block on coordinator.correlation in setDefaults below for
	// the previous point fix that this systemic patch supersedes.
	var cfg Config
	defaults.SetDefaults(&cfg)

	// WeaklyTypedInput enables type coercion at the mapstructure layer:
	// strings "8" / "true" / "0.8" coming from ENV variables are
	// correctly decoded into int / bool / float64 both for scalar fields
	// and inside slice elements (volumes, nodes), which we feed into
	// viper from applySliceEnvOverrides() as []map[string]any.
	if err := v.Unmarshal(&cfg, func(dc *mapstructure.DecoderConfig) {
		dc.WeaklyTypedInput = true
		dc.DecodeHook = mapstructure.ComposeDecodeHookFunc(
			decodeCoordinatorAuthHook,
		)
	}); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	migrateCoordinatorOAuth2(&cfg, v)

	// Legacy safety net for fields that don't carry struct tags yet.
	applyDefaults(&cfg)

	if err := normalizeCoordinatorAuth(&cfg.Coordinator); err != nil {
		return nil, err
	}

	if err := validateDuckLakeCatalogTypes(&cfg); err != nil {
		return nil, err
	}

	if err := validateRetentionUnits(&cfg); err != nil {
		return nil, err
	}

	MainConfig = &cfg
	return &cfg, nil
}

// EnsureNodeDuckLakeVolumes synthesizes a single "default" volume when
// node.ducklake.volumes is empty but catalog_path / data_path are set.
// This keeps wizard-generated and older all-in-one configs working after
// Node began requiring an explicit volumes array.
func EnsureNodeDuckLakeVolumes(dl *DuckLakeConfig) {
	if dl == nil || len(dl.Volumes) > 0 {
		return
	}
	catalogPath := strings.TrimSpace(dl.CatalogPath)
	dataPath := strings.TrimSpace(dl.DataPath)
	if catalogPath == "" || dataPath == "" {
		return
	}
	catalogType := strings.TrimSpace(dl.CatalogType)
	if catalogType == "" {
		catalogType = "sqlite"
	}
	volType := "local"
	if strings.HasPrefix(dataPath, "s3://") {
		volType = "s3"
	}
	vol := VolumeConfig{
		Name:        "default",
		Type:        volType,
		CatalogType: catalogType,
		CatalogPath: catalogPath,
		Path:        dataPath,
	}
	if volType == "s3" {
		vol.S3Region = dl.S3.Region
		vol.S3AccessKeyID = dl.S3.AccessKeyID
		vol.S3SecretKey = dl.S3.SecretAccessKey
		vol.S3Endpoint = dl.S3.Endpoint
		vol.S3UseSSL = dl.S3.UseSSL
		vol.S3URLStyle = dl.S3.URLStyle
	}
	dl.Volumes = []VolumeConfig{vol}
}

// validateDuckLakeCatalogTypes ensures catalog_type is sqlite (or empty).
func validateDuckLakeCatalogTypes(cfg *Config) error {
	check := func(field, v string) error {
		s := strings.TrimSpace(v)
		if s == "" {
			return nil
		}
		if strings.EqualFold(s, "sqlite") {
			return nil
		}
		return fmt.Errorf("%s: DuckLake catalog — sqlite (got %q)", field, v)
	}
	if err := check("storage.ducklake.catalog_type", cfg.Storage.DuckLake.CatalogType); err != nil {
		return err
	}
	for i, v := range cfg.Storage.DuckLake.Volumes {
		if err := check(fmt.Sprintf("storage.ducklake.volumes[%d].catalog_type", i), v.CatalogType); err != nil {
			return err
		}
	}
	for i, v := range cfg.Storage.DuckLake.StoragePolicy.Volumes {
		if err := check(fmt.Sprintf("storage.ducklake.storage_policy.volumes[%d].catalog_type", i), v.CatalogType); err != nil {
			return err
		}
	}
	if err := check("node.ducklake.catalog_type", cfg.Node.DuckLake.CatalogType); err != nil {
		return err
	}
	for i, v := range cfg.Node.DuckLake.Volumes {
		if err := check(fmt.Sprintf("node.ducklake.volumes[%d].catalog_type", i), v.CatalogType); err != nil {
			return err
		}
	}
	return nil
}

// validateRetentionUnits fails fast on an unrecognized retention_unit so a
// typo doesn't silently reinterpret retention windows.
func validateRetentionUnits(cfg *Config) error {
	if _, err := NormalizeRetentionUnit(cfg.Storage.DuckLake.Compaction.RetentionUnit); err != nil {
		return fmt.Errorf("storage.ducklake.compaction.retention_unit: %w", err)
	}
	if _, err := NormalizeRetentionUnit(cfg.Node.DuckLake.Compaction.RetentionUnit); err != nil {
		return fmt.Errorf("node.ducklake.compaction.retention_unit: %w", err)
	}
	return nil
}

func coordinatorOAuth2Active(p *OAuthProviderConfig) bool {
	if p == nil || !p.Enable {
		return false
	}
	if strings.TrimSpace(p.Name) == "" {
		return false
	}
	return OAuthAuthorizationCodeConfigured(p)
}

func pickCoordinatorOAuthFromLegacyList(configs []OAuthProviderConfig) *OAuthProviderConfig {
	var enabled []OAuthProviderConfig
	for _, row := range configs {
		if !row.Enable {
			continue
		}
		if strings.TrimSpace(row.Name) == "" {
			continue
		}
		if !OAuthAuthorizationCodeConfigured(&row) {
			continue
		}
		enabled = append(enabled, row)
	}
	if len(enabled) == 0 {
		return nil
	}
	sort.Slice(enabled, func(i, j int) bool {
		if enabled[i].Position != enabled[j].Position {
			return enabled[i].Position < enabled[j].Position
		}
		return strings.ToLower(enabled[i].Name) < strings.ToLower(enabled[j].Name)
	})
	if len(enabled) > 1 {
		var ignored []string
		for _, c := range enabled[1:] {
			ignored = append(ignored, c.Name)
		}
		slog.Warn("config: multiple enabled rows in deprecated coordinator.oauth2_providers; using one",
			"kept", enabled[0].Name, "ignored", strings.Join(ignored, ", "))
	}
	cp := enabled[0]
	return &cp
}

// migrateCoordinatorOAuth2 maps deprecated coordinator.oauth2_providers ([]) into OAuth2Provider.
func migrateCoordinatorOAuth2(cfg *Config, v *viper.Viper) {
	c := &cfg.Coordinator
	if coordinatorOAuth2Active(c.OAuth2Provider) {
		if v.IsSet("coordinator.oauth2_providers") {
			slog.Warn("config: coordinator.oauth2_providers is deprecated and ignored; use coordinator.oauth2_provider")
		}
		return
	}
	if !v.IsSet("coordinator.oauth2_providers") {
		return
	}
	var legacy []OAuthProviderConfig
	if err := v.UnmarshalKey("coordinator.oauth2_providers", &legacy); err != nil || len(legacy) == 0 {
		return
	}
	chosen := pickCoordinatorOAuthFromLegacyList(legacy)
	if chosen == nil {
		return
	}
	cp := *chosen
	c.OAuth2Provider = &cp
	slog.Warn("config: coordinator.oauth2_providers is deprecated; use coordinator.oauth2_provider (object)",
		"migrated_provider", cp.Name)
}

// setDefaults sets default values in viper
func setDefaults(v *viper.Viper) {
	// Ingest defaults
	v.SetDefault("ingest.enable", true)
	v.SetDefault("ingest.udp.enable", true)
	v.SetDefault("ingest.udp.host", "0.0.0.0")
	v.SetDefault("ingest.udp.port", 9060)
	v.SetDefault("ingest.udp.multicore", true)
	v.SetDefault("ingest.worker_metrics_flush_packets", 0)
	v.SetDefault("ingest.http.enable", true)
	v.SetDefault("ingest.http.host", "0.0.0.0")
	v.SetDefault("ingest.http.port", 9080)
	v.SetDefault("ingest.hep.hepv2_enable", true)
	v.SetDefault("ingest.hep.hepv3_enable", true)
	v.SetDefault("ingest.hep.protobuf_enable", true)
	v.SetDefault("ingest.hep.deduplicate", false)

	// Storage defaults
	v.SetDefault("storage.enable", true)
	v.SetDefault("storage.ducklake.catalog_type", "sqlite")
	v.SetDefault("storage.ducklake.catalog_path", "homer_catalog.sqlite")
	v.SetDefault("storage.ducklake.data_path", "/var/lib/homer/parquet")
	v.SetDefault("storage.ducklake.lake_name", "homer_lake")
	v.SetDefault("storage.ducklake.batch_size", 10000)
	v.SetDefault("storage.ducklake.flush_interval_sec", 60)

	// Node defaults
	v.SetDefault("node.enable", false)
	v.SetDefault("node.flight_server.enable", true)
	v.SetDefault("node.flight_server.host", "0.0.0.0")
	v.SetDefault("node.flight_server.port", 50051)
	v.SetDefault("node.flight_server.max_message_size", 16777216)
	v.SetDefault("node.flightsql_server.enable", false)
	v.SetDefault("node.flightsql_server.host", "0.0.0.0")
	v.SetDefault("node.flightsql_server.port", 50055)
	v.SetDefault("node.flightsql_server.catalog_refresh_interval_sec", 30)
	v.SetDefault("coordinator.flightsql_server.enable", false)
	v.SetDefault("coordinator.flightsql_server.host", "0.0.0.0")
	v.SetDefault("coordinator.flightsql_server.port", 32010)
	v.SetDefault("coordinator.flightsql_server.catalog_refresh_interval_sec", 30)
	v.SetDefault("node.ducklake.catalog_type", "sqlite")
	v.SetDefault("node.ducklake.catalog_path", "homer_catalog.sqlite")
	v.SetDefault("node.ducklake.data_path", "/var/lib/homer/parquet")
	v.SetDefault("node.ducklake.lake_name", "homer_lake")

	// Coordinator defaults
	v.SetDefault("coordinator.enable", false)
	v.SetDefault("coordinator.http_server.enable", true)
	v.SetDefault("coordinator.http_server.host", "0.0.0.0")
	v.SetDefault("coordinator.http_server.port", 8080)
	v.SetDefault("coordinator.http_server.gamedata_dir", "/usr/local/homer-core/gamedata")
	v.SetDefault("coordinator.settings_db_path", "/var/lib/homer/homer_settings.duckdb")
	v.SetDefault("coordinator.lake_name", "homer_lake")
	v.SetDefault("coordinator.jwt.expire_hours", 24)
	v.SetDefault("coordinator.auth.admin_user", "admin")
	v.SetDefault("coordinator.ldap.enable", false)
	v.SetDefault("coordinator.ldap.port", 389)
	v.SetDefault("coordinator.ldap.skip_tls", true)
	v.SetDefault("coordinator.ldap.insecure_skip_verify", true)
	v.SetDefault("coordinator.ldap.use_ssl", false)
	v.SetDefault("coordinator.ldap.anonymous", false)
	v.SetDefault("coordinator.ldap.admin_mode", false)
	v.SetDefault("coordinator.ldap.user_mode", false)
	v.SetDefault("coordinator.ldap.use_dn_for_group_search", false)

	// Coordinator Lua correlation defaults.
	//
	// The struct uses `default:"..."` tags for documentation, but viper
	// does not read those tags — zero-value fields would leave
	// SeedDefault=false and the Settings → Scripts UI empty on any
	// installation that does not carry an explicit `coordinator.correlation`
	// section in its config. Register real defaults here so the UI ships
	// with the bundled templates even on upgrades from older versions.
	v.SetDefault("coordinator.correlation.enable", false)
	v.SetDefault("coordinator.correlation.sql_timeout_ms", 5000)
	v.SetDefault("coordinator.correlation.script_timeout_ms", 3000)
	v.SetDefault("coordinator.correlation.max_sql_calls", 16)
	v.SetDefault("coordinator.correlation.max_sql_rows", 10000)
	v.SetDefault("coordinator.correlation.sync_interval_sec", 30)
	v.SetDefault("coordinator.correlation.seed_default", true)

	// Live HEP stream defaults. Opt-in on both sides (publisher ring
	// buffer and coordinator WebSocket); the hot path short-circuits
	// to a no-op when Enable is false, so it is safe to leave the
	// defaults here regardless of operator choice.
	v.SetDefault("ingest.hep_stream.enable", true)
	v.SetDefault("ingest.hep_stream.buffer_size", 10000)
	v.SetDefault("ingest.hep_stream.max_subscribers", 32)
	v.SetDefault("ingest.hep_stream.per_sub_queue_len", 256)
	v.SetDefault("ingest.hep_stream.rate_per_sub_pps", 500)

	// OTLP receiver defaults. Whole feature stays off by default —
	// operators opt in by setting ingest.otlp.enable=true. The
	// per-listener / per-sink defaults below are seeded so that
	// enabling the feature without filling in nested keys produces a
	// canonical OTLP/gRPC :4317 + OTLP/HTTP :4318 deployment that
	// stores traces/metrics in DuckLake and forwards logs as HEP100.
	v.SetDefault("ingest.otlp.enable", false)
	v.SetDefault("ingest.otlp.max_recv_msg_bytes", 4194304)
	v.SetDefault("ingest.otlp.grpc.enable", true)
	v.SetDefault("ingest.otlp.grpc.listen", ":4317")
	v.SetDefault("ingest.otlp.http.enable", true)
	v.SetDefault("ingest.otlp.http.listen", ":4318")
	v.SetDefault("ingest.otlp.http.read_timeout_sec", 30)
	v.SetDefault("ingest.otlp.http.write_timeout_sec", 30)
	v.SetDefault("ingest.otlp.sinks.store_traces", true)
	v.SetDefault("ingest.otlp.sinks.store_metrics", true)
	v.SetDefault("ingest.otlp.sinks.store_logs", true)
	// InfluxDB Line Protocol receiver defaults. Off by default; when
	// enabled, the receiver listens on :8086 (the canonical InfluxDB
	// v1 port) and lazily creates one DuckLake table per measurement
	// with configurable `table_prefix` (default empty = measurement name).
	v.SetDefault("ingest.line_protocol.enable", false)
	v.SetDefault("ingest.line_protocol.listen", ":8086")
	v.SetDefault("ingest.line_protocol.max_body_bytes", 8388608)
	v.SetDefault("ingest.line_protocol.default_precision", "ns")
	v.SetDefault("ingest.line_protocol.default_db", "")
	v.SetDefault("ingest.line_protocol.table_prefix", "")
	v.SetDefault("ingest.line_protocol.allow_hep_sip_call", false)
	v.SetDefault("ingest.line_protocol.read_timeout_sec", 30)
	v.SetDefault("ingest.line_protocol.write_timeout_sec", 30)
	v.SetDefault("coordinator.hep_stream.enable", true)
	v.SetDefault("coordinator.hep_stream.allow_payload", false)
	v.SetDefault("coordinator.hep_stream.fan_out_timeout_ms", 2000)
	v.SetDefault("coordinator.hep_stream.history_limit", 200)
	for _, cat := range WidgetControlCategories {
		v.SetDefault("coordinator.widgets.control."+cat, true)
	}

	// MCP defaults
	v.SetDefault("mcp.enable", false)
	v.SetDefault("mcp.mode", "hybrid")
	v.SetDefault("mcp.homer_base_url", "http://127.0.0.1:8080")
	v.SetDefault("mcp.default_limit", 100)
	v.SetDefault("mcp.sql_default_limit", 100)
	v.SetDefault("mcp.request_timeout_sec", 30)
	v.SetDefault("mcp.llm.enable", false)
	v.SetDefault("mcp.llm.provider", "openai")
	v.SetDefault("mcp.llm.base_url", "https://api.openai.com/v1")
	v.SetDefault("mcp.llm.model", "gpt-4o-mini")
	v.SetDefault("mcp.llm.temperature", 0.1)
	v.SetDefault("mcp.llm.max_tokens", 400)
	v.SetDefault("mcp.llm.timeout_sec", 15)

	// Log defaults
	v.SetDefault("log.level", "info")
	v.SetDefault("log.output", []string{"stdout"})
	v.SetDefault("log.json", false)

	// Prometheus defaults
	v.SetDefault("prometheus.enable", false)
	v.SetDefault("prometheus.host", "0.0.0.0")
	v.SetDefault("prometheus.port", 9090)
	v.SetDefault("prometheus.path", "/metrics")
	v.SetDefault("prometheus.agent_label", "node_id")
}

// applyDefaults applies default values for missing fields
func applyDefaults(cfg *Config) {
	// Ingest defaults
	if cfg.Ingest.UDP.Port == 0 {
		cfg.Ingest.UDP.Port = 9060
	}
	if cfg.Ingest.HTTP.Port == 0 {
		cfg.Ingest.HTTP.Port = 9080
	}

	// Storage defaults
	if cfg.Storage.DuckLake.CatalogType == "" {
		cfg.Storage.DuckLake.CatalogType = "sqlite"
	}
	if cfg.Storage.DuckLake.LakeName == "" {
		cfg.Storage.DuckLake.LakeName = "homer_lake"
	}
	if cfg.Storage.DuckLake.BatchSize == 0 {
		cfg.Storage.DuckLake.BatchSize = 10000
	}
	if cfg.Storage.DuckLake.FlushInterval == 0 {
		cfg.Storage.DuckLake.FlushInterval = 30
	}

	// Node defaults
	if cfg.Node.FlightServer.Port == 0 {
		cfg.Node.FlightServer.Port = 50051
	}
	if cfg.Node.FlightServer.MaxMessageSize == 0 {
		cfg.Node.FlightServer.MaxMessageSize = 16777216
	}
	if cfg.Node.DuckLake.CatalogType == "" {
		cfg.Node.DuckLake.CatalogType = "sqlite"
	}
	if cfg.Node.DuckLake.LakeName == "" {
		cfg.Node.DuckLake.LakeName = "homer_lake"
	}
	// Wizard / legacy configs used node.ducklake.catalog_path + data_path
	// without volumes; Node now requires node.ducklake.volumes.
	EnsureNodeDuckLakeVolumes(&cfg.Node.DuckLake)
	if cfg.Node.FlightSQLServer.CatalogRefreshIntervalSec <= 0 {
		cfg.Node.FlightSQLServer.CatalogRefreshIntervalSec = 30
	}
	if cfg.Node.FlightSQLServer.Enable && cfg.Node.FlightSQLServer.Port == 0 {
		cfg.Node.FlightSQLServer.Port = 50055
	}

	// Coordinator defaults
	if cfg.Coordinator.HTTPServer.Port == 0 {
		cfg.Coordinator.HTTPServer.Port = 8080
	}
	if cfg.Coordinator.QueryTimeoutSec <= 0 {
		cfg.Coordinator.QueryTimeoutSec = 30
	}
	if cfg.Coordinator.SettingsDBPath == "" {
		cfg.Coordinator.SettingsDBPath = "/var/lib/homer/homer_settings.duckdb"
	}
	if cfg.Coordinator.TransactionViewMaxOpens <= 0 {
		cfg.Coordinator.TransactionViewMaxOpens = 3
	}
	if cfg.Coordinator.TransactionViewMaxOpens > 1000 {
		cfg.Coordinator.TransactionViewMaxOpens = 1000
	}
	if cfg.Coordinator.IPAliasCacheTTLSec <= 0 {
		cfg.Coordinator.IPAliasCacheTTLSec = 30
	}
	if cfg.Coordinator.IPAliasCacheTTLSec < 5 {
		cfg.Coordinator.IPAliasCacheTTLSec = 5
	}
	if cfg.Coordinator.IPAliasCacheTTLSec > 86400 {
		cfg.Coordinator.IPAliasCacheTTLSec = 86400
	}
	if cfg.Coordinator.LakeName == "" {
		cfg.Coordinator.LakeName = "homer_lake"
	}
	if cfg.Coordinator.FlightSQLServer.CatalogRefreshIntervalSec <= 0 {
		cfg.Coordinator.FlightSQLServer.CatalogRefreshIntervalSec = 30
	}
	if cfg.Coordinator.FlightSQLServer.Enable && cfg.Coordinator.FlightSQLServer.Port == 0 {
		cfg.Coordinator.FlightSQLServer.Port = 32010
	}
	if cfg.Coordinator.JWT.ExpireHours == 0 {
		cfg.Coordinator.JWT.ExpireHours = 24
	}
	if cfg.Coordinator.JWT.CookieEnable == nil {
		enable := true
		cfg.Coordinator.JWT.CookieEnable = &enable
	}
	if cfg.Coordinator.JWT.CookieName == "" {
		cfg.Coordinator.JWT.CookieName = "homer_session"
	}
	if cfg.Coordinator.JWT.CookieSameSite == "" {
		cfg.Coordinator.JWT.CookieSameSite = "Lax"
	}
	if cfg.Coordinator.Auth.AdminUser == "" {
		cfg.Coordinator.Auth.AdminUser = "admin"
	}
	if cfg.Coordinator.APISettings.AuthTokenHeader == "" {
		cfg.Coordinator.APISettings.AuthTokenHeader = "Auth-Token"
	}

	// Coordinator Lua correlation numeric fallbacks. SeedDefault is a
	// bool, so its default is carried by viper (setDefaults above) — we
	// cannot safely promote zero-value booleans here without overriding
	// an explicit `seed_default: false` in the user's config.
	if cfg.Coordinator.Correlation.SQLTimeoutMS <= 0 {
		cfg.Coordinator.Correlation.SQLTimeoutMS = 5000
	}
	if cfg.Coordinator.Correlation.ScriptTimeoutMS <= 0 {
		cfg.Coordinator.Correlation.ScriptTimeoutMS = 3000
	}
	if cfg.Coordinator.Correlation.MaxSQLCalls <= 0 {
		cfg.Coordinator.Correlation.MaxSQLCalls = 16
	}
	if cfg.Coordinator.Correlation.MaxSQLRows <= 0 {
		cfg.Coordinator.Correlation.MaxSQLRows = 10000
	}
	if cfg.Coordinator.Correlation.SyncIntervalSec < 0 {
		cfg.Coordinator.Correlation.SyncIntervalSec = 30
	}

	// Live HEP stream numeric fallbacks. Booleans (Enable, AllowPayload)
	// stay under viper's control so an operator who explicitly writes
	// `enable: false` in YAML doesn't get it silently flipped back.
	if cfg.Ingest.HepStream.BufferSize <= 0 {
		cfg.Ingest.HepStream.BufferSize = 10000
	}
	if cfg.Ingest.HepStream.MaxSubscribers <= 0 {
		cfg.Ingest.HepStream.MaxSubscribers = 32
	}
	if cfg.Ingest.HepStream.PerSubQueueLen <= 0 {
		cfg.Ingest.HepStream.PerSubQueueLen = 256
	}
	if cfg.Ingest.HepStream.RatePerSubPPS < 0 {
		cfg.Ingest.HepStream.RatePerSubPPS = 500
	}
	if cfg.Coordinator.HepStream.FanOutTimeoutMS <= 0 {
		cfg.Coordinator.HepStream.FanOutTimeoutMS = 2000
	}
	if cfg.Coordinator.HepStream.HistoryLimit < 0 {
		cfg.Coordinator.HepStream.HistoryLimit = 200
	}

	// OTLP receiver numeric fallbacks. Booleans (Enable, GRPC.Enable,
	// HTTP.Enable, Sinks.*) stay under viper/struct-tag control so an
	// operator who explicitly writes `enable: false` is honoured.
	if cfg.Ingest.OTLP.MaxRecvMsgBytes <= 0 {
		cfg.Ingest.OTLP.MaxRecvMsgBytes = 4194304
	}
	if cfg.Ingest.OTLP.GRPC.Listen == "" {
		cfg.Ingest.OTLP.GRPC.Listen = ":4317"
	}
	if cfg.Ingest.OTLP.HTTP.Listen == "" {
		cfg.Ingest.OTLP.HTTP.Listen = ":4318"
	}
	if cfg.Ingest.OTLP.HTTP.ReadTimeoutSec <= 0 {
		cfg.Ingest.OTLP.HTTP.ReadTimeoutSec = 30
	}
	if cfg.Ingest.OTLP.HTTP.WriteTimeoutSec <= 0 {
		cfg.Ingest.OTLP.HTTP.WriteTimeoutSec = 30
	}

	// Line Protocol receiver numeric / string fallbacks. Booleans
	// (Enable) stay under viper/struct-tag control so an operator who
	// explicitly writes `enable: false` is honoured.
	if cfg.Ingest.LineProto.Listen == "" {
		cfg.Ingest.LineProto.Listen = ":8086"
	}
	if cfg.Ingest.LineProto.MaxBodyBytes <= 0 {
		cfg.Ingest.LineProto.MaxBodyBytes = 8 * 1024 * 1024
	}
	if cfg.Ingest.LineProto.DefaultPrecision == "" {
		cfg.Ingest.LineProto.DefaultPrecision = "ns"
	}
	if cfg.Ingest.LineProto.ReadTimeoutSec <= 0 {
		cfg.Ingest.LineProto.ReadTimeoutSec = 30
	}
	if cfg.Ingest.LineProto.WriteTimeoutSec <= 0 {
		cfg.Ingest.LineProto.WriteTimeoutSec = 30
	}
	if cfg.Ingest.Siprec.SIPPort <= 0 {
		cfg.Ingest.Siprec.SIPPort = 5062
	}
	if len(cfg.Ingest.Siprec.Transports) == 0 {
		cfg.Ingest.Siprec.Transports = []string{"udp", "tcp"}
	}
	if cfg.Ingest.Siprec.UserAgent == "" {
		cfg.Ingest.Siprec.UserAgent = "homer-siprec-srs"
	}
	if cfg.Ingest.Siprec.NodeID == "" {
		cfg.Ingest.Siprec.NodeID = "homer"
	}
	if cfg.Ingest.Vqrtcp.SIPPort <= 0 {
		cfg.Ingest.Vqrtcp.SIPPort = 5063
	}
	if len(cfg.Ingest.Vqrtcp.Transports) == 0 {
		cfg.Ingest.Vqrtcp.Transports = []string{"udp", "tcp"}
	}
	if len(cfg.Ingest.Vqrtcp.Methods) == 0 {
		cfg.Ingest.Vqrtcp.Methods = []string{"PUBLISH", "MESSAGE"}
	}
	if cfg.Ingest.Vqrtcp.AsyncQueueDepth <= 0 {
		cfg.Ingest.Vqrtcp.AsyncQueueDepth = 1024
	}
	if cfg.Ingest.Vqrtcp.NodeID == "" {
		cfg.Ingest.Vqrtcp.NodeID = "homer"
	}
	cfg.Coordinator.LineProtoTablePrefix = cfg.Ingest.LineProto.TablePrefix

	// MCP defaults
	if cfg.MCP.Mode == "" {
		cfg.MCP.Mode = "hybrid"
	}
	if cfg.MCP.HomerBaseURL == "" {
		cfg.MCP.HomerBaseURL = "http://127.0.0.1:8080"
	}
	if cfg.MCP.DefaultLimit <= 0 {
		cfg.MCP.DefaultLimit = 100
	}
	if cfg.MCP.SQLDefaultLimit <= 0 {
		cfg.MCP.SQLDefaultLimit = 100
	}
	if cfg.MCP.RequestTimeoutSec <= 0 {
		cfg.MCP.RequestTimeoutSec = 30
	}
	if cfg.MCP.LLM.Provider == "" {
		cfg.MCP.LLM.Provider = "openai"
	}
	if cfg.MCP.LLM.BaseURL == "" {
		cfg.MCP.LLM.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.MCP.LLM.Model == "" {
		cfg.MCP.LLM.Model = "gpt-4o-mini"
	}
	if cfg.MCP.LLM.TimeoutSec <= 0 {
		cfg.MCP.LLM.TimeoutSec = 15
	}
	if cfg.MCP.LLM.MaxTokens <= 0 {
		cfg.MCP.LLM.MaxTokens = 400
	}

	// Resolve node HTTP / Airport Bearer token before wiring coordinator nodes.
	// Exposed binds (default 0.0.0.0) with empty auth_token get an auto-generated
	// persisted token (GHSA-rm5w-rqr7-2h54). Loopback-only binds may stay empty.
	if cfg.Node.Enable {
		catalogPath := strings.TrimSpace(cfg.Node.DuckLake.CatalogPath)
		if catalogPath == "" && len(cfg.Node.DuckLake.Volumes) > 0 {
			catalogPath = strings.TrimSpace(cfg.Node.DuckLake.Volumes[0].CatalogPath)
		}
		tok, _, _, err := ResolveNodeFlightAuthToken(
			cfg.Node.FlightServer.Host,
			cfg.Node.FlightServer.AuthToken,
			catalogPath,
		)
		if err == nil {
			cfg.Node.FlightServer.AuthToken = tok
		}
		if cfg.Node.FlightSQLServer.Enable {
			fsqlTok := strings.TrimSpace(cfg.Node.FlightSQLServer.AuthToken)
			if fsqlTok == "" {
				fsqlTok = strings.TrimSpace(cfg.Node.FlightServer.AuthToken)
			}
			tok, _, _, err := ResolveRequiredAuthToken(fsqlTok, catalogPath)
			if err == nil {
				cfg.Node.FlightSQLServer.AuthToken = tok
			}
		}
	}

	if cfg.Coordinator.Enable && cfg.Coordinator.FlightSQLServer.Enable {
		fsqlTok := strings.TrimSpace(cfg.Coordinator.FlightSQLServer.AuthToken)
		if fsqlTok == "" && cfg.Node.Enable {
			fsqlTok = strings.TrimSpace(cfg.Node.FlightSQLServer.AuthToken)
			if fsqlTok == "" {
				fsqlTok = strings.TrimSpace(cfg.Node.FlightServer.AuthToken)
			}
		}
		settingsPath := strings.TrimSpace(cfg.Coordinator.SettingsDBPath)
		tok, _, _, err := ResolveRequiredAuthToken(fsqlTok, settingsPath)
		if err == nil {
			cfg.Coordinator.FlightSQLServer.AuthToken = tok
		}
	}

	// If no nodes configured but node is enabled locally, add local node
	if cfg.Coordinator.Enable && len(cfg.Coordinator.Nodes) == 0 && cfg.Node.Enable {
		cfg.Coordinator.Nodes = []NodeEndpoint{
			{
				Name:  "local",
				Host:  "127.0.0.1",
				Port:  cfg.Node.FlightServer.Port,
				Token: cfg.Node.FlightServer.AuthToken,
			},
		}
	}

	// Same-process local nodes: copy flight_server.auth_token into empty node tokens
	// so FlightService can send Authorization: Bearer on POST /query.
	if cfg.Node.Enable && cfg.Node.FlightServer.AuthToken != "" {
		for i := range cfg.Coordinator.Nodes {
			n := &cfg.Coordinator.Nodes[i]
			if n.Token != "" {
				continue
			}
			if IsLoopbackBindHost(n.Host) && n.Port == cfg.Node.FlightServer.Port {
				n.Token = cfg.Node.FlightServer.AuthToken
			}
		}
	}

	// Prometheus defaults
	if cfg.Prometheus.Port == 0 {
		cfg.Prometheus.Port = 9090
	}
	if cfg.Prometheus.Path == "" {
		cfg.Prometheus.Path = "/metrics"
	}
	cfg.Prometheus.AgentLabel = NormalizePrometheusAgentLabel(cfg.Prometheus.AgentLabel)
}

// SaveExample writes an example configuration file
func SaveExample(path string) error {
	cfg := Config{
		Ingest: IngestConfig{
			Enable: true,
			UDP: UDPServerConfig{
				Enable:    true,
				Host:      "0.0.0.0",
				Port:      9060,
				Multicore: true,
			},
			TCP: TCPServerConfig{
				Enable: false,
				Host:   "0.0.0.0",
				Port:   9060,
			},
			HTTP: HTTPServerConfig{
				Enable:          true,
				Host:            "0.0.0.0",
				Port:            9080,
				WebSocketEnable: false,
			},
			HEP: HEPConfig{
				HepV2Enable:    true,
				HepV3Enable:    true,
				ProtobufEnable: true,
				Deduplicate:    false,
			},
			Siprec: SiprecConfig{
				Enable: false,
			},
			Vqrtcp: VqrtcpConfig{
				Enable: false,
			},
		},
		Storage: StorageConfig{
			Enable: true,
			DuckLake: DuckLakeConfig{
				CatalogType:   "sqlite",
				CatalogPath:   "/data/homer_catalog.sqlite",
				DataPath:      "/data/homer_parquet",
				LakeName:      "homer_lake",
				BatchSize:     10000,
				FlushInterval: 30,
			},
		},
		Node: NodeConfig{
			Enable: true,
			FlightServer: FlightServerConfig{
				Enable:         true,
				Host:           "0.0.0.0",
				Port:           50051,
				MaxMessageSize: 16777216,
			},
			FlightSQLServer: FlightSQLServerConfig{
				Enable:                    false,
				Host:                      "0.0.0.0",
				Port:                      50055,
				AuthToken:                 "",
				CatalogRefreshIntervalSec: 30,
			},
			DuckLake: DuckLakeConfig{
				CatalogType: "sqlite",
				CatalogPath: "/data/homer_catalog.sqlite",
				DataPath:    "/data/homer_parquet",
				LakeName:    "homer_lake",
			},
		},
		Coordinator: CoordinatorConfig{
			Enable: true,
			HTTPServer: CoordinatorHTTPServerConfig{
				Enable: true,
				Host:   "0.0.0.0",
				Port:   8080,
			},
			FlightSQLServer: FlightSQLServerConfig{
				Enable:                    false,
				Host:                      "0.0.0.0",
				Port:                      32010,
				AuthToken:                 "",
				CatalogRefreshIntervalSec: 30,
			},
			Nodes: []NodeEndpoint{
				{
					Name:          "local",
					Host:          "127.0.0.1",
					Port:          50051,
					FlightSQLPort: 0,
				},
			},
			JWT: JWTConfig{
				Secret:      "change-me-in-production",
				ExpireHours: 24,
			},
			Auth: AuthConfig{
				AdminUser:         "admin",
				AdminPasswordHash: "",
			},
			LDAP: LDAPConfig{},
		},
		MCP: MCPConfig{
			Enable:            false,
			Mode:              "hybrid",
			HomerBaseURL:      "http://127.0.0.1:8080",
			HomerToken:        "",
			DefaultLimit:      100,
			SQLDefaultLimit:   100,
			RequestTimeoutSec: 30,
			LLM: MCPLLMConfig{
				Enable:      false,
				Provider:    "openai",
				BaseURL:     "https://api.openai.com/v1",
				APIKey:      "",
				Model:       "gpt-4o-mini",
				Temperature: 0.1,
				MaxTokens:   400,
				TimeoutSec:  15,
			},
		},
		Log: LogConfig{
			Level:  "info",
			JSON:   false,
			Output: []string{"stdout"},
			File: FileLogConfig{
				Path:          "/var/log/homer",
				Name:          "homer.log",
				MaxAgeDays:    7,
				RotationHours: 24,
				MaxSizeMB:     100,
			},
			Stdout: StdoutLogConfig{
				Colored: true,
			},
			Syslog: SyslogLogConfig{
				Facility: "local0",
				Level:    "LOG_INFO",
				Tag:      "homer",
			},
		},
		Prometheus: PrometheusConfig{
			Enable:     false,
			Host:       "0.0.0.0",
			Port:       9090,
			Path:       "/metrics",
			AgentLabel: "node_id",
		},
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Write JSON file
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// findConfigFile searches well-known directories for a Homer config file.
// It checks multiple candidate names so that both deb/rpm installs (homer.json)
// and source/dev installs (homer-core.json) are discovered automatically.
// Returns the first matching absolute path, or "" if none found.
func findConfigFile(v *viper.Viper) string {
	home, _ := os.UserHomeDir()
	searchDirs := []string{
		".",
		"./config",
		"/etc/homer",
		"/etc/homer-core",
		"/usr/local/homer-core/etc",
		filepath.Join(home, ".homer-core"),
	}
	names := []string{"homer", "homer-core"}
	exts := []string{".json", ".yaml", ".yml"}

	for _, dir := range searchDirs {
		for _, name := range names {
			for _, ext := range exts {
				candidate := filepath.Join(dir, name+ext)
				if _, err := os.Stat(candidate); err == nil {
					abs, err := filepath.Abs(candidate)
					if err == nil {
						return abs
					}
					return candidate
				}
			}
		}
	}
	return ""
}

// GetEnabledModules returns a list of enabled module names
func (c *Config) GetEnabledModules() []string {
	var modules []string
	if c.Ingest.Enable {
		modules = append(modules, "ingest")
	}
	if c.Storage.Enable {
		modules = append(modules, "storage")
	}
	if c.Node.Enable {
		modules = append(modules, "node")
	}
	if c.Coordinator.Enable {
		modules = append(modules, "coordinator")
	}
	if c.MCP.Enable {
		modules = append(modules, "mcp")
	}
	return modules
}
