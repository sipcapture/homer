// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package node

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestBufferTimeFilter(t *testing.T) {
	bounded := "SELECT * FROM homer_lake.main.hep_proto_1_call WHERE timestamp >= (to_timestamp(1000 / 1000.0) AT TIME ZONE 'UTC') AND timestamp <= (to_timestamp(5000 / 1000.0) AT TIME ZONE 'UTC') ORDER BY timestamp DESC LIMIT 10"
	got := bufferTimeFilter(bounded)
	if !strings.Contains(got, "to_timestamp(1000 / 1000.0)") || !strings.Contains(got, "to_timestamp(5000 / 1000.0)") {
		t.Fatalf("bounded filter = %q", got)
	}
	withOr := strings.Replace(bounded, "ORDER BY", "OR sid = 'x' ORDER BY", 1)
	if got := bufferTimeFilter(withOr); got != "" {
		t.Fatalf("a query with OR must copy the whole buffer, got %q", got)
	}
	if got := bufferTimeFilter("SELECT * FROM homer_lake.main.hep_proto_1_call LIMIT 10"); got != "" {
		t.Fatalf("an unbounded query must copy the whole buffer, got %q", got)
	}
}

func TestMemoryUnionUsesStagedBuffers(t *testing.T) {
	n := &Node{
		db:       &sql.DB{},
		writerDB: &sql.DB{},
		config:   &config.NodeConfig{DuckLake: config.DuckLakeConfig{LakeName: "homer_lake", SearchBuffer: true}},
	}
	mq := n.buildMemoryUnionQueries("SELECT count(*) FROM homer_lake.main.hep_proto_1_call")
	if !mq.ok {
		t.Fatal("expected a memory union")
	}
	if len(mq.memTables) != 2 || mq.memTables[0] != "mem_hep_proto_1_call_a" || mq.memTables[1] != "mem_hep_proto_1_call_b" {
		t.Fatalf("memTables = %v", mq.memTables)
	}
	if !strings.Contains(mq.combinedSQL, "memory.main.mem_hep_proto_1_call_a") {
		t.Fatalf("staged buffers must be read from the node catalog: %s", mq.combinedSQL)
	}
	for _, name := range mq.memTables {
		if !memBufferTableRe.MatchString(name) {
			t.Fatalf("%q does not match memBufferTableRe", name)
		}
	}
	for _, bad := range []string{"mem_hep_proto_1_call_a; DROP", "hep_proto_1_call", "mem_hep_proto_1_call_c"} {
		if memBufferTableRe.MatchString(bad) {
			t.Fatalf("%q must not match memBufferTableRe", bad)
		}
	}
}

// TestAllInOneSandboxCopiesWriterBuffers runs a search_buffer search the way an
// all-in-one node does with the sandbox on: lake rows from the sandboxed node
// DuckDB plus unflushed rows copied out of the writer's memory buffers.
func TestAllInOneSandboxCopiesWriterBuffers(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "parquet") + "/"
	spill := filepath.Join(dir, "spill")
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
	for _, q := range []string{
		fmt.Sprintf("ATTACH 'ducklake:sqlite:%s' AS homer_lake (DATA_PATH '%s');", catalog, data),
		"CREATE TABLE homer_lake.main.hep_proto_1_call (id INTEGER, timestamp TIMESTAMP);",
		"INSERT INTO homer_lake.main.hep_proto_1_call SELECT range, TIMESTAMP '2026-01-01' FROM range(5);",
		// Writer buffers, as ducklake.NewTableWriter creates them.
		"CREATE TABLE mem_hep_proto_1_call_a (id INTEGER, timestamp TIMESTAMP);",
		"CREATE TABLE mem_hep_proto_1_call_b (id INTEGER, timestamp TIMESTAMP);",
		"INSERT INTO mem_hep_proto_1_call_a SELECT range + 100, TIMESTAMP '2026-01-02' FROM range(3);",
		"INSERT INTO mem_hep_proto_1_call_b SELECT range + 200, TIMESTAMP '2026-01-02' FROM range(2);",
	} {
		if _, err := w.Exec(q); err != nil {
			t.Fatalf("writer %q: %v", q, err)
		}
	}

	on := true
	cfg := &config.NodeConfig{
		DuckDBSandbox: &on,
		DuckLake: config.DuckLakeConfig{
			LakeName:     "homer_lake",
			SearchBuffer: true,
			Tuning:       config.DuckDBTuning{TempDirectory: spill},
			Volumes: []config.VolumeConfig{{
				Name: "default", Type: "local", Path: data,
				CatalogType: "sqlite", CatalogPath: catalog,
			}},
		},
	}
	db, vols, err := openNodeDuckDB(cfg)
	if err != nil {
		t.Skipf("node DuckDB unavailable: %v", err)
	}
	defer db.Close()
	n := &Node{config: cfg, db: db, volumes: vols}
	n.SetWriterDB(w)
	if !n.SandboxActive() {
		t.Fatal("node DuckDB sandbox fell back")
	}
	if !n.bufferCopyEnabled() {
		t.Fatal("buffer copy must be on for sandboxed all-in-one reads with search_buffer")
	}

	ctx := context.Background()
	rows, _, plan, err := n.runSharedSelectWithMemoryPolicy(ctx, db, "SELECT count(*) AS c FROM homer_lake.main.hep_proto_1_call")
	if err != nil {
		t.Fatalf("aggregate search (%s): %v", plan.mode, err)
	}
	if len(rows) != 1 || fmt.Sprint(rows[0]["c"]) != "10" {
		t.Fatalf("count with buffers = %v (plan %s), want 10", rows, plan.mode)
	}

	rows, _, plan, err = n.runSharedSelectWithMemoryPolicy(ctx, db,
		"SELECT id FROM homer_lake.main.hep_proto_1_call ORDER BY timestamp DESC LIMIT 100")
	if err != nil {
		t.Fatalf("top-N search (%s): %v", plan.mode, err)
	}
	if len(rows) != 10 {
		t.Fatalf("top-N rows = %d (plan %s), want 10", len(rows), plan.mode)
	}

	// Copies are dropped after the search and no copy file is left behind.
	var left int
	if err := db.QueryRow("SELECT count(*) FROM duckdb_tables() WHERE table_name LIKE 'mem_hep_proto_%'").Scan(&left); err != nil || left != 0 {
		t.Fatalf("staged buffer tables left = %d err=%v", left, err)
	}
	if matches, _ := filepath.Glob(filepath.Join(spill, "homer-buffer-*.parquet")); len(matches) != 0 {
		t.Fatalf("copy files left: %v", matches)
	}

	// The sandbox still refuses host files on the node DuckDB.
	if _, err := db.Exec("SELECT * FROM read_text('/etc/passwd')"); err == nil {
		t.Fatal("expected read_text to be refused")
	}
}
