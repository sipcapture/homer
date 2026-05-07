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

// ShareExportService stores short-lived payloads for unauthenticated transaction exports (Homer "share link").
type ShareExportService struct {
	db *sql.DB
}

// NewShareExportService returns nil if db is nil.
func NewShareExportService(db *sql.DB) *ShareExportService {
	if db == nil {
		return nil
	}
	return &ShareExportService{db: db}
}

func (s *ShareExportService) cleanupExpired(ctx context.Context) {
	if s == nil || s.db == nil {
		return
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM export_share_links WHERE expires_at < now()`)
}

// Create stores payload JSON and returns a new share id (32 hex chars). Default TTL 72h.
func (s *ShareExportService) Create(ctx context.Context, owner string, payload []byte, ttl time.Duration) (string, error) {
	if s == nil || s.db == nil {
		return "", fmt.Errorf("share export service not available")
	}
	if ttl <= 0 {
		ttl = 72 * time.Hour
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
		`INSERT INTO export_share_links (id, owner_username, payload, expires_at) VALUES (?, ?, ?, ?)`,
		id, owner, string(payload), exp,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

// GetPayload returns the stored JSON body if the link exists and is not expired.
func (s *ShareExportService) GetPayload(ctx context.Context, id string) ([]byte, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("share export service not available")
	}
	id = strings.TrimSpace(strings.ToLower(id))
	if len(id) != 32 {
		return nil, fmt.Errorf("invalid share id")
	}
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return nil, fmt.Errorf("invalid share id")
		}
	}

	s.cleanupExpired(ctx)

	var raw string
	err := s.db.QueryRowContext(ctx,
		`SELECT payload FROM export_share_links WHERE id = ? AND expires_at > now()`,
		id,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("share link not found or expired")
	}
	if err != nil {
		return nil, err
	}
	return []byte(raw), nil
}
