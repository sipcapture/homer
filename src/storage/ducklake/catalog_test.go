// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
package ducklake

import (
	"github.com/jackc/pgx/v5"
	"strings"
	"testing"
)

func TestPostgresCatalogConfiguration(t *testing.T) {
	for _, dsn := range []string{
		"postgres://user:p%40ss@localhost/base?sslmode=require&application_name=homer",
		"postgresql://user@localhost/ignored?dbname=base&sslmode=require",
		"host=localhost dbname=base user=homer password='a b' sslmode=require",
	} {
		ct, err := ResolveCatalogType(CatalogSQLite, dsn)
		if err != nil || ct != CatalogPostgres {
			t.Fatalf("resolve: %s %v", ct, err)
		}
		shard, err := suffixedPostgresCatalog(dsn, "1")
		if err != nil {
			t.Fatal(err)
		}
		baseCfg, _ := pgx.ParseConfig(dsn)
		shardCfg, err := pgx.ParseConfig(shard)
		if err != nil || shardCfg.Database != "base_1" || shardCfg.Password != baseCfg.Password || shardCfg.Host != baseCfg.Host {
			t.Fatalf("shard changed connection options or database: %v", err)
		}
		stmt, err := CatalogAttachSQL(ct, dsn, "lake", "/tmp/data's", true)
		if err != nil || !strings.Contains(stmt, "ducklake:postgres:") || !strings.Contains(stmt, "data''s") || !strings.Contains(stmt, "OVERRIDE_DATA_PATH TRUE") {
			t.Fatalf("attach: %q %v", stmt, err)
		}
		if DefaultSpillDirectory(dsn) != "" {
			t.Fatal("DSN was interpreted as a spill path")
		}
		MarkCatalogAttached(dsn)()
		if IsCatalogAttachedInProcess(dsn) {
			t.Fatal("DSN registered as a SQLite file")
		}
	}
	if _, err := ResolveCatalogType(CatalogPostgres, "catalog.sqlite"); err == nil {
		t.Fatal("accepted a file as PostgreSQL DSN")
	}
}
