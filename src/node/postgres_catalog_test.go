// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
package node

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/sipcapture/homer-core/src/config"
)

func TestNodeReadsPostgresDuckLakeCatalog(t *testing.T) {
	dsn := os.Getenv("HOMER_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set HOMER_TEST_POSTGRES_DSN for PostgreSQL node integration")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "homer_node_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		_ = admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); _ = admin.Close(ctx) })
	cfg, _ := pgx.ParseConfig(dsn)
	u := url.URL{Scheme: "postgresql", Host: net.JoinHostPort(cfg.Host, strconv.Itoa(int(cfg.Port))), User: url.UserPassword(cfg.User, cfg.Password), Path: "/" + name, RawQuery: "sslmode=disable"}
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, ext := range []string{"ducklake", "postgres"} {
		if _, err := db.Exec("LOAD " + ext); err != nil {
			t.Fatalf("load checksum-verified %s extension (run scripts/download_duckdb_extensions.sh): %v", ext, err)
		}
	}
	data := filepath.Join(t.TempDir(), "data")
	vol := config.VolumeConfig{Name: "default", Type: "local", CatalogType: "postgres", CatalogPath: u.String(), Path: data}
	info, err := attachVolume(db, "lake", vol)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE lake.test (id BIGINT); INSERT INTO lake.test VALUES (42)"); err != nil {
		t.Fatal(err)
	}
	reader, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if _, err := reader.Exec("LOAD ducklake; LOAD postgres"); err != nil {
		t.Fatal(err)
	}
	if _, err := attachVolume(reader, "lake", vol); err != nil {
		t.Fatal(err)
	}
	lease, acquired, err := ducklake.AcquireMaintenance(ctx, true, ducklake.CatalogPostgres, u.String())
	if err != nil || !acquired {
		t.Fatalf("vacuum test owner: %v %v", acquired, err)
	}
	defer lease.Release()
	n := &Node{config: &config.NodeConfig{DuckLake: config.DuckLakeConfig{Stateless: true}}, db: reader, volumes: []VolumeInfo{info}}
	response := httptest.NewRecorder()
	n.handleVacuum(response, httptest.NewRequest(http.MethodPost, "/vacuum", nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("vacuum bypassed maintenance lock: status=%d body=%s", response.Code, response.Body.String())
	}

	n.config.DuckLake.Stateless = false
	response = httptest.NewRecorder()
	n.handleVacuum(response, httptest.NewRequest(http.MethodPost, "/vacuum", nil))
	var result struct {
		Success bool `json:"success"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil || !result.Success {
		t.Fatalf("non-stateless vacuum was gated by another replica: %d %s", response.Code, response.Body.String())
	}
	lease.Release()
	var id int
	if err := reader.QueryRow("SELECT id FROM " + info.LakeName + ".test").Scan(&id); err != nil || id != 42 {
		t.Fatalf("node read: %d %v", id, err)
	}
}

// TestPostgresNodeDuckDBSandbox checks that the node sandbox keeps a
// PostgreSQL DuckLake catalog readable, including rows written after the
// sandbox went on, and does not fall back.
func TestPostgresNodeDuckDBSandbox(t *testing.T) {
	dsn := os.Getenv("HOMER_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set HOMER_TEST_POSTGRES_DSN for PostgreSQL node integration")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "homer_sbx_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		_ = admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); _ = admin.Close(ctx) })
	pc, _ := pgx.ParseConfig(dsn)
	u := url.URL{Scheme: "postgresql", Host: net.JoinHostPort(pc.Host, strconv.Itoa(int(pc.Port))), User: url.UserPassword(pc.User, pc.Password), Path: "/" + name, RawQuery: "sslmode=disable"}

	data := filepath.Join(t.TempDir(), "data") + "/"
	vol := config.VolumeConfig{Name: "default", Type: "local", CatalogType: "postgres", CatalogPath: u.String(), Path: data}

	writer, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	writer.SetMaxOpenConns(1)
	if _, err := writer.Exec("LOAD ducklake; LOAD postgres"); err != nil {
		t.Fatalf("load checksum-verified extensions (run scripts/download_duckdb_extensions.sh): %v", err)
	}
	if _, err := attachVolume(writer, "homer_lake", vol); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec("CREATE TABLE homer_lake.main.t (id BIGINT); INSERT INTO homer_lake.main.t VALUES (1)"); err != nil {
		t.Fatal(err)
	}

	on := true
	cfg := &config.NodeConfig{
		DuckDBSandbox: &on,
		DuckLake: config.DuckLakeConfig{
			LakeName: "homer_lake",
			Tuning:   config.DuckDBTuning{TempDirectory: filepath.Join(t.TempDir(), "spill")},
			Volumes:  []config.VolumeConfig{vol},
		},
	}
	db, _, err := openNodeDuckDB(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var ext string
	if err := db.QueryRow("SELECT current_setting('enable_external_access')::VARCHAR").Scan(&ext); err != nil || ext != "false" {
		t.Fatalf("sandbox fell back with a PostgreSQL catalog: %q %v", ext, err)
	}
	if _, err := writer.Exec("INSERT INTO homer_lake.main.t VALUES (2)"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM homer_lake.main.t").Scan(&n); err != nil || n != 2 {
		t.Fatalf("sandboxed read: n=%d err=%v", n, err)
	}
	if _, err := db.Exec("SELECT * FROM read_text('/etc/passwd')"); err == nil {
		t.Fatal("expected read_text to be refused")
	}
}
