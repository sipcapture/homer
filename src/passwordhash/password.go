// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package passwordhash provides password hashing and verification for Homer.
// New passwords use bcrypt; legacy SHA-256 hex hashes (homer-app) remain
// verifiable except the well-known sipcapture digest.
package passwordhash

import (
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = bcrypt.DefaultCost

// LegacySHA256SipcaptureHash is SHA-256("sipcapture"), the historical default
// admin password. Login and bootstrap refuse this digest (GHSA-263f-5xrw-c34r).
const LegacySHA256SipcaptureHash = "883ffc1f37fd0fe542b0fb9740035c4383e7d976c411161d24e62edace280f90"

// IsDisallowedDefaultHash reports the well-known sipcapture SHA-256 hex.
func IsDisallowedDefaultHash(stored string) bool {
	return strings.EqualFold(strings.TrimSpace(stored), LegacySHA256SipcaptureHash)
}

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
	if IsDisallowedDefaultHash(stored) {
		return false
	}
	return legacySHA256HexEqual(password, stored)
}

func isBcryptHash(stored string) bool {
	return strings.HasPrefix(stored, "$2a$") ||
		strings.HasPrefix(stored, "$2b$") ||
		strings.HasPrefix(stored, "$2y$")
}
