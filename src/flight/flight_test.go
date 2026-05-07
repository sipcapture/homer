// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package flight

import (
	"context"
	"testing"
	"time"

	"github.com/sipcapture/homer-core/src/decoder"
	"github.com/sipcapture/homer-core/src/sipparser"
)

func TestRingBuffer(t *testing.T) {
	rb := NewRingBuffer(5)

	// Test empty buffer
	if rb.Count() != 0 {
		t.Errorf("Expected count 0, got %d", rb.Count())
	}

	// Add items
	for i := 0; i < 3; i++ {
		rb.Add(&decoder.HEP{NodeID: uint32(i)})
	}

	if rb.Count() != 3 {
		t.Errorf("Expected count 3, got %d", rb.Count())
	}

	// Get all
	items := rb.GetAll()
	if len(items) != 3 {
		t.Errorf("Expected 3 items, got %d", len(items))
	}

	// Add more items to trigger wrap-around
	for i := 3; i < 8; i++ {
		rb.Add(&decoder.HEP{NodeID: uint32(i)})
	}

	if rb.Count() != 5 {
		t.Errorf("Expected count 5 (buffer size), got %d", rb.Count())
	}

	// Verify oldest items were overwritten
	items = rb.GetAll()
	if len(items) != 5 {
		t.Errorf("Expected 5 items, got %d", len(items))
	}

	// First item should be 3 (oldest remaining)
	if items[0].NodeID != 3 {
		t.Errorf("Expected first item NodeID=3, got %d", items[0].NodeID)
	}
}

func TestHEPTable(t *testing.T) {
	table := NewHEPTable("test", "Test table", 100)

	// Test schema
	schema := table.ArrowSchema(nil)
	if schema == nil {
		t.Fatal("Schema should not be nil")
	}

	if schema.NumFields() != 26 {
		t.Errorf("Expected 26 fields, got %d", schema.NumFields())
	}

	// Test projected schema
	projected := table.ArrowSchema([]string{"src_ip", "dst_ip", "payload"})
	if projected.NumFields() != 3 {
		t.Errorf("Expected 3 projected fields, got %d", projected.NumFields())
	}

	// Add HEP packet
	hep := &decoder.HEP{
		Version:   3,
		Protocol:  17,
		SrcIP:     "192.168.1.1",
		DstIP:     "192.168.1.2",
		SrcPort:   5060,
		DstPort:   5060,
		ProtoType: 1,
		NodeID:    1,
		Timestamp: time.Now(),
		SIP: &sipparser.SipMsg{
			CallID:     "test-call-id",
			CseqMethod: "INVITE",
		},
	}
	table.AddHEP(hep)

	// Test scan
	reader, err := table.Scan(context.Background(), nil)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	defer reader.Release()

	if !reader.Next() {
		t.Fatal("Expected at least one record")
	}

	record := reader.Record()
	if record.NumRows() != 1 {
		t.Errorf("Expected 1 row, got %d", record.NumRows())
	}
}

func TestHEPCatalog(t *testing.T) {
	catalog := NewHEPCatalog(100)

	// Test name
	if catalog.Name() != "homer" {
		t.Errorf("Expected catalog name 'homer', got '%s'", catalog.Name())
	}

	// Test schemas
	schemas, err := catalog.Schemas(context.Background())
	if err != nil {
		t.Fatalf("Schemas() failed: %v", err)
	}
	if len(schemas) != 1 {
		t.Errorf("Expected 1 schema, got %d", len(schemas))
	}

	// Test schema lookup
	schema, err := catalog.Schema(context.Background(), "hep")
	if err != nil {
		t.Fatalf("Schema() failed: %v", err)
	}
	if schema.Name() != "hep" {
		t.Errorf("Expected schema name 'hep', got '%s'", schema.Name())
	}

	// Test tables
	tables, err := schema.Tables(context.Background())
	if err != nil {
		t.Fatalf("Tables() failed: %v", err)
	}
	if len(tables) != 4 {
		t.Errorf("Expected 4 tables (packets, sip, rtcp, logs), got %d", len(tables))
	}

	// Add HEP packet
	hep := &decoder.HEP{
		Version:   3,
		Protocol:  17,
		SrcIP:     "192.168.1.1",
		DstIP:     "192.168.1.2",
		ProtoType: 1, // SIP
		Timestamp: time.Now(),
	}
	catalog.AddHEP(hep)

	// Verify packet was added to 'packets' and 'sip' tables
	stats := catalog.GetStats()
	hepStats := stats["hep"].(map[string]interface{})

	packetsStats := hepStats["packets"].(map[string]interface{})
	if packetsStats["count"].(int) != 1 {
		t.Errorf("Expected 1 packet in 'packets' table, got %d", packetsStats["count"].(int))
	}

	sipStats := hepStats["sip"].(map[string]interface{})
	if sipStats["count"].(int) != 1 {
		t.Errorf("Expected 1 packet in 'sip' table, got %d", sipStats["count"].(int))
	}
}

func TestFlightServerConfig(t *testing.T) {
	config := DefaultServerConfig()

	if config.ListenAddr != ":50051" {
		t.Errorf("Expected default listen addr ':50051', got '%s'", config.ListenAddr)
	}

	if config.BufferSize != 100000 {
		t.Errorf("Expected default buffer size 100000, got %d", config.BufferSize)
	}

	if config.MaxMessageSize != 16*1024*1024 {
		t.Errorf("Expected default max message size 16MB, got %d", config.MaxMessageSize)
	}
}

func BenchmarkRingBuffer_Add(b *testing.B) {
	rb := NewRingBuffer(100000)
	hep := &decoder.HEP{
		Version:   3,
		Protocol:  17,
		SrcIP:     "192.168.1.1",
		DstIP:     "192.168.1.2",
		SrcPort:   5060,
		DstPort:   5060,
		ProtoType: 1,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rb.Add(hep)
	}
}

func BenchmarkHEPTable_Scan(b *testing.B) {
	table := NewHEPTable("bench", "Benchmark table", 10000)

	// Pre-fill with data
	for i := 0; i < 10000; i++ {
		table.AddHEP(&decoder.HEP{
			Version:   3,
			Protocol:  17,
			SrcIP:     "192.168.1.1",
			DstIP:     "192.168.1.2",
			SrcPort:   5060,
			DstPort:   5060,
			ProtoType: 1,
			Timestamp: time.Now(),
		})
	}

	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		reader, err := table.Scan(ctx, nil)
		if err != nil {
			b.Fatal(err)
		}
		for reader.Next() {
			_ = reader.Record()
		}
		reader.Release()
	}
}
