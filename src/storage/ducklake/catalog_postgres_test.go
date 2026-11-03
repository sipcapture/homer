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
		if _, err := installer.Exec("LOAD " + ext); err != nil {
			t.Fatalf("load checksum-verified %s extension (run scripts/download_duckdb_extensions.sh): %v", ext, err)
		}
	}
	return u.String(), t.TempDir()
}

func TestPostgresMissingCatalogError(t *testing.T) {
	dsn, dir := postgresLakeFixture(t)
	missing, err := suffixedPostgresCatalog(dsn, "missing")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := pgx.ParseConfig(missing)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := loadLakeExtensions(db, CatalogPostgres); err != nil {
		t.Fatal(err)
	}
	stmt, err := CatalogAttachSQL("", missing, "lake", dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(stmt); err == nil {
		t.Fatal("attached a database that does not exist")
	} else {
		message := CatalogError(missing, "attach", err).Error()
		for _, want := range []string{cfg.Database, cfg.Host, "does not exist"} {
			if !strings.Contains(message, want) {
				t.Fatalf("missing diagnostic %q: %s", want, message)
			}
		}
		if (cfg.Password != "" && strings.Contains(message, cfg.Password)) || strings.Contains(message, missing) {
			t.Fatal("attach error exposed credentials")
		}
	}
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
	tsm, err := NewTieredStorageManager(TieredStorageConfig{Enable: true, CatalogPath: strings.Replace(dsn, "postgresql://", "POSTGRES://", 1),
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
	if _, err := tsm.db.Exec("CALL hot.set_option('DATA_INLINING_ROW_LIMIT', 0); CALL cold.set_option('DATA_INLINING_ROW_LIMIT', 0); CREATE TABLE hot.test (id BIGINT, date DATE, payload VARCHAR); CREATE TABLE cold.test (id BIGINT, date DATE, payload VARCHAR)"); err != nil {
		t.Fatal(err)
	}
	hep, err := decoder.DecodeHEP(decoder.BenchHEP3SIPPacket())
	if err != nil {
		t.Fatal(err)
	}
	defer decoder.ReleaseHEP(hep)
	if _, err := tsm.db.Exec("INSERT INTO hot.test VALUES (1, DATE '2026-01-01', ?), (2, DATE '2026-01-01', ?), (3, DATE '2026-01-02', ?)", hep.Payload, hep.Payload, hep.Payload); err != nil {
		t.Fatal(err)
	}
	hot, cold := tsm.volumes[0], tsm.volumes[1]
	if hot.CatalogPath == cold.CatalogPath {
		t.Fatal("volumes share a PostgreSQL catalog")
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := tsm.MovePartition("test", "2026-01-01", hot, cold); err != nil {
			t.Fatal(err)
		}
		var hotOld, hotRecent, coldOld int
		if err := tsm.db.QueryRow("SELECT count(*) FROM hot.test WHERE date = DATE '2026-01-01'").Scan(&hotOld); err != nil {
			t.Fatal(err)
		}
		if err := tsm.db.QueryRow("SELECT count(*) FROM hot.test WHERE date = DATE '2026-01-02' AND id = 3 AND payload = ?", hep.Payload).Scan(&hotRecent); err != nil {
			t.Fatal(err)
		}
		if err := tsm.db.QueryRow("SELECT count(*) FROM cold.test WHERE date = DATE '2026-01-01' AND id IN (1, 2) AND payload = ?", hep.Payload).Scan(&coldOld); err != nil {
			t.Fatal(err)
		}
		if hotOld != 0 || hotRecent != 1 || coldOld != 2 {
			t.Fatalf("move attempt %d: hot old=%d hot recent=%d cold old=%d", attempt, hotOld, hotRecent, coldOld)
		}
	}
	files := 0
	if err := filepath.Walk(cold.Path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".parquet") {
			files++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("tier move did not write parquet data to the cold volume")
	}
	t.Logf("verified tier move with %d cold-volume parquet files", files)

	maintenance, err := tsm.fileCleanupDB(cold)
	if err != nil {
		t.Fatal(err)
	}
	var tierSpill, maintenanceSpill string
	if err := tsm.db.QueryRow("SELECT current_setting('temp_directory')").Scan(&tierSpill); err != nil {
		t.Fatal(err)
	}
	if err := maintenance.QueryRow("SELECT current_setting('temp_directory')").Scan(&maintenanceSpill); err != nil {
		t.Fatal(err)
	}
	if tierSpill == maintenanceSpill {
		t.Fatal("tiering and maintenance share a spill directory")
	}

}

func TestPostgresTierStartupClosesOnAttachFailure(t *testing.T) {
	dsn, dir := postgresLakeFixture(t)
	dsn, err := suffixedPostgresCatalog(dsn, "missing")
	if err != nil {
		t.Fatal(err)
	}
	tsm, err := NewTieredStorageManager(TieredStorageConfig{Enable: true, CatalogPath: dsn, TuningThreads: 1, TuningMemoryLimit: "128MB", TuningTempDirectory: dir,
		Volumes: []Volume{{Name: "hot", Type: VolumeTypeLocal, Path: dir, LakeName: "hot"}}})
	if err != nil {
		t.Fatal(err)
	}
	err = tsm.Start()
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing catalog diagnostic: %v", err)
	}
	if tsm.db != nil {
		_ = tsm.db.Close()
		t.Fatal("failed start retained DuckDB")
	}
}
