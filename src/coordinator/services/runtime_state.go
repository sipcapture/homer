// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
)

// RuntimeState holds expiring coordinator state shared by stateless replicas.
// Keys are hashed so bearer credentials do not appear in table keys or errors.
type RuntimeState struct {
	native *sql.DB
	owner  *settingsPostgresDriver
}

// LockSettingsBootstrap serializes first-run DDL and seeds across replicas.
// Closing the dedicated session releases the lock even after startup fails.
func LockSettingsBootstrap(db *sql.DB) (func(), error) {
	backend, ok := db.Driver().(*settingsPostgresDriver)
	if !ok {
		return nil, fmt.Errorf("stateless bootstrap requires PostgreSQL settings")
	}
	native := stdlib.OpenDB(*backend.config.Copy())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conn, err := native.Conn(ctx)
	if err != nil {
		_ = native.Close()
		return nil, fmt.Errorf("connect for settings bootstrap lock: %w", err)
	}
	// Close the handle first so releasing the reserved connection also closes
	// the physical PostgreSQL session and its session advisory lock.
	_ = native.Close()
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock(724193507)"); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("acquire settings bootstrap lock: %w", err)
	}
	return func() { _ = conn.Close() }, nil
}

func NewRuntimeState(db *sql.DB) (*RuntimeState, error) {
	if !isPostgresSettings(db) {
		return nil, fmt.Errorf("stateless runtime state requires PostgreSQL settings")
	}
	_, err := db.Exec("CALL postgres_execute('"+settingsPostgresCatalog+"', ?)", `
		SELECT pg_advisory_xact_lock(724193508);
		CREATE TABLE IF NOT EXISTS public.homer_runtime_state (
			namespace TEXT NOT NULL, key TEXT NOT NULL, payload BYTEA NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL, PRIMARY KEY (namespace, key)
		);
		CREATE INDEX IF NOT EXISTS homer_runtime_state_expiry ON public.homer_runtime_state (expires_at);`)
	if err != nil {
		return nil, fmt.Errorf("initialize shared runtime state: %w", err)
	}
	if _, err := db.Exec("CALL pg_clear_cache()"); err != nil {
		return nil, fmt.Errorf("refresh shared runtime state schema: %w", err)
	}
	owner := db.Driver().(*settingsPostgresDriver)
	native := stdlib.OpenDB(*owner.config.Copy())
	native.SetMaxOpenConns(8)
	native.SetMaxIdleConns(8)
	native.SetConnMaxIdleTime(5 * time.Minute)
	if !owner.registerRuntimePool(native) {
		_ = native.Close()
		return nil, fmt.Errorf("settings database closed while initializing runtime state")
	}
	return &RuntimeState{native: native, owner: owner}, nil
}

// Close releases the native pool. Closing the owning settings database also
// closes registered pools, including on coordinator construction failures.
func (s *RuntimeState) Close() error {
	s.owner.unregisterRuntimePool(s.native)
	return s.native.Close()
}

func runtimeKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func (s *RuntimeState) Put(ctx context.Context, namespace, key string, payload []byte, expires time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var ok int
	return s.native.QueryRowContext(ctx, `
		WITH expired AS (
			DELETE FROM public.homer_runtime_state WHERE (namespace, key) IN (
				SELECT namespace, key FROM public.homer_runtime_state WHERE expires_at <= CURRENT_TIMESTAMP
				AND NOT (namespace=$1 AND key=$2) LIMIT 128
			)
		)
		INSERT INTO public.homer_runtime_state (namespace, key, payload, expires_at) VALUES ($1, $2, $3, $4)
		ON CONFLICT (namespace, key) DO UPDATE SET payload = EXCLUDED.payload, expires_at = EXCLUDED.expires_at
		RETURNING 1`, namespace, runtimeKey(key), payload, expires).Scan(&ok)
}

// Get atomically deletes the entry when consume is true. Expired values are
// never returned, including when two replicas consume the same OAuth token.
func (s *RuntimeState) Get(ctx context.Context, namespace, key string, consume bool) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	queryRow := s.native.QueryRowContext
	query := `SELECT payload FROM public.homer_runtime_state WHERE namespace=$1 AND key=$2 AND expires_at > CURRENT_TIMESTAMP`
	if consume {
		query = `DELETE FROM public.homer_runtime_state WHERE namespace=$1 AND key=$2 AND expires_at > CURRENT_TIMESTAMP RETURNING payload`
	}
	var payload []byte
	err := queryRow(ctx, query, namespace, runtimeKey(key)).Scan(&payload)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	return payload, err == nil, err
}
