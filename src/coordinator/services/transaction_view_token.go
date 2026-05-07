// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// TransactionViewTokenService stores payloads for unauthenticated HTML transaction views
// (GET /export/view/:id) with a bounded number of opens per token.
type TransactionViewTokenService struct {
	db *sql.DB
}

// NewTransactionViewTokenService returns nil if db is nil.
func NewTransactionViewTokenService(db *sql.DB) *TransactionViewTokenService {
	if db == nil {
		return nil
	}
	return &TransactionViewTokenService{db: db}
}

// DDL kept in sync with services/settings_db.go EnsureSettingsSchema.
const transactionViewTokensDDL = `CREATE TABLE IF NOT EXISTS transaction_view_tokens (
			id VARCHAR PRIMARY KEY,
			owner_username VARCHAR NOT NULL,
			payload VARCHAR NOT NULL,
			created_at TIMESTAMP DEFAULT current_timestamp,
			expires_at TIMESTAMP NOT NULL,
			max_opens INTEGER NOT NULL DEFAULT 3,
			open_count INTEGER NOT NULL DEFAULT 0
		)`

func (s *TransactionViewTokenService) ensureTable(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("transaction view token service not available")
	}
	if _, err := s.db.ExecContext(ctx, transactionViewTokensDDL); err != nil {
		return err
	}
	return nil
}

func (s *TransactionViewTokenService) cleanupExpired(ctx context.Context) {
	if s == nil || s.db == nil {
		return
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM transaction_view_tokens WHERE expires_at < now()`)
}

func validateViewTokenID(id string) error {
	id = strings.TrimSpace(strings.ToLower(id))
	if len(id) != 32 {
		return fmt.Errorf("invalid view id")
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return fmt.Errorf("invalid view id")
		}
	}
	return nil
}

func clampMaxOpens(n int) int {
	if n <= 0 {
		return 3
	}
	if n > 1000 {
		return 1000
	}
	return n
}

// Create stores payload JSON and returns a new view id (32 hex chars). Default TTL 72h.
// maxOpens is stored on the row (how many successful GET opens allowed before exhaustion).
func (s *TransactionViewTokenService) Create(ctx context.Context, owner string, payload []byte, ttl time.Duration, maxOpens int) (string, error) {
	if s == nil || s.db == nil {
		return "", fmt.Errorf("transaction view token service not available")
	}
	if ttl <= 0 {
		ttl = 72 * time.Hour
	}
	maxOpens = clampMaxOpens(maxOpens)
	if err := s.ensureTable(ctx); err != nil {
		return "", err
	}
	s.cleanupExpired(ctx)

	var idBytes [16]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(idBytes[:])
	owner = strings.TrimSpace(owner)
	if owner == "" {
		owner = "unknown"
	}
	exp := time.Now().UTC().Add(ttl)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO transaction_view_tokens (id, owner_username, payload, expires_at, max_opens, open_count) VALUES (?, ?, ?, ?, ?, 0)`,
		id, owner, string(payload), exp, maxOpens,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

// ConsumePayload increments open count and returns the payload while open_count < max_opens and not expired.
func (s *TransactionViewTokenService) ConsumePayload(ctx context.Context, id string) ([]byte, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("transaction view token service not available")
	}
	if err := s.ensureTable(ctx); err != nil {
		return nil, err
	}
	if err := validateViewTokenID(id); err != nil {
		return nil, err
	}
	s.cleanupExpired(ctx)

	id = strings.TrimSpace(strings.ToLower(id))
	var raw string
	err := s.db.QueryRowContext(ctx,
		`UPDATE transaction_view_tokens
		 SET open_count = open_count + 1
		 WHERE id = ? AND expires_at > now() AND open_count < max_opens
		 RETURNING payload`,
		id,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("view link not found, expired, or maximum opens reached")
	}
	if err != nil {
		return nil, err
	}
	return []byte(raw), nil
}
