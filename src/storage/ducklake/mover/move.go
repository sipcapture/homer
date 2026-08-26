package mover

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// ErrFallback tells the caller to use DuckDB INSERT…SELECT instead. Typical
// reasons: row-level deletes, inlined rows, missing ducklake_add_data_files,
// or a remote source that we cannot open as a local file.
var ErrFallback = errors.New("native file move unavailable")

func fallback(reason string) error {
	return fmt.Errorf("%w: %s", ErrFallback, reason)
}

// Options configures one native partition file-move.
type Options struct {
	DB          *sql.DB
	SrcLake     string
	DstLake     string
	SrcDataPath string
	DstDataPath string
	TableName   string
	Partition   string
	Copier      Copier
	S3          *S3Config
	// Lock/Unlock serialize hot-catalog access with the writer flush. Held only
	// for metadata reads and the source DELETE — never during the byte copy.
	Lock   func()
	Unlock func()
	// BeforeRegister runs after parquet bytes are on the destination and
	// before ducklake_add_data_files. Native S3 copies can outlive an IMDS
	// token; this is how Homer recreates the DuckDB credential_chain secret.
	BeforeRegister func() error
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

// Result is one successful native move.
type Result struct {
	FilesCopied  int
	RowsMoved    int64
	BytesCopied  int64
	AlreadyOnDst bool
}

// Move copies one date partition's parquet files to the destination lake without
// asking DuckDB to rewrite them, then registers the copies with
// ducklake_add_data_files and retires the source partition with a short DELETE.
//
// The catalog lock is not held while bytes move (local disk or S3 PUT). That is
// the point: a multi-million-row INSERT…SELECT used to pin the writer's
// CatalogLock for the whole copy and then SIGSEGV on the next flush
// (sipcapture/homer#969).
func Move(ctx context.Context, opts Options) (Result, error) {
	if opts.DB == nil {
		return Result{}, fmt.Errorf("mover: no database handle")
	}
	if opts.TableName == "" || opts.Partition == "" {
		return Result{}, fmt.Errorf("mover: table and partition are required")
	}
	if isS3Path(opts.SrcDataPath) {
		return Result{}, fallback("source data_path is remote")
	}

	ok, reason := addDataFilesSupported(ctx, opts.DB)
	if !ok {
		return Result{}, fallback(reason)
	}

	copier := opts.Copier
	if copier == nil {
		var err error
		copier, err = defaultCopier(opts.DstDataPath, opts.S3)
		if err != nil {
			return Result{}, err
		}
	}

	var (
		meta  tableMeta
		files []sourceFile
	)
	err := withRetryOnLocked(func() error {
		opts.lock()
		defer opts.unlock()

		var found bool
		var err error
		meta, found, err = readTableMeta(ctx, opts.DB, opts.SrcLake, opts.TableName)
		if err != nil {
			return err
		}
		if !found {
			return fallback("source table not found")
		}
		if !meta.partition.usable() {
			return fallback(fmt.Sprintf(
				"table is not partitioned by a single identity column (column=%q transform=%q type=%q)",
				meta.partition.columnName, meta.partition.transform, meta.partition.columnType))
		}
		inlined, err := hasInlinedData(ctx, opts.DB, opts.SrcLake)
		if err != nil {
			return err
		}
		if inlined {
			return fallback("source lake still holds inlined rows")
		}
		files, err = partitionFiles(ctx, opts.DB, opts.SrcLake, meta.tableID, opts.Partition)
		return err
	})
	if err != nil {
		return Result{}, err
	}
	if len(files) == 0 {
		return Result{}, fallback("no parquet files for partition (inlined or empty)")
	}
	if filesHaveDeletes(files) {
		return Result{}, fallback("partition has row-level delete files")
	}

	expected := sumRecords(files)
	copied := make([]string, 0, len(files))
	var bytesCopied int64

	logger.Info("native mover: copying partition files",
		"table", opts.TableName,
		"partition", opts.Partition,
		"files", len(files),
		"rows", expected,
		"from", opts.SrcLake,
		"to", opts.DstLake)

	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		src := fileAbs(opts.SrcDataPath, meta.tablePath, f.path, f.pathIsRel)
		rel := destRelPath(f.path, src, opts.SrcDataPath, meta.tablePath, f.pathIsRel)
		dst := destAbs(opts.DstDataPath, meta.tablePath, rel)
		if err := copier.Copy(ctx, src, dst, f.fileSizeBytes); err != nil {
			return Result{}, fmt.Errorf("copy %s → %s: %w", src, dst, err)
		}
		copied = append(copied, dst)
		bytesCopied += f.fileSizeBytes
	}

	if opts.BeforeRegister != nil {
		if err := opts.BeforeRegister(); err != nil {
			return Result{}, fmt.Errorf("refresh destination credentials before register: %w", err)
		}
	}

	// Destination catalog is a different SQLite file than the writer hot catalog.
	// Registering there does not need the writer CatalogLock.
	if err := withRetryOnLocked(func() error {
		return registerCopiedFiles(ctx, opts.DB, opts.DstLake, opts.TableName, meta.partition, opts.Partition, copied, expected)
	}); err != nil {
		return Result{}, err
	}

	// Source DELETE mutates the hot catalog — same lock the writer flush takes.
	if err := withRetryOnLocked(func() error {
		opts.lock()
		defer opts.unlock()
		return retireSourcePartition(ctx, opts.DB, opts.SrcLake, opts.TableName, meta.partition, opts.Partition, expected)
	}); err != nil {
		return Result{}, fmt.Errorf("copied and registered, source delete pending: %w", err)
	}

	logger.Info("native mover: partition moved",
		"table", opts.TableName,
		"partition", opts.Partition,
		"files", len(copied),
		"rows", expected,
		"bytes", bytesCopied)

	return Result{FilesCopied: len(copied), RowsMoved: expected, BytesCopied: bytesCopied}, nil
}
