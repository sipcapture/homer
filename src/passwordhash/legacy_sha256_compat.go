// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package passwordhash

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// legacySHA256HexEqual verifies homer-app SHA-256 hex password hashes (read-only compat).
// New passwords must use bcrypt via Hash(); this path is not used for hashing at rest.
func legacySHA256HexEqual(password, storedHex string) bool {
	sum := sha256.Sum256([]byte(password))
	return strings.EqualFold(hex.EncodeToString(sum[:]), storedHex)
}
