package compactor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// addDataFilesProc is the DuckLake procedure used to register merged parquet.
const addDataFilesProc = "ducklake_add_data_files"

// errTableNotRegisterable reports that DuckLake will not accept a merged file for
// this table because of its column types, no matter how the file is produced.
var errTableNotRegisterable = errors.New("table columns cannot be registered from a parquet file")

// isColumnMappingErr recognises DuckLake's column type mapping refusal, e.g.
// `Failed to map column "big" ... Expected HUGEINT, found type DOUBLE`.
func isColumnMappingErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Failed to map column") ||
		(strings.Contains(msg, "Expected type") && strings.Contains(msg, "but found type"))
}

// SwapSupported reports whether the loaded DuckLake extension exposes the
// procedure the native compactor needs to register merged files. When it does
// not, the caller must stay on the DuckDB merge: there is no other way to hand
// a pre-written parquet file to DuckLake, and writing the catalog out-of-band is
// what corrupted snapshots before.
func SwapSupported(ctx context.Context, db *sql.DB) (bool, string) {
	if db == nil {
		return false, "no database handle"
	}
	var n int64
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM duckdb_functions() WHERE function_name = ?`, addDataFilesProc).Scan(&n)
	if err != nil {
		return false, fmt.Sprintf("cannot probe duckdb_functions(): %v", err)
	}
	if n == 0 {
		return false, addDataFilesProc + " is not available in the loaded ducklake extension"
	}
	return true, ""
}

// quoteLiteral renders a SQL string literal, doubling embedded quotes.
func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// isLockedErr reports whether an error is SQLite refusing concurrent access to
// the catalog. The catalog lock keeps this writer's own flush out of the way, but
// a search node with its own ATTACH can still hold a short lock.
func isLockedErr(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "database is locked")
}

// withRetryOnLocked retries an operation while the catalog is momentarily locked
// by another process, matching the writer's own retry behaviour. Any other error
// is returned immediately — in particular errPartitionChanged, which is a
// deliberate decision to skip, not a transient failure.
func withRetryOnLocked(fn func() error) error {
	const attempts = 4
	backoff := 200 * time.Millisecond
	var err error
	for i := 0; i < attempts; i++ {
		if err = fn(); !isLockedErr(err) {
			return err
		}
		if i < attempts-1 {
			time.Sleep(backoff)
			backoff *= 2
		}
	}
	return err
}

// swapRequest is one partition's atomic replacement: retire every active file of
// the partition and register the merged outputs in its place.
type swapRequest struct {
	tableName    string
	partition    partitionKey
	partitionVal string
	// expectedRows is the number of rows the merged files contain, which must
	// equal the partition's live row count both before and after the swap.
	expectedRows int64
	mergedAbs    []string
}

// swapPartition replaces a partition's files with the merged ones in a single
// DuckDB transaction, so DuckLake itself allocates the snapshot and the file
// ids. That is the whole point of this design: the live writer's metadata cache
// can never fall behind a snapshot the compactor allocated out-of-band, which is
// what produced "Corrupt DuckLake - multiple snapshots returned from database".
//
// The retirement is a DELETE over the partition column. Because the predicate
// covers every row of every file in the partition, DuckLake retires whole files
// instead of writing row-level delete files. The merged files are registered
// after the DELETE and are therefore untouched by it.
//
// Both before and after the swap the partition's live row count is compared with
// the merged row count while still inside the transaction. A mismatch means the
// partition changed after planning (a concurrent flush, or inlined rows) and the
// transaction is rolled back, leaving the catalog exactly as it was.
func swapPartition(ctx context.Context, db *sql.DB, lakeName string, req swapRequest) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Close()

	// Session-scoped: the retirement DELETE reads the partition to prove the
	// files are fully covered; one thread keeps that scan's memory bounded.
	if _, err := conn.ExecContext(ctx, "SET threads = 1"); err != nil {
		return fmt.Errorf("set threads=1: %w", err)
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin swap tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	ref := tableRef(lakeName, req.tableName)
	predicate := fmt.Sprintf("%s = CAST(%s AS %s)",
		quoteIdent(req.partition.columnName),
		quoteLiteral(req.partitionVal),
		req.partition.columnType)

	countRows := func(stage string) (int64, error) {
		var n int64
		q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", ref, predicate)
		if err := tx.QueryRowContext(ctx, q).Scan(&n); err != nil {
			return 0, fmt.Errorf("count partition rows (%s): %w", stage, err)
		}
		return n, nil
	}

	before, err := countRows("before")
	if err != nil {
		return err
	}
	if before != req.expectedRows {
		return fmt.Errorf("%w: partition %s holds %d rows but %d were merged",
			errPartitionChanged, req.partitionVal, before, req.expectedRows)
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s", ref, predicate)); err != nil {
		return fmt.Errorf("retire partition %s: %w", req.partitionVal, err)
	}

	// hive_partitioning is required for a partitioned table: DuckLake derives
	// each added file's partition value from its "col=value" directory, and
	// rejects the file outright when told not to. That inference walks the whole
	// absolute path, so any other "key=value" directory above the table yields a
	// column the table does not have; ignore_extra_columns tolerates those, and
	// also tolerates DuckLake internal columns should a future version write them
	// into the parquet. Columns *missing* from the file still fail the call,
	// which is what protects against schema drift.
	fileList := make([]string, len(req.mergedAbs))
	for i, p := range req.mergedAbs {
		fileList[i] = quoteLiteral(p)
	}
	addSQL := fmt.Sprintf("CALL %s(%s, %s, [%s], hive_partitioning => true, ignore_extra_columns => true)",
		addDataFilesProc,
		quoteLiteral(lakeName),
		quoteLiteral(req.tableName),
		strings.Join(fileList, ", "))
	if _, err := tx.ExecContext(ctx, addSQL); err != nil {
		if isColumnMappingErr(err) {
			// DuckLake refuses to register a file whose column types do not map to
			// the table's declared types — and it can refuse a file that is
			// type-identical to the ones it wrote itself, because some DuckDB types
			// have no exact parquet representation (HUGEINT is stored as DOUBLE).
			// This is a property of the table, so report it as such: the caller
			// stops offering the table instead of rewriting it every cycle.
			return fmt.Errorf("%w: %v", errTableNotRegisterable, err)
		}
		return fmt.Errorf("register merged files for partition %s: %w", req.partitionVal, err)
	}

	after, err := countRows("after")
	if err != nil {
		return err
	}
	if after != req.expectedRows {
		return fmt.Errorf("refusing to commit partition %s: %d rows after swap, expected %d",
			req.partitionVal, after, req.expectedRows)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit swap for partition %s: %w", req.partitionVal, err)
	}
	committed = true
	return nil
}
