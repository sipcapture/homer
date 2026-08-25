// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package node

import (
	"context"
	"database/sql"
	"io"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/flight/flightsql"
	"github.com/sipcapture/homer-core/src/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	_ "github.com/duckdb/duckdb-go/v2"
)

func startAuthFsql(t *testing.T, token string) (*fsqlServer, *flightsql.Client) {
	t.Helper()
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)

	n := &Node{
		db:     db,
		config: &config.NodeConfig{DuckLake: config.DuckLakeConfig{LakeName: "homer_lake"}},
	}
	fsql := newFsqlServer(n, config.FlightSQLServerConfig{
		Enable:    true,
		Host:      "127.0.0.1",
		Port:      0,
		AuthToken: token,
	}, "homer_lake")
	if err := fsql.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fsql.Stop)

	client, err := flightsql.NewClient(fsql.listener.Addr().String(), nil, nil,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("flightsql client: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return fsql, client
}

// TestFlightSQLHandshakeWithoutMetadata reproduces Grafana/GizmoSQL/ADBC:
// Handshake is the first RPC and often has no Authorization header.
func TestFlightSQLHandshakeWithoutMetadata(t *testing.T) {
	_, client := startAuthFsql(t, "secret-token")
	stream, err := client.Client.Handshake(context.Background())
	if err != nil {
		t.Fatalf("Handshake RPC: %v", err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}
	_, err = stream.Recv()
	if err != nil && err != io.EOF {
		t.Fatalf("Handshake Recv: %v", err)
	}
}

func TestFlightSQLGetSqlInfoWithBearer(t *testing.T) {
	_, client := startAuthFsql(t, "secret-token")
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer secret-token")
	info, err := client.GetSqlInfo(ctx, nil)
	if err != nil {
		t.Fatalf("GetSqlInfo: %v", err)
	}
	if len(info.Endpoint) == 0 {
		t.Fatal("no endpoints")
	}
	rdr, err := client.DoGet(ctx, info.Endpoint[0].Ticket)
	if err != nil {
		t.Fatalf("DoGet SqlInfo: %v", err)
	}
	defer rdr.Release()
	if !rdr.Next() {
		t.Fatal("expected SqlInfo batch")
	}
}

func TestFlightSQLGetCatalogsWithBearer(t *testing.T) {
	_, client := startAuthFsql(t, "secret-token")
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer secret-token")
	info, err := client.GetCatalogs(ctx)
	if err != nil {
		t.Fatalf("GetCatalogs: %v", err)
	}
	if len(info.Endpoint) == 0 {
		t.Fatal("no endpoints")
	}
	rdr, err := client.DoGet(ctx, info.Endpoint[0].Ticket)
	if err != nil {
		t.Fatalf("DoGet catalogs: %v", err)
	}
	defer rdr.Release()
	if !rdr.Next() {
		t.Fatal("expected catalogs batch")
	}
	rec := rdr.Record()
	if rec.NumRows() != 1 {
		t.Fatalf("catalog rows=%d", rec.NumRows())
	}
}

func TestFlightSQLExecuteRejectedWithoutToken(t *testing.T) {
	_, client := startAuthFsql(t, "secret-token")
	_, err := client.Execute(context.Background(), "SELECT 1 AS x")
	if err == nil {
		t.Fatal("expected unauthenticated execute")
	}
}
