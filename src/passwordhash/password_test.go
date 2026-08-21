// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package passwordhash

import (
	"strings"
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
	legacy := "13d249f2cb4127b40cfa757866850278793f814ded3c587fe5889e889a7a9f6c" // sha256("testpass")
	if !Verify("testpass", legacy) {
		t.Fatal("legacy sha256 verify failed")
	}
	if Verify("wrong", legacy) {
		t.Fatal("verify should fail")
	}
}

func TestVerifyAcceptsSipcaptureDefaultHash(t *testing.T) {
	if !Verify("sipcapture", LegacySHA256SipcaptureHash) {
		t.Fatal("sipcapture SHA-256 should still verify so upgrades can force a password change")
	}
	if !Verify("sipcapture", strings.ToUpper(LegacySHA256SipcaptureHash)) {
		t.Fatal("uppercase sipcapture SHA-256 should still verify")
	}
}

func TestIsDisallowedDefaultHash(t *testing.T) {
	if !IsDisallowedDefaultHash(LegacySHA256SipcaptureHash) {
		t.Fatal("expected sipcapture digest")
	}
	if IsDisallowedDefaultHash("13d249f2cb4127b40cfa757866850278793f814ded3c587fe5889e889a7a9f6c") {
		t.Fatal("other SHA-256 hashes must not be treated as the default")
	}
}
