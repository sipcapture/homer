// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package node

import (
	"context"
	"database/sql"
	"os"
	"regexp"
	"strings"
	"sync"

	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// DuckDB's external file cache keys a local file by device, inode, size and
// mtime truncated to seconds (v1.5.5). A read that observes a parquet file
// while compaction is still writing the footer stores that short view under
// the final version tag when the size and the mtime second do not change
// again. Every later query on the same DuckDB instance then fails with
// "No magic bytes found at end of file" even though the bytes on disk are a
// valid parquet file. Restart works only because it drops the cache.
// See sipcapture/homer#1061.
var reNoMagicBytes = regexp.MustCompile(`No magic bytes found at end of file '([^']+)'`)

// parquetCacheResetMu keeps a cache flush from racing another recovery's
// retry: SET enable_external_file_cache=false drops every cached range.
var parquetCacheResetMu sync.Mutex

// queryContextRetryingStaleParquet runs query and, on the sticky magic-bytes
// error for a local file that is valid on disk, drops the external file cache
// and retries once.
func queryContextRetryingStaleParquet(ctx context.Context, db *sql.DB, query string) (*sql.Rows, error) {
	rows, err := db.QueryContext(ctx, query)
	if err == nil || !staleParquetMagic(err) || !dropStaleParquetCache(ctx, db, err) {
		return rows, err
	}
	return db.QueryContext(ctx, query)
}

// dropStaleParquetCache clears the external file cache when err is the sticky
// magic-bytes failure for a file that is readable on disk. It reports false
// when the error is a real corrupt file or the cache could not be cleared.
func dropStaleParquetCache(ctx context.Context, db *sql.DB, err error) bool {
	path, ok := magicBytesPath(err)
	if !ok || !parquetWorthCacheReset(path) {
		return false
	}
	parquetCacheResetMu.Lock()
	defer parquetCacheResetMu.Unlock()
	reset, resetErr := resetExternalFileCache(ctx, db)
	if resetErr != nil {
		logger.Warn("Node: failed to reset DuckDB external file cache", "error", resetErr)
		return false
	}
	if !reset {
		logger.Warn("Node: external file cache is off; retrying once",
			"path", path,
			"issue", "https://github.com/sipcapture/homer/issues/1061")
		return true
	}
	logger.Warn("Node: parquet read failed on a file that is valid on disk; dropping DuckDB external file cache and retrying",
		"path", path,
		"issue", "https://github.com/sipcapture/homer/issues/1061")
	return true
}

func staleParquetMagic(err error) bool {
	return err != nil && strings.Contains(err.Error(), "No magic bytes found at end of file")
}

func magicBytesPath(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	m := reNoMagicBytes.FindStringSubmatch(err.Error())
	if len(m) < 2 || m[1] == "" {
		return "", false
	}
	return m[1], true
}

// parquetWorthCacheReset reports whether a failed read is worth a cache flush.
// A local file that does not start and end with PAR1 is actually corrupt;
// flushing would not help and would discard cached ranges on every search.
// Remote objects are left alone: we cannot see their bytes from here, and a
// permanently bad object would flush the whole cache on every search.
func parquetWorthCacheReset(path string) bool {
	if remoteParquetPath(path) {
		return false
	}
	return localParquetHasMagic(path)
}

func remoteParquetPath(path string) bool {
	p := strings.ToLower(path)
	return strings.Contains(p, "://") || strings.HasPrefix(p, "s3:") || strings.HasPrefix(p, "az:")
}

func localParquetHasMagic(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.Size() < 12 {
		return false
	}
	head := make([]byte, 4)
	if _, err := f.Read(head); err != nil || string(head) != "PAR1" {
		return false
	}
	tail := make([]byte, 4)
	if _, err := f.ReadAt(tail, info.Size()-4); err != nil || string(tail) != "PAR1" {
		return false
	}
	return true
}

// resetExternalFileCache drops cached file ranges by toggling the setting off
// and back on. SetEnabled is a no-op when the flag does not change, so the
// cache is cleared only by the transition to false. When the cache is already
// off (local node, or tuning.external_file_cache = false) the toggle would
// turn it on for the rest of the process, and there is no cached footer to
// drop. reset reports whether a flush actually ran.
func resetExternalFileCache(ctx context.Context, db *sql.DB) (bool, error) {
	var setting string
	if err := db.QueryRowContext(ctx, "SELECT current_setting('enable_external_file_cache')").Scan(&setting); err != nil {
		return false, err
	}
	if strings.EqualFold(strings.TrimSpace(setting), "false") {
		return false, nil
	}
	if _, err := db.ExecContext(ctx, "SET enable_external_file_cache = false"); err != nil {
		return false, err
	}
	if _, err := db.ExecContext(ctx, "SET enable_external_file_cache = true"); err != nil {
		return false, err
	}
	return true, nil
}
