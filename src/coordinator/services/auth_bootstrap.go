// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/sipcapture/homer-core/src/coordinator/sqlvalidator"
	"github.com/sipcapture/homer-core/src/passwordhash"
)

func generateBootstrapPassword() (plain string, hash string, err error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generate bootstrap password: %w", err)
	}
	plain = base64.RawURLEncoding.EncodeToString(b)
	hash, err = passwordhash.Hash(plain)
	if err != nil {
		return "", "", fmt.Errorf("hash bootstrap password: %w", err)
	}
	return plain, hash, nil
}

// EnsureBootstrapAdminUser inserts the admin user once when coordinator.auth
// is the JSON string "internal" and no row exists for username. If a row
// already exists but password_hash is NULL or empty (broken / legacy state),
// it is updated to passwordHash so internal login can succeed.
//
// When passwordHash is empty, a random password is generated and its bcrypt hash
// is stored. The cleartext password is returned so the caller can log it once.
func EnsureBootstrapAdminUser(ctx context.Context, db *sql.DB, username, passwordHash string) (bootstrapPassword string, err error) {
	if db == nil {
		return "", fmt.Errorf("settings db not available")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		username = "admin"
	}
	passwordHash = strings.TrimSpace(passwordHash)
	if passwordHash == "" || passwordhash.IsDisallowedDefaultHash(passwordHash) {
		bootstrapPassword, passwordHash, err = generateBootstrapPassword()
		if err != nil {
			return "", err
		}
	}

	u := sqlvalidator.SafeString(username)
	const selByName = `SELECT id FROM users WHERE lower(trim(username)) = lower(trim(?)) LIMIT 1`
	var id int64
	err = db.QueryRowContext(ctx, selByName, username).Scan(&id)
	if err == nil {
		var ph sql.NullString
		qHash := fmt.Sprintf(`SELECT password_hash FROM users WHERE id = %d`, id)
		if err := db.QueryRowContext(ctx, qHash).Scan(&ph); err != nil {
			return "", fmt.Errorf("read admin password_hash: %w", err)
		}
		hashMissing := !ph.Valid || strings.TrimSpace(ph.String) == ""
		if !hashMissing {
			return "", nil
		}
		h := sqlvalidator.SafeString(passwordHash)
		upd := fmt.Sprintf(
			`UPDATE users SET password_hash = '%s', updated_at = current_timestamp, is_active = true WHERE id = %d`,
			h, id,
		)
		if _, err := db.ExecContext(ctx, upd); err != nil {
			return "", fmt.Errorf("repair empty admin password_hash: %w", err)
		}
		return bootstrapPassword, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("check existing admin user: %w", err)
	}

	h := sqlvalidator.SafeString(passwordHash)
	insert := fmt.Sprintf(
		`INSERT INTO users (username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at)
		 VALUES ('%s', '%s', '', '', true, true, current_timestamp, current_timestamp)`,
		u, h,
	)
	if _, err := db.ExecContext(ctx, insert); err != nil {
		return "", fmt.Errorf("insert bootstrap admin: %w", err)
	}
	return bootstrapPassword, nil
}

// ResetOrInsertAdminPassword sets password_hash for username from config, or
// inserts the row if missing (used by homer --reset-admin-password).
func ResetOrInsertAdminPassword(ctx context.Context, db *sql.DB, username, passwordHash string) error {
	if db == nil {
		return fmt.Errorf("settings db not available")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		username = "admin"
	}
	passwordHash = strings.TrimSpace(passwordHash)
	if passwordHash == "" {
		return fmt.Errorf("admin_password_hash is required")
	}
	if passwordhash.IsDisallowedDefaultHash(passwordHash) {
		return fmt.Errorf("refusing well-known sipcapture password hash; set a unique bcrypt or SHA-256 hash")
	}

	u := sqlvalidator.SafeString(username)
	h := sqlvalidator.SafeString(passwordHash)

	const selByName = `SELECT id FROM users WHERE lower(trim(username)) = lower(trim(?)) LIMIT 1`
	var id int64
	err := db.QueryRowContext(ctx, selByName, username).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		insert := fmt.Sprintf(
			`INSERT INTO users (username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at)
			 VALUES ('%s', '%s', '', '', true, true, current_timestamp, current_timestamp)`,
			u, h,
		)
		if _, err := db.ExecContext(ctx, insert); err != nil {
			return fmt.Errorf("insert admin user: %w", err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("lookup admin user: %w", err)
	default:
		upd := fmt.Sprintf(
			`UPDATE users SET password_hash = '%s', updated_at = current_timestamp WHERE id = %d`,
			h, id,
		)
		if _, err := db.ExecContext(ctx, upd); err != nil {
			return fmt.Errorf("update admin password: %w", err)
		}
		return nil
	}
}
