// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package ducklake

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
)

// ErrCatalogAttachedInProcess is returned by helpers that would open a SQLite
// catalog through the pure-Go driver while a DuckDB instance in this process has
// the same file attached.
//
// The pure-Go driver and DuckDB's sqlite extension are two separate copies of
// SQLite. POSIX advisory locks belong to the process, so when one copy closes
// its file descriptor the kernel drops the locks the other copy still relies on
// (https://www.sqlite.org/howtocorrupt.html §2.2). The two then write without
// mutual exclusion and corrupt pages (sipcapture/homer#1048). Work on a live
// catalog must go through DuckDB or a child process instead.
var ErrCatalogAttachedInProcess = errors.New("catalog is attached by DuckDB in this process; " +
	"refusing to open it with a second SQLite library (run the operation out of process)")

var attachedCatalogs = struct {
	sync.Mutex
	refs map[string]int
}{refs: map[string]int{}}

func catalogKey(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return filepath.Clean(path)
}

// MarkCatalogAttached records that a DuckDB instance in this process attached
// catalogPath. The returned release function is idempotent; call it after that
// DuckDB instance is closed.
func MarkCatalogAttached(catalogPath string) (release func()) {
	if catalogPath == "" {
		return func() {}
	}
	key := catalogKey(catalogPath)
	attachedCatalogs.Lock()
	attachedCatalogs.refs[key]++
	attachedCatalogs.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			attachedCatalogs.Lock()
			defer attachedCatalogs.Unlock()
			if attachedCatalogs.refs[key] <= 1 {
				delete(attachedCatalogs.refs, key)
				return
			}
			attachedCatalogs.refs[key]--
		})
	}
}

// IsCatalogAttachedInProcess reports whether a DuckDB instance in this process
// currently has catalogPath attached.
func IsCatalogAttachedInProcess(catalogPath string) bool {
	if catalogPath == "" {
		return false
	}
	key := catalogKey(catalogPath)
	attachedCatalogs.Lock()
	defer attachedCatalogs.Unlock()
	return attachedCatalogs.refs[key] > 0
}

func ensureCatalogNotAttached(catalogPath string) error {
	if IsCatalogAttachedInProcess(catalogPath) {
		return fmt.Errorf("%s: %w", catalogPath, ErrCatalogAttachedInProcess)
	}
	return nil
}
