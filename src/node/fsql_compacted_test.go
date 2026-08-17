// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package node

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/flight/flightsql"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/storage/ducklake/compactor"

	_ "github.com/duckdb/duckdb-go/v2"
)

// TestFlightSQLReadsNativelyCompactedLake closes the one gap the compactor's own
// tests cannot: they prove a second DuckDB reader sees natively compacted data,
// but not that it survives the node's actual serving path. Here a writer handle
// compacts the lake and a separate read-only handle — the node — serves the query
// over Arrow FlightSQL, which is how search reaches a node in production.
func TestFlightSQLReadsNativelyCompactedLake(t *testing.T) {
	dir := t.TempDir()
	if comp := compactor.HiveLikePathComponent(dir); comp != "" {
		t.Skipf("temp dir %q has a hive-like component %q", dir, comp)
	}
	data := filepath.Join(dir, "parquet")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(dir, "catalog.sqlite")

	attach := func(readOnly bool) *sql.DB {
		t.Helper()
		db, err := sql.Open("duckdb", "")
		if err != nil {
			t.Skipf("duckdb unavailable: %v", err)
		}
		db.SetMaxOpenConns(1)
		for _, stmt := range []string{"LOAD ducklake;", "LOAD sqlite;"} {
			if _, err := db.Exec(stmt); err != nil {
				db.Close()
				t.Skipf("extension unavailable (%q): %v", stmt, err)
			}
		}
		opts := fmt.Sprintf("DATA_PATH '%s'", data)
		if readOnly {
			opts += ", READ_ONLY"
		}
		if _, err := db.Exec(fmt.Sprintf(
			"ATTACH 'ducklake:sqlite:%s' AS homer_lake (%s);", catalog, opts)); err != nil {
			db.Close()
			t.Skipf("ATTACH failed: %v", err)
		}
		return db
	}

	writer := attach(false)
	defer writer.Close()

	mustExec := func(q string) {
		t.Helper()
		if _, err := writer.Exec(q); err != nil {
			t.Fatalf("exec %s: %v", q, err)
		}
	}
	// Inlining would keep rows in the catalog instead of parquet, leaving the
	// compactor nothing to merge.
	mustExec(`CALL homer_lake.set_option('data_inlining_row_limit', 0)`)
	mustExec(`CREATE TABLE homer_lake.hep_proto_1_call (
	    date DATE, id BIGINT, extra JSON, payload VARCHAR)`)
	mustExec(`ALTER TABLE homer_lake.hep_proto_1_call SET PARTITIONED BY (date)`)
	const batches = 6
	for i := 0; i < batches; i++ {
		mustExec(fmt.Sprintf(
			`INSERT INTO homer_lake.hep_proto_1_call
			 SELECT DATE '2026-05-30', i, json_object('call_id', i::VARCHAR), repeat('z', 120)
			   FROM range(%d, %d) tbl(i)`, i*10, i*10+10))
	}

	var wantRows int64
	if err := writer.QueryRow(`SELECT COUNT(*) FROM homer_lake.hep_proto_1_call`).Scan(&wantRows); err != nil {
		t.Fatal(err)
	}

	res, err := compactor.CompactTable(context.Background(), compactor.Options{
		DB:                  writer,
		LakeName:            "homer_lake",
		DataPath:            data,
		TargetFileSizeBytes: 64 << 10,
		MaxRowGroupBytes:    1 << 30,
	}, "hep_proto_1_call")
	if err != nil {
		t.Fatalf("CompactTable: %v", err)
	}
	if res.Skipped || res.PartitionsCompacted != 1 {
		t.Fatalf("lake was not compacted: %+v", res)
	}

	// The node attaches the same catalog separately, exactly as a search node does.
	reader := attach(true)
	defer reader.Close()

	n := &Node{
		db:     reader,
		config: &config.NodeConfig{DuckLake: config.DuckLakeConfig{LakeName: "homer_lake"}},
	}
	fsql := newFsqlServer(n, config.FlightSQLServerConfig{
		Enable: true, Host: "127.0.0.1", Port: 0,
	}, "homer_lake")
	if err := fsql.Start(); err != nil {
		t.Fatal(err)
	}
	defer fsql.Stop()

	ctx := context.Background()
	client, err := flightsql.NewClient(fsql.listener.Addr().String(), nil, nil,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("flightsql client: %v", err)
	}
	defer client.Close()

	// Counting over the merged file proves the node reads what the compactor
	// registered; extracting from the JSON column proves the logical type
	// survived the trip as well.
	got := flightScalar(t, ctx, client,
		`SELECT COUNT(*) FROM homer_lake.hep_proto_1_call`)
	if got != wantRows {
		t.Errorf("node counted %d rows over FlightSQL, want %d", got, wantRows)
	}
	tagged := flightScalar(t, ctx, client,
		`SELECT COUNT(*) FROM homer_lake.hep_proto_1_call
		  WHERE json_extract_string(extra, '$.call_id') IS NOT NULL`)
	if tagged != wantRows {
		t.Errorf("node matched %d rows on the JSON column, want %d", tagged, wantRows)
	}
}

// flightScalar runs a query over FlightSQL and returns its single integer result.
func flightScalar(t *testing.T, ctx context.Context, client *flightsql.Client, query string) int64 {
	t.Helper()
	info, err := client.Execute(ctx, query)
	if err != nil {
		t.Fatalf("execute %q: %v", query, err)
	}
	if len(info.Endpoint) == 0 {
		t.Fatalf("no flight endpoints for %q", query)
	}
	rdr, err := client.DoGet(ctx, info.Endpoint[0].Ticket)
	if err != nil {
		t.Fatalf("doget %q: %v", query, err)
	}
	defer rdr.Release()
	if !rdr.Next() {
		t.Fatalf("no batch for %q", query)
	}
	rec := rdr.Record()
	if rec.NumRows() != 1 || rec.NumCols() != 1 {
		t.Fatalf("unexpected shape for %q: rows=%d cols=%d", query, rec.NumRows(), rec.NumCols())
	}
	col, ok := rec.Column(0).(interface{ Value(int) int64 })
	if !ok {
		t.Fatalf("result of %q is %s, not an integer", query, rec.Column(0).DataType())
	}
	return col.Value(0)
}
