// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestEscapeJSONData_QuotesAndControls(t *testing.T) {
	got := escapeJSONData("O'Reilly\x00\n")
	want := "O''Reilly\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestEscapeJSONData_DoesNotDoubleBackslashes(t *testing.T) {
	in := `{"a":"hello\nworld","p":"C:\\tmp"}`
	got := escapeJSONData(in)
	if got != in {
		t.Fatalf("backslash doubling would corrupt JSON: got %q want %q", got, in)
	}
}

func TestEscapeJSONData_DuckDBRoundtrip(t *testing.T) {
	db, err := sql.Open("duckdb", filepath.Join(t.TempDir(), "t.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	payloads := []string{
		`{"a":"hello\nworld","p":"C:\\tmp","q":"O'Reilly"}`,
		`x\' OR 1=1 --`,
		`hello\`,
		strings.Repeat(`local s = "a\nb"\n`, 80),
	}
	for _, payload := range payloads {
		var got string
		q := `SELECT '` + escapeJSONData(payload) + `' AS v`
		if err := db.QueryRow(q).Scan(&got); err != nil {
			t.Fatalf("payload %q: %v", payload, err)
		}
		if got != payload {
			t.Fatalf("roundtrip mismatch\n got %q\nwant %q", got, payload)
		}
	}
}

func TestDuckDB_BackslashIsNotAnEscapeInSingleQuotes(t *testing.T) {
	db, err := sql.Open("duckdb", filepath.Join(t.TempDir(), "t.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var a, b string
	err = db.QueryRow(`SELECT 'hello\', 'next'`).Scan(&a, &b)
	if err != nil {
		t.Fatalf("expected two columns when content ends with backslash: %v", err)
	}
	if a != `hello\` || b != "next" {
		t.Fatalf("got a=%q b=%q", a, b)
	}

	var doubled string
	sqlBoth := `SELECT '` + strings.ReplaceAll(`{"a":"\n"}`, `\`, `\\`) + `'`
	if err := db.QueryRow(sqlBoth).Scan(&doubled); err != nil {
		t.Fatal(err)
	}
	if doubled == `{"a":"\n"}` {
		t.Fatal("unexpected: DuckDB unescaped doubled backslashes (SafeString-style escaping would be OK)")
	}
	if doubled != `{"a":"\\n"}` {
		t.Fatalf("stored %q", doubled)
	}
}
