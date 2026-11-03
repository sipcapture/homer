// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
package ducklake

import (
	"path/filepath"
	"testing"
)

func TestSQLiteShardSpillDirectoryUnchanged(t *testing.T) {
	dir := t.TempDir()
	spill := filepath.Join(dir, "spill")
	writer, err := NewShardedWriter(Config{CatalogPath: filepath.Join(dir, "catalog.sqlite"),
		LakeName: "lake", DataPath: filepath.Join(dir, "data"), ShardCount: 2,
		TuningThreads: 1, TuningMemoryLimit: "128MB", TuningTempDirectory: spill})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Stop()
	for _, shard := range writer.Shards() {
		var actual string
		if err := shard.db.QueryRow("SELECT current_setting('temp_directory')").Scan(&actual); err != nil {
			t.Fatal(err)
		}
		if actual != spill {
			t.Fatalf("SQLite shard spill directory changed: %q, want %q", actual, spill)
		}
	}
}
