// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestUsesAzureNativeFileCleanup(t *testing.T) {
	cases := []struct {
		name string
		vol  *Volume
		want bool
	}{
		{name: "nil", vol: nil, want: false},
		{name: "local", vol: &Volume{Type: VolumeTypeLocal}, want: false},
		{name: "s3", vol: &Volume{Type: VolumeTypeS3}, want: false},
		{
			name: "azure account key keeps duckdb path",
			vol:  &Volume{Type: VolumeTypeAzure, AzureAccountName: "acct", AzureAccountKey: "key"},
			want: false,
		},
		{
			name: "azure connection string keeps duckdb path",
			vol:  &Volume{Type: VolumeTypeAzure, AzureConnectionString: "AccountName=a;AccountKey=k"},
			want: false,
		},
		{
			name: "azure credential_chain uses native path",
			vol:  &Volume{Type: VolumeTypeAzure, AzureAccountName: "acct"},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := usesAzureNativeFileCleanup(tc.vol); got != tc.want {
				t.Fatalf("usesAzureNativeFileCleanup = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAzureConcatAndObjectID(t *testing.T) {
	dataPath := "az://homer-data/lake"
	got := azureConcatPath(dataPath, "main/hep_proto_1_call/date=2026-09-16/a.parquet", true)
	want := "az://homer-data/lake/main/hep_proto_1_call/date=2026-09-16/a.parquet"
	if got != want {
		t.Fatalf("relative concat: got %q want %q", got, want)
	}
	abs := azureConcatPath(dataPath, "azure://homer-data/lake/main/x.parquet", false)
	if abs != "azure://homer-data/lake/main/x.parquet" {
		t.Fatalf("absolute concat: got %q", abs)
	}

	id1, ok1 := azureObjectID("az://homer-data/lake/main/x.parquet")
	id2, ok2 := azureObjectID("azure://homer-data/lake/main/x.parquet")
	if !ok1 || !ok2 || id1 != id2 {
		t.Fatalf("az:// and azure:// must share identity: %q / %q", id1, id2)
	}
	id3, ok3 := azureObjectID("s3://bucket/key.parquet")
	if ok3 || id3 != "" {
		t.Fatalf("s3 path must not parse as azure: %q", id3)
	}
}

type fakeAzureStore struct {
	blobs   map[string]struct{}
	deletes []string
}

func (f *fakeAzureStore) DeleteBlob(_ context.Context, azURL string) error {
	f.deletes = append(f.deletes, azURL)
	if id, ok := azureObjectID(azURL); ok {
		delete(f.blobs, id)
	}
	return nil
}

func (f *fakeAzureStore) ListDataFiles(_ context.Context, _ string) ([]string, error) {
	out := make([]string, 0, len(f.blobs))
	for id := range f.blobs {
		parts := strings.SplitN(id, "\x00", 2)
		if len(parts) != 2 {
			continue
		}
		out = append(out, "az://"+parts[0]+"/"+parts[1])
	}
	return out, nil
}

func blobID(t *testing.T, u string) string {
	t.Helper()
	id, ok := azureObjectID(u)
	if !ok {
		t.Fatalf("not an azure url: %s", u)
	}
	return id
}

func TestRunAzureNativeFileCleanup(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	const lake = "testlake"
	dataPath := "az://homer-data/lake/"
	md := ducklakeMetadataIdent(lake)
	stmts := []string{
		`CREATE SCHEMA ` + md,
		`CREATE TABLE ` + md + `.ducklake_schema (schema_id BIGINT, path VARCHAR, path_is_relative BOOLEAN)`,
		`CREATE TABLE ` + md + `.ducklake_table (table_id BIGINT, schema_id BIGINT, path VARCHAR, path_is_relative BOOLEAN)`,
		`CREATE TABLE ` + md + `.ducklake_data_file (data_file_id BIGINT, table_id BIGINT, path VARCHAR, path_is_relative BOOLEAN)`,
		`CREATE TABLE ` + md + `.ducklake_delete_file (data_file_id BIGINT, table_id BIGINT, path VARCHAR, path_is_relative BOOLEAN)`,
		`CREATE TABLE ` + md + `.ducklake_files_scheduled_for_deletion (data_file_id BIGINT, path VARCHAR, path_is_relative BOOLEAN, schedule_start TIMESTAMPTZ)`,
		`INSERT INTO ` + md + `.ducklake_schema VALUES (1, 'main/', true)`,
		`INSERT INTO ` + md + `.ducklake_table VALUES (10, 1, 'hep_proto_1_call/', true)`,
		`INSERT INTO ` + md + `.ducklake_data_file VALUES (1, 10, 'date=2026-09-16/live.parquet', true)`,
		`INSERT INTO ` + md + `.ducklake_files_scheduled_for_deletion VALUES (2, 'main/hep_proto_1_call/date=2026-09-15/old.parquet', true, NOW())`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("setup %s: %v", stmt, err)
		}
	}

	live := "az://homer-data/lake/main/hep_proto_1_call/date=2026-09-16/live.parquet"
	old := "az://homer-data/lake/main/hep_proto_1_call/date=2026-09-15/old.parquet"
	orphan := "az://homer-data/lake/main/hep_proto_1_call/date=2026-09-14/orphan.parquet"
	store := &fakeAzureStore{blobs: map[string]struct{}{
		blobID(t, live):   {},
		blobID(t, old):    {},
		blobID(t, orphan): {},
	}}

	if err := runAzureNativeFileCleanup(context.Background(), db, lake, dataPath, store); err != nil {
		t.Fatalf("runAzureNativeFileCleanup: %v", err)
	}

	if _, ok := store.blobs[blobID(t, live)]; !ok {
		t.Fatal("live catalog file must not be deleted")
	}
	if _, ok := store.blobs[blobID(t, old)]; ok {
		t.Fatal("scheduled file must be deleted")
	}
	if _, ok := store.blobs[blobID(t, orphan)]; ok {
		t.Fatal("orphan must be deleted")
	}

	var scheduled int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + md + `.ducklake_files_scheduled_for_deletion`).Scan(&scheduled); err != nil {
		t.Fatal(err)
	}
	if scheduled != 0 {
		t.Fatalf("scheduled rows left = %d, want 0", scheduled)
	}
}

func TestQueryKnownAzureFilesIncludesScheduled(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	const lake = "knownlake"
	md := ducklakeMetadataIdent(lake)
	for _, stmt := range []string{
		`CREATE SCHEMA ` + md,
		`CREATE TABLE ` + md + `.ducklake_schema (schema_id BIGINT, path VARCHAR, path_is_relative BOOLEAN)`,
		`CREATE TABLE ` + md + `.ducklake_table (table_id BIGINT, schema_id BIGINT, path VARCHAR, path_is_relative BOOLEAN)`,
		`CREATE TABLE ` + md + `.ducklake_data_file (data_file_id BIGINT, table_id BIGINT, path VARCHAR, path_is_relative BOOLEAN)`,
		`CREATE TABLE ` + md + `.ducklake_delete_file (data_file_id BIGINT, table_id BIGINT, path VARCHAR, path_is_relative BOOLEAN)`,
		`CREATE TABLE ` + md + `.ducklake_files_scheduled_for_deletion (data_file_id BIGINT, path VARCHAR, path_is_relative BOOLEAN, schedule_start TIMESTAMPTZ)`,
		`INSERT INTO ` + md + `.ducklake_schema VALUES (1, 'main/', true)`,
		`INSERT INTO ` + md + `.ducklake_table VALUES (10, 1, 't/', true)`,
		`INSERT INTO ` + md + `.ducklake_data_file VALUES (1, 10, 'live.parquet', true)`,
		`INSERT INTO ` + md + `.ducklake_files_scheduled_for_deletion VALUES (2, 'old.parquet', true, NOW())`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	known, err := queryKnownAzureFiles(db, lake, "az://c/lake/")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, p := range known {
		if id, ok := azureObjectID(p); ok {
			got[id] = true
		} else {
			t.Fatalf("known path is not az://: %q", p)
		}
	}
	if !got[blobID(t, "az://c/lake/main/t/live.parquet")] {
		t.Fatalf("missing live file in known set: %v", known)
	}
	if !got[blobID(t, "az://c/lake/old.parquet")] {
		t.Fatalf("missing scheduled file in known set: %v", known)
	}
}
