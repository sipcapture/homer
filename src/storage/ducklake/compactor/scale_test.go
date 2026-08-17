package compactor

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
)

// TestScaleMergeMemoryIsBounded is the measurement behind the native engine's
// reason to exist. ducklake_merge_adjacent_files needs a working set proportional
// to the partition, which is what got Homer OOM-killed; merging row group by row
// group should not.
//
// The assertion is about scaling, not an absolute size: the same merge runs on a
// partition and on one several times larger, and peak memory must stay roughly
// flat. A fraction-of-partition threshold would say nothing, since the useful
// property is independence from partition size.
//
// Gated because it writes gigabytes. The merge is pure Go/Arrow on the default
// allocator, so the Go heap is the honest measure of what it costs — process RSS
// would be dominated by DuckDB's own buffers from the insert phase.
//
//	HOMER_SCALE_TEST=1 \
//	HOMER_SCALE_DIR=/path/on/disk \      # default: os.TempDir(), often tmpfs
//	HOMER_SCALE_FILES=8 \                # files in the smaller run
//	HOMER_SCALE_FACTOR=4 \               # how much larger the second run is
//	HOMER_SCALE_MB_PER_FILE=200 \
//	go test ./storage/ducklake/compactor/ -run TestScaleMergeMemoryIsBounded -v -timeout 60m
func TestScaleMergeMemoryIsBounded(t *testing.T) {
	if os.Getenv("HOMER_SCALE_TEST") != "1" {
		t.Skip("set HOMER_SCALE_TEST=1 to run (writes gigabytes)")
	}
	files := envInt(t, "HOMER_SCALE_FILES", 8)
	factor := envInt(t, "HOMER_SCALE_FACTOR", 4)
	mbPerFile := envInt(t, "HOMER_SCALE_MB_PER_FILE", 200)

	small := measureMerge(t, files, mbPerFile)
	large := measureMerge(t, files*factor, mbPerFile)

	t.Logf("small: %d files, %.0f MB on disk, %d rows, peak heap %.0f MB, merge %s",
		files, mb(small.partitionBytes), small.rows, mbU(small.peakHeap), small.dur.Round(time.Second))
	t.Logf("large: %d files, %.0f MB on disk, %d rows, peak heap %.0f MB, merge %s",
		files*factor, mb(large.partitionBytes), large.rows, mbU(large.peakHeap), large.dur.Round(time.Second))

	dataGrowth := float64(large.partitionBytes) / float64(small.partitionBytes)
	heapGrowth := float64(large.peakHeap) / float64(small.peakHeap)
	t.Logf("data grew %.1fx, peak heap grew %.2fx", dataGrowth, heapGrowth)

	if dataGrowth < 2 {
		t.Fatalf("the two runs are too similar to compare (data grew %.1fx); "+
			"raise HOMER_SCALE_FACTOR", dataGrowth)
	}
	// Streaming means peak memory tracks a row group, so it must stay far below
	// proportional. Allowing a modest rise absorbs GC slack without letting a
	// regression to whole-partition buffering pass.
	if heapGrowth > 1.6 {
		t.Errorf("peak heap grew %.2fx while data grew %.1fx: the merge is buffering "+
			"more than a row group at a time", heapGrowth, dataGrowth)
	}
}

type mergeMeasurement struct {
	partitionBytes int64
	rows           int64
	peakHeap       uint64
	dur            time.Duration
}

// measureMerge builds a single-partition lake of the requested size and records
// peak Go heap while the whole partition is merged into one file.
func measureMerge(t *testing.T, files, mbPerFile int) mergeMeasurement {
	t.Helper()

	base := os.Getenv("HOMER_SCALE_DIR")
	if base == "" {
		base = os.TempDir()
	}
	dir, err := os.MkdirTemp(base, "homer-scale-")
	if err != nil {
		t.Fatalf("temp dir in %s: %v", base, err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	if comp := HiveLikePathComponent(dir); comp != "" {
		t.Skipf("scale dir %q contains a hive-like component %q", dir, comp)
	}

	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(dir, "catalog.sqlite")

	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{"LOAD ducklake;", "LOAD sqlite;"} {
		if _, err := db.Exec(stmt); err != nil {
			t.Skipf("extension unavailable (%q): %v", stmt, err)
		}
	}
	if _, err := db.Exec(fmt.Sprintf(
		"ATTACH 'ducklake:sqlite:%s' AS lake (DATA_PATH '%s');", catalog, data)); err != nil {
		t.Skipf("ATTACH failed: %v", err)
	}
	mustExec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("exec %s: %v", q, err)
		}
	}
	mustExec(`CALL lake.set_option('data_inlining_row_limit', 0)`)
	// Mirrors a HEP table: a wide SIP payload plus a JSON column.
	mustExec(`CREATE TABLE lake.calls (
	    d DATE, id BIGINT, port UINTEGER, ts TIMESTAMP, extra JSON, payload VARCHAR)`)
	mustExec(`ALTER TABLE lake.calls SET PARTITIONED BY (d)`)

	// ~1KB per row, so rows per file ≈ MB per file * 1024.
	rowsPerFile := mbPerFile * 1024
	for i := 0; i < files; i++ {
		from := i * rowsPerFile
		mustExec(fmt.Sprintf(
			`INSERT INTO lake.calls
			 SELECT DATE '2026-05-30', i, 5060 + (i %% 10),
			        TIMESTAMP '2026-05-30 10:00:00' + INTERVAL (i %% 86400) SECOND,
			        json_object('call_id', i::VARCHAR),
			        repeat('A', 900) || i::VARCHAR
			   FROM range(%d, %d) tbl(i)`, from, from+rowsPerFile))
	}

	var m mergeMeasurement
	if err := db.QueryRow(`SELECT COUNT(*) FROM lake.calls`).Scan(&m.rows); err != nil {
		t.Fatal(err)
	}
	m.partitionBytes = dirBytes(t, data)

	// Drop everything the insert phase held so the sample reflects the merge.
	runtime.GC()

	stop := make(chan struct{})
	var peak atomic.Uint64
	done := make(chan struct{})
	go func() {
		defer close(done)
		var ms runtime.MemStats
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				runtime.ReadMemStats(&ms)
				for {
					old := peak.Load()
					if ms.HeapAlloc <= old || peak.CompareAndSwap(old, ms.HeapAlloc) {
						break
					}
				}
			}
		}
	}()

	start := time.Now()
	// A target above the partition size forces one batch covering every file —
	// the worst case for memory.
	res, err := CompactTable(context.Background(), Options{
		DB:                  db,
		LakeName:            "lake",
		DataPath:            data,
		TargetFileSizeBytes: m.partitionBytes * 2,
	}, "calls")
	m.dur = time.Since(start)
	close(stop)
	<-done
	if err != nil {
		t.Fatalf("CompactTable: %v", err)
	}
	if res.PartitionsCompacted != 1 || res.FilesCreated != 1 {
		t.Fatalf("expected the partition merged into one file: %+v", res)
	}
	m.peakHeap = peak.Load()

	var after int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM lake.calls`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != m.rows {
		t.Errorf("row count = %d, want %d", after, m.rows)
	}
	return m
}

func mb(b int64) float64   { return float64(b) / (1 << 20) }
func mbU(b uint64) float64 { return float64(b) / (1 << 20) }

func envInt(t *testing.T, key string, def int) int {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		t.Fatalf("%s=%q is not a positive integer", key, v)
	}
	return n
}

func dirBytes(t *testing.T, root string) int64 {
	t.Helper()
	var total int64
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	}); err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return total
}
