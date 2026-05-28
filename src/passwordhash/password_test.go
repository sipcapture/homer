// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package passwordhash

import (
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
	// Default homer-app admin digest (sha256 hex of "sipcapture").
	legacy := "883ffc1f37fd0fe542b0fb9740035c4383e7d976c411161d24e62edace280f90"
	if !Verify("sipcapture", legacy) {
		t.Fatal("legacy sha256 verify failed")
	}
}
