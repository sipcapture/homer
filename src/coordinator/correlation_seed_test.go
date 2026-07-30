// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package coordinator

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/coordinator/services"
	"github.com/sipcapture/homer-core/src/scripting/correlation"
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

// TestSeedDefaultCorrelationScript_UpgradesLegacyTemplate verifies that a
// stored script matching a superseded bundled template (fingerprinted in
// legacySeedSHA256) is upgraded in place — preserving status — while
// operator-edited scripts whose hash does not match stay untouched.
func TestSeedDefaultCorrelationScript_UpgradesLegacyTemplate(t *testing.T) {
	db := openTestSettingsDB(t)

	callSeed := defaultCorrelationSeeds[0] // profile=call

	legacyScript := "-- legacy bundled template\nfunction correlate(data, nodes) return {} end\n"
	operatorEdited := "-- operator tweaked\nfunction correlate(data, nodes) return {\"x\"} end\n"

	sum := sha256.Sum256([]byte(legacyScript))
	old := legacySeedSHA256["call"]
	legacySeedSHA256["call"] = append([]string{hex.EncodeToString(sum[:])}, old...)
	t.Cleanup(func() { legacySeedSHA256["call"] = old })

	// Enabled legacy row (must be upgraded, status preserved) and an
	// operator row under another guid (must stay).
	for _, r := range []struct {
		guid, script string
		status       bool
	}{
		{"legacy-template-guid", legacyScript, true},
		{"operator-call-guid", operatorEdited, false},
	} {
		if _, err := db.Exec(
			fmt.Sprintf(`INSERT INTO %s (guid, profile, hep_alias, type, hepid, status, script, create_date)
			 VALUES (?, ?, ?, 'correlation', ?, ?, ?, current_timestamp)`,
				services.CorrelationScriptsTable),
			r.guid, callSeed.profile, callSeed.hepAlias, callSeed.hepID, r.status, r.script); err != nil {
			t.Fatalf("insert %s: %v", r.guid, err)
		}
	}

	if err := seedDefaultCorrelationScript(db); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var script string
	var status bool
	if err := db.QueryRow(
		fmt.Sprintf(`SELECT script, status FROM %s WHERE guid = ?`, services.CorrelationScriptsTable),
		"legacy-template-guid").Scan(&script, &status); err != nil {
		t.Fatalf("read upgraded row: %v", err)
	}
	if script != callSeed.script {
		t.Errorf("legacy template row was not upgraded to the current template")
	}
	if !status {
		t.Errorf("upgrade must preserve status=TRUE")
	}

	if err := db.QueryRow(
		fmt.Sprintf(`SELECT script FROM %s WHERE guid = ?`, services.CorrelationScriptsTable),
		"operator-call-guid").Scan(&script); err != nil {
		t.Fatalf("read operator row: %v", err)
	}
	if script != operatorEdited {
		t.Errorf("operator-edited row must not be overwritten")
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

// seedChainGraph models a multi-hop B2B call as leg -> x_call_id (its parent).
// leg_a is the first leg and has no parent:
//
//	leg_a <- leg_b <- leg_c
//	      <- leg_d <- leg_e
var seedChainGraph = map[string]string{
	"leg_a": "",
	"leg_b": "leg_a",
	"leg_c": "leg_b",
	"leg_d": "leg_a",
	"leg_e": "leg_d",
}

var seedQuotedID = regexp.MustCompile(`'([^']*)'`)

// chainExecutor answers the bundled script's SELECT out of seedChainGraph:
// every leg whose own id, or whose parent id, appears in the query's IN list.
type chainExecutor struct{ calls int }

func (c *chainExecutor) Query(_ context.Context, sql string) ([]map[string]interface{}, error) {
	c.calls++
	wanted := map[string]bool{}
	for _, m := range seedQuotedID.FindAllStringSubmatch(sql, -1) {
		if m[1] != "" {
			wanted[m[1]] = true
		}
	}
	var out []map[string]interface{}
	for leg, parent := range seedChainGraph {
		if wanted[leg] || (parent != "" && wanted[parent]) {
			out = append(out, map[string]interface{}{"session_id": leg, "x_call_id": parent})
		}
	}
	return out, nil
}

type staticScriptLoader struct{ items []correlation.LoadedScript }

func (s *staticScriptLoader) LoadActiveCorrelation(context.Context) ([]correlation.LoadedScript, error) {
	return s.items, nil
}

// TestDefaultCallSeedExpandsWholeChain runs the bundled sip_call.lua and asserts
// that every leg of a multi-hop B2B chain resolves the whole call. A single-pass
// expansion only reaches the immediate neighbours (opening leg_e would return
// just leg_e and leg_d), so this is what pins the transitive behaviour.
func TestDefaultCallSeedExpandsWholeChain(t *testing.T) {
	cfg := config.CorrelationScriptingConfig{
		Enable:          true,
		SQLTimeoutMS:    500,
		ScriptTimeoutMS: 3000,
		MaxSQLCalls:     16,
		MaxSQLRows:      64,
		SyncIntervalSec: 0,
	}
	loader := &staticScriptLoader{items: []correlation.LoadedScript{
		{GUID: "seed", HepID: 1, Profile: "call", Script: defaultCallCorrelationLua},
	}}
	want := []string{"leg_a", "leg_b", "leg_c", "leg_d", "leg_e"}

	for leg, parent := range seedChainGraph {
		exec := &chainExecutor{}
		e := correlation.NewEngine(cfg, loader, exec, "homer_lake")
		if e == nil {
			t.Fatal("NewEngine returned nil despite Enable=true")
		}
		if err := e.ReloadFromDB(context.Background()); err != nil {
			t.Fatalf("ReloadFromDB: %v", err)
		}

		dataExtra := "{}"
		if parent != "" {
			dataExtra = fmt.Sprintf(`{"x_call_id":%q}`, parent)
		}
		res := e.Correlate(context.Background(), correlation.CorrelationInput{
			HepID:      1,
			Profile:    "call",
			ProtoType:  1,
			EventType:  "call",
			SessionIDs: []string{leg},
			BaseRows: []map[string]interface{}{
				{"session_id": leg, "data_extra": dataExtra},
			},
			TimeFrom: 1,
			TimeTo:   2,
		})
		if res == nil {
			t.Fatalf("start %s: nil correlation result", leg)
		}

		got := append([]string(nil), res.ExtraSessionIDs...)
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("start %s: extras = %v, want %v", leg, got, want)
		}
		if exec.calls > cfg.MaxSQLCalls {
			t.Errorf("start %s: %d executeSQL calls exceeds budget %d", leg, exec.calls, cfg.MaxSQLCalls)
		}
	}
}
