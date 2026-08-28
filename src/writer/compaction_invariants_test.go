package writer

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
	_ "modernc.org/sqlite"
)

// newInvariantFixture builds a real DuckLake and returns a CompactionService
// wired to it. Skipped when the DuckDB extensions are unavailable (offline CI).
func newInvariantFixture(t *testing.T) *CompactionService {
	t.Helper()
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(dir, "catalog.sqlite")

	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{"LOAD ducklake;", "LOAD sqlite;"} {
		if _, err := db.Exec(stmt); err != nil {
			t.Skipf("extension unavailable (%q): %v", stmt, err)
		}
	}
	if _, err := db.Exec(fmt.Sprintf(
		"ATTACH 'ducklake:sqlite:%s' AS lake (DATA_PATH '%s');", catalog, data)); err != nil {
		t.Skipf("ATTACH failed: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE lake.hep_proto_1_call (d DATE, id BIGINT)`,
		`INSERT INTO lake.hep_proto_1_call VALUES (DATE '2026-05-30', 1)`,
		`INSERT INTO lake.hep_proto_1_call VALUES (DATE '2026-05-30', 2)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	svc := NewCompactionService(db, "lake", data, catalog,
		CompactionConfig{Enable: true, Engine: EngineNativeGo}, nil, nil, nil)
	return svc
}

func TestCheckCatalogInvariantsHealthy(t *testing.T) {
	svc := newInvariantFixture(t)
	if err := svc.checkCatalogInvariants(); err != nil {
		t.Errorf("healthy catalog reported as broken: %v", err)
	}
}

// TestVerifySnapshotInvariants exercises the check against a stand-in metadata
// schema. A real catalog cannot be corrupted this way any more — snapshot_id is a
// primary key — so the detection logic is tested directly rather than through an
// injection the database now refuses.
func TestVerifySnapshotInvariants(t *testing.T) {
	tests := []struct {
		name      string
		ids       []string
		wantError bool
	}{
		{name: "healthy", ids: []string{"1", "2", "3"}},
		{name: "single snapshot", ids: []string{"1"}},
		{name: "duplicated latest", ids: []string{"1", "2", "2"}, wantError: true},
		{name: "duplicated older", ids: []string{"1", "1", "2"}, wantError: true},
		{name: "empty", ids: nil, wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, err := sql.Open("duckdb", "")
			if err != nil {
				t.Skipf("duckdb unavailable: %v", err)
			}
			defer db.Close()
			db.SetMaxOpenConns(1)
			if _, err := db.Exec(`CREATE SCHEMA md; CREATE TABLE md.ducklake_snapshot (snapshot_id BIGINT)`); err != nil {
				t.Fatal(err)
			}
			for _, id := range tc.ids {
				if _, err := db.Exec(`INSERT INTO md.ducklake_snapshot VALUES (` + id + `)`); err != nil {
					t.Fatal(err)
				}
			}
			err = verifySnapshotInvariants(db, "md")
			if tc.wantError && err == nil {
				t.Error("expected a violation, got none")
			}
			if !tc.wantError && err != nil {
				t.Errorf("unexpected violation: %v", err)
			}
		})
	}
}

// TestCheckCatalogInvariantsRejectsTextSnapshotID covers the one duplicate a
// primary key does not stop: an id stored with a different storage class. SQLite's
// unique index sees a distinct value while DuckLake casts the column to BIGINT and
// finds two rows for MAX(snapshot_id) — the state RepairCatalogSnapshots repairs.
func TestCheckCatalogInvariantsRejectsTextSnapshotID(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Skipf("duckdb unavailable: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE SCHEMA md; CREATE TABLE md.ducklake_snapshot (snapshot_id VARCHAR)`); err != nil {
		t.Fatal(err)
	}
	// '02' and '2' are different strings but the same integer.
	for _, v := range []string{"'1'", "'2'", "'02'"} {
		if _, err := db.Exec(`INSERT INTO md.ducklake_snapshot VALUES (` + v + `)`); err != nil {
			t.Fatal(err)
		}
	}
	if err := verifySnapshotInvariants(db, "md"); err == nil {
		t.Error("duplicate snapshot id differing only by storage class was not detected")
	} else {
		t.Logf("detected as expected: %v", err)
	}
}

// TestUseNativeEngineLatchesOffAfterViolation covers the safety latch: once a
// cycle has left the catalog broken, the process must stop choosing the native
// engine even though the config still asks for it.
func TestUseNativeEngineLatchesOffAfterViolation(t *testing.T) {
	svc := newInvariantFixture(t)
	if !svc.useNativeEngine() {
		t.Skip("native engine not usable in this environment (extension capability)")
	}
	svc.nativeDisabled.Store(true)
	if svc.useNativeEngine() {
		t.Error("native engine still selected after being disabled")
	}
}

// TestUseNativeEngineRejectsHivePath guards the trap found while verifying
// ducklake_add_data_files: DuckLake derives partition values from an added file's
// whole path, so a "key=value" directory in data_path would be read as a column
// the table does not have.
func TestUseNativeEngineRejectsHivePath(t *testing.T) {
	svc := newInvariantFixture(t)
	if !svc.useNativeEngine() {
		t.Skip("native engine not usable in this environment (extension capability)")
	}
	svc.dataPath = filepath.Join(svc.dataPath, "env=prod")
	if svc.useNativeEngine() {
		t.Error("native engine accepted a data_path with a hive-style component")
	}
}

// TestUseNativeEngineIgnoredForDuckDBEngine confirms the engine stays opt-in.
func TestUseNativeEngineIgnoredForDuckDBEngine(t *testing.T) {
	svc := newInvariantFixture(t)
	for _, engine := range []string{"", EngineDuckDB} {
		svc.config.Engine = engine
		if svc.useNativeEngine() {
			t.Errorf("engine %q selected the native compactor", engine)
		}
	}
}
