// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package node

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/sipcapture/homer-core/src/config"
)

func allInOneConfigs() (*config.NodeConfig, *config.StorageConfig) {
	n := &config.NodeConfig{DuckLake: config.DuckLakeConfig{
		LakeName: "homer_lake",
		Volumes: []config.VolumeConfig{{
			Name: "default", Type: "local", Path: "/data/homer/parquet",
			CatalogType: "sqlite", CatalogPath: "/data/homer/homer_catalog.sqlite",
		}},
	}}
	s := &config.StorageConfig{DuckLake: config.DuckLakeConfig{
		LakeName:    "homer_lake",
		CatalogType: "sqlite",
		CatalogPath: "/data/homer/./homer_catalog.sqlite",
		DataPath:    "/data/homer/parquet",
	}}
	return n, s
}

func TestIsolatedReadsPlan(t *testing.T) {
	off := false
	cases := []struct {
		name   string
		mutate func(*config.NodeConfig, *config.StorageConfig)
		want   bool
	}{
		{"matching catalog", func(*config.NodeConfig, *config.StorageConfig) {}, true},
		{"sandbox off", func(n *config.NodeConfig, _ *config.StorageConfig) { n.DuckDBSandbox = &off }, false},
		{"tiered storage", func(_ *config.NodeConfig, s *config.StorageConfig) { s.DuckLake.StoragePolicy.Enable = true }, false},
		{"other lake name", func(n *config.NodeConfig, _ *config.StorageConfig) { n.DuckLake.LakeName = "other" }, false},
		{"other catalog", func(n *config.NodeConfig, _ *config.StorageConfig) {
			n.DuckLake.Volumes[0].CatalogPath = "/data/other.sqlite"
		}, false},
		{"volume not default", func(n *config.NodeConfig, _ *config.StorageConfig) { n.DuckLake.Volumes[0].Name = "hot" }, false},
		{"no storage catalog", func(_ *config.NodeConfig, s *config.StorageConfig) { s.DuckLake.CatalogPath = "" }, false},
		{"postgres catalog", func(n *config.NodeConfig, s *config.StorageConfig) {
			dsn := "postgres://homer:pw@db:5432/homer_catalog"
			s.DuckLake.CatalogType, s.DuckLake.CatalogPath = "postgres", dsn
			n.DuckLake.Volumes[0].CatalogType, n.DuckLake.Volumes[0].CatalogPath = "postgres", dsn
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, s := allInOneConfigs()
			tc.mutate(n, s)
			got, reason := IsolatedReadsPlan(n, s)
			if got != tc.want {
				t.Fatalf("IsolatedReadsPlan = %v (%q), want %v", got, reason, tc.want)
			}
			if !got && reason == "" {
				t.Fatal("a refusal must say why")
			}
		})
	}
}

func TestExecDBPrefersWriter(t *testing.T) {
	own, writer := &sql.DB{}, &sql.DB{}
	n := &Node{db: own}
	if n.execDB() != own {
		t.Fatal("standalone node must insert into its own DuckDB")
	}
	n.SetWriterDB(writer)
	if n.execDB() != writer {
		t.Fatal("all-in-one node must insert through the writer DuckDB")
	}
	if n.queryDB() != own {
		t.Fatal("reads must stay on the node DuckDB when only SetWriterDB is used")
	}
}

// TestAllInOneSandboxSeesWriterFlushes is the all-in-one read path: the writer
// keeps its DuckDB open and keeps writing, while the node reads the same lake
// on its own sandboxed DuckDB. Rows and tables written after the node opened
// must be visible without a reconnect.
func TestAllInOneSandboxSeesWriterFlushes(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "parquet") + "/"
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(dir, "catalog.sqlite")

	w, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	defer w.Close()
	w.SetMaxOpenConns(1)
	for _, stmt := range []string{"LOAD ducklake;", "LOAD sqlite;"} {
		if _, err := w.Exec(stmt); err != nil {
			t.Skipf("extension unavailable (%q): %v", stmt, err)
		}
	}
	writerExec := func(q string) {
		t.Helper()
		if _, err := w.Exec(q); err != nil {
			t.Fatalf("writer %q: %v", q, err)
		}
	}
	writerExec(fmt.Sprintf("ATTACH 'ducklake:sqlite:%s' AS homer_lake (DATA_PATH '%s');", catalog, data))
	writerExec("CREATE TABLE homer_lake.main.hep_proto_1_call AS SELECT range AS id FROM range(5);")

	on := true
	nodeCfg := &config.NodeConfig{
		DuckDBSandbox: &on,
		DuckLake: config.DuckLakeConfig{
			LakeName: "homer_lake",
			Tuning:   config.DuckDBTuning{TempDirectory: filepath.Join(dir, "spill")},
			Volumes: []config.VolumeConfig{{
				Name: "default", Type: "local", Path: data,
				CatalogType: "sqlite", CatalogPath: catalog,
			}},
		},
	}
	storageCfg := &config.StorageConfig{DuckLake: config.DuckLakeConfig{
		LakeName: "homer_lake", CatalogType: "sqlite", CatalogPath: catalog, DataPath: data,
	}}
	if ok, reason := IsolatedReadsPlan(nodeCfg, storageCfg); !ok {
		t.Fatalf("IsolatedReadsPlan refused: %s", reason)
	}

	db, _, err := openNodeDuckDB(nodeCfg)
	if err != nil {
		t.Skipf("node DuckDB unavailable: %v", err)
	}
	defer db.Close()
	n := &Node{db: db}
	if !n.SandboxActive() {
		t.Fatal("node DuckDB sandbox fell back")
	}

	count := func(table string) int {
		t.Helper()
		var c int
		if err := db.QueryRow("SELECT count(*) FROM homer_lake.main." + table).Scan(&c); err != nil {
			t.Fatalf("node read %s: %v", table, err)
		}
		return c
	}
	if got := count("hep_proto_1_call"); got != 5 {
		t.Fatalf("initial rows = %d, want 5", got)
	}

	// A flush into an existing table and a first flush into a new table.
	writerExec("INSERT INTO homer_lake.main.hep_proto_1_call SELECT range + 100 FROM range(3);")
	writerExec("CREATE TABLE homer_lake.main.hep_proto_1_registration AS SELECT range AS id FROM range(2);")

	if got := count("hep_proto_1_call"); got != 8 {
		t.Fatalf("rows after writer flush = %d, want 8", got)
	}
	if got := count("hep_proto_1_registration"); got != 2 {
		t.Fatalf("new table rows = %d, want 2", got)
	}

	// The node still refuses host files while reading the live lake.
	if _, err := db.Exec("SELECT * FROM read_text('/etc/passwd')"); err == nil {
		t.Fatal("expected read_text to be refused")
	}
}
