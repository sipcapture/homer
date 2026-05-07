// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//

//go:build integration
// +build integration

package input

import (
	"os"
	"path/filepath"
	"testing"

	gtls "github.com/gnet-io/tls"
)

// TestTLSIntegration tests TLS server integration
func TestTLSIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Generate test certificates
	certPath, keyPath, err := generateTestCert()
	if err != nil {
		t.Fatalf("Failed to generate test certificates: %v", err)
	}
	defer os.RemoveAll(filepath.Dir(certPath))

	// Test TLS config loading
	tlsSettings := &TLSSettings{
		Enable:        true,
		Cert:          certPath,
		Key:           keyPath,
		MinTLSVersion: "TLS1.2",
		MaxTLSVersion: "TLS1.3",
	}

	config, err := loadTLSConfig(tlsSettings)
	if err != nil {
		t.Fatalf("Failed to load TLS config: %v", err)
	}
	if config == nil {
		t.Fatal("Config is nil")
	}

	// Test TLS version parsing
	if parseTLSVersion("TLS1.2") != gtls.VersionTLS12 {
		t.Error("TLS version parsing failed")
	}
	if parseTLSVersion("TLS1.3") != gtls.VersionTLS13 {
		t.Error("TLS version parsing failed")
	}

	t.Log("TLS integration test passed")
}
