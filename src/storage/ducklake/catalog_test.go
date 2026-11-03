// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
package ducklake

import (
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/sipcapture/homer-core/src/config"
	"strings"
	"testing"
)

func TestAttachSQLBuildersReturnErrors(t *testing.T) {
	const invalidDSN = "postgres://user:private@host:%zz/catalog"
	if stmt, err := buildLakeAttachSQL(Config{CatalogType: CatalogPostgres, CatalogPath: invalidDSN}); err == nil || stmt != "" {
		t.Fatalf("writer attach: statement=%q error=%v", stmt, err)
	}
	if stmt, err := volumeAttachSQL(CatalogPostgres, &Volume{LakeName: "lake"}, invalidDSN); err == nil || stmt != "" {
		t.Fatalf("volume attach: statement=%q error=%v", stmt, err)
	}
	if _, err := buildLakeAttachSQL(Config{CatalogType: "unsupported"}); err == nil {
		t.Fatal("unsupported catalog type was ignored")
	}
}

func TestCatalogErrorKeepsDiagnosticsWithoutCredentials(t *testing.T) {
	for _, dsn := range []string{
		"postgres://catalog_user:p%40ss%27word@db.example:5433/homer_1?sslmode=require",
		"POSTGRES://catalog_user:p%40ss%27word@db.example:5433/homer_1?sslmode=require",
		"host=db.example port=5433 dbname=homer_1 user=catalog_user password='p@ss\\'word' sslmode=require",
	} {
		cfg, err := pgx.ParseConfig(config.NormalizePostgresDSN(dsn))
		if err != nil {
			t.Fatal(err)
		}
		stmt, err := CatalogAttachSQL(CatalogPostgres, dsn, "lake", "/data/parquet", false)
		if err != nil {
			t.Fatal(err)
		}
		original := fmt.Errorf("ATTACH failed: %s\nconnection to server at db.example:5433: FATAL: database \"homer_1\" does not exist", stmt)
		message := CatalogError(dsn, "attach", original).Error()
		for _, want := range []string{"db.example", "5433", "homer_1", "does not exist"} {
			if !strings.Contains(message, want) {
				t.Errorf("missing diagnostic %q: %s", want, message)
			}
		}
		for _, secret := range []string{dsn, cfg.User, cfg.Password, "p%40ss%27word", "p@ss''word"} {
			if strings.Contains(message, secret) {
				t.Error("attach error exposed credentials")
			}
		}
		message = CatalogError(dsn, "attach", fmt.Errorf("password %s rejected for user %q", cfg.Password, cfg.User)).Error()
		if strings.Contains(message, cfg.Password) || strings.Contains(message, cfg.User) {
			t.Error("server diagnostic exposed credentials")
		}
	}
	original := errors.New("local attach failed")
	if !errors.Is(CatalogError("catalog.sqlite", "attach", original), original) {
		t.Fatal("local catalog error was not preserved")
	}
}

func TestPostgresCatalogConfiguration(t *testing.T) {
	for _, dsn := range []string{
		"postgres://user:p%40ss@localhost/base?sslmode=require&application_name=homer",
		"POSTGRES://user:p%40ss@localhost/base?sslmode=require",
		"postgresql://user:p%40ss@localhost/base?sslmode=require",
		"postgresql://user@localhost/ignored?dbname=base&sslmode=require",
		"host=localhost dbname=base user=homer password='a b' sslmode=require",
	} {
		ct, err := ResolveCatalogType(CatalogPostgres, dsn)
		if err != nil || ct != CatalogPostgres {
			t.Fatalf("resolve: %s %v", ct, err)
		}
		shard, err := suffixedPostgresCatalog(dsn, "1")
		if err != nil {
			t.Fatal(err)
		}
		baseCfg, _ := pgx.ParseConfig(config.NormalizePostgresDSN(dsn))
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

func TestPostgresCatalogTypeMismatch(t *testing.T) {
	for _, typ := range []CatalogType{"", CatalogSQLite} {
		if _, err := ResolveCatalogType(typ, "postgres://user:private@localhost/lake"); err == nil {
			t.Fatalf("accepted PostgreSQL DSN with type %q", typ)
		}
	}
	if typ, err := ResolveCatalogType("", "catalog.sqlite"); err != nil || typ != CatalogSQLite {
		t.Fatalf("SQLite default: %q, %v", typ, err)
	}
}
