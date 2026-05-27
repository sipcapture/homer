// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package passwordhash

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestHashAndVerifyBcrypt(t *testing.T) {
	h, err := Hash("secret")
	if err != nil {
		t.Fatal(err)
	}
	if !isBcryptHash(h) {
		t.Fatalf("expected bcrypt hash, got %q", h)
	}
	if !Verify("secret", h) {
		t.Fatal("verify failed")
	}
	if Verify("wrong", h) {
		t.Fatal("verify should fail")
	}
}

func TestVerifyLegacySHA256(t *testing.T) {
	sum := sha256.Sum256([]byte("sipcapture"))
	legacy := hex.EncodeToString(sum[:])
	if !Verify("sipcapture", legacy) {
		t.Fatal("legacy sha256 verify failed")
	}
}
