// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Compare native PostgreSQL writes read through DuckDB with DuckDB's local
// settings backend, using shipped settings data.
func TestPostgresNativeWriteDuckDBRead(t *testing.T) {
	ctx := context.Background()
	pg := openPostgresSettingsForTest(t)
	native := postgresQueryRow(pg)
	var setting string
	if err := native(ctx, "SHOW standard_conforming_strings").Scan(&setting); err != nil {
		t.Fatal(err)
	}
	if setting != "on" {
		t.Fatalf("standard_conforming_strings = %q, want on", setting)
	}

	local, err := OpenSettingsDB(filepath.Join(t.TempDir(), "settings.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = local.Close() })

	backends := []struct {
		name string
		db   *sql.DB
	}{
		{"duckdb", local}, {"postgres_native", pg},
	}
	// The same text columns isolate literal handling from backend-specific DDL.
	const ddl = "CREATE TABLE literal_readback (name VARCHAR PRIMARY KEY, value VARCHAR)"
	for _, backend := range backends {
		if backend.db == pg {
			_, err = pg.Exec("CALL postgres_execute('"+settingsPostgresCatalog+"', ?)", "CREATE TABLE public.literal_readback (name VARCHAR PRIMARY KEY, value VARCHAR)")
		} else {
			_, err = backend.db.Exec(ddl)
		}
		if err != nil {
			t.Fatalf("%s schema: %v", backend.name, err)
		}
	}

	readScript := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("..", "correlation_seed_lua", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	call := HepScript{Profile: "call", HepAlias: "SIP", Type: "correlation", HepID: 1, Script: readScript("sip_call.lua")}
	registration := HepScript{Profile: "registration", HepAlias: "SIP", Type: "correlation", HepID: 1, Script: readScript("sip_registration.lua")}
	requestBody := func(script HepScript) string {
		t.Helper()
		data, err := json.Marshal(script)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	cases := []struct {
		name, initial, updated string
		escape                 func(string) string // nil uses the existing bound JSON parameter helper.
	}{
		{"correlation_profile", call.Profile, registration.Profile, escapeSQL},
		{"correlation_lua", call.Script, registration.Script, escapeJSONData},
		{"sip_field_mapping", string(fieldsMappingSIPCall), string(fieldsMappingSIPRegistration), escapeJSONData},
		{"script_api_json", requestBody(call), requestBody(registration), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, backend := range backends {
				t.Run(backend.name, func(t *testing.T) {
					for step, want := range []string{tc.initial, tc.updated} {
						var query string
						var args []any
						if tc.escape == nil {
							if step == 0 {
								query = "INSERT INTO literal_readback (name, value) VALUES ($1, $2)"
							} else {
								query = "UPDATE literal_readback SET value = $2 WHERE name = $1"
							}
							args = []any{tc.name, jsonDataParam(want)}
						} else {
							if step == 0 {
								query = fmt.Sprintf("INSERT INTO literal_readback (name, value) VALUES ('%s', '%s')", escapeSQL(tc.name), tc.escape(want))
							} else {
								query = fmt.Sprintf("UPDATE literal_readback SET value = '%s' WHERE name = '%s'", tc.escape(want), escapeSQL(tc.name))
							}
						}
						if backend.db == pg {
							var returned string
							if err := native(ctx, query+" RETURNING value", args...).Scan(&returned); err != nil {
								t.Fatal(err)
							}
							if returned != want {
								t.Fatalf("native RETURNING changed fixture at step %d", step)
							}
						} else if _, err := backend.db.ExecContext(ctx, query, args...); err != nil {
							t.Fatal(err)
						}
						var got string
						// PostgreSQL reads deliberately go through DuckDB, not the native helper.
						if err := backend.db.QueryRowContext(ctx, "SELECT value FROM literal_readback WHERE name = $1", tc.name).Scan(&got); err != nil {
							t.Fatal(err)
						}
						if got != want {
							t.Fatalf("step %d: stored fixture differs: got %d bytes, want %d bytes", step, len(got), len(want))
						}
					}
				})
			}
		})
	}
}
