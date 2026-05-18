// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

//go:build !vet
// +build !vet

package ducklake

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sipcapture/homer-core/src/decoder"
)

func benchDuckLakeManager(b *testing.B) (*Manager, func()) {
	b.Helper()
	dir := b.TempDir()
	mgr, err := NewManager(Config{
		CatalogType:   CatalogSQLite,
		CatalogPath:   filepath.Join(dir, "bench_catalog.sqlite"),
		DataPath:      filepath.Join(dir, "data"),
		LakeName:      "bench_lake",
		BatchSize:     10000,
		FlushInterval: 24 * time.Hour,
		ShardCount:    1,
	})
	if err != nil {
		b.Fatalf("NewManager: %v", err)
	}
	if err := mgr.Start(); err != nil {
		b.Fatalf("Start: %v", err)
	}
	return mgr, func() {
		_ = mgr.Stop()
	}
}

// BenchmarkWriteHEP_SIP measures DuckLake WriteHEP for a decoded SIP INVITE
// (convert + batch append, no periodic flush during the loop).
func BenchmarkWriteHEP_SIP(b *testing.B) {
	mgr, cleanup := benchDuckLakeManager(b)
	defer cleanup()

	pkt := decoder.BenchHEP3SIPPacket()
	hep, err := decoder.DecodeHEP(pkt)
	if err != nil {
		b.Fatalf("DecodeHEP: %v", err)
	}
	defer decoder.ReleaseHEP(hep)

	for i := 0; i < 1000; i++ {
		if err := mgr.WriteHEP(hep); err != nil {
			b.Fatalf("warmup WriteHEP: %v", err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := mgr.WriteHEP(hep); err != nil {
			b.Fatalf("WriteHEP: %v", err)
		}
	}
}

// BenchmarkDecodeAndWriteHEP_SIP is the full ingest worker hot path:
// DecodeHEP + WriteHEP per packet.
func BenchmarkDecodeAndWriteHEP_SIP(b *testing.B) {
	mgr, cleanup := benchDuckLakeManager(b)
	defer cleanup()

	pkt := decoder.BenchHEP3SIPPacket()
	// Warm up table creation / Appender path outside the timed section.
	for i := 0; i < 1000; i++ {
		hep, err := decoder.DecodeHEP(pkt)
		if err != nil {
			b.Fatalf("warmup DecodeHEP: %v", err)
		}
		if err := mgr.WriteHEP(hep); err != nil {
			decoder.ReleaseHEP(hep)
			b.Fatalf("warmup WriteHEP: %v", err)
		}
		decoder.ReleaseHEP(hep)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hep, err := decoder.DecodeHEP(pkt)
		if err != nil {
			b.Fatalf("DecodeHEP: %v", err)
		}
		if err := mgr.WriteHEP(hep); err != nil {
			decoder.ReleaseHEP(hep)
			b.Fatalf("WriteHEP: %v", err)
		}
		decoder.ReleaseHEP(hep)
	}
}
