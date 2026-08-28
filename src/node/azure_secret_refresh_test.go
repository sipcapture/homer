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
