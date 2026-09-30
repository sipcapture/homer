// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package writer

import (
	"database/sql"
	"errors"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

var errFatal = errors.New("FATAL Error: Failed: database has been invalidated because of a previous fatal error")

func TestMaintenanceDBReopenedAfterFatal(t *testing.T) {
	writerDB, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	defer writerDB.Close()

	var opened, closed int
	open := func() (*sql.DB, func() error, error) {
		db, err := sql.Open("duckdb", "")
		if err != nil {
			return nil, nil, err
		}
		opened++
		return db, func() error { closed++; return db.Close() }, nil
	}

	svc := NewCompactionService(writerDB, "lake", t.TempDir(), "", CompactionConfig{}, nil, nil, nil)
	if err := svc.UseMaintenanceDB(open); err != nil {
		t.Fatalf("UseMaintenanceDB: %v", err)
	}
	first := svc.database()
	if first == writerDB {
		t.Fatal("compaction still runs on the writer DuckDB")
	}

	if svc.noteDBError(errors.New("database is locked")) {
		t.Fatal("non-fatal error treated as fatal")
	}
	if !svc.noteDBError(errFatal) {
		t.Fatal("fatal error not recognised")
	}
	if !svc.ensureDB() {
		t.Fatal("ensureDB failed to reopen")
	}
	if opened != 2 || closed != 1 {
		t.Errorf("opened=%d closed=%d, want 2 and 1", opened, closed)
	}
	if svc.database() == first {
		t.Error("invalidated instance still in use after reopen")
	}
	if err := writerDB.Ping(); err != nil {
		t.Errorf("writer DuckDB affected: %v", err)
	}
	if !svc.ensureDB() || opened != 2 {
		t.Error("healthy instance reopened again")
	}

	svc.Stop()
	if closed != 2 {
		t.Errorf("Stop did not close the maintenance instance (closed=%d)", closed)
	}
}

func TestSharedDBNotReopenedAfterFatal(t *testing.T) {
	svc := &CompactionService{lakeName: "lake"}
	svc.noteDBError(errFatal)
	if svc.ensureDB() {
		t.Error("ensureDB reported a usable instance for an invalidated shared DuckDB")
	}
}

func TestRecordMergeFatalQuarantinesTable(t *testing.T) {
	svc := &CompactionService{lakeName: "lake"}
	svc.recordMergeFatal("hep_proto_1_call")
	if _, q := svc.mergeQuarantined.Load("hep_proto_1_call"); q {
		t.Fatal("table quarantined after a single fatal merge")
	}
	svc.recordMergeFatal("hep_proto_1_call")
	if _, q := svc.mergeQuarantined.Load("hep_proto_1_call"); !q {
		t.Fatal("table not quarantined after repeated fatal merges")
	}
	if _, q := svc.mergeQuarantined.Load("hep_proto_1_default"); q {
		t.Error("unrelated table quarantined")
	}
}
