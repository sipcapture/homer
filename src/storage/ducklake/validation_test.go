// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"errors"
	"testing"
)

func TestValidateTableKey_valid(t *testing.T) {
	cases := []TableKey{
		{ProtoType: ProtoTypeSIP, SubType: SIPTypeCall},
		{ProtoType: ProtoTypeRTCPJSON},
		{ProtoType: ProtoTypeDNS},
	}
	for _, key := range cases {
		if err := ValidateTableKey(key); err != nil {
			t.Fatalf("ValidateTableKey(%+v): %v", key, err)
		}
	}
}

func TestValidateTableKey_injectionSubType(t *testing.T) {
	key := TableKey{ProtoType: ProtoTypeSIP, SubType: "'); DROP--"}
	if err := ValidateTableKey(key); err == nil {
		t.Fatal("expected error for injected sub_type")
	}
}

func TestParseTableKey(t *testing.T) {
	key, err := ParseTableKey(ProtoTypeSIP, "")
	if err != nil {
		t.Fatal(err)
	}
	if key.SubType != SIPTypeCall {
		t.Fatalf("default sub_type: got %q", key.SubType)
	}

	_, err = ParseTableKey(ProtoTypeRTCPJSON, "evil")
	if err == nil {
		t.Fatal("expected error for sub_type on non-SIP proto")
	}

	_, err = ParseTableKey(ProtoTypeSIP, "not-a-type")
	if !errors.Is(err, ErrInvalidTableKey) {
		t.Fatalf("got %v", err)
	}
}

func TestResolveTableFQN_canonicalWithoutTable(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LakeName = "test_lake"
	mtw, err := NewMultiTableWriter(cfg)
	if err != nil {
		t.Skipf("duckdb not available: %v", err)
	}
	defer mtw.Stop()

	key := TableKey{ProtoType: ProtoTypeSIP, SubType: SIPTypeCall}
	fqn, err := ResolveTableFQN(mtw, key)
	if err != nil {
		t.Fatal(err)
	}
	want := "test_lake.hep_proto_1_call"
	if fqn != want {
		t.Fatalf("got %q want %q", fqn, want)
	}
}

func TestResolveTableFQN_rejectsUnknownKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LakeName = "test_lake"
	mtw, err := NewMultiTableWriter(cfg)
	if err != nil {
		t.Skipf("duckdb not available: %v", err)
	}
	defer mtw.Stop()

	_, err = ResolveTableFQN(mtw, TableKey{ProtoType: 999, SubType: "x"})
	if !IsInvalidTableKey(err) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateWhereClause_valid(t *testing.T) {
	cols := []string{"session_id", "src_ip", "timestamp"}
	cases := []string{
		"",
		"session_id = 'abc123@host'",
		"src_ip = '192.168.1.100' AND timestamp > 0",
		"session_id IS NULL",
	}
	for _, where := range cases {
		if err := ValidateWhereClause(where, cols); err != nil {
			t.Fatalf("where %q: %v", where, err)
		}
	}
}

func TestValidateWhereClause_rejectsInjection(t *testing.T) {
	cols := []string{"session_id", "src_ip"}
	cases := []string{
		"'; DROP TABLE",
		"session_id = 'x' UNION SELECT",
		"unknown_col = '1'",
	}
	for _, where := range cases {
		err := ValidateWhereClause(where, cols)
		if err == nil {
			t.Fatalf("expected error for where %q", where)
		}
		if !errors.Is(err, ErrInvalidWhere) {
			t.Fatalf("where %q: expected ErrInvalidWhere, got %v", where, err)
		}
	}
}

func TestClampLimit(t *testing.T) {
	if got := ClampLimit(0, 100, 1000); got != 100 {
		t.Fatalf("got %d", got)
	}
	if got := ClampLimit(5000, 100, 1000); got != 1000 {
		t.Fatalf("got %d", got)
	}
}

func TestMultiTableReader_ListSnapshots_rejectsInvalidKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LakeName = "test_lake"
	mtw, err := NewMultiTableWriter(cfg)
	if err != nil {
		t.Skipf("duckdb not available: %v", err)
	}
	defer mtw.Stop()

	reader := NewMultiTableReader(mtw)
	_, err = reader.ListSnapshots(TableKey{ProtoType: 1, SubType: "'); DROP--"}, 10)
	if !IsInvalidTableKey(err) {
		t.Fatalf("got %v", err)
	}
}

func TestMultiTableReader_Query_rejectsInvalidWhere(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LakeName = "test_lake"
	mtw, err := NewMultiTableWriter(cfg)
	if err != nil {
		t.Skipf("duckdb not available: %v", err)
	}
	defer mtw.Stop()

	reader := NewMultiTableReader(mtw)
	key := TableKey{ProtoType: ProtoTypeSIP, SubType: SIPTypeCall}
	_, err = reader.Query(key, "'; DROP TABLE", 10)
	if !errors.Is(err, ErrInvalidWhere) {
		t.Fatalf("got %v", err)
	}
}
