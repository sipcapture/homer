// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package passwordhash provides password hashing and verification for Homer.
// New passwords use bcrypt; legacy SHA-256 hex hashes (homer-app) remain verifiable.
package passwordhash

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = bcrypt.DefaultCost

// Hash returns a bcrypt hash for a new password.
func Hash(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// MustHash is like Hash but panics on error (bootstrap paths only).
func MustHash(password string) string {
	h, err := Hash(password)
	if err != nil {
		panic(err)
	}
	return h
}

// Verify checks password against a stored hash (bcrypt or legacy SHA-256 hex).
func Verify(password, stored string) bool {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return false
	}
	if isBcryptHash(stored) {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(password)) == nil
	}
	return verifyLegacySHA256Hex(password, stored)
}

// verifyLegacySHA256Hex checks homer-app SHA-256 hex hashes (not used for new passwords).
func verifyLegacySHA256Hex(password, storedHex string) bool {
	// lgtm[go/weak-sensitive-data-hashing] legacy homer-app hex only; Hash() uses bcrypt for new passwords.

	sum := sha256.Sum256([]byte(password)) // codeql[go/weak-sensitive-data-hashing]
	return strings.EqualFold(hex.EncodeToString(sum[:]), storedHex)
}

func isBcryptHash(stored string) bool {
	return strings.HasPrefix(stored, "$2a$") ||
		strings.HasPrefix(stored, "$2b$") ||
		strings.HasPrefix(stored, "$2y$")
}
