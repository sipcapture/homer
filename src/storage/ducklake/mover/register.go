package mover

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const addDataFilesProc = "ducklake_add_data_files"

func isLockedErr(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "database is locked")
}

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

func isColumnMappingErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Failed to map column") ||
		(strings.Contains(msg, "Expected type") && strings.Contains(msg, "but found type"))
}

func partitionPredicate(pk partitionKey, partVal string) (string, error) {
	if !pk.usable() {
		return "", fmt.Errorf("unusable partition key column=%q transform=%q type=%q",
			pk.columnName, pk.transform, pk.columnType)
	}
	return fmt.Sprintf("%s = CAST(%s AS %s)",
		quoteIdent(pk.columnName), quoteLiteral(partVal), pk.columnType), nil
}

func countPartition(ctx context.Context, tx *sql.Tx, lake, table, predicate string) (int64, error) {
	var n int64
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", tableRef(lake, table), predicate)
	if err := tx.QueryRowContext(ctx, q).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func countPartitionDB(ctx context.Context, db *sql.DB, lake, table, predicate string) (int64, error) {
	var n int64
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", tableRef(lake, table), predicate)
	if err := db.QueryRowContext(ctx, q).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// registerCopiedFiles adds already-copied parquet to the destination lake.
// DuckLake allocates snapshot and file ids. The hot catalog is not touched.
func registerCopiedFiles(ctx context.Context, db *sql.DB, dstLake, table string, pk partitionKey, partVal string, destAbs []string, expectedRows int64) error {
	if len(destAbs) == 0 {
		return fmt.Errorf("no destination files to register")
	}
	predicate, err := partitionPredicate(pk, partVal)
	if err != nil {
		return err
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "SET threads = 1"); err != nil {
		return fmt.Errorf("set threads=1: %w", err)
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin register tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	before, err := countPartition(ctx, tx, dstLake, table, predicate)
	if err != nil {
		return fmt.Errorf("count destination before register: %w", err)
	}
	if before == expectedRows {
		// Idempotent retry: files already in the cold catalog.
		if err := tx.Commit(); err != nil {
			return err
		}
		committed = true
		return nil
	}
	if before > 0 && before != expectedRows {
		return fmt.Errorf("destination partition %s already has %d rows, expected %d", partVal, before, expectedRows)
	}

	fileList := make([]string, len(destAbs))
	for i, p := range destAbs {
		fileList[i] = quoteLiteral(p)
	}
	addSQL := fmt.Sprintf("CALL %s(%s, %s, [%s], hive_partitioning => true, ignore_extra_columns => true)",
		addDataFilesProc,
		quoteLiteral(dstLake),
		quoteLiteral(table),
		strings.Join(fileList, ", "))
	if _, err := tx.ExecContext(ctx, addSQL); err != nil {
		if isColumnMappingErr(err) {
			return fmt.Errorf("%w: %v", ErrFallback, err)
		}
		return fmt.Errorf("register copied files: %w", err)
	}

	after, err := countPartition(ctx, tx, dstLake, table, predicate)
	if err != nil {
		return fmt.Errorf("count destination after register: %w", err)
	}
	if after != expectedRows {
		return fmt.Errorf("refusing to commit destination partition %s: %d rows after add, expected %d",
			partVal, after, expectedRows)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit register: %w", err)
	}
	committed = true
	return nil
}

// retireSourcePartition deletes the source partition after the copy is catalogued
// on the destination. Whole-file retirement (predicate covers every row).
func retireSourcePartition(ctx context.Context, db *sql.DB, srcLake, table string, pk partitionKey, partVal string, expectedRows int64) error {
	predicate, err := partitionPredicate(pk, partVal)
	if err != nil {
		return err
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "SET threads = 1"); err != nil {
		return fmt.Errorf("set threads=1: %w", err)
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin retire tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	before, err := countPartition(ctx, tx, srcLake, table, predicate)
	if err != nil {
		return fmt.Errorf("count source before delete: %w", err)
	}
	if before == 0 {
		if err := tx.Commit(); err != nil {
			return err
		}
		committed = true
		return nil
	}
	if before != expectedRows {
		return fmt.Errorf("source partition %s changed during copy (%d rows, planned %d); leaving source in place",
			partVal, before, expectedRows)
	}

	del := fmt.Sprintf("DELETE FROM %s WHERE %s", tableRef(srcLake, table), predicate)
	if _, err := tx.ExecContext(ctx, del); err != nil {
		return fmt.Errorf("delete source partition: %w", err)
	}
	after, err := countPartition(ctx, tx, srcLake, table, predicate)
	if err != nil {
		return fmt.Errorf("count source after delete: %w", err)
	}
	if after != 0 {
		return fmt.Errorf("source partition %s still has %d rows after delete", partVal, after)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit retire: %w", err)
	}
	committed = true
	return nil
}
