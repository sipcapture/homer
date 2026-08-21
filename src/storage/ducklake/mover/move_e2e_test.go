package mover

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

type twoLakeFixture struct {
	db      *sql.DB
	hotData string
	coldData string
	hotLake  string
	coldLake string
	table    string
}

func newTwoLakeFixture(t *testing.T) *twoLakeFixture {
	t.Helper()
	dir := t.TempDir()
	hotData := filepath.Join(dir, "hot")
	coldData := filepath.Join(dir, "cold")
	if err := os.MkdirAll(hotData, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(coldData, 0o755); err != nil {
		t.Fatal(err)
	}
	hotCat := filepath.Join(dir, "hot.sqlite")
	coldCat := filepath.Join(dir, "cold.sqlite")

	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{"LOAD ducklake;", "LOAD sqlite;"} {
		if _, err := db.Exec(stmt); err != nil {
			t.Skipf("ducklake/sqlite extension unavailable (%q): %v", stmt, err)
		}
	}
	if _, err := db.Exec(fmt.Sprintf(
		"ATTACH 'ducklake:sqlite:%s' AS hot (DATA_PATH '%s');", hotCat, hotData)); err != nil {
		t.Skipf("ATTACH hot failed: %v", err)
	}
	if _, err := db.Exec(fmt.Sprintf(
		"ATTACH 'ducklake:sqlite:%s' AS cold (DATA_PATH '%s');", coldCat, coldData)); err != nil {
		t.Skipf("ATTACH cold failed: %v", err)
	}

	f := &twoLakeFixture{
		db: db, hotData: hotData, coldData: coldData,
		hotLake: "hot", coldLake: "cold", table: "calls",
	}
	f.mustExec(t, `CALL hot.set_option('data_inlining_row_limit', 0)`)
	f.mustExec(t, `CALL cold.set_option('data_inlining_row_limit', 0)`)
	ddl := `CREATE TABLE %s.calls (date DATE, id BIGINT, method VARCHAR, payload VARCHAR)`
	f.mustExec(t, fmt.Sprintf(ddl, "hot"))
	f.mustExec(t, fmt.Sprintf(ddl, "cold"))
	f.mustExec(t, `ALTER TABLE hot.calls SET PARTITIONED BY (date)`)
	f.mustExec(t, `ALTER TABLE cold.calls SET PARTITIONED BY (date)`)
	return f
}

func (f *twoLakeFixture) mustExec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := f.db.Exec(q, args...); err != nil {
		t.Fatalf("exec %s: %v", q, err)
	}
}

func (f *twoLakeFixture) insert(t *testing.T, day string, from, to int64) {
	t.Helper()
	f.mustExec(t, fmt.Sprintf(
		`INSERT INTO hot.calls
		 SELECT DATE '%s', i, 'INVITE', repeat('x', 80) FROM range(%d, %d) tbl(i)`,
		day, from, to))
}

func (f *twoLakeFixture) count(t *testing.T, lake, day string) int64 {
	t.Helper()
	var n int64
	q := fmt.Sprintf(`SELECT COUNT(*) FROM %s.calls WHERE date = DATE '%s'`, lake, day)
	if err := f.db.QueryRow(q).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", q, err)
	}
	return n
}

func (f *twoLakeFixture) options(day string) Options {
	return Options{
		DB:          f.db,
		SrcLake:     f.hotLake,
		DstLake:     f.coldLake,
		SrcDataPath: f.hotData,
		DstDataPath: f.coldData,
		TableName:   f.table,
		Partition:   day,
	}
}

func TestPartitionValueMatches(t *testing.T) {
	if !partitionValueMatches("2026-07-18", "2026-07-18") {
		t.Fatal("exact")
	}
	if !partitionValueMatches("2026-07-18 00:00:00", "2026-07-18") {
		t.Fatal("datetime stored")
	}
	if partitionValueMatches("2026-07-19", "2026-07-18") {
		t.Fatal("different day")
	}
}

func TestNativeMoveLocalToLocal(t *testing.T) {
	f := newTwoLakeFixture(t)
	if ok, reason := addDataFilesSupported(context.Background(), f.db); !ok {
		t.Skip(reason)
	}

	day := "2026-07-18"
	f.insert(t, day, 0, 40)
	f.insert(t, day, 40, 80)
	f.insert(t, "2026-07-19", 0, 10)
	if got := f.count(t, "hot", day); got != 80 {
		t.Fatalf("hot before = %d", got)
	}

	res, err := Move(context.Background(), f.options(day))
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if res.RowsMoved != 80 {
		t.Errorf("rows moved = %d, want 80", res.RowsMoved)
	}
	if res.FilesCopied < 2 {
		t.Errorf("files copied = %d, want at least 2", res.FilesCopied)
	}
	if got := f.count(t, "hot", day); got != 0 {
		t.Errorf("hot after = %d, want 0", got)
	}
	if got := f.count(t, "cold", day); got != 80 {
		t.Errorf("cold after = %d, want 80", got)
	}
	if got := f.count(t, "hot", "2026-07-19"); got != 10 {
		t.Errorf("untouched partition vanished: hot 2026-07-19 = %d", got)
	}

	// Writer-equivalent: source lake must still accept inserts after the delete.
	f.insert(t, "2026-07-20", 0, 5)
	if got := f.count(t, "hot", "2026-07-20"); got != 5 {
		t.Errorf("post-move insert = %d, want 5", got)
	}
}

func TestNativeMoveIdempotentWhenAlreadyOnDestination(t *testing.T) {
	f := newTwoLakeFixture(t)
	if ok, reason := addDataFilesSupported(context.Background(), f.db); !ok {
		t.Skip(reason)
	}
	day := "2026-07-18"
	f.insert(t, day, 0, 20)
	if _, err := Move(context.Background(), f.options(day)); err != nil {
		t.Fatalf("first Move: %v", err)
	}
	// Replay: source empty. Move should fallback (no files) so the caller can
	// treat it as already done via the existing dstCount path — or succeed as no-op.
	_, err := Move(context.Background(), f.options(day))
	if err == nil {
		t.Fatal("expected fallback on empty source")
	}
	if !errors.Is(err, ErrFallback) {
		t.Fatalf("want ErrFallback, got %v", err)
	}
	if got := f.count(t, "cold", day); got != 20 {
		t.Errorf("cold rows = %d after retry", got)
	}
}

func TestNativeMoveFallsBackOnRowLevelDeletes(t *testing.T) {
	f := newTwoLakeFixture(t)
	if ok, reason := addDataFilesSupported(context.Background(), f.db); !ok {
		t.Skip(reason)
	}
	day := "2026-07-18"
	f.insert(t, day, 0, 30)
	f.mustExec(t, `DELETE FROM hot.calls WHERE id < 5`)
	var deletes int64
	if err := f.db.QueryRow(
		`SELECT COUNT(*) FROM __ducklake_metadata_hot.ducklake_delete_file WHERE end_snapshot IS NULL`).
		Scan(&deletes); err != nil {
		t.Fatalf("delete files: %v", err)
	}
	if deletes == 0 {
		t.Skip("this DuckLake version retired whole files instead of row-level deletes")
	}
	_, err := Move(context.Background(), f.options(day))
	if !errors.Is(err, ErrFallback) {
		t.Fatalf("want ErrFallback for deletes, got %v", err)
	}
}

type holdTracker struct {
	held      atomic.Bool
	heldCopy  atomic.Bool
	lockCalls atomic.Int32
}

func (h *holdTracker) Lock() {
	h.held.Store(true)
	h.lockCalls.Add(1)
}

func (h *holdTracker) Unlock() {
	h.held.Store(false)
}

func TestNativeMoveDoesNotHoldLockDuringCopy(t *testing.T) {
	f := newTwoLakeFixture(t)
	if ok, reason := addDataFilesSupported(context.Background(), f.db); !ok {
		t.Skip(reason)
	}
	day := "2026-07-18"
	f.insert(t, day, 0, 15)

	h := &holdTracker{}
	local := LocalCopier{}
	opts := f.options(day)
	opts.Lock = h.Lock
	opts.Unlock = h.Unlock
	opts.Copier = CopierFunc(func(ctx context.Context, src, dst string, size int64) error {
		if h.held.Load() {
			h.heldCopy.Store(true)
		}
		return local.Copy(ctx, src, dst, size)
	})

	if _, err := Move(context.Background(), opts); err != nil {
		t.Fatalf("Move: %v", err)
	}
	if h.heldCopy.Load() {
		t.Fatal("catalog lock was held during parquet copy")
	}
	if h.lockCalls.Load() < 2 {
		t.Fatalf("expected lock around plan and source delete, got %d", h.lockCalls.Load())
	}
	if got := f.count(t, "cold", day); got != 15 {
		t.Errorf("cold = %d", got)
	}
}
