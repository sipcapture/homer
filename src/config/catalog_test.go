// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
package config

import (
	"strings"
	"testing"
)

func TestPostgresCatalogValidation(t *testing.T) {
	for _, tc := range []struct {
		name, typ, path, want string
		fail                  bool
	}{
		{"default", "", "catalog.sqlite", "sqlite", false},
		{"sqlite normalization", " SQLITE ", "catalog.sqlite", "sqlite", false},
		{"postgres", "postgres", "POSTGRES://user:private@localhost/lake", "postgres", false},
		{"alias", "PostgreSQL", "postgresql://user:private@localhost/lake", "postgres", false},
		{"keyword", "postgres", "host=localhost dbname=lake password=private", "postgres", false},
		{"unsupported type", "other", "catalog.sqlite", "", true},
		{"sqlite mismatch", "sqlite", "postgres://user:private@localhost/lake", "", true},
		{"default mismatch", "", "postgres://user:private@localhost/lake", "", true},
		{"postgres mismatch", "postgres", "catalog.sqlite", "", true},
		{"malformed", "postgres", "postgres://user:private@host:%zz/lake", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveCatalogType(tc.typ, tc.path)
			if got != tc.want || (err != nil) != tc.fail {
				t.Fatalf("got %q, %v; want %q, failure=%t", got, err, tc.want, tc.fail)
			}
			if err != nil && strings.Contains(err.Error(), "private") {
				t.Fatal("validation exposed credentials")
			}
			cfg := &Config{}
			cfg.Storage.DuckLake.CatalogType = tc.typ
			cfg.Storage.DuckLake.CatalogPath = tc.path
			loadErr := validateDuckLakeCatalogTypes(cfg)
			if (loadErr != nil) != tc.fail {
				t.Fatalf("startup validation differs: %v", loadErr)
			}
			if loadErr != nil && !strings.Contains(loadErr.Error(), "storage.ducklake.catalog_type") {
				t.Fatal("missing configuration field context")
			}
			if loadErr == nil && cfg.Storage.DuckLake.CatalogType != got {
				t.Fatal("startup normalization differs")
			}
		})
	}
}
