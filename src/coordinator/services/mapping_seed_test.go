// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"context"
	"path/filepath"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestSeedDefaultMappingSchema_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.duckdb")
	db, err := OpenSettingsDB(path)
	if err != nil {
		t.Fatalf("OpenSettingsDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSettingsSchema(db); err != nil {
		t.Fatalf("EnsureSettingsSchema: %v", err)
	}
	ctx := context.Background()
	if err := SeedDefaultMappingSchema(ctx, db); err != nil {
		t.Fatalf("SeedDefaultMappingSchema first: %v", err)
	}
	var n int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mapping_schema`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	const expectedRows = 10 // SIP{call,default,registration} + SIPREC + RTCP + DNS + LOG + OTLP{traces,metrics,logs}
	if n != expectedRows {
		t.Fatalf("expected %d seeded rows, got %d", expectedRows, n)
	}
	if err := SeedDefaultMappingSchema(ctx, db); err != nil {
		t.Fatalf("SeedDefaultMappingSchema second: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mapping_schema`).Scan(&n); err != nil {
		t.Fatalf("count2: %v", err)
	}
	if n != expectedRows {
		t.Fatalf("expected %d rows after second seed, got %d", expectedRows, n)
	}

	// Verify the OTLP virtual mappings landed with their expected hepid /
	// hep_alias / profile combination so the Proto Search widget can
	// route requests to the otlp_* DuckLake tables.
	rows, err := db.QueryContext(ctx, `SELECT hepid, hep_alias, profile FROM mapping_schema WHERE hepid IN (200,201,202) ORDER BY hepid`)
	if err != nil {
		t.Fatalf("query OTLP mappings: %v", err)
	}
	defer rows.Close()
	got := map[int]string{}
	for rows.Next() {
		var hepid int
		var alias, profile string
		if err := rows.Scan(&hepid, &alias, &profile); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[hepid] = alias + "/" + profile
	}
	want := map[int]string{
		200: "OTLP_TRACES/default",
		201: "OTLP_METRICS/default",
		202: "OTLP_LOGS/default",
	}
	for hepid, w := range want {
		if g, ok := got[hepid]; !ok || g != w {
			t.Fatalf("OTLP mapping hepid=%d: got %q, want %q", hepid, g, w)
		}
	}
}

// Regression: when an existing deployment already has some seeded
// rows (the historical SIP/RTCP/DNS/LOG set) but is missing newer
// defaults (OTLP, added in 11.0.118), SeedDefaultMappingSchema must
// add the missing rows on the next start instead of bailing out
// because "the table is non-empty".
func TestSeedDefaultMappingSchema_FillsMissingOnUpgrade(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.duckdb")
	db, err := OpenSettingsDB(path)
	if err != nil {
		t.Fatalf("OpenSettingsDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := EnsureSettingsSchema(db); err != nil {
		t.Fatalf("EnsureSettingsSchema: %v", err)
	}
	ctx := context.Background()

	// Pre-seed the SIP "call" row only — simulates an old deployment
	// where the global "table is empty" check used to short-circuit
	// the rest of the defaults forever.
	if _, err := db.ExecContext(ctx, `INSERT INTO mapping_schema (
		guid, profile, hepid, hep_alias, partid, version, retention, partition_step,
		create_index, create_table, correlation_mapping, fields_mapping, mapping_settings,
		schema_mapping, schema_settings, create_date
	) VALUES (
		'`+defaultMappingGUIDCall+`', 'call', 1, 'SIP', 10, 1, 14, 3600,
		'{}', 'CREATE TABLE noop(id integer);', '`+correlationMappingEmpty+`', '{}', '{}', '{}', '{}',
		current_timestamp
	)`); err != nil {
		t.Fatalf("pre-seed legacy row: %v", err)
	}

	if err := SeedDefaultMappingSchema(ctx, db); err != nil {
		t.Fatalf("SeedDefaultMappingSchema: %v", err)
	}

	var n int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mapping_schema`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 10 {
		t.Fatalf("expected 10 rows after upgrade-style seed, got %d", n)
	}

	// The pre-existing SIP/call row must still be exactly one — i.e.
	// the per-row check must use the well-known guid, not just hepid+profile.
	var sipCall int64
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mapping_schema WHERE guid = '`+defaultMappingGUIDCall+`'`,
	).Scan(&sipCall); err != nil {
		t.Fatalf("count sip call: %v", err)
	}
	if sipCall != 1 {
		t.Fatalf("legacy SIP/call row was duplicated, got %d", sipCall)
	}

	// And all three OTLP rows must have appeared.
	var otlp int64
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mapping_schema WHERE hepid IN (200,201,202)`,
	).Scan(&otlp); err != nil {
		t.Fatalf("count otlp: %v", err)
	}
	if otlp != 3 {
		t.Fatalf("expected 3 OTLP rows after upgrade-style seed, got %d", otlp)
	}
}
