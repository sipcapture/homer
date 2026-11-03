// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
package node

import (
	"context"
	"database/sql"
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
	lease, acquired, err := ducklake.AcquireMaintenance(ctx, ducklake.CatalogPostgres, u.String())
	if err != nil || !acquired {
		t.Fatalf("vacuum test owner: %v %v", acquired, err)
	}
	defer lease.Release()
	n := &Node{db: reader, volumes: []VolumeInfo{info}}
	response := httptest.NewRecorder()
	n.handleVacuum(response, httptest.NewRequest(http.MethodPost, "/vacuum", nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("vacuum bypassed maintenance lock: status=%d body=%s", response.Code, response.Body.String())
	}
	lease.Release()
	var id int
	if err := reader.QueryRow("SELECT id FROM " + info.LakeName + ".test").Scan(&id); err != nil || id != 42 {
		t.Fatalf("node read: %d %v", id, err)
	}
}
