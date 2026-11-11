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
	"path/filepath"
	"sort"
	"strings"
)

// QuerySandbox lists what a sandboxed DuckDB may still open once external
// access is off. Everything else is refused by DuckDB itself, whatever SQL
// reaches it: read_text('/etc/passwd'), COPY ... TO '/tmp/x', ATTACH of
// another file, INSTALL/LOAD of an extension, or a URL outside the lake.
type QuerySandbox struct {
	// Directories are local paths or remote prefixes (s3://, az://, ...).
	// Every file below them stays readable and writable.
	Directories []string
	// Paths are single files, such as a SQLite catalog and its -wal/-shm.
	Paths []string
}

// ApplyQuerySandbox turns off DuckDB external access on db, leaving only the
// directories and files in s. Call it after every extension is loaded, every
// secret is created and every catalog is attached: none of that works later.
//
// DuckDB refuses to turn external access back on, or to widen
// allowed_directories, allowed_paths or temp_directory, for the rest of the
// instance's life. So SQL that slips past the validator cannot undo this.
// lock_configuration is not set because the node still toggles
// enable_external_file_cache to drop a stale parquet footer (#1061).
//
// On error before the final SET, external access is still on and db is
// unchanged apart from the allow lists, which have no effect until then.
func ApplyQuerySandbox(db *sql.DB, s QuerySandbox) error {
	if db == nil {
		return fmt.Errorf("sandbox: nil DuckDB handle")
	}
	dirs := sandboxDirectories(s.Directories)
	paths := sandboxPaths(s.Paths)

	// The secret manager reads ~/.duckdb/stored_secrets the first time it is
	// used. Touch it now so a later credential_chain refresh does not fail.
	if _, err := db.Exec("SELECT count(*) FROM duckdb_secrets()"); err != nil {
		return fmt.Errorf("sandbox: init secret manager: %w", err)
	}
	pre := []string{
		"SET autoinstall_known_extensions = false",
		"SET autoload_known_extensions = false",
		"SET allow_community_extensions = false",
		"SET allowed_directories = " + sqlStringList(dirs),
		"SET allowed_paths = " + sqlStringList(paths),
	}
	for _, stmt := range pre {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("sandbox: %s: %w", stmt, err)
		}
	}

	// DuckDB expands an entry against the cwd when no loaded file system
	// claims its scheme (httpfs for s3://, azure for az://). A remote volume
	// would then be locked out, so stop before external access goes off.
	var got string
	if err := db.QueryRow("SELECT current_setting('allowed_directories')::VARCHAR").Scan(&got); err != nil {
		return fmt.Errorf("sandbox: read allowed_directories: %w", err)
	}
	for _, d := range dirs {
		if isRemotePath(d) && !strings.Contains(got, d) {
			return fmt.Errorf("sandbox: DuckDB did not accept remote prefix %q (is its extension loaded?)", d)
		}
	}

	if _, err := db.Exec("SET enable_external_access = false"); err != nil {
		return fmt.Errorf("sandbox: SET enable_external_access = false: %w", err)
	}
	var ext string
	if err := db.QueryRow("SELECT current_setting('enable_external_access')::VARCHAR").Scan(&ext); err != nil {
		return fmt.Errorf("sandbox: read enable_external_access: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(ext), "false") {
		return fmt.Errorf("sandbox: enable_external_access is still %q", ext)
	}
	return nil
}

// IsSandboxDenial reports a DuckDB error caused by the sandbox, as opposed to
// a missing table or a corrupt file.
func IsSandboxDenial(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "disabled by configuration") ||
		strings.Contains(msg, "disabled through configuration")
}

// SandboxCatalogPaths returns the files a SQLite or DuckDB catalog opens next
// to itself. Only the files, never their directory: the catalog often sits
// beside homer_settings.duckdb, which holds users and password hashes.
func SandboxCatalogPaths(ct CatalogType, catalogPath string) []string {
	p := strings.TrimSpace(catalogPath)
	if p == "" || ct == CatalogPostgres {
		return nil
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return []string{p, p + "-wal", p + "-shm", p + "-journal", p + ".wal"}
}

func sandboxDirectories(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range in {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if !isRemotePath(d) {
			if abs, err := filepath.Abs(d); err == nil {
				d = abs
			}
			d = filepath.Clean(d)
		}
		// A trailing slash keeps /var/lib/homer/parquet from also allowing
		// /var/lib/homer/parquet-old.
		if !strings.HasSuffix(d, "/") {
			d += "/"
		}
		if d == "/" {
			// Allowing the root would allow everything.
			continue
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}

func sandboxPaths(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func isRemotePath(p string) bool {
	return strings.Contains(p, "://")
}

func sqlStringList(items []string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
