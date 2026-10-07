package writer

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func countParquet(t *testing.T, root string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".parquet") {
			n++
		}
		return nil
	})
	return n
}

// A Line Protocol ?db= write lands in its own schema. Startup ghost recovery
// followed by a full cycle (which deletes orphaned files) must keep it.
func TestGhostRecoveryKeepsTablesOutsideMain(t *testing.T) {
	svc, db := newLPCompactionFixture(t, CompactionConfig{Enable: true}, 1)
	for _, stmt := range []string{
		`CREATE SCHEMA lake.telegraf`,
		`CREATE TABLE lake.telegraf.mem (date DATE, ts TIMESTAMP, used BIGINT)`,
		`ALTER TABLE lake.telegraf.mem SET PARTITIONED BY (date)`,
		`INSERT INTO lake.telegraf.mem VALUES (DATE '2026-10-07', TIMESTAMP '2026-10-07 10:00:00', 1)`,
		`INSERT INTO lake.telegraf.mem VALUES (DATE '2026-10-07', TIMESTAMP '2026-10-07 10:01:00', 2)`,
		`INSERT INTO lake.telegraf.mem VALUES (DATE '2026-10-07', TIMESTAMP '2026-10-07 10:02:00', 3)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	dir := filepath.Join(svc.dataPath, "telegraf", "mem")
	if n := countParquet(t, dir); n != 3 {
		t.Fatalf("parquet files before = %d, want 3", n)
	}

	svc.recoverGhostFiles()
	svc.runCompaction()

	if got := rowCount(t, db, "lake.telegraf.mem"); got != 3 {
		t.Errorf("rows after ghost recovery + cycle = %d, want 3", got)
	}
	if n := countParquet(t, dir); n == 0 {
		t.Errorf("parquet files of telegraf.mem deleted from disk")
	}
}
