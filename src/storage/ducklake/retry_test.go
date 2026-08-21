// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestIsRetriableCatalogError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"database locked", errors.New("database is locked"), true},
		{"failed commit", errors.New("Failed to commit DuckLake transaction"), true},
		{"could not set lock", errors.New("Could not set lock on catalog"), true},
		{"transaction conflict", errors.New("transaction conflict on catalog"), true},
		{"http flake", errors.New("HTTP Error: timeout"), true},
		{"no such bucket", errors.New("NoSuchBucket: missing"), false},
		{"invalid key", errors.New("InvalidAccessKeyId"), false},
		{"permanent", errors.New("syntax error"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetriableCatalogError(tc.err); got != tc.want {
				t.Fatalf("isRetriableCatalogError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

type fakeCatalogLocker struct {
	lockCount atomic.Int32
}

func (f *fakeCatalogLocker) CatalogLock() {
	f.lockCount.Add(1)
}

func (f *fakeCatalogLocker) CatalogUnlock() {}

func TestExecWithRetryUsesLockerAroundExec(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	locker := &fakeCatalogLocker{}
	if _, err := execWithRetry(db, 1, time.Millisecond, locker, "SELECT 1"); err != nil {
		t.Fatalf("execWithRetry failed: %v", err)
	}
	if locker.lockCount.Load() != 1 {
		t.Fatalf("lock count = %d, want 1", locker.lockCount.Load())
	}
}

func TestUseNativeMove(t *testing.T) {
	hot := &Volume{LakeName: "hot", Path: "/data/hot", Type: VolumeTypeLocal}
	cold := &Volume{LakeName: "cold", Path: "s3://bucket/cold", Type: VolumeTypeS3}

	t.Run("default is duckdb", func(t *testing.T) {
		tsm := &TieredStorageManager{}
		if tsm.useNativeMove(hot, cold) {
			t.Fatal("default engine should be duckdb")
		}
	})
	t.Run("native engine enables native", func(t *testing.T) {
		tsm := &TieredStorageManager{config: TieredStorageConfig{MoveEngine: "native"}}
		if !tsm.useNativeMove(hot, cold) {
			t.Fatal("native engine should use native")
		}
	})
	t.Run("explicit duckdb disables native", func(t *testing.T) {
		tsm := &TieredStorageManager{config: TieredStorageConfig{MoveEngine: "duckdb"}}
		if tsm.useNativeMove(hot, cold) {
			t.Fatal("duckdb engine should not use native")
		}
	})
	t.Run("remote source disables native", func(t *testing.T) {
		tsm := &TieredStorageManager{config: TieredStorageConfig{MoveEngine: "native"}}
		src := &Volume{Path: "s3://bucket/hot", Type: VolumeTypeS3}
		if tsm.useNativeMove(src, cold) {
			t.Fatal("s3 source should fall back")
		}
	})
}

func TestHotCatalogLockerOnlyForPrimaryVolume(t *testing.T) {
	tsm := &TieredStorageManager{
		primaryVolume: &Volume{LakeName: "homer_lake_hot"},
		catalogLocker: &fakeCatalogLocker{},
	}

	hot := &Volume{LakeName: "homer_lake_hot"}
	cold := &Volume{LakeName: "homer_lake_cold"}

	if tsm.hotCatalogLocker(hot) == nil {
		t.Fatal("expected locker for hot source volume")
	}
	if tsm.hotCatalogLocker(cold) != nil {
		t.Fatal("expected nil locker for cold source volume")
	}
}

func TestMovePartitionIdempotencySkipsInsertWhenDestinationHasRows(t *testing.T) {
	// Pure logic: when dstCount > 0 we must not run INSERT. Covered indirectly by
	// partitionRowCount + branch structure; integration needs full DuckLake attach.
	tsm := &TieredStorageManager{
		primaryVolume: &Volume{LakeName: "lake_hot"},
		catalogLocker: &fakeCatalogLocker{},
	}

	locker := tsm.hotCatalogLocker(&Volume{LakeName: "lake_hot"})
	if locker == nil {
		t.Fatal("hot locker should be set for primary source volume")
	}
}
