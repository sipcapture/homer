// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package cli

import (
	"crypto/tls"
	"os"
	"strings"
)

// cliInsecureTLS reports whether Homer CLI should skip TLS certificate verification.
// Set HOMER_INSECURE_TLS=1 (or true/yes) for coordinators using private/self-signed CAs.
func cliInsecureTLS() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("HOMER_INSECURE_TLS")))
	return v == "1" || v == "true" || v == "yes"
}

// cliTLSConfig returns TLS settings for Homer CLI HTTP clients.
func cliTLSConfig() *tls.Config {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if cliInsecureTLS() {
		cfg.InsecureSkipVerify = true
	}
	return cfg
}
