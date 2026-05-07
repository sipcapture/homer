// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package coordinator

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

// openTestSettingsDB opens a fresh file-backed DuckDB (file-backed so the
// duckdb driver behaves exactly like in production; in-memory has subtly
// different parsing in some versions) and runs EnsureSettingsSchema.
func openTestSettingsDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.db")
	db, err := sql.Open("duckdb", path)
	if err != nil {
		t.Fatalf("open duckdb: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := services.EnsureSettingsSchema(db); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	return db
}

func TestSeedDefaultCorrelationScript_InsertsBothTemplates(t *testing.T) {
	db := openTestSettingsDB(t)

	if err := seedDefaultCorrelationScript(db); err != nil {
		t.Fatalf("seedDefaultCorrelationScript returned error: %v", err)
	}

	// All correlation rows exist?
	var count int
	if err := db.QueryRow(
		fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE type = 'correlation'`, services.CorrelationScriptsTable)).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 correlation rows after seeding, got %d", count)
	}

	// Each (hepid, profile) pair present with status=false and a non-empty script.
	for _, s := range defaultCorrelationSeeds {
		var (
			profile  string
			hepid    int
			status   bool
			script   string
			hepAlias string
			typ      string
		)
		err := db.QueryRow(
			fmt.Sprintf(`SELECT profile, hepid, status, script, hep_alias, type
			 FROM %s
			 WHERE type = 'correlation' AND hepid = ? AND profile = ?`,
				services.CorrelationScriptsTable),
			s.hepID, s.profile).Scan(&profile, &hepid, &status, &script, &hepAlias, &typ)
		if err != nil {
			t.Fatalf("select seeded row (%d,%s): %v", s.hepID, s.profile, err)
		}
		if status {
			t.Errorf("seeded row (%d,%s): status must be FALSE", hepid, profile)
		}
		if typ != "correlation" {
			t.Errorf("seeded row (%d,%s): type=%q, want correlation", hepid, profile, typ)
		}
		if hepAlias != s.hepAlias {
			t.Errorf("seeded row (%d,%s): hep_alias=%q, want %q", hepid, profile, hepAlias, s.hepAlias)
		}
		if script == "" {
			t.Errorf("seeded row (%d,%s): empty script stored", hepid, profile)
		}
		// The stored script must be identical to the source template.
		// If we accidentally double-escape (ReplaceAll '→'' then driver
		// quotes it again) the stored string will contain `''` pairs that
		// do not exist in the source — we detect that here.
		if script != s.script {
			// Compute the first diff to make the failure actionable.
			minLen := len(s.script)
			if len(script) < minLen {
				minLen = len(script)
			}
			diff := -1
			for i := 0; i < minLen; i++ {
				if script[i] != s.script[i] {
					diff = i
					break
				}
			}
			t.Errorf("seeded row (%d,%s): stored script differs from template. "+
				"len(stored)=%d len(src)=%d firstDiff=%d\n"+
				"stored around diff: %q\nsource around diff: %q",
				hepid, profile, len(script), len(s.script), diff,
				safeSlice(script, diff-10, diff+40),
				safeSlice(s.script, diff-10, diff+40))
		}
		if strings.Contains(script, "''") && !strings.Contains(s.script, "''") {
			t.Errorf("seeded row (%d,%s): stored script has doubled quotes that "+
				"are not in the template (double-escape bug)", hepid, profile)
		}
	}
}

// TestSeedDefaultCorrelationScript_RepairsLegacyDoubleEscape simulates a
// database left behind by homer-core 11.0.58/59 where the seeder
// double-escaped single quotes in the stored Lua. Running the seeder
// again must rewrite exactly that mangled row back to the clean template,
// without touching operator-edited scripts.
func TestSeedDefaultCorrelationScript_RepairsLegacyDoubleEscape(t *testing.T) {
	db := openTestSettingsDB(t)

	// Pre-populate exactly what the buggy release would have stored: the
	// `call` seed with doubled quotes, and a hand-edited `registration`
	// seed that we expect the repair to leave alone.
	callSeed := defaultCorrelationSeeds[0] // profile=call
	regSeed := defaultCorrelationSeeds[1]  // profile=registration

	mangledCall := strings.ReplaceAll(callSeed.script, "'", "''")
	operatorEdited := "-- custom operator script\nfunction correlate(data, nodes) return {} end\n"

	if _, err := db.Exec(
		fmt.Sprintf(`INSERT INTO %s (guid, profile, hep_alias, type, hepid, status, script, create_date)
		 VALUES (?, ?, ?, 'correlation', ?, FALSE, ?, current_timestamp)`,
			services.CorrelationScriptsTable),
		"legacy-call-guid", callSeed.profile, callSeed.hepAlias, callSeed.hepID, mangledCall); err != nil {
		t.Fatalf("seed legacy call row: %v", err)
	}
	if _, err := db.Exec(
		fmt.Sprintf(`INSERT INTO %s (guid, profile, hep_alias, type, hepid, status, script, create_date)
		 VALUES (?, ?, ?, 'correlation', ?, TRUE, ?, current_timestamp)`,
			services.CorrelationScriptsTable),
		"operator-reg-guid", regSeed.profile, regSeed.hepAlias, regSeed.hepID, operatorEdited); err != nil {
		t.Fatalf("seed operator row: %v", err)
	}

	if err := seedDefaultCorrelationScript(db); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Mangled seed must now equal the clean template.
	var got string
	if err := db.QueryRow(
		fmt.Sprintf(`SELECT script FROM %s WHERE type='correlation' AND hepid=? AND profile=?`,
			services.CorrelationScriptsTable),
		callSeed.hepID, callSeed.profile).Scan(&got); err != nil {
		t.Fatalf("read repaired row: %v", err)
	}
	if got != callSeed.script {
		t.Errorf("legacy mangled `call` row was not repaired: len(got)=%d len(want)=%d",
			len(got), len(callSeed.script))
	}

	// Operator's hand-edited row must be left untouched.
	if err := db.QueryRow(
		fmt.Sprintf(`SELECT script FROM %s WHERE type='correlation' AND hepid=? AND profile=?`,
			services.CorrelationScriptsTable),
		regSeed.hepID, regSeed.profile).Scan(&got); err != nil {
		t.Fatalf("read operator row: %v", err)
	}
	if got != operatorEdited {
		t.Errorf("operator-edited `registration` row must not be overwritten; got %q",
			safeSlice(got, 0, 80))
	}
}

func TestSeedDefaultCorrelationScript_Idempotent(t *testing.T) {
	db := openTestSettingsDB(t)

	for i := 0; i < 3; i++ {
		if err := seedDefaultCorrelationScript(db); err != nil {
			t.Fatalf("seed run %d: %v", i, err)
		}
	}

	var count int
	if err := db.QueryRow(
		fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE type = 'correlation'`, services.CorrelationScriptsTable)).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 rows after 3 seed runs, got %d", count)
	}
}

// TestSeedDefaultCorrelationScript_VisibleViaScriptsService mirrors what
// the UI does: call ScriptsService.List and verify the seeded rows are
// returned. This guards the end-to-end path Settings → Scripts depends on.
func TestSeedDefaultCorrelationScript_VisibleViaScriptsService(t *testing.T) {
	db := openTestSettingsDB(t)

	if err := seedDefaultCorrelationScript(db); err != nil {
		t.Fatalf("seed: %v", err)
	}

	svc := services.NewScriptsService(db)
	items, err := svc.List(context.Background(), services.ScriptsListFilters{Limit: 100})
	if err != nil {
		t.Fatalf("ScriptsService.List: %v", err)
	}

	// Must contain both seeds.
	want := map[string]bool{"call": false, "registration": false}
	for _, it := range items {
		if it.Type == "correlation" && it.HepID == 1 {
			want[it.Profile] = true
			if it.Script == "" {
				t.Errorf("profile=%s visible but script is empty", it.Profile)
			}
		}
	}
	for profile, seen := range want {
		if !seen {
			t.Errorf("profile=%q not visible via ScriptsService.List (this is what the UI sees)", profile)
		}
	}
}

func safeSlice(s string, lo, hi int) string {
	if lo < 0 {
		lo = 0
	}
	if hi > len(s) {
		hi = len(s)
	}
	if lo >= hi {
		return ""
	}
	return s[lo:hi]
}
