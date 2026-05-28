// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package passwordhash

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestLegacySHA256HexEqual(t *testing.T) {
	sum := sha256.Sum256([]byte("sipcapture"))
	legacy := hex.EncodeToString(sum[:])
	if !legacySHA256HexEqual("sipcapture", legacy) {
		t.Fatal("legacy sha256 verify failed")
	}
	if legacySHA256HexEqual("wrong", legacy) {
		t.Fatal("expected mismatch")
	}
}
