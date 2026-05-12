// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"database/sql"
	"fmt"
	"strings"

	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// ApplyDuckDBS3ClientSettings configures DuckDB's built-in S3/httpfs client
// (SET s3_*) before any s3:// reads or writes. Use the same rules everywhere:
// writer MultiTableWriter, node global prefix (configureDuckLake), CLI helpers.
//
// If accessKeyID is empty, this is a no-op (returns nil).
//
// When endpoint is non-empty, the scheme is stripped (DuckDB expects host:port)
// and s3_url_style is set to path — required for most S3-compatible stores
// (MinIO, RustFS) and aligned with per-volume CREATE SECRET in attachVolume.
// If region is empty but a custom endpoint is set, region defaults to us-east-1
// (signing requires a non-empty region for many S3-compatible backends).
func ApplyDuckDBS3ClientSettings(db *sql.DB, region, accessKeyID, secretAccessKey, endpoint string, useSSL bool) error {
	if db == nil {
		return nil
	}
	if strings.TrimSpace(accessKeyID) == "" {
		return nil
	}
	reg := strings.TrimSpace(region)
	epRaw := strings.TrimSpace(endpoint)
	// DuckDB/httpfs expects a non-empty region for signing; MinIO/RustFS/R2
	// typically ignore the value but misbehave with region ''.
	if reg == "" && epRaw != "" {
		reg = "us-east-1"
	}
	// One SET per Exec: DuckDB rejects a single multi-statement script with
	// multiple bound parameters ("incorrect argument count ... have 0 want 1").
	if _, err := db.Exec("SET s3_region = ?;", reg); err != nil {
		return fmt.Errorf("duckdb SET s3_region: %w", err)
	}
	if _, err := db.Exec("SET s3_access_key_id = ?;", accessKeyID); err != nil {
		return fmt.Errorf("duckdb SET s3_access_key_id: %w", err)
	}
	if _, err := db.Exec("SET s3_secret_access_key = ?;", secretAccessKey); err != nil {
		return fmt.Errorf("duckdb SET s3_secret_access_key: %w", err)
	}
	if ep := epRaw; ep != "" {
		ep = strings.TrimPrefix(ep, "http://")
		ep = strings.TrimPrefix(ep, "https://")
		if _, err := db.Exec("SET s3_endpoint = ?;", ep); err != nil {
			return fmt.Errorf("duckdb SET s3_endpoint: %w", err)
		}
		if _, err := db.Exec("SET s3_url_style = 'path';"); err != nil {
			return fmt.Errorf("duckdb SET s3_url_style: %w", err)
		}
	}
	if !useSSL {
		if _, err := db.Exec("SET s3_use_ssl = false;"); err != nil {
			return fmt.Errorf("duckdb SET s3_use_ssl: %w", err)
		}
	}
	return nil
}

// writerLakeS3Secret is the DuckDB secret name for storage.ducklake single-volume
// S3 data_path. DuckLake maintenance (e.g. delete_orphaned_files) uses read_blob
// paths that do not always inherit SET s3_*; a TYPE S3 secret matches tiered
// attachVolume / node volume behaviour on MinIO, RustFS, and R2.
const writerLakeS3Secret = "homer_writer_s3"

// EnsureWriterS3Secret creates a session-scoped DuckDB TYPE S3 secret so DuckLake
// internals hit the same endpoint as ApplyDuckDBS3ClientSettings. Call after
// ApplyDuckDBS3ClientSettings and before ATTACH ducklake. No-op if accessKeyID is empty.
func EnsureWriterS3Secret(db *sql.DB, region, accessKeyID, secretAccessKey, endpoint string, useSSL bool) error {
	if db == nil || strings.TrimSpace(accessKeyID) == "" {
		return nil
	}
	reg := strings.TrimSpace(region)
	epRaw := strings.TrimSpace(endpoint)
	if reg == "" && epRaw != "" {
		reg = "us-east-1"
	}
	sqlQuote := func(s string) string { return strings.ReplaceAll(s, "'", "''") }
	epHost := strings.TrimPrefix(strings.TrimPrefix(epRaw, "http://"), "https://")

	drop := fmt.Sprintf("DROP SECRET IF EXISTS %s;", writerLakeS3Secret)
	if _, err := db.Exec(drop); err != nil {
		return fmt.Errorf("duckdb DROP SECRET %s: %w", writerLakeS3Secret, err)
	}
	var create string
	if epHost != "" {
		create = fmt.Sprintf(`
CREATE SECRET %s (
	TYPE S3,
	KEY_ID '%s',
	SECRET '%s',
	REGION '%s',
	ENDPOINT '%s',
	URL_STYLE 'path',
	USE_SSL %t
);`, writerLakeS3Secret, sqlQuote(accessKeyID), sqlQuote(secretAccessKey), sqlQuote(reg), sqlQuote(epHost), useSSL)
	} else {
		create = fmt.Sprintf(`
CREATE SECRET %s (
	TYPE S3,
	KEY_ID '%s',
	SECRET '%s',
	REGION '%s'
);`, writerLakeS3Secret, sqlQuote(accessKeyID), sqlQuote(secretAccessKey), sqlQuote(reg))
	}
	if _, err := db.Exec(create); err != nil {
		return fmt.Errorf("duckdb CREATE SECRET %s: %w", writerLakeS3Secret, err)
	}
	return nil
}

// ApplyDuckDBTuning issues the per-connection DuckDB SET statements
// implied by the operator-supplied DuckDBTuning config block:
//
//   - threads        -> SET threads = N
//   - memory_limit   -> SET memory_limit = '<string>'
//   - temp_directory -> SET temp_directory = '<path>'
//
// Empty / zero values mean "do not touch this setting" so the function
// is safe to call unconditionally on every freshly-opened DuckDB
// connection (writer, node-side reader, CLI, ...).
//
// Errors are logged at WARN level and swallowed: a bad memory_limit
// string (e.g. "8 angstrom") should not stop the writer from coming up,
// it should just leave DuckDB on its built-in default. Pass `who` to
// describe the call site in the warning ("writer", "node", ...).
func ApplyDuckDBTuning(db *sql.DB, threads int, memoryLimit, tempDirectory, who string) {
	if db == nil {
		return
	}
	if threads > 0 {
		if _, err := db.Exec(fmt.Sprintf("SET threads = %d", threads)); err != nil {
			logger.Warn(fmt.Sprintf("DuckDB tuning (%s): SET threads = %d failed: %v", who, threads, err))
		} else {
			logger.Info("DuckDB tuning: threads set", "where", who, "threads", threads)
		}
	}
	if s := strings.TrimSpace(memoryLimit); s != "" {
		// Single-quote-escape the operator-supplied string so a value
		// like "O'Brien" can't break the SET statement. DuckDB accepts
		// double single quotes as an embedded quote.
		safe := strings.ReplaceAll(s, "'", "''")
		if _, err := db.Exec(fmt.Sprintf("SET memory_limit = '%s'", safe)); err != nil {
			logger.Warn(fmt.Sprintf("DuckDB tuning (%s): SET memory_limit = %q failed: %v", who, s, err))
		} else {
			logger.Info("DuckDB tuning: memory_limit set", "where", who, "memory_limit", s)
		}
	}
	if s := strings.TrimSpace(tempDirectory); s != "" {
		safe := strings.ReplaceAll(s, "'", "''")
		if _, err := db.Exec(fmt.Sprintf("SET temp_directory = '%s'", safe)); err != nil {
			logger.Warn(fmt.Sprintf("DuckDB tuning (%s): SET temp_directory = %q failed: %v", who, s, err))
		} else {
			logger.Info("DuckDB tuning: temp_directory set", "where", who, "temp_directory", s)
		}
	}
}
