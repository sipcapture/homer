// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsLoopbackBindHost(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1":   true,
		"localhost":   true,
		"::1":         true,
		"[::1]":       true,
		"0.0.0.0":     false,
		"::":          false,
		"":            false,
		"192.168.1.1": false,
	}
	for host, want := range cases {
		if got := IsLoopbackBindHost(host); got != want {
			t.Errorf("IsLoopbackBindHost(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestResolveNodeFlightAuthTokenConfiguredWins(t *testing.T) {
	tok, auto, file, err := ResolveNodeFlightAuthToken("0.0.0.0", "explicit-token", t.TempDir()+"/cat.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "explicit-token" || auto || file != "" {
		t.Fatalf("got tok=%q auto=%v file=%q", tok, auto, file)
	}
}

func TestResolveNodeFlightAuthTokenLoopbackStaysEmpty(t *testing.T) {
	tok, auto, file, err := ResolveNodeFlightAuthToken("127.0.0.1", "", t.TempDir()+"/cat.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "" || auto || file != "" {
		t.Fatalf("got tok=%q auto=%v file=%q", tok, auto, file)
	}
}

func TestResolveNodeFlightAuthTokenAutoGenerateAndReuse(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "homer_catalog.sqlite")

	tok1, auto1, file1, err := ResolveNodeFlightAuthToken("0.0.0.0", "", catalog)
	if err != nil {
		t.Fatal(err)
	}
	if !auto1 || tok1 == "" || file1 == "" {
		t.Fatalf("expected auto-generated token, got tok=%q auto=%v file=%q", tok1, auto1, file1)
	}
	if _, err := os.Stat(file1); err != nil {
		t.Fatalf("token file missing: %v", err)
	}

	tok2, auto2, file2, err := ResolveNodeFlightAuthToken("0.0.0.0", "", catalog)
	if err != nil {
		t.Fatal(err)
	}
	if auto2 {
		t.Fatal("second resolve should reuse file, not regenerate")
	}
	if tok2 != tok1 || file2 != file1 {
		t.Fatalf("reuse mismatch: %q/%q vs %q/%q", tok2, file2, tok1, file1)
	}
}

func TestResolveRequiredAuthTokenIgnoresLoopback(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "homer_catalog.sqlite")
	tok, auto, file, err := ResolveRequiredAuthToken("", catalog)
	if err != nil {
		t.Fatal(err)
	}
	if tok == "" || !auto || file == "" {
		t.Fatalf("expected generated token, got tok=%q auto=%v file=%q", tok, auto, file)
	}
}

func TestApplyDefaultsFlightSQLTokenOnLoopback(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "cat.sqlite")
	cfg := Config{}
	cfg.Node.Enable = true
	cfg.Node.FlightServer.Host = "127.0.0.1"
	cfg.Node.FlightSQLServer.Enable = true
	cfg.Node.FlightSQLServer.Host = "127.0.0.1"
	cfg.Node.DuckLake.CatalogPath = catalog
	applyDefaults(&cfg)
	if cfg.Node.FlightServer.AuthToken != "" {
		t.Fatalf("HTTP loopback should stay empty, got %q", cfg.Node.FlightServer.AuthToken)
	}
	if cfg.Node.FlightSQLServer.AuthToken == "" {
		t.Fatal("FlightSQL must have a token even on loopback")
	}
}
