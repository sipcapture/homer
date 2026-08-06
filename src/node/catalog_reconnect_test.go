// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package node

import (
	"database/sql"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestCatalogReplaceDB(t *testing.T) {
	t.Parallel()

	oldDB, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open old: %v", err)
	}
	t.Cleanup(func() { _ = oldDB.Close() })

	cat := NewDuckLakeCatalog(oldDB, "homer_lake", nil)
	if cat.db != oldDB {
		t.Fatal("expected catalog to hold old DB")
	}

	newDB, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open new: %v", err)
	}
	t.Cleanup(func() { _ = newDB.Close() })

	vols := []VolumeInfo{{Name: "default", LakeName: "homer_lake", Path: "/tmp"}}
	cat.replaceDB(newDB, vols)

	if cat.db != newDB {
		t.Fatal("catalog.db not swapped")
	}
	if len(cat.volumes) != 1 || cat.volumes[0].Name != "default" {
		t.Fatalf("volumes not swapped: %+v", cat.volumes)
	}
	main, err := cat.Schema(t.Context(), "main")
	if err != nil {
		t.Fatalf("main schema missing after replace: %v", err)
	}
	schema, ok := main.(*DuckLakeSchema)
	if !ok {
		t.Fatalf("unexpected schema type %T", main)
	}
	if schema.db != newDB {
		t.Fatal("schema still points at old DB")
	}
}

func TestQueryDBSeesSwappedHandle(t *testing.T) {
	t.Parallel()

	oldDB, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open old: %v", err)
	}
	t.Cleanup(func() { _ = oldDB.Close() })

	newDB, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open new: %v", err)
	}
	t.Cleanup(func() { _ = newDB.Close() })

	n := &Node{db: oldDB, catalog: NewDuckLakeCatalog(oldDB, "homer_lake", nil)}
	if n.queryDB() != oldDB {
		t.Fatal("expected old DB from queryDB")
	}

	n.mu.Lock()
	n.db = newDB
	n.mu.Unlock()
	n.catalog.replaceDB(newDB, nil)

	if n.queryDB() != newDB {
		t.Fatal("queryDB did not observe swapped handle")
	}
}
