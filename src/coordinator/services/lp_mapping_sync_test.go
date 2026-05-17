// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"context"
	"path/filepath"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestLPMappingGUID_Stable(t *testing.T) {
	a := LPMappingGUID("main", "lp_cpu")
	b := LPMappingGUID("main", "lp_cpu")
	if a != b {
		t.Fatalf("LPMappingGUID not stable: %q vs %q", a, b)
	}
	if len(a) != 36 {
		t.Fatalf("expected canonical UUID length, got %q", a)
	}
	if c := LPMappingGUID("main", "lp_mem"); c == a {
		t.Fatalf("LPMappingGUID collided across measurements: %q", a)
	}
	// case-insensitive on schema/table — same logical identity
	// produces the same guid regardless of how the catalog returns
	// the names back to us.
	if up := LPMappingGUID("MAIN", "LP_CPU"); up != a {
		t.Fatalf("LPMappingGUID case-sensitive: %q vs %q", up, a)
	}
}

func TestLPProfileRoundTrip(t *testing.T) {
	cases := []struct {
		schema, table string
	}{
		{"main", "lp_cpu"},
		{"apps", "lp_http_requests"},
		{"sys_metrics", "lp_uptime_seconds"},
	}
	for _, tc := range cases {
		profile := LPProfileFor(tc.schema, tc.table)
		s, n, ok := SplitLPProfile(profile)
		if !ok {
			t.Fatalf("SplitLPProfile(%q) ok=false", profile)
		}
		if s != tc.schema || n != tc.table {
			t.Fatalf("SplitLPProfile(%q) = (%q,%q), want (%q,%q)", profile, s, n, tc.schema, tc.table)
		}
	}
}

func TestSplitLPProfile_Malformed(t *testing.T) {
	bad := []string{"", "noprefix", "__", "main__", "__cpu", "main_cpu"}
	for _, b := range bad {
		if _, _, ok := SplitLPProfile(b); ok {
			t.Errorf("SplitLPProfile(%q) ok=true, want false", b)
		}
	}
}

func TestLPHepAlias(t *testing.T) {
	if got := LPHepAlias("lp_cpu"); got != "LP_CPU" {
		t.Fatalf("LPHepAlias(lp_cpu) = %q, want LP_CPU", got)
	}
	if got := LPHepAlias("LP_MEM"); got != "LP_MEM" {
		t.Fatalf("LPHepAlias(LP_MEM) = %q, want LP_MEM", got)
	}
	if got := LPHepAlias("temperature"); got != "LP_TEMPERATURE" {
		t.Fatalf("LPHepAlias(temperature) = %q, want LP_TEMPERATURE", got)
	}
}

// upsertMapping is the heart of the sync loop. It must:
//
//   1. INSERT a brand-new row when the stable guid is missing.
//   2. UPDATE only when fields_mapping has actually changed.
//   3. NEVER duplicate rows on repeated SyncOnce calls.
//
// We can drive it directly without the FlightService by stubbing the
// public surface — the only collaborator upsertMapping reaches outside
// the struct is s.db, and BuildLPFieldsMapping which is pure.
func TestLPMappingSync_UpsertLifecycle(t *testing.T) {
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

	svc := NewLPMappingSyncService(db, &FlightService{}, "lp_", 0)

	cpu := discoveredLPTable{
		Schema: "main", Name: "lp_cpu",
		Columns: []LPColumn{
			{Name: "time", DataType: "TIMESTAMP", Position: 1},
			{Name: "host", DataType: "VARCHAR", Position: 2},
			{Name: "usage", DataType: "DOUBLE", Position: 3},
		},
	}

	changed, err := svc.upsertMapping(ctx, cpu)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if !changed {
		t.Fatalf("first upsert should report changed=true")
	}

	var n int64
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mapping_schema WHERE hepid = `+itoa(LPVirtualHepID)).Scan(&n); err != nil {
		t.Fatalf("count after insert: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 lp row, got %d", n)
	}

	// Repeat with identical column set — must be a no-op (changed=false)
	// and must not duplicate the row.
	changed, err = svc.upsertMapping(ctx, cpu)
	if err != nil {
		t.Fatalf("idempotent upsert: %v", err)
	}
	if changed {
		t.Fatalf("identical upsert should report changed=false")
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mapping_schema WHERE hepid = `+itoa(LPVirtualHepID)).Scan(&n); err != nil {
		t.Fatalf("count after no-op: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected still 1 lp row after no-op, got %d", n)
	}

	// Add a new column → must UPDATE (still 1 row, but different fields_mapping).
	cpu.Columns = append(cpu.Columns, LPColumn{Name: "new_field", DataType: "VARCHAR", Position: 4})
	changed, err = svc.upsertMapping(ctx, cpu)
	if err != nil {
		t.Fatalf("schema-evolution upsert: %v", err)
	}
	if !changed {
		t.Fatalf("schema-evolution upsert should report changed=true")
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mapping_schema WHERE hepid = `+itoa(LPVirtualHepID)).Scan(&n); err != nil {
		t.Fatalf("count after evolve: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected still 1 lp row after schema evolve, got %d", n)
	}

	// Different (schema, table) must produce a second row.
	mem := discoveredLPTable{
		Schema: "main", Name: "lp_mem",
		Columns: []LPColumn{
			{Name: "time", DataType: "TIMESTAMP", Position: 1},
			{Name: "free_bytes", DataType: "BIGINT", Position: 2},
		},
	}
	if _, err := svc.upsertMapping(ctx, mem); err != nil {
		t.Fatalf("second-table upsert: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mapping_schema WHERE hepid = `+itoa(LPVirtualHepID)).Scan(&n); err != nil {
		t.Fatalf("count after second table: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 lp rows after adding mem, got %d", n)
	}
}

// TestLPMappingSync_PurgeInternalMappings drives purgeInternalLPMappings
// against a real DuckDB-backed settings DB. It pre-seeds the
// mapping_schema with a mix of legitimate LP rows (lp_cpu) and
// catalog-internal rows that earlier versions of this service would
// have published (DuckLake metadata tables, information_schema), then
// verifies the purge:
//
//   - deletes every catalog-internal row,
//   - leaves the genuine lp_* row intact,
//   - is idempotent (a second call leaves the table unchanged).
func TestLPMappingSync_PurgeInternalMappings(t *testing.T) {
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

	svc := NewLPMappingSyncService(db, &FlightService{}, "", 0)

	// Seed: one legitimate measurement plus a sampling of the
	// internal-catalog rows we want gone.
	seedRows := []struct {
		schema, table string
	}{
		{"main", "lp_cpu"}, // keep
		{"main", "ducklake_table"},
		{"main", "ducklake_column"},
		{"main", "ducklake_column_tag"},
		{"main", "ducklake_snapshot"},
		{"main", "ducklake_data_file"},
		{"main", "ducklake_file_column_stats"},
		{"main", "ducklake_partition_column"},
		{"main", "ducklake_partition_info"},
		{"information_schema", "tables"},
		{"pg_catalog", "pg_class"},
	}
	cols := []LPColumn{
		{Name: "time", DataType: "TIMESTAMP", Position: 1},
		{Name: "value", DataType: "DOUBLE", Position: 2},
	}
	for _, sr := range seedRows {
		row := discoveredLPTable{Schema: sr.schema, Name: sr.table, Columns: cols}
		if _, err := svc.upsertMapping(ctx, row); err != nil {
			t.Fatalf("seed %s.%s: %v", sr.schema, sr.table, err)
		}
	}

	var before int64
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mapping_schema WHERE hepid = `+itoa(LPVirtualHepID)).Scan(&before); err != nil {
		t.Fatalf("count before: %v", err)
	}
	if before != int64(len(seedRows)) {
		t.Fatalf("seeded %d rows, COUNT(*) = %d", len(seedRows), before)
	}

	if err := svc.purgeInternalLPMappings(ctx); err != nil {
		t.Fatalf("purgeInternalLPMappings: %v", err)
	}

	var after int64
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mapping_schema WHERE hepid = `+itoa(LPVirtualHepID)).Scan(&after); err != nil {
		t.Fatalf("count after: %v", err)
	}
	if after != 1 {
		t.Fatalf("expected 1 row left after purge (lp_cpu), got %d", after)
	}

	// And what's left must be the legitimate measurement.
	var profile string
	if err := db.QueryRowContext(ctx,
		`SELECT profile FROM mapping_schema WHERE hepid = `+itoa(LPVirtualHepID)).Scan(&profile); err != nil {
		t.Fatalf("inspect remaining: %v", err)
	}
	if profile != "main__lp_cpu" {
		t.Fatalf("unexpected surviving profile %q", profile)
	}

	// Idempotency: second purge is a no-op.
	if err := svc.purgeInternalLPMappings(ctx); err != nil {
		t.Fatalf("idempotent purge: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mapping_schema WHERE hepid = `+itoa(LPVirtualHepID)).Scan(&after); err != nil {
		t.Fatalf("count after idempotent: %v", err)
	}
	if after != 1 {
		t.Fatalf("idempotent purge changed row count: %d", after)
	}
}

// itoa keeps the test SQL legible without pulling strconv into the
// scope of every assertion.
func itoa(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	buf := [20]byte{}
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = digits[i%10]
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
