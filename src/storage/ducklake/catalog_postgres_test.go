// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
//go:build !vet

package ducklake

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sipcapture/homer-core/src/decoder"
)

func postgresLakeFixture(t *testing.T) (string, string) {
	t.Helper()
	dsn := os.Getenv("HOMER_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set HOMER_TEST_POSTGRES_DSN for DuckLake PostgreSQL integration")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "homer_lake_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	names := []string{name, name + "_0", name + "_1", name + "_cold"}
	created := []string{}
	t.Cleanup(func() {
		for _, db := range created {
			_, _ = admin.Exec(ctx, "DROP DATABASE "+db+" WITH (FORCE)")
		}
		_ = admin.Close(ctx)
	})
	for _, db := range names {
		if _, err := admin.Exec(ctx, "CREATE DATABASE "+db); err != nil {
			t.Fatal(err)
		}
		created = append(created, db)
	}
	cfg, _ := pgx.ParseConfig(dsn)
	u := url.URL{Scheme: "postgresql", User: url.UserPassword(cfg.User, cfg.Password), Host: net.JoinHostPort(cfg.Host, strconv.Itoa(int(cfg.Port))), Path: "/" + name, RawQuery: "sslmode=disable"}
	installer, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer installer.Close()
	for _, ext := range []string{"ducklake", "postgres"} {
		if _, err := installer.Exec("LOAD " + ext); err == nil {
			continue
		}
		if _, err := installer.Exec("INSTALL " + ext + " FROM 'https://extensions.duckdb.org'"); err != nil {
			t.Fatal(err)
		}
	}
	return u.String(), t.TempDir()
}

func TestPostgresDuckLakeShards(t *testing.T) {
	dsn, dir := postgresLakeFixture(t)
	cfg := Config{CatalogPath: dsn, DataPath: filepath.Join(dir, "data"), LakeName: "lake", ShardCount: 2,
		BatchSize: 100, FlushInterval: time.Hour, TuningThreads: 1, TuningMemoryLimit: "256MB", TuningTempDirectory: filepath.Join(dir, "spill")}
	mgr, err := NewManager(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mgr.Stop() })
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	hep, err := decoder.DecodeHEP(decoder.BenchHEP3SIPPacket())
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.ReleaseHEP(hep)
	key, _, err := ConvertHEPToLakeRow(hep)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err := mgr.WriteHEP(hep); err != nil {
			t.Fatal(err)
		}
	}
	shards := mgr.GetShards()
	if shards[0].config.CatalogPath == shards[1].config.CatalogPath {
		t.Fatal("shards share one PostgreSQL catalog")
	}
	if shards[0].config.TuningTempDirectory == shards[1].config.TuningTempDirectory {
		t.Fatal("shards share one DuckDB spill directory")
	}
	if err := mgr.Stop(); err != nil {
		t.Fatal(err)
	}
	for i, shard := range shards {
		m, err := OpenMaintenanceDB(shard.config, "256MB", nil)
		if err != nil {
			t.Fatal(err)
		}
		var n int
		if err := m.DB.QueryRow(fmt.Sprintf("SELECT count(*) FROM %s.main.%s", shard.config.LakeName, "hep_proto_"+key.String())).Scan(&n); err != nil {
			_ = m.Close()
			t.Fatal(err)
		}
		if n != 2 {
			t.Errorf("shard %d contains %d records, want 2", i, n)
		}
		if _, err := m.DB.Exec("CALL ducklake_merge_adjacent_files('" + shard.config.LakeName + "')"); err != nil {
			_ = m.Close()
			t.Fatal(err)
		}
		_ = m.Close()
	}
}

func TestPostgresDuckLakeTieredVolumes(t *testing.T) {
	dsn, dir := postgresLakeFixture(t)
	tsm, err := NewTieredStorageManager(TieredStorageConfig{Enable: true, CatalogPath: dsn,
		TuningThreads: 1, TuningMemoryLimit: "256MB", TuningTempDirectory: filepath.Join(dir, "spill"),
		Volumes: []Volume{{Name: "hot", Type: VolumeTypeLocal, Priority: 0, Path: filepath.Join(dir, "hot"), LakeName: "hot"},
			{Name: "cold", Type: VolumeTypeLocal, Priority: 1, Path: filepath.Join(dir, "cold"), LakeName: "cold"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := tsm.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tsm.Stop() })
	if _, err := tsm.db.Exec("CREATE TABLE hot.test (id BIGINT); INSERT INTO hot.test VALUES (1); CREATE TABLE cold.test (id BIGINT); INSERT INTO cold.test VALUES (2)"); err != nil {
		t.Fatal(err)
	}
	var hot, cold int
	if err := tsm.db.QueryRow("SELECT id FROM hot.test").Scan(&hot); err != nil {
		t.Fatal(err)
	}
	if err := tsm.db.QueryRow("SELECT id FROM cold.test").Scan(&cold); err != nil {
		t.Fatal(err)
	}
	if hot != 1 || cold != 2 {
		t.Fatal("volumes are not isolated")
	}
}
