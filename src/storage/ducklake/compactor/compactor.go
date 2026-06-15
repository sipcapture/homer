package compactor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// Options configures a native compaction run.
type Options struct {
	// CatalogPath is the DuckLake SQLite catalog file (e.g. homer_catalog.sqlite).
	CatalogPath string
	// DataPath is the parquet root; files live under {DataPath}/main/{table}/.
	DataPath string
	// TargetFileSizeBytes caps each merged output file (default 512MB).
	TargetFileSizeBytes int64
	// SnapshotRetention controls how long retired snapshots/files are kept for
	// time travel before the reaper removes them.
	SnapshotRetention time.Duration
	// Lock/Unlock serialize catalog access with the DuckLake writer (flush).
	// They are held only for the short catalog read/commit/reap phases — never
	// during the slow parquet merge — so ingestion is not blocked for minutes.
	// Both may be nil (e.g. in tests) for no locking.
	Lock   func()
	Unlock func()
	// Invalidate drops the DuckLake writer's in-memory snapshot/stats cache so
	// its next flush re-reads the catalog. It is called right after each
	// out-of-band commit while the catalog lock is still held, so the DuckDB
	// writer never reuses a snapshot id this compactor just allocated. May be
	// nil (tests / no live writer).
	Invalidate func()
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

func (o Options) invalidate() {
	if o.Invalidate != nil {
		o.Invalidate()
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
	NewSnapshot         int64
	FilesReaped         int
	SnapshotsPruned     int
}

const defaultTargetFileSizeBytes int64 = 512 << 20 // 512MB

// tableMeta is the table metadata read once under the catalog lock.
type tableMeta struct {
	tableID    int64
	colTypes   map[int64]string
	tablePath  string
	partitions map[string][]sourceFile
}

// CompactTable merges the small parquet files of one table into files up to
// TargetFileSizeBytes, **partition by partition**: for each partition it writes
// the merged parquet (lock-free), then commits a snapshot that registers the
// new files / retires the old ones and reaps superseded data — all without
// DuckDB. Committing per partition means progress is durable incrementally, the
// CatalogLock is only held for the short commit/reap phases (never during the
// slow merge), and disk never holds the old+new set for more than one partition
// at a time.
func CompactTable(ctx context.Context, opts Options, tableName string) (Result, error) {
	target := opts.TargetFileSizeBytes
	if target <= 0 {
		target = defaultTargetFileSizeBytes
	}
	retention := opts.SnapshotRetention
	if retention <= 0 {
		retention = time.Hour
	}

	cat, err := OpenCatalog(opts.CatalogPath)
	if err != nil {
		return Result{}, err
	}
	defer cat.Close()

	// Phase 1: read table metadata under the lock (fast).
	var meta tableMeta
	skip, err := func() (string, error) {
		opts.lock()
		defer opts.unlock()
		tableID, ok, err := cat.tableID(tableName)
		if err != nil {
			return "", err
		}
		if !ok {
			return "table not found", nil
		}
		// Native compaction reassigns row ids; only safe with no delete files.
		hasDeletes, err := cat.hasDeleteFiles(tableID)
		if err != nil {
			return "", err
		}
		if hasDeletes {
			return "table has delete files", nil
		}
		cols, err := cat.columns(tableID)
		if err != nil {
			return "", err
		}
		colTypes := make(map[int64]string, len(cols))
		for _, c := range cols {
			colTypes[c.columnID] = c.colType
		}
		tablePath, err := cat.tablePath(tableID)
		if err != nil {
			return "", err
		}
		partitions, err := cat.activeFilesByPartition(tableID)
		if err != nil {
			return "", err
		}
		meta = tableMeta{tableID: tableID, colTypes: colTypes, tablePath: tablePath, partitions: partitions}
		return "", nil
	}()
	if err != nil {
		return Result{}, err
	}
	if skip != "" {
		return Result{Skipped: true, SkipReason: skip}, nil
	}

	res := Result{}
	for partVal, files := range meta.partitions {
		res.FilesBefore += len(files)
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if len(files) < 2 {
			continue
		}
		sizes := make([]int64, len(files))
		for i, f := range files {
			sizes[i] = f.fileSizeBytes
		}
		batches := planBatches(sizes, target)
		if len(batches) == 0 {
			continue
		}

		pr, err := compactPartition(ctx, cat, opts, meta, partVal, files, batches, retention)
		if err != nil {
			return res, err
		}
		res.PartitionsCompacted++
		res.FilesMerged += pr.filesMerged
		res.FilesCreated += pr.filesCreated
		res.FilesReaped += pr.filesReaped
		res.SnapshotsPruned += pr.snapshotsPruned
		res.NewSnapshot = pr.snapshot
	}

	return res, nil
}

type partitionResult struct {
	filesMerged     int
	filesCreated    int
	snapshot        int64
	filesReaped     int
	snapshotsPruned int
}

// compactPartition merges one partition's batches (lock-free), then commits and
// reaps under the catalog lock.
func compactPartition(
	ctx context.Context, cat *Catalog, opts Options, meta tableMeta,
	partVal string, files []sourceFile, batches [][]int, retention time.Duration,
) (partitionResult, error) {
	var pr partitionResult
	in := commitInput{tableID: meta.tableID}
	var writtenOut []string

	// Phase 2: write merged parquet without holding the catalog lock.
	for _, batch := range batches {
		if err := ctx.Err(); err != nil {
			return pr, err
		}
		batchFiles := make([]sourceFile, len(batch))
		for i, idx := range batch {
			batchFiles[i] = files[idx]
		}

		srcAbs := make([]string, len(batchFiles))
		var expectedRows, batchSrcSize int64
		for i, sf := range batchFiles {
			srcAbs[i] = tableFileAbs(opts.DataPath, meta.tablePath, sf.path, sf.pathIsRel)
			expectedRows += sf.recordCount
			batchSrcSize += sf.fileSizeBytes
		}
		outRel := newOutputRelPath(batchFiles[0].path)
		outAbs := tableFileAbs(opts.DataPath, meta.tablePath, outRel, 1)

		mr, err := mergeParquetFiles(ctx, srcAbs, outAbs)
		if err != nil {
			cleanupFiles(writtenOut)
			return pr, fmt.Errorf("merge partition %s: %w", partVal, err)
		}
		if mr.recordCount != expectedRows {
			_ = os.Remove(outAbs)
			cleanupFiles(writtenOut)
			return pr, fmt.Errorf("partition %s: merged row count %d != expected %d",
				partVal, mr.recordCount, expectedRows)
		}
		writtenOut = append(writtenOut, outAbs)

		for _, sf := range batchFiles {
			in.retired = append(in.retired, retiredFile{
				dataFileID: sf.dataFileID, path: sf.path, pathIsRel: sf.pathIsRel,
			})
		}
		in.netSizeDelta += mr.fileSizeBytes - batchSrcSize

		ref := batchFiles[0]
		in.newFiles = append(in.newFiles, newFile{
			relPath: outRel, fileFormat: "parquet", recordCount: mr.recordCount,
			fileSizeBytes: mr.fileSizeBytes, footerSize: mr.footerSize,
			partitionID: ref.partitionID, mappingID: ref.mappingID, fileOrder: ref.fileOrder,
			partitionValue: partVal,
			batchFiles:     batchFiles, // stats read at commit, under the lock
		})
		pr.filesMerged += len(batchFiles)
		pr.filesCreated++

		logger.Info("native compactor: merged batch",
			"table", partTableName(meta), "partition", partVal,
			"input_files", len(batchFiles), "output_bytes", mr.fileSizeBytes, "rows", mr.recordCount)
	}

	if len(in.newFiles) == 0 {
		return pr, nil
	}

	// Phase 3: commit + reap under the catalog lock (fast).
	opts.lock()
	defer opts.unlock()

	// Aggregate per-column stats now (consistent catalog read under the lock).
	for i := range in.newFiles {
		perFileStats := make([][]colStat, len(in.newFiles[i].batchFiles))
		for j, sf := range in.newFiles[i].batchFiles {
			st, err := cat.columnStats(sf.dataFileID, meta.tableID)
			if err != nil {
				cleanupFiles(writtenOut)
				return pr, fmt.Errorf("read column stats: %w", err)
			}
			perFileStats[j] = st
		}
		in.newFiles[i].stats = aggregateColumnStats(perFileStats, meta.colTypes)
	}

	snap, err := cat.commit(in)
	if err != nil {
		cleanupFiles(writtenOut)
		return pr, fmt.Errorf("commit partition %s: %w", partVal, err)
	}
	pr.snapshot = snap
	logger.Info("native compactor: committed partition snapshot",
		"partition", partVal, "snapshot", snap,
		"new_files", pr.filesCreated, "retired_files", pr.filesMerged)

	reaped, pruned, err := cat.reap(opts.DataPath, retention)
	if err != nil {
		logger.Warn("native compactor: reap failed", "partition", partVal, "error", err)
	} else {
		pr.filesReaped = reaped
		pr.snapshotsPruned = pruned
		if reaped > 0 || pruned > 0 {
			logger.Info("native compactor: reaped superseded data",
				"partition", partVal, "files_deleted", reaped, "snapshots_pruned", pruned)
		}
	}

	// Drop the DuckLake writer's cached snapshot/file-id counters while still
	// holding the catalog lock, so the next flush re-reads this just-committed
	// snapshot instead of reusing its id (which would corrupt the catalog).
	opts.invalidate()

	return pr, nil
}

func cleanupFiles(paths []string) {
	for _, p := range paths {
		_ = os.Remove(p)
	}
}

func partTableName(meta tableMeta) string {
	return filepath.Clean(meta.tablePath)
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
func newOutputRelPath(srcRelPath string) string {
	dir := filepath.Dir(srcRelPath)
	name := "ducklake-" + uuid.NewString() + ".parquet"
	if dir == "." || dir == "" {
		return name
	}
	return filepath.Join(dir, name)
}

// tablePath returns the catalog path of a table (e.g. "hep_proto_1_call/").
func (c *Catalog) tablePath(tableID int64) (string, error) {
	var p string
	if err := c.db.QueryRow(
		`SELECT path FROM ducklake_table WHERE table_id = ? AND end_snapshot IS NULL`,
		tableID,
	).Scan(&p); err != nil {
		return "", fmt.Errorf("read table path: %w", err)
	}
	return p, nil
}
