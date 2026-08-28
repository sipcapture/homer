// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package node

import (
	"database/sql"
	"testing"
	"time"

	"github.com/sipcapture/homer-core/src/config"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestNodeUsesAzureCredentialChain(t *testing.T) {
	cases := []struct {
		name    string
		volumes []config.VolumeConfig
		want    bool
	}{
		{name: "no volumes", volumes: nil, want: false},
		{name: "s3 volume only", volumes: []config.VolumeConfig{{Type: "s3"}}, want: false},
		{
			name:    "azure with account key",
			volumes: []config.VolumeConfig{{Type: "azure", AzureAccountKey: "k"}},
			want:    false,
		},
		{
			name:    "azure with connection string",
			volumes: []config.VolumeConfig{{Type: "azure", AzureConnectionString: "cs"}},
			want:    false,
		},
		{
			name:    "azure with no key and no connection string",
			volumes: []config.VolumeConfig{{Type: "azure", AzureAccountName: "acct"}},
			want:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.NodeConfig{DuckLake: config.DuckLakeConfig{Volumes: tc.volumes}}
			if got := nodeUsesAzureCredentialChain(cfg); got != tc.want {
				t.Errorf("nodeUsesAzureCredentialChain() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRefreshAzureSecrets_RecreatesCredentialChainSecret proves the refresh
// actually reaches DuckDB: it drops and recreates the credential_chain
// secret for an Azure volume, leaving a static-key volume's secret alone.
// CREATE SECRET for credential_chain never resolves credentials itself (only
// first use does, per DuckDB's azure extension), so this cannot hang like
// the write-path tests that go on to ATTACH/glob.
func TestRefreshAzureSecrets_RecreatesCredentialChainSecret(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open duckdb: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("INSTALL azure; LOAD azure;"); err != nil {
		t.Skipf("azure extension unavailable: %v", err)
	}

	n := &Node{
		db: db,
		config: &config.NodeConfig{
			DuckLake: config.DuckLakeConfig{
				Volumes: []config.VolumeConfig{
					{Name: "cold", Type: "azure", AzureAccountName: "chainacct"},
					{Name: "static", Type: "azure", AzureAccountName: "statacct", AzureAccountKey: "key"},
				},
			},
		},
	}

	n.refreshAzureSecrets()

	assertNodeSecret(t, db, "azure_secret_cold", true)
	assertNodeSecret(t, db, "azure_secret_static", false)
}

// TestRefreshAzureSecrets_NilDBIsNoOp is a defensive smoke: a node that
// hasn't attached yet must not panic when the ticker fires early.
func TestRefreshAzureSecrets_NilDBIsNoOp(t *testing.T) {
	n := &Node{config: &config.NodeConfig{DuckLake: config.DuckLakeConfig{
		Volumes: []config.VolumeConfig{{Name: "cold", Type: "azure"}},
	}}}
	n.refreshAzureSecrets() // must not panic
}

// TestRefreshAzureSecrets_HoldsRefreshMu: FlightSQL catalog reconnect
// closes n.db under refreshMu. DROP SECRET must take the same mutex so it
// cannot run against a handle that refreshCatalog is about to Close.
func TestRefreshAzureSecrets_HoldsRefreshMu(t *testing.T) {
	n := &Node{config: &config.NodeConfig{DuckLake: config.DuckLakeConfig{
		Volumes: []config.VolumeConfig{{Name: "cold", Type: "azure"}},
	}}}
	n.refreshMu.Lock()
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		close(started)
		n.refreshAzureSecrets()
		close(done)
	}()
	<-started
	select {
	case <-done:
		t.Fatal("refreshAzureSecrets returned while refreshMu is held")
	case <-time.After(80 * time.Millisecond):
	}
	n.refreshMu.Unlock()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("refreshAzureSecrets did not return after refreshMu was released")
	}
}

// TestStopAzureSecretRefresh_WaitsForInFlight: Stop must join the refresh
// goroutine before closing n.db, otherwise DROP SECRET can run on a closed handle.
func TestStopAzureSecretRefresh_WaitsForInFlight(t *testing.T) {
	n := &Node{}
	n.stopAzureRefresh = make(chan struct{})
	n.azureRefreshWg.Add(1)
	started := make(chan struct{})
	go func() {
		defer n.azureRefreshWg.Done()
		close(started)
		<-n.stopAzureRefresh
		time.Sleep(80 * time.Millisecond)
	}()
	<-started
	start := time.Now()
	n.stopAzureSecretRefresh()
	if time.Since(start) < 50*time.Millisecond {
		t.Fatal("expected stopAzureSecretRefresh to wait for the in-flight goroutine")
	}
	n.stopAzureSecretRefresh() // second call must be a no-op
}

func assertNodeSecret(t *testing.T, db *sql.DB, name string, wantExists bool) {
	t.Helper()
	var count int
	row := db.QueryRow("SELECT COUNT(*) FROM duckdb_secrets() WHERE name = '" + name + "'")
	if err := row.Scan(&count); err != nil {
		t.Skipf("duckdb_secrets() query unavailable: %v", err)
	}
	exists := count > 0
	if exists != wantExists {
		t.Errorf("secret %q exists=%v, want %v", name, exists, wantExists)
	}
}
