// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package coordinator

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipcapture/homer-core/src/config"
)

// A bad coordinator.flightsql_server.tls_cert/tls_key must fail coordinator
// startup, not just log a warning and leave the rest of the coordinator up
// with Grafana's entrypoint silently missing.
func TestCoordinatorStartFailsOnBadFlightSQLProxyTLSCert(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.CoordinatorConfig{
		Enable:         true,
		SettingsDBPath: filepath.Join(dir, "settings.duckdb"),
		HTTPServer: config.CoordinatorHTTPServerConfig{
			Enable: true,
			Host:   "127.0.0.1",
			Port:   0,
		},
		FlightSQLServer: config.FlightSQLServerConfig{
			Enable:    true,
			Host:      "127.0.0.1",
			Port:      0,
			TLSEnable: true,
			TLSCert:   filepath.Join(dir, "missing.crt"),
			TLSKey:    filepath.Join(dir, "missing.key"),
		},
	}

	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = c.Stop() }()

	err = c.Start()
	if err == nil {
		t.Fatal("expected Start to fail when the FlightSQL proxy's TLS cert cannot be loaded")
	}
	if !strings.Contains(err.Error(), "FlightSQL proxy failed to start") {
		t.Fatalf("expected the FlightSQL proxy failure to be named, got: %v", err)
	}
}
