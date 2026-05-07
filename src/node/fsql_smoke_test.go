// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package node

import (
	"context"
	"database/sql"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/flight/flightsql"
	"github.com/sipcapture/homer-core/src/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	_ "github.com/duckdb/duckdb-go/v2"
)

// TestFlightSQLSelectConstant runs a minimal Arrow FlightSQL round-trip against
// an in-memory DuckDB handle (no DuckLake attach) to guard the gRPC stack.
func TestFlightSQLSelectConstant(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	cfg := &config.NodeConfig{
		DuckLake: config.DuckLakeConfig{LakeName: "homer_lake"},
	}
	n := &Node{
		db:     db,
		config: cfg,
	}
	fsql := newFsqlServer(n, config.FlightSQLServerConfig{
		Enable: true,
		Host:   "127.0.0.1",
		Port:   0,
	}, "homer_lake")
	if err := fsql.Start(); err != nil {
		t.Fatal(err)
	}
	defer fsql.Stop()

	addr := fsql.listener.Addr().String()
	ctx := context.Background()
	client, err := flightsql.NewClient(addr, nil, nil,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("flightsql client: %v", err)
	}
	defer client.Close()

	info, err := client.Execute(ctx, "SELECT 1 AS x")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(info.Endpoint) == 0 {
		t.Fatal("no flight endpoints")
	}
	reader, err := client.DoGet(ctx, info.Endpoint[0].Ticket)
	if err != nil {
		t.Fatalf("doget: %v", err)
	}
	defer reader.Release()
	if !reader.Next() {
		t.Fatal("expected one batch")
	}
	rec := reader.Record()
	if rec.NumRows() != 1 || rec.NumCols() != 1 {
		t.Fatalf("unexpected shape: rows=%d cols=%d", rec.NumRows(), rec.NumCols())
	}
}
