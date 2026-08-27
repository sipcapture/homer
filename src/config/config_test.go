// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeTmpConfig writes the given JSON body into a temporary
// homer-core.json inside t.TempDir() and returns the resulting path.
func writeTmpConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "homer-core.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("failed to write tmp config: %v", err)
	}
	return path
}

// TestLoad_LokiPartialEnableKeepsTagDefaults pins the regression that
// motivated the v5.0.62-equivalent fix here: an operator turning Loki on
// with `{"loki": {"enable": true}}` must still get the URL/Bulk/Timer/
// Buffer numbers from struct-tag defaults. Before defaults.SetDefaults
// was wired in, these would all stay at Go zero values and the Loki
// pusher would either run with Bulk=0 (no batching) or crash trying to
// POST to "".
func TestLoad_LokiPartialEnableKeepsTagDefaults(t *testing.T) {
	path := writeTmpConfig(t, `{
  "ingest": { "enable": true },
  "remote_logging": {
    "loki": { "enable": true }
  }
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	loki := cfg.RemoteLogging.Loki
	if !loki.Enable {
		t.Fatalf("loki.enable: want true (explicit), got false")
	}
	if loki.URL != "http://localhost:3100/loki/api/v1/push" {
		t.Errorf("loki.url: want default, got %q", loki.URL)
	}
	if loki.Bulk != 400 {
		t.Errorf("loki.bulk: want 400, got %d", loki.Bulk)
	}
	if loki.Timer != 4 {
		t.Errorf("loki.timer: want 4, got %d", loki.Timer)
	}
	if loki.Buffer != 100000 {
		t.Errorf("loki.buffer: want 100000, got %d", loki.Buffer)
	}
}

// TestLoad_ElasticsearchPartialEnableKeepsTagDefaults mirrors the Loki
// case for Elasticsearch.
func TestLoad_ElasticsearchPartialEnableKeepsTagDefaults(t *testing.T) {
	path := writeTmpConfig(t, `{
  "ingest": { "enable": true },
  "remote_logging": {
    "elasticsearch": { "enable": true }
  }
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	es := cfg.RemoteLogging.Elasticsearch
	if !es.Enable {
		t.Fatalf("elasticsearch.enable: want true (explicit), got false")
	}
	if es.Addr != "http://localhost:9200" {
		t.Errorf("elasticsearch.addr: want default, got %q", es.Addr)
	}
	if !es.Discovery {
		t.Errorf("elasticsearch.discovery: want true (default), got false")
	}
	if !es.IndexDaily {
		t.Errorf("elasticsearch.index_daily: want true (default), got false")
	}
	if es.IndexName != "hep" {
		t.Errorf("elasticsearch.index_name: want \"hep\", got %q", es.IndexName)
	}
}

// TestLoad_LineProtoPartialEnableKeepsTagDefaults mirrors the Loki case
// for InfluxDB Line Protocol.
func TestLoad_LineProtoPartialEnableKeepsTagDefaults(t *testing.T) {
	path := writeTmpConfig(t, `{
  "ingest": { "enable": true },
  "remote_logging": {
    "lineproto": { "enable": true }
  }
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	lp := cfg.RemoteLogging.LineProto
	if !lp.Enable {
		t.Fatalf("lineproto.enable: want true (explicit), got false")
	}
	if lp.URL != "http://localhost:8086/write?db=hep" {
		t.Errorf("lineproto.url: want default, got %q", lp.URL)
	}
	if lp.Bulk != 400 {
		t.Errorf("lineproto.bulk: want 400, got %d", lp.Bulk)
	}
	if lp.Timer != 4 {
		t.Errorf("lineproto.timer: want 4, got %d", lp.Timer)
	}
	if lp.Buffer != 100000 {
		t.Errorf("lineproto.buffer: want 100000, got %d", lp.Buffer)
	}
}

// TestLoad_ScriptingPartialEnableKeepsEngine ensures the scripting
// engine identifier (currently the only supported value, "lua") is
// applied even when the operator only flips Enable.
func TestLoad_ScriptingPartialEnableKeepsEngine(t *testing.T) {
	path := writeTmpConfig(t, `{
  "ingest": {
    "enable": true,
    "scripting": { "enable": true, "folder": "/etc/homer/scripts" }
  }
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	sc := cfg.Ingest.Scripting
	if !sc.Enable {
		t.Fatalf("scripting.enable: want true, got false")
	}
	if sc.Engine != "lua" {
		t.Errorf("scripting.engine: want \"lua\" (default), got %q", sc.Engine)
	}
	if sc.Folder != "/etc/homer/scripts" {
		t.Errorf("scripting.folder: want override, got %q", sc.Folder)
	}
}

// TestLoad_RemoteLoggingAbsentSectionStaysDisabled is the safety check
// for the most common case: operators who do not use remote logging at
// all. None of the three backends should accidentally flip to Enable=true
// just because we pre-seeded sibling string/int defaults.
func TestLoad_RemoteLoggingAbsentSectionStaysDisabled(t *testing.T) {
	path := writeTmpConfig(t, `{
  "ingest": { "enable": true }
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.RemoteLogging.Loki.Enable {
		t.Errorf("loki.enable: want false (default), got true")
	}
	if cfg.RemoteLogging.Elasticsearch.Enable {
		t.Errorf("elasticsearch.enable: want false (default), got true")
	}
	if cfg.RemoteLogging.LineProto.Enable {
		t.Errorf("lineproto.enable: want false (default), got true")
	}
	if cfg.Ingest.Scripting.Enable {
		t.Errorf("scripting.enable: want false (default), got true")
	}
}

// TestLoad_PartialOverrideKeepsOtherDefaults ensures that overriding one
// numeric tunable (loki.bulk) does not wipe sibling fields filled in by
// struct tags.
func TestLoad_PartialOverrideKeepsOtherDefaults(t *testing.T) {
	path := writeTmpConfig(t, `{
  "ingest": { "enable": true },
  "remote_logging": {
    "loki": { "enable": true, "bulk": 1234 }
  }
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	loki := cfg.RemoteLogging.Loki
	if loki.Bulk != 1234 {
		t.Errorf("loki.bulk: want 1234 (override), got %d", loki.Bulk)
	}
	if loki.Timer != 4 {
		t.Errorf("loki.timer: want 4 (untouched default), got %d", loki.Timer)
	}
	if loki.Buffer != 100000 {
		t.Errorf("loki.buffer: want 100000 (untouched default), got %d", loki.Buffer)
	}
	if loki.URL != "http://localhost:3100/loki/api/v1/push" {
		t.Errorf("loki.url: want default (untouched), got %q", loki.URL)
	}
}

// TestLoad_HEPStreamDefaultsHonoured covers a section that already has
// duplicated viper SetDefault entries — the new struct-tag path must
// agree with the existing viper-side numbers and not regress them.
func TestLoad_HEPStreamDefaultsHonoured(t *testing.T) {
	path := writeTmpConfig(t, `{
  "ingest": { "enable": true }
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	hs := cfg.Ingest.HepStream
	if !hs.Enable {
		t.Errorf("ingest.hep_stream.enable: want true, got false")
	}
	if hs.BufferSize != 10000 {
		t.Errorf("ingest.hep_stream.buffer_size: want 10000, got %d", hs.BufferSize)
	}
	if hs.MaxSubscribers != 32 {
		t.Errorf("ingest.hep_stream.max_subscribers: want 32, got %d", hs.MaxSubscribers)
	}
	if hs.PerSubQueueLen != 256 {
		t.Errorf("ingest.hep_stream.per_sub_queue_len: want 256, got %d", hs.PerSubQueueLen)
	}
	if hs.RatePerSubPPS != 500 {
		t.Errorf("ingest.hep_stream.rate_per_sub_pps: want 500, got %d", hs.RatePerSubPPS)
	}
}

// TestLoad_HEPIngestDefaultsHonoured is the catch-all smoke test for the
// HEP ingest stack: with a completely empty config file, every receiver
// (UDP/TCP/TLS/HTTP/HTTPS), the HEP processing flags, and IngestConfig
// itself must come up with their documented struct-tag defaults. This is
// the regression that backs the "homer-core HEP ingest will not bite"
// answer when reasoning about the v11.0.73 fix.
func TestLoad_HEPIngestDefaultsHonoured(t *testing.T) {
	path := writeTmpConfig(t, `{}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// IngestConfig top-level
	if !cfg.Ingest.Enable {
		t.Errorf("ingest.enable: want true, got false")
	}
	if cfg.Ingest.QueueSize != 200000 {
		t.Errorf("ingest.queue_size: want 200000, got %d", cfg.Ingest.QueueSize)
	}

	// UDP HEP receiver
	if !cfg.Ingest.UDP.Enable {
		t.Errorf("ingest.udp.enable: want true, got false")
	}
	if cfg.Ingest.UDP.Host != "0.0.0.0" {
		t.Errorf("ingest.udp.host: want 0.0.0.0, got %q", cfg.Ingest.UDP.Host)
	}
	if cfg.Ingest.UDP.Port != 9060 {
		t.Errorf("ingest.udp.port: want 9060, got %d", cfg.Ingest.UDP.Port)
	}
	if !cfg.Ingest.UDP.Multicore {
		t.Errorf("ingest.udp.multicore: want true, got false")
	}
	if cfg.Ingest.UDP.SocketRecvBuffer != 8388608 {
		t.Errorf("ingest.udp.socket_recv_buffer: want 8388608, got %d", cfg.Ingest.UDP.SocketRecvBuffer)
	}
	if cfg.Ingest.UDP.ReadBufferCap != 131072 {
		t.Errorf("ingest.udp.read_buffer_cap: want 131072, got %d", cfg.Ingest.UDP.ReadBufferCap)
	}

	// TCP HEP receiver (off by default but the rest of the knobs must hold)
	if cfg.Ingest.TCP.Enable {
		t.Errorf("ingest.tcp.enable: want false, got true")
	}
	if cfg.Ingest.TCP.Port != 9060 {
		t.Errorf("ingest.tcp.port: want 9060, got %d", cfg.Ingest.TCP.Port)
	}
	if cfg.Ingest.TCP.Host != "0.0.0.0" {
		t.Errorf("ingest.tcp.host: want 0.0.0.0, got %q", cfg.Ingest.TCP.Host)
	}

	// TLS HEP receiver
	if cfg.Ingest.TLS.Enable {
		t.Errorf("ingest.tls.enable: want false, got true")
	}
	if cfg.Ingest.TLS.Port != 9061 {
		t.Errorf("ingest.tls.port: want 9061, got %d", cfg.Ingest.TLS.Port)
	}
	if cfg.Ingest.TLS.MinTLSVersion != "TLS1.2" {
		t.Errorf("ingest.tls.min_tls_version: want TLS1.2, got %q", cfg.Ingest.TLS.MinTLSVersion)
	}
	if cfg.Ingest.TLS.MaxTLSVersion != "TLS1.3" {
		t.Errorf("ingest.tls.max_tls_version: want TLS1.3, got %q", cfg.Ingest.TLS.MaxTLSVersion)
	}

	// HTTP HEP receiver
	if !cfg.Ingest.HTTP.Enable {
		t.Errorf("ingest.http.enable: want true, got false")
	}
	if cfg.Ingest.HTTP.Port != 9080 {
		t.Errorf("ingest.http.port: want 9080, got %d", cfg.Ingest.HTTP.Port)
	}
	if cfg.Ingest.HTTP.ReadTimeout != 30 {
		t.Errorf("ingest.http.read_timeout: want 30, got %d", cfg.Ingest.HTTP.ReadTimeout)
	}
	if cfg.Ingest.HTTP.MaxRequestBodySize != 67108864 {
		t.Errorf("ingest.http.max_request_body_size: want 67108864, got %d", cfg.Ingest.HTTP.MaxRequestBodySize)
	}

	// HTTPS HEP receiver
	if cfg.Ingest.HTTPS.Enable {
		t.Errorf("ingest.https.enable: want false, got true")
	}
	if cfg.Ingest.HTTPS.Port != 9443 {
		t.Errorf("ingest.https.port: want 9443, got %d", cfg.Ingest.HTTPS.Port)
	}

	// HEP processing flags (the part of the stack that actually parses HEP)
	if !cfg.Ingest.HEP.HepV2Enable {
		t.Errorf("ingest.hep.hepv2_enable: want true, got false")
	}
	if !cfg.Ingest.HEP.HepV3Enable {
		t.Errorf("ingest.hep.hepv3_enable: want true, got false")
	}
	if !cfg.Ingest.HEP.ProtobufEnable {
		t.Errorf("ingest.hep.protobuf_enable: want true, got false")
	}
	if cfg.Ingest.HEP.Deduplicate {
		t.Errorf("ingest.hep.deduplicate: want false, got true")
	}
}

// TestLoad_ExplicitFalseOverrideWins guards against the most dangerous
// kind of regression in this layer: defaults silently re-enabling a
// feature the operator turned off.
func TestLoad_ExplicitFalseOverrideWins(t *testing.T) {
	path := writeTmpConfig(t, `{
  "ingest": {
    "enable": true,
    "hep_stream": { "enable": false }
  },
  "remote_logging": {
    "elasticsearch": { "enable": true, "discovery": false, "index_daily": false }
  }
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Ingest.HepStream.Enable {
		t.Errorf("hep_stream.enable: want false (explicit override), got true")
	}
	if cfg.RemoteLogging.Elasticsearch.Discovery {
		t.Errorf("elasticsearch.discovery: want false (explicit override), got true")
	}
	if cfg.RemoteLogging.Elasticsearch.IndexDaily {
		t.Errorf("elasticsearch.index_daily: want false (explicit override), got true")
	}
}

// TestLoad_DuckDBTuningPropagates verifies that the new
// storage.ducklake.tuning section parses and reaches the DuckLake
// config struct unchanged. Both writer- and node-side DuckDB opens
// read this same path through the storage block, so this is the only
// place that needs an explicit regression test.
func TestLoad_DuckDBTuningPropagates(t *testing.T) {
	path := writeTmpConfig(t, `{
  "storage": {
    "ducklake": {
      "tuning": {
        "memory_limit": "4GB",
        "threads": 4,
        "temp_directory": "/var/lib/homer/spill"
      }
    }
  },
  "node": {
    "ducklake": {
      "tuning": {
        "memory_limit": "2GB"
      }
    }
  }
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	stw := cfg.Storage.DuckLake.Tuning
	if stw.MemoryLimit != "4GB" {
		t.Errorf("storage.ducklake.tuning.memory_limit: want 4GB, got %q", stw.MemoryLimit)
	}
	if stw.Threads != 4 {
		t.Errorf("storage.ducklake.tuning.threads: want 4, got %d", stw.Threads)
	}
	if stw.TempDirectory != "/var/lib/homer/spill" {
		t.Errorf("storage.ducklake.tuning.temp_directory: want /var/lib/homer/spill, got %q", stw.TempDirectory)
	}

	node := cfg.Node.DuckLake.Tuning
	if node.MemoryLimit != "2GB" {
		t.Errorf("node.ducklake.tuning.memory_limit: want 2GB, got %q", node.MemoryLimit)
	}
}

// TestLoad_OTLPDefaultsHonoured covers the brand-new OTLP receiver
// section: with the feature off (the documented default) every nested
// listener still parses cleanly and the struct-tag numbers/strings come
// through unchanged. This is the section equivalent of the HepStream
// regression — the bug we want to keep out is "operator turns
// ingest.otlp.enable=true and the listeners come up at :0 / :0 because
// the inner defaults silently dropped to zero values".
func TestLoad_OTLPDefaultsHonoured(t *testing.T) {
	path := writeTmpConfig(t, `{}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	otlp := cfg.Ingest.OTLP
	if otlp.Enable {
		t.Errorf("ingest.otlp.enable: want false (default), got true")
	}
	if otlp.MaxRecvMsgBytes != 4194304 {
		t.Errorf("ingest.otlp.max_recv_msg_bytes: want 4194304, got %d", otlp.MaxRecvMsgBytes)
	}
	if !otlp.GRPC.Enable {
		t.Errorf("ingest.otlp.grpc.enable: want true (default), got false")
	}
	if otlp.GRPC.Listen != ":4317" {
		t.Errorf("ingest.otlp.grpc.listen: want :4317, got %q", otlp.GRPC.Listen)
	}
	if !otlp.HTTP.Enable {
		t.Errorf("ingest.otlp.http.enable: want true (default), got false")
	}
	if otlp.HTTP.Listen != ":4318" {
		t.Errorf("ingest.otlp.http.listen: want :4318, got %q", otlp.HTTP.Listen)
	}
	if otlp.HTTP.ReadTimeoutSec != 30 {
		t.Errorf("ingest.otlp.http.read_timeout_sec: want 30, got %d", otlp.HTTP.ReadTimeoutSec)
	}
	if otlp.HTTP.WriteTimeoutSec != 30 {
		t.Errorf("ingest.otlp.http.write_timeout_sec: want 30, got %d", otlp.HTTP.WriteTimeoutSec)
	}
	if !otlp.Sinks.StoreTraces {
		t.Errorf("ingest.otlp.sinks.store_traces: want true (default), got false")
	}
	if !otlp.Sinks.StoreMetrics {
		t.Errorf("ingest.otlp.sinks.store_metrics: want true (default), got false")
	}
	if !otlp.Sinks.StoreLogs {
		t.Errorf("ingest.otlp.sinks.store_logs: want true (default), got false")
	}
}

// TestLoad_OTLPPartialEnableKeepsTagDefaults pins the same flavour of
// regression as the Loki/Elasticsearch tests above: enabling OTLP with
// `{"enable": true}` only must leave the listener defaults intact.
func TestLoad_OTLPPartialEnableKeepsTagDefaults(t *testing.T) {
	path := writeTmpConfig(t, `{
  "ingest": {
    "enable": true,
    "otlp": { "enable": true, "grpc": { "listen": ":24317" } }
  }
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	otlp := cfg.Ingest.OTLP
	if !otlp.Enable {
		t.Fatalf("ingest.otlp.enable: want true (explicit), got false")
	}
	if otlp.GRPC.Listen != ":24317" {
		t.Errorf("ingest.otlp.grpc.listen: want :24317 (override), got %q", otlp.GRPC.Listen)
	}
	if otlp.HTTP.Listen != ":4318" {
		t.Errorf("ingest.otlp.http.listen: want :4318 (default), got %q", otlp.HTTP.Listen)
	}
	if !otlp.HTTP.Enable {
		t.Errorf("ingest.otlp.http.enable: want true (default), got false")
	}
	if otlp.MaxRecvMsgBytes != 4194304 {
		t.Errorf("ingest.otlp.max_recv_msg_bytes: want 4194304 (default), got %d", otlp.MaxRecvMsgBytes)
	}
	if !otlp.Sinks.StoreTraces || !otlp.Sinks.StoreMetrics || !otlp.Sinks.StoreLogs {
		t.Errorf("ingest.otlp.sinks: want all defaults true, got %+v", otlp.Sinks)
	}
}

// TestLoad_LineProtoIngestDefaultsHonoured locks in the documented
// defaults for the InfluxDB Line Protocol ingest receiver
// (`ingest.line_protocol.*`, distinct from the remote_logging.lineproto
// outbound forwarder): feature off, listener on :8086, 8 MiB body cap,
// ns precision, empty table prefix (measurement = table name).
func TestLoad_LineProtoIngestDefaultsHonoured(t *testing.T) {
	path := writeTmpConfig(t, `{}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	lp := cfg.Ingest.LineProto
	if lp.Enable {
		t.Errorf("ingest.line_protocol.enable: want false (default), got true")
	}
	if lp.Listen != ":8086" {
		t.Errorf("ingest.line_protocol.listen: want :8086, got %q", lp.Listen)
	}
	if lp.MaxBodyBytes != 8388608 {
		t.Errorf("ingest.line_protocol.max_body_bytes: want 8388608, got %d", lp.MaxBodyBytes)
	}
	if lp.DefaultPrecision != "ns" {
		t.Errorf("ingest.line_protocol.default_precision: want ns, got %q", lp.DefaultPrecision)
	}
	if lp.TablePrefix != "" {
		t.Errorf("ingest.line_protocol.table_prefix: want empty default, got %q", lp.TablePrefix)
	}
	if lp.ReadTimeoutSec != 30 || lp.WriteTimeoutSec != 30 {
		t.Errorf("ingest.line_protocol timeouts: want 30/30, got %d/%d",
			lp.ReadTimeoutSec, lp.WriteTimeoutSec)
	}
	if lp.AllowHepSipCall {
		t.Errorf("ingest.line_protocol.allow_hep_sip_call: want false (default), got true")
	}
}

// TestLoad_LineProtoIngestAllowHepSipCallOverride checks explicit
// allow_hep_sip_call in JSON.
func TestLoad_LineProtoIngestAllowHepSipCallOverride(t *testing.T) {
	path := writeTmpConfig(t, `{
  "ingest": {
    "line_protocol": { "enable": true, "allow_hep_sip_call": true }
  }
}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !cfg.Ingest.LineProto.AllowHepSipCall {
		t.Fatalf("allow_hep_sip_call: want true, got false")
	}
}

// TestLoad_LineProtoIngestExplicitEmptyTablePrefix honours an explicit
// empty table_prefix (measurement name = DuckLake table name).
func TestLoad_LineProtoIngestExplicitEmptyTablePrefix(t *testing.T) {
	path := writeTmpConfig(t, `{
  "ingest": {
    "line_protocol": { "enable": true, "table_prefix": "" }
  }
}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Ingest.LineProto.TablePrefix != "" {
		t.Fatalf("ingest.line_protocol.table_prefix: want empty, got %q", cfg.Ingest.LineProto.TablePrefix)
	}
	if cfg.Coordinator.LineProtoTablePrefix != "" {
		t.Fatalf("coordinator LineProtoTablePrefix: want empty, got %q", cfg.Coordinator.LineProtoTablePrefix)
	}
}

// TestLoad_LineProtoIngestExplicitLegacyPrefix checks optional namespacing.
func TestLoad_LineProtoIngestExplicitLegacyPrefix(t *testing.T) {
	path := writeTmpConfig(t, `{
  "ingest": {
    "line_protocol": { "enable": true, "table_prefix": "lp_" }
  }
}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Ingest.LineProto.TablePrefix != "lp_" {
		t.Fatalf("ingest.line_protocol.table_prefix: want lp_, got %q", cfg.Ingest.LineProto.TablePrefix)
	}
	if cfg.Coordinator.LineProtoTablePrefix != "lp_" {
		t.Fatalf("coordinator LineProtoTablePrefix: want lp_, got %q", cfg.Coordinator.LineProtoTablePrefix)
	}
}

// TestLoad_LineProtoIngestPartialEnableKeepsTagDefaults mirrors the
// OTLP regression: enabling the ingest LP receiver with
// `{"enable": true}` only must leave the listener / precision / prefix
// defaults intact.
func TestLoad_LineProtoIngestPartialEnableKeepsTagDefaults(t *testing.T) {
	path := writeTmpConfig(t, `{
  "ingest": {
    "enable": true,
    "line_protocol": { "enable": true, "listen": ":18086" }
  }
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	lp := cfg.Ingest.LineProto
	if !lp.Enable {
		t.Fatalf("ingest.line_protocol.enable: want true (explicit), got false")
	}
	if lp.Listen != ":18086" {
		t.Errorf("ingest.line_protocol.listen: want :18086 (override), got %q", lp.Listen)
	}
	if lp.MaxBodyBytes != 8388608 {
		t.Errorf("ingest.line_protocol.max_body_bytes: want default 8388608, got %d", lp.MaxBodyBytes)
	}
	if lp.DefaultPrecision != "ns" {
		t.Errorf("ingest.line_protocol.default_precision: want ns (default), got %q", lp.DefaultPrecision)
	}
	if lp.TablePrefix != "" {
		t.Errorf("ingest.line_protocol.table_prefix: want empty (default), got %q", lp.TablePrefix)
	}
}

// TestLoad_DuckDBTuningEmptySectionStaysZero pins the desired default:
// without an explicit tuning section the struct stays zero-valued so
// ApplyDuckDBTuning becomes a no-op and DuckDB keeps its own defaults.
func TestLoad_DuckDBTuningEmptySectionStaysZero(t *testing.T) {
	path := writeTmpConfig(t, `{
  "storage": { "ducklake": {} }
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	tn := cfg.Storage.DuckLake.Tuning
	if tn.MemoryLimit != "" || tn.Threads != 0 || tn.TempDirectory != "" {
		t.Errorf("expected zero-valued tuning, got %+v", tn)
	}
}

func TestLoad_CoordinatorAuthOmittedDefaultsToInternal(t *testing.T) {
	path := writeTmpConfig(t, `{
  "coordinator": {
    "enable": true
  }
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	a := cfg.Coordinator.Auth
	if a.Type != "internal" {
		t.Fatalf("Type: got %q, want internal", a.Type)
	}
	if !a.AuthFromInternalString {
		t.Fatal("AuthFromInternalString: want true when auth omitted")
	}
	if a.AdminUser != "admin" || a.AdminPasswordHash != "" {
		t.Fatalf("auth: %+v", a)
	}
}

func TestLoad_CoordinatorAuthInternalString(t *testing.T) {
	path := writeTmpConfig(t, `{
  "coordinator": {
    "enable": true,
    "auth": "internal"
  }
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	a := cfg.Coordinator.Auth
	if !a.AuthFromInternalString {
		t.Fatal("AuthFromInternalString: want true")
	}
	if a.AdminUser != "admin" {
		t.Fatalf("AdminUser: got %q", a.AdminUser)
	}
	if a.AdminPasswordHash != "" {
		t.Fatalf("AdminPasswordHash: got %q", a.AdminPasswordHash)
	}
	if a.Type != "internal" {
		t.Fatalf("Type: got %q", a.Type)
	}
}

func TestLoad_CoordinatorAuthObjectTypeInternal(t *testing.T) {
	path := writeTmpConfig(t, `{
  "coordinator": {
    "enable": true,
    "auth": { "type": "internal" }
  }
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	a := cfg.Coordinator.Auth
	if a.Type != "internal" {
		t.Fatalf("Type: got %q", a.Type)
	}
	if !a.AuthFromInternalString {
		t.Fatal("AuthFromInternalString: want true")
	}
	if a.AdminUser != "admin" {
		t.Fatalf("AdminUser: got %q", a.AdminUser)
	}
	if a.AdminPasswordHash != "" {
		t.Fatalf("AdminPasswordHash: got %q", a.AdminPasswordHash)
	}
}

func TestLoad_CoordinatorAuthObjectTypeLDAP(t *testing.T) {
	path := writeTmpConfig(t, `{
  "coordinator": {
    "enable": true,
    "auth": { "type": "ldap" }
  }
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	a := cfg.Coordinator.Auth
	if a.Type != "ldap" {
		t.Fatalf("Type: got %q", a.Type)
	}
	if a.AuthFromInternalString {
		t.Fatal("AuthFromInternalString: want false for ldap type")
	}
}

func TestLoad_CoordinatorAuthObjectTypeOAuth(t *testing.T) {
	path := writeTmpConfig(t, `{
  "coordinator": {
    "enable": true,
    "auth": { "type": "oauth" }
  }
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	a := cfg.Coordinator.Auth
	if a.Type != "oauth" {
		t.Fatalf("Type: got %q", a.Type)
	}
	if a.AuthFromInternalString {
		t.Fatal("AuthFromInternalString: want false for oauth type")
	}
}

func TestLoad_CoordinatorAuthObjectTypeInvalid(t *testing.T) {
	path := writeTmpConfig(t, `{
  "coordinator": { "enable": true, "auth": { "type": "saml" } }
}`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for invalid coordinator.auth.type")
	}
}

func TestLoad_InvalidRetentionUnit(t *testing.T) {
	path := writeTmpConfig(t, `{
  "storage": { "ducklake": { "compaction": { "retention_unit": "fortnights" } } }
}`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for invalid storage.ducklake.compaction.retention_unit")
	}
}

func TestLoad_CoordinatorAuthObjectWithoutTypeDefaultsToInternal(t *testing.T) {
	path := writeTmpConfig(t, `{
  "coordinator": {
    "enable": true,
    "auth": {
      "admin_user": "root",
      "admin_password_hash": "deadbeef"
    }
  }
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	a := cfg.Coordinator.Auth
	if !a.AuthFromInternalString {
		t.Fatal("AuthFromInternalString: want true when type omitted (internal)")
	}
	if a.Type != "internal" {
		t.Fatalf("Type: want internal, got %q", a.Type)
	}
	if a.AdminUser != "root" || a.AdminPasswordHash != "deadbeef" {
		t.Fatalf("auth fields: %+v", a)
	}
}

func TestLoad_CoordinatorAuthInvalidString(t *testing.T) {
	path := writeTmpConfig(t, `{
  "coordinator": { "enable": true, "auth": "ldap-only" }
}`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for invalid coordinator.auth string")
	}
}

func TestLoad_CoordinatorAuthFallbackTypeInvalid(t *testing.T) {
	path := writeTmpConfig(t, `{
  "coordinator": {
    "enable": true,
    "auth": { "type": "internal", "fallback_auth_type": "oauth" }
  }
}`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for invalid coordinator.auth.fallback_auth_type")
	}
}

func TestLoad_CoordinatorAuthFallbackTypeOK(t *testing.T) {
	path := writeTmpConfig(t, `{
  "coordinator": {
    "enable": true,
    "auth": { "type": "internal", "fallback_auth_type": "ldap" }
  }
}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Coordinator.Auth.FallbackAuthType != "ldap" {
		t.Fatalf("FallbackAuthType: got %q", cfg.Coordinator.Auth.FallbackAuthType)
	}
}

func TestAuthConfigMarshalJSON_Internal(t *testing.T) {
	b, err := json.Marshal(AuthConfig{
		AuthFromInternalString: true,
		Type:                   "internal",
		AdminUser:              "admin",
		AdminPasswordHash:      LegacySHA256SipcaptureHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"type":"internal","admin_password_hash":"883ffc1f37fd0fe542b0fb9740035c4383e7d976c411161d24e62edace280f90"}` {
		t.Fatalf("marshal: got %s", b)
	}
}

func TestAuthConfigMarshalJSON_InternalCustomAdmin(t *testing.T) {
	b, err := json.Marshal(AuthConfig{
		AuthFromInternalString: true,
		Type:                   "internal",
		AdminUser:              "root",
		AdminPasswordHash:      LegacySHA256SipcaptureHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"type":"internal","admin_user":"root","admin_password_hash":"883ffc1f37fd0fe542b0fb9740035c4383e7d976c411161d24e62edace280f90"}` {
		t.Fatalf("marshal: got %s", b)
	}
}

func TestAuthConfigMarshalJSON_InternalWithFallback(t *testing.T) {
	b, err := json.Marshal(AuthConfig{
		AuthFromInternalString: true,
		Type:                   "internal",
		AdminUser:              "admin",
		AdminPasswordHash:      LegacySHA256SipcaptureHash,
		FallbackAuthType:       "ldap",
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"type":"internal","admin_password_hash":"883ffc1f37fd0fe542b0fb9740035c4383e7d976c411161d24e62edace280f90","fallback_auth_type":"ldap"}` {
		t.Fatalf("marshal: got %s", b)
	}
}

// TestLoad_LegacyNodeDuckLakeWithoutVolumes synthesizes node.ducklake.volumes
// from catalog_path/data_path (wizard output before volumes were required).
func TestLoad_LegacyNodeDuckLakeWithoutVolumes(t *testing.T) {
	path := writeTmpConfig(t, `{
  "node": {
    "enable": true,
    "ducklake": {
      "lake_name": "homer_lake",
      "catalog_type": "sqlite",
      "catalog_path": "/data/homer/homer_catalog.sqlite",
      "data_path": "/data/homer/parquet"
    }
  }
}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	vols := cfg.Node.DuckLake.Volumes
	if len(vols) != 1 {
		t.Fatalf("node.ducklake.volumes: want 1 synthesized entry, got %d (%+v)", len(vols), vols)
	}
	v := vols[0]
	if v.Name != "default" || v.Type != "local" || v.CatalogType != "sqlite" {
		t.Errorf("volume identity: %+v", v)
	}
	if v.CatalogPath != "/data/homer/homer_catalog.sqlite" || v.Path != "/data/homer/parquet" {
		t.Errorf("volume paths: %+v", v)
	}
}

func TestEnsureNodeDuckLakeVolumes_NoOpWhenPresent(t *testing.T) {
	dl := DuckLakeConfig{
		CatalogPath: "/a/catalog.sqlite",
		DataPath:    "/a/parquet",
		Volumes: []VolumeConfig{{
			Name: "hot", Type: "local", CatalogPath: "/hot/c.sqlite", Path: "/hot/p",
		}},
	}
	EnsureNodeDuckLakeVolumes(&dl)
	if len(dl.Volumes) != 1 || dl.Volumes[0].Name != "hot" {
		t.Fatalf("expected existing volumes unchanged, got %+v", dl.Volumes)
	}
}

func TestEnsureNodeDuckLakeVolumes_S3CopiesCredentials(t *testing.T) {
	dl := DuckLakeConfig{
		CatalogPath: "/var/lib/homer/homer_catalog.sqlite",
		DataPath:    "s3://bucket/lake/",
		CatalogType: "sqlite",
		S3: S3Config{
			Region: "us-east-1", AccessKeyID: "ak", SecretAccessKey: "sk",
			Endpoint: "http://127.0.0.1:9000", UseSSL: false, URLStyle: "path",
		},
	}
	EnsureNodeDuckLakeVolumes(&dl)
	if len(dl.Volumes) != 1 {
		t.Fatalf("want 1 volume, got %d", len(dl.Volumes))
	}
	v := dl.Volumes[0]
	if v.Type != "s3" || v.S3AccessKeyID != "ak" || v.S3SecretKey != "sk" || v.S3Endpoint != "http://127.0.0.1:9000" {
		t.Fatalf("s3 volume credentials: %+v", v)
	}
}

func TestEnsureNodeDuckLakeVolumes_AzureCopiesCredentials(t *testing.T) {
	dl := DuckLakeConfig{
		CatalogPath: "/var/lib/homer/homer_catalog.sqlite",
		DataPath:    "az://container/lake/",
		CatalogType: "sqlite",
		Azure: AzureConfig{
			AccountName: "myaccount", AccountKey: "key",
		},
	}
	EnsureNodeDuckLakeVolumes(&dl)
	if len(dl.Volumes) != 1 {
		t.Fatalf("want 1 volume, got %d", len(dl.Volumes))
	}
	v := dl.Volumes[0]
	if v.Type != "azure" || v.AzureAccountName != "myaccount" || v.AzureAccountKey != "key" {
		t.Fatalf("azure volume credentials: %+v", v)
	}
}

func TestLoad_InvalidVolumeType(t *testing.T) {
	path := writeTmpConfig(t, `{
  "storage": { "ducklake": { "storage_policy": { "volumes": [
    { "name": "cold", "type": "azur", "path": "az://bucket/cold/" }
  ] } } }
}`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for unknown volume type")
	}
}

func TestLoad_AzureVolumeTypeOK(t *testing.T) {
	path := writeTmpConfig(t, `{
  "storage": { "ducklake": { "storage_policy": { "volumes": [
    { "name": "cold", "type": "azure", "path": "az://bucket/cold/", "azure_account_name": "myaccount" }
  ] } } }
}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	vols := cfg.Storage.DuckLake.StoragePolicy.Volumes
	if len(vols) != 1 || vols[0].Type != "azure" || vols[0].AzureAccountName != "myaccount" {
		t.Fatalf("azure volume: %+v", vols)
	}
}

func TestLoad_CoordinatorBareFlightSQLPortEnablesProxy(t *testing.T) {
	dir := t.TempDir()
	path := writeTmpConfig(t, `{
  "node": {
    "enable": true,
    "flight_server": { "host": "127.0.0.1", "port": 50051 },
    "flightsql_server": { "enable": true, "host": "127.0.0.1", "port": 50055, "auth_token": "fsql-token" },
    "ducklake": { "catalog_path": "`+dir+`/cat.sqlite" }
  },
  "coordinator": {
    "enable": true,
    "flightsql_port": 50055,
    "settings_db_path": "`+dir+`/settings.duckdb"
  }
}`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Coordinator.FlightSQLServer.Enable {
		t.Fatal("coordinator.flightsql_port should enable flightsql_server")
	}
	if cfg.Coordinator.FlightSQLServer.Port != 50055 {
		t.Fatalf("port=%d", cfg.Coordinator.FlightSQLServer.Port)
	}
	if len(cfg.Coordinator.Nodes) == 0 || cfg.Coordinator.Nodes[0].FlightSQLPort != 50055 {
		t.Fatalf("local node flightsql_port not wired: %+v", cfg.Coordinator.Nodes)
	}
}
