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
	"time"

	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

const (
	tieringMaxRetries  = 5
	tieringBaseBackoff = 100 * time.Millisecond
)

// CatalogLocker serializes catalog-modifying operations with the writer flush path.
type CatalogLocker interface {
	CatalogLock()
	CatalogUnlock()
}

// isRetriableCatalogError reports whether a DuckLake/SQLite catalog error may clear
// with a short backoff (lock contention, transaction conflicts).
func isRetriableCatalogError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	if errStr == "" {
		return false
	}
	if strings.Contains(errStr, "NoSuchBucket") {
		return false
	}
	if strings.Contains(errStr, "InvalidAccessKeyId") ||
		strings.Contains(errStr, "SignatureDoesNotMatch") {
		return false
	}
	if strings.Contains(strings.ToLower(errStr), "database is locked") ||
		strings.Contains(errStr, "Failed to commit") ||
		strings.Contains(errStr, "Could not set lock") ||
		strings.Contains(errStr, "catalog") ||
		strings.Contains(errStr, "transaction conflict") {
		return true
	}
	if strings.Contains(errStr, "HTTP Error") {
		return true
	}
	return false
}

// execWithRetry runs db.Exec with exponential backoff on retriable catalog errors.
// The lock callback, when non-nil, is held only around Exec (not during backoff).
func execWithRetry(
	db *sql.DB,
	maxAttempts int,
	baseBackoff time.Duration,
	locker CatalogLocker,
	query string,
	args ...any,
) (sql.Result, error) {
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	backoff := baseBackoff
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var result sql.Result
		if locker != nil {
			locker.CatalogLock()
			result, lastErr = db.Exec(query, args...)
			locker.CatalogUnlock()
		} else {
			result, lastErr = db.Exec(query, args...)
		}
		if lastErr == nil {
			return result, nil
		}
		if !isRetriableCatalogError(lastErr) {
			return nil, lastErr
		}
		if attempt < maxAttempts {
			logger.Warn("TieredStorageManager: retriable catalog error, retrying",
				"attempt", attempt,
				"max_attempts", maxAttempts,
				"backoff", backoff,
				"error", lastErr)
			time.Sleep(backoff)
			backoff *= 2
		}
	}
	return nil, fmt.Errorf("after %d attempts: %w", maxAttempts, lastErr)
}
