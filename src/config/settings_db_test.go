// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import "testing"

func TestIsPostgresSettingsPath(t *testing.T) {
	for _, value := range []string{
		"postgres://user:password@localhost/settings", "postgresql://localhost/settings",
		"POSTGRES://user:password@localhost/settings",
		" postgres://localhost/settings ", "host=localhost dbname=settings user=homer",
		"user = 'homer' password='with spaces' dbname=settings", "dbname=settings",
		"sslmode=require host=localhost", "service=homer", "postgres://bad:%zz@host/db",
	} {
		if !IsPostgresSettingsPath(value) {
			t.Errorf("did not recognize PostgreSQL syntax: %q", value)
		}
		if SettingsDBLocalPath(value, "") != defaultSettingsDBPath {
			t.Error("DSN used as local secret path")
		}
	}
	for _, value := range []string{
		"", "homer_settings.duckdb", "/data/homer/settings.duckdb", "./data/settings.duckdb",
		"/data/host=local/settings.duckdb", "settings=db.duckdb", "settings.duckdb?access_mode=read_only",
	} {
		if IsPostgresSettingsPath(value) {
			t.Errorf("misclassified local path: %q", value)
		}
		if value != "" && SettingsDBLocalPath(value, "") != value {
			t.Errorf("changed local path: %q", value)
		}
	}
}

func TestNormalizePostgresDSN(t *testing.T) {
	for input, want := range map[string]string{
		"POSTGRES://User:Pass@Host/Database?application_name=Homer": "postgres://User:Pass@Host/Database?application_name=Homer",
		"postgresql://User:Pass@Host/Database":                      "postgresql://User:Pass@Host/Database",
		"host=Host dbname=Database user=User password='Pass Word'":  "host=Host dbname=Database user=User password='Pass Word'",
	} {
		if got := NormalizePostgresDSN(input); got != want {
			t.Fatalf("normalized DSN changed connection values")
		}
	}
}
