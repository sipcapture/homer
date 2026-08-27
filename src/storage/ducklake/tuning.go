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
	"os"
	"path/filepath"
	"runtime"
	"strings"

	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// S3ClientSettingsSQL returns the session-scoped SET statements that configure
// DuckDB's built-in S3/httpfs client. SET s3_* applies per connection, so the
// statements must be replayed on every freshly pooled connection (see the
// connector init in MultiTableWriter.connect). Values are embedded as
// single-quote-escaped literals so the statements are self-contained.
//
// If accessKeyID is empty, returns nil (nothing to apply).
//
// When endpoint is non-empty, the scheme is stripped (DuckDB expects host:port)
// and s3_url_style defaults to path — required for most S3-compatible stores
// (MinIO, RustFS) and aligned with per-volume CREATE SECRET in attachVolume.
// If region is empty but a custom endpoint is set, region defaults to us-east-1
// (signing requires a non-empty region for many S3-compatible backends).
func S3ClientSettingsSQL(region, accessKeyID, secretAccessKey, endpoint string, useSSL bool, urlStyle string) []string {
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
	quote := func(s string) string { return strings.ReplaceAll(s, "'", "''") }
	stmts := []string{
		fmt.Sprintf("SET s3_region = '%s';", quote(reg)),
		fmt.Sprintf("SET s3_access_key_id = '%s';", quote(accessKeyID)),
		fmt.Sprintf("SET s3_secret_access_key = '%s';", quote(secretAccessKey)),
	}
	if epRaw != "" {
		ep := strings.TrimPrefix(strings.TrimPrefix(epRaw, "http://"), "https://")
		stmts = append(stmts,
			fmt.Sprintf("SET s3_endpoint = '%s';", quote(ep)),
			fmt.Sprintf("SET s3_url_style = '%s';", quote(NormalizeS3URLStyle(urlStyle))),
		)
	}
	if !useSSL {
		stmts = append(stmts, "SET s3_use_ssl = false;")
	}
	return stmts
}

// NormalizeS3URLStyle returns the DuckDB S3 URL style used for custom
// endpoints. Empty keeps the historical Homer behaviour: path-style URLs.
func NormalizeS3URLStyle(urlStyle string) string {
	style := strings.ToLower(strings.TrimSpace(urlStyle))
	if style == "" {
		return "path"
	}
	return style
}

// s3SettingName extracts the setting identifier from a "SET name = ..."
// statement for log-safe error messages (values may contain secrets).
func s3SettingName(stmt string) string {
	fields := strings.Fields(stmt)
	if len(fields) >= 2 {
		return fields[1]
	}
	return "s3 setting"
}

// ApplyDuckDBS3ClientSettings configures DuckDB's built-in S3/httpfs client
// (SET s3_*) on the given DB handle. Use the same rules everywhere:
// writer MultiTableWriter, node global prefix (configureDuckLake), CLI helpers.
// Note: SET s3_* is session-scoped — this only reliably covers pools with
// MaxOpenConns(1); pooled writers replay S3ClientSettingsSQL per connection.
func ApplyDuckDBS3ClientSettings(db *sql.DB, region, accessKeyID, secretAccessKey, endpoint string, useSSL bool, urlStyle string) error {
	if db == nil {
		return nil
	}
	for _, stmt := range S3ClientSettingsSQL(region, accessKeyID, secretAccessKey, endpoint, useSSL, urlStyle) {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("duckdb SET %s: %w", s3SettingName(stmt), err)
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
func EnsureWriterS3Secret(db *sql.DB, region, accessKeyID, secretAccessKey, endpoint string, useSSL bool, urlStyle string) error {
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
	style := NormalizeS3URLStyle(urlStyle)

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
	URL_STYLE '%s',
	USE_SSL %t
);`, writerLakeS3Secret, sqlQuote(accessKeyID), sqlQuote(secretAccessKey), sqlQuote(reg), sqlQuote(epHost), sqlQuote(style), useSSL)
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

// redhatCACertPath and debianCACertPath are the two most common Linux CA
// bundle locations. See EnsureAzureCACertPath.
const (
	redhatCACertPath = "/etc/pki/tls/certs/ca-bundle.crt"
	debianCACertPath = "/etc/ssl/certs/ca-certificates.crt"
)

// EnsureAzureCACertPath works around a bug in DuckDB's azure extension: its
// bundled Azure C++ SDK / libcurl only defaults to the RedHat-family CA
// bundle path (redhatCACertPath), unlike Go's crypto/x509 (which checks both
// RedHat and Debian paths). On Debian/Ubuntu — including Homer's own
// debian:bookworm-slim image — the RedHat path never exists, so every HTTPS
// request over any Azure Blob endpoint (standard public cloud, Gov cloud,
// China cloud, or a custom endpoint) fails with "Problem with the SSL CA
// cert (path? access rights?)", regardless of auth method (static key,
// connection string, or Managed Identity all hit this identically —
// verified against a real Azure storage account and a real Azure VM).
// Reported upstream: https://github.com/duckdb/duckdb-azure/issues/185
// Remove this workaround once that's fixed and the fix has shipped in a
// released azure extension build.
//
// No-op if the RedHat path already exists (RedHat-family host, container
// already fixed, or a prior call already symlinked it) or if the Debian
// path is missing (nothing to link from — not a Debian/Ubuntu host).
// Failures are logged, not fatal: this is best-effort defense-in-depth: the
// Dockerfile also creates this symlink directly for the official image.
func EnsureAzureCACertPath() {
	if _, err := os.Stat(redhatCACertPath); err == nil {
		return
	}
	if _, err := os.Stat(debianCACertPath); err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(redhatCACertPath), 0o755); err != nil {
		logger.Warn("EnsureAzureCACertPath: failed to create directory", "path", filepath.Dir(redhatCACertPath), "error", err)
		return
	}
	if err := os.Symlink(debianCACertPath, redhatCACertPath); err != nil {
		if os.IsExist(err) {
			// Something created the symlink between the os.Stat check above
			// and this call — the symlink exists either way, so this is
			// success, not failure.
			return
		}
		logger.Warn("EnsureAzureCACertPath: failed to symlink CA bundle (Azure HTTPS access may fail with 'SSL CA cert' errors); "+
			"on a non-root deployment (bare Debian/Ubuntu host, or 'docker run' as a non-root user), create it manually: "+
			"mkdir -p /etc/pki/tls/certs && ln -sf /etc/ssl/certs/ca-certificates.crt /etc/pki/tls/certs/ca-bundle.crt "+
			"(see duckdb/duckdb-azure#185)",
			"src", debianCACertPath, "dst", redhatCACertPath, "error", err)
		return
	}
	logger.Info("EnsureAzureCACertPath: symlinked CA bundle for DuckDB azure extension (works around duckdb-azure CA path bug)",
		"src", debianCACertPath, "dst", redhatCACertPath)
}

// writerLakeAzureSecret is the DuckDB secret name for storage.ducklake
// single-volume Azure data_path, mirroring writerLakeS3Secret.
const writerLakeAzureSecret = "homer_writer_azure"

// EnsureWriterAzureSecret creates a session-scoped DuckDB TYPE azure secret
// for the single-volume writer/CLI path, mirroring EnsureWriterS3Secret.
// Unlike S3, DuckDB's azure extension has no global SET azure_* session
// settings — it is secret-driven only — so there is no ApplyDuckDBAzure...
// counterpart to ApplyDuckDBS3ClientSettings; this is the only step needed.
// No-op if accountName, accountKey, and connectionString are all empty.
//
// Safe to call repeatedly: DROP+CREATE each time, so callers can also use
// this to refresh a credential_chain secret before its Managed Identity
// token (~1h lifetime, no REFRESH clause exists for azure secrets) goes
// stale — see CompactionService.ensureAzureClientSettings.
func EnsureWriterAzureSecret(db *sql.DB, accountName, accountKey, connectionString, endpoint string) error {
	if db == nil {
		return nil
	}
	connectionString = strings.TrimSpace(connectionString)
	accountKey = strings.TrimSpace(accountKey)
	accountName = strings.TrimSpace(accountName)
	if connectionString == "" && accountKey == "" && accountName == "" {
		return nil
	}
	drop := fmt.Sprintf("DROP SECRET IF EXISTS %s;", writerLakeAzureSecret)
	if _, err := db.Exec(drop); err != nil {
		return fmt.Errorf("duckdb DROP SECRET %s: %w", writerLakeAzureSecret, err)
	}
	create := BuildAzureSecretSQL(writerLakeAzureSecret, accountName, accountKey, connectionString, endpoint)
	if _, err := db.Exec(create); err != nil {
		return fmt.Errorf("duckdb CREATE SECRET %s: %w", writerLakeAzureSecret, err)
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

// DefaultSpillDirectory returns the spill path to use when the operator did
// not set tuning.temp_directory: "<dir of catalogPath>/.duckdb_spill".
//
// This matters because the writer (and the node when it shares the writer DB)
// runs DuckDB as an IN-MEMORY instance, and in-memory DuckDB disables disk
// spilling entirely unless temp_directory is set explicitly. Without it any
// query whose working set exceeds memory_limit fails with Out of Memory
// instead of spilling — e.g. long-range LIKE searches over wide SIP tables.
// The catalog directory is used (not os.TempDir) because /tmp is commonly a
// RAM-backed tmpfs in containers, which would defeat the purpose.
//
// The directory is created eagerly; on failure an empty string is returned
// and the caller should leave temp_directory untouched.
func DefaultSpillDirectory(catalogPath string) string {
	dir := strings.TrimSpace(filepath.Dir(strings.TrimSpace(catalogPath)))
	if dir == "" || dir == "." {
		return ""
	}
	spill := filepath.Join(dir, ".duckdb_spill")
	if err := os.MkdirAll(spill, 0o755); err != nil {
		logger.Warn("DuckDB tuning: cannot create default spill directory, disk spilling stays disabled",
			"path", spill, "error", err)
		return ""
	}
	return spill
}

// ApplyDuckDBMemorySafety applies instance-global settings that keep
// memory-heavy queries from taking down the shared writer DuckDB:
//
//   - preserve_insertion_order = false: lets DuckDB stream large scans and
//     UNION ALL results instead of buffering them to keep row order. Safe
//     here because search queries always ORDER BY explicitly and HEP ingest
//     order within a flush batch is irrelevant (timestamp column rules).
//
// Call once per DuckDB instance after ApplyDuckDBTuning. Errors are logged
// and swallowed, same contract as ApplyDuckDBTuning.
func ApplyDuckDBMemorySafety(db *sql.DB, who string) {
	if db == nil {
		return
	}
	if _, err := db.Exec("SET preserve_insertion_order = false"); err != nil {
		logger.Warn(fmt.Sprintf("DuckDB tuning (%s): SET preserve_insertion_order = false failed: %v", who, err))
	} else {
		logger.Info("DuckDB tuning: preserve_insertion_order disabled", "where", who)
	}
}

// AutoThreads is the writer/node default when tuning.threads is 0:
// min(NumCPU/2, 4), at least 1. Leaves headroom for Go ingest goroutines
// and matches DuckDB's own advice to lower threads under memory pressure.
func AutoThreads() int {
	n := runtime.NumCPU() / 2
	if n < 1 {
		n = 1
	}
	if n > 4 {
		n = 4
	}
	return n
}
