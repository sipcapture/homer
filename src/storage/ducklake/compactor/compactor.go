// Package compactor consolidates a DuckLake table's small parquet files without
// letting DuckDB load a whole partition into memory, which is what makes
// ducklake_merge_adjacent_files OOM on wide SIP parquet.
//
// It works in two phases per partition:
//
//  1. Merge: concatenate the partition's parquet row groups into new files of at
//     most a target size, copying one row group at a time, so peak memory is
//     about one row group. No lock is held.
//  2. Swap: in a single short DuckDB transaction, DELETE the partition (which
//     retires whole files, since the predicate covers every row) and register the
//     merged files with ducklake_add_data_files.
//
// The swap is the important part. An earlier version of this package wrote the
// SQLite catalog directly, allocating snapshot ids itself; the live writer's
// cached ids then went stale and the catalog ended up with duplicate snapshots
// ("Corrupt DuckLake - multiple snapshots returned from database"). Here DuckLake
// allocates every snapshot and file id, so the writer's cache cannot fall behind
// and no DETACH/ATTACH cache refresh is needed — ingest and search keep running.
//
// The compactor never opens the catalog file: all reads and writes go through the
// DuckDB handle it is given.
package compactor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// errPartitionChanged reports that a partition no longer matches what was
// planned, so its swap was abandoned. It is expected under concurrent ingest and
// simply defers the partition to the next cycle.
var errPartitionChanged = errors.New("partition changed since planning")

// Options configures a native compaction run.
type Options struct {
	// DB is the DuckDB handle with the lake ATTACHed — the same pool the writer
	// uses. All catalog access goes through it; the compactor never opens the
	// SQLite catalog file itself.
	DB *sql.DB
	// LakeName is the ATTACH alias of the lake (e.g. "homer_lake").
	LakeName string
	// DataPath is the parquet root; files live under {DataPath}/main/{table}/.
	DataPath string
	// TargetFileSizeBytes caps each merged output file (default 512MB).
	TargetFileSizeBytes int64
	// Lock/Unlock serialize catalog access with the DuckLake writer (flush).
	// They are held only for the short swap transaction — never during the slow
	// parquet merge — so ingestion is not blocked for minutes. Both may be nil
	// (e.g. in tests) for no locking.
	Lock   func()
	Unlock func()
}

func (o Options) lock() {
	if o.Lock != nil {
		o.Lock()
	}
}

func (o Options) unlock() {
	if o.Unlock != nil {
		o.Unlock()
	}
}

// Result summarizes one CompactTable call.
type Result struct {
	Skipped             bool
	SkipReason          string
	FilesBefore         int
	FilesMerged         int
	FilesCreated        int
	PartitionsCompacted int
	PartitionsDeferred  int
}

const defaultTargetFileSizeBytes int64 = 512 << 20 // 512MB

// CompactTable consolidates one table's small parquet files, partition by
// partition. For each partition it writes the merged parquet without holding any
// lock, then swaps it in with a single short DuckDB transaction (see
// swapPartition) that lets DuckLake allocate the snapshot.
//
// Committing per partition keeps progress durable incrementally, holds the
// catalog lock only for the swap, and never leaves more than one partition's
// old+new files on disk at once. Superseded files are removed by the caller's
// regular ducklake_expire_snapshots / ducklake_cleanup_old_files maintenance,
// which respects the configured time-travel window.
func CompactTable(ctx context.Context, opts Options, tableName string) (Result, error) {
	if opts.DB == nil {
		return Result{}, fmt.Errorf("compactor: no database handle")
	}
	target := opts.TargetFileSizeBytes
	if target <= 0 {
		target = defaultTargetFileSizeBytes
	}

	// Planning reads the catalog, so it runs under the same lock the writer's
	// flush takes. These are indexed metadata reads on a small SQLite file, so the
	// lock is held for milliseconds — unlike the merge below, which must not hold
	// it. Reading without the lock races a flush's commit and both sides fail with
	// "database is locked".
	var (
		meta       tableMeta
		partitions map[string][]sourceFile
		skip       string
	)
	err := withRetryOnLocked(func() error {
		opts.lock()
		defer opts.unlock()

		var ok bool
		var err error
		meta, ok, err = readTableMeta(ctx, opts.DB, opts.LakeName, tableName)
		if err != nil {
			return err
		}
		if !ok {
			skip = "table not found"
			return nil
		}
		if !meta.partition.usable() {
			skip = fmt.Sprintf(
				"table is not partitioned by a single identity column (column=%q transform=%q type=%q)",
				meta.partition.columnName, meta.partition.transform, meta.partition.columnType)
			return nil
		}

		// A partition is retired with one DELETE, which cannot preserve row-level
		// deletes, and inlined rows are invisible to the parquet merge but would
		// be swept away by that DELETE.
		deletes, err := hasDeleteFiles(ctx, opts.DB, opts.LakeName, meta.tableID)
		if err != nil {
			return err
		}
		if deletes {
			skip = "table has active row-level delete files"
			return nil
		}
		inlined, err := hasInlinedData(ctx, opts.DB, opts.LakeName)
		if err != nil {
			return err
		}
		if inlined {
			skip = "lake still holds inlined rows; flush them first"
			return nil
		}

		partitions, err = activeFilesByPartition(ctx, opts.DB, opts.LakeName, meta.tableID)
		return err
	})
	if err != nil {
		return Result{}, err
	}
	if skip != "" {
		return Result{Skipped: true, SkipReason: skip}, nil
	}

	res := Result{}
	for partVal, files := range partitions {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		res.FilesBefore += len(files)
		if partVal == "" {
			// No partition value means no safe retirement predicate.
			continue
		}
		sizes := make([]int64, len(files))
		for i, f := range files {
			sizes[i] = f.fileSizeBytes
		}
		batches, worth := planPartition(sizes, target)
		if !worth {
			continue
		}

		pr, err := compactPartition(ctx, opts, meta, tableName, partVal, files, batches)
		if err != nil {
			if errors.Is(err, errPartitionChanged) {
				logger.Info("native compactor: partition changed during merge, deferring",
					"table", tableName, "partition", partVal, "reason", err)
				res.PartitionsDeferred++
				continue
			}
			return res, err
		}
		res.PartitionsCompacted++
		res.FilesMerged += pr.filesMerged
		res.FilesCreated += pr.filesCreated
	}

	return res, nil
}

type partitionResult struct {
	filesMerged  int
	filesCreated int
}

// compactPartition merges one partition's batches (lock-free), then swaps them
// in under the catalog lock. Every active file of the partition is rewritten,
// because the swap retires the partition wholesale.
func compactPartition(
	ctx context.Context, opts Options, meta tableMeta,
	tableName, partVal string, files []sourceFile, batches [][]int,
) (partitionResult, error) {
	var pr partitionResult
	var writtenOut []string
	var expectedRows int64

	cleanup := func() {
		for _, p := range writtenOut {
			_ = os.Remove(p)
		}
	}

	// Phase 1: write merged parquet without holding the catalog lock.
	for _, batch := range batches {
		if err := ctx.Err(); err != nil {
			cleanup()
			return pr, err
		}
		srcAbs := make([]string, len(batch))
		var batchRows int64
		for i, idx := range batch {
			sf := files[idx]
			srcAbs[i] = tableFileAbs(opts.DataPath, meta.tablePath, sf.path, sf.pathIsRel)
			batchRows += sf.recordCount
		}
		outRel := newOutputRelPath(files[batch[0]].path)
		outAbs := tableFileAbs(opts.DataPath, meta.tablePath, outRel, 1)

		mr, err := mergeParquetFiles(ctx, srcAbs, outAbs)
		if err != nil {
			cleanup()
			return pr, fmt.Errorf("merge partition %s: %w", partVal, err)
		}
		if mr.recordCount != batchRows {
			_ = os.Remove(outAbs)
			cleanup()
			return pr, fmt.Errorf("partition %s: merged row count %d != expected %d",
				partVal, mr.recordCount, batchRows)
		}
		writtenOut = append(writtenOut, outAbs)
		expectedRows += mr.recordCount
		pr.filesMerged += len(batch)
		pr.filesCreated++

		logger.Info("native compactor: merged batch",
			"table", tableName, "partition", partVal,
			"input_files", len(batch), "output_bytes", mr.fileSizeBytes, "rows", mr.recordCount)
	}

	if len(writtenOut) == 0 {
		return partitionResult{}, nil
	}

	// Phase 2: swap under the catalog lock (short).
	if err := withRetryOnLocked(func() error {
		opts.lock()
		defer opts.unlock()
		return swapPartition(ctx, opts.DB, opts.LakeName, swapRequest{
			tableName:    tableName,
			partition:    meta.partition,
			partitionVal: partVal,
			expectedRows: expectedRows,
			mergedAbs:    writtenOut,
		})
	}); err != nil {
		// The transaction rolled back, so the merged files are unreferenced.
		cleanup()
		return partitionResult{}, err
	}

	logger.Info("native compactor: swapped partition",
		"table", tableName, "partition", partVal,
		"new_files", pr.filesCreated, "retired_files", pr.filesMerged, "rows", expectedRows)
	return pr, nil
}

// tableFileAbs builds the absolute filesystem path of a catalog file entry.
func tableFileAbs(dataPath, tablePath, relPath string, isRel int64) string {
	if isRel == 0 {
		return relPath
	}
	return filepath.Join(dataPath, "main", tablePath, relPath)
}

// newOutputRelPath returns the relative path for a new merged file in the same
// partition directory as the source files (e.g. "date=2026-05-30/ducklake-<uuid>.parquet").
// Staying in the partition directory is required: DuckLake derives the added
// file's partition value from that directory name.
func newOutputRelPath(srcRelPath string) string {
	dir := filepath.Dir(srcRelPath)
	name := "ducklake-" + uuid.NewString() + ".parquet"
	if dir == "." || dir == "" {
		return name
	}
	return filepath.Join(dir, name)
}

// HiveLikePathComponent returns the first component of an absolute data path
// that looks like a hive partition ("key=value"). DuckLake infers partition
// values from the entire path of an added file, so such a component would be
// read as a column the table does not have.
func HiveLikePathComponent(dataPath string) string {
	for _, part := range strings.Split(filepath.Clean(dataPath), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		if i := strings.IndexByte(part, '='); i > 0 {
			return part
		}
	}
	return ""
}
