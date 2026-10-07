package ducklake

import (
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestResolveDataFilePath(t *testing.T) {
	tests := []struct {
		name                string
		data, schema, sPath string
		sRel                bool
		table, tPath        string
		tRel                bool
		file                string
		fRel                bool
		want                string
	}{
		{"relative chain", "/lake/data/", "default", "default/", true, "net_flow", "net_flow/", true,
			"date=2026-10-07/a.parquet", true, "/lake/data/default/net_flow/date=2026-10-07/a.parquet"},
		{"main schema", "/lake/data", "main", "main/", true, "t", "t/", true,
			"a.parquet", true, "/lake/data/main/t/a.parquet"},
		{"absolute file", "/lake/data", "main", "main/", true, "t", "t/", true,
			"/elsewhere/a.parquet", false, "/elsewhere/a.parquet"},
		{"absolute schema", "/lake/data", "s", "/other/s/", false, "t", "t/", true,
			"a.parquet", true, "/other/s/t/a.parquet"},
		{"empty paths fall back to names", "/lake/data", "s", "", true, "t", "", true,
			"a.parquet", true, "/lake/data/s/t/a.parquet"},
		{"remote data path", "s3://bucket/lake/", "default", "default/", true, "t", "t/", true,
			"a.parquet", true, "s3://bucket/lake/default/t/a.parquet"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveDataFilePath(tc.data, tc.schema, tc.sPath, tc.sRel,
				tc.table, tc.tPath, tc.tRel, tc.file, tc.fRel)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// newGhostLake attaches a DuckLake as "lake" with one parquet file per insert.
func newGhostLake(t *testing.T) (*sql.DB, string) {
	t.Helper()
	db := newDatePartitionLake(t)
	var data string
	if err := db.QueryRow(`SELECT value FROM "__ducklake_metadata_lake".ducklake_metadata
		WHERE key = 'data_path'`).Scan(&data); err != nil {
		t.Fatalf("data_path: %v", err)
	}
	execAll(t, db, `CALL lake.set_option('data_inlining_row_limit', 0)`)
	return db, data
}

func ghostTable(t *testing.T, db *sql.DB, schema, table string, files int) {
	t.Helper()
	if schema != "main" {
		execAll(t, db, fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS lake."%s"`, schema))
	}
	ref := fmt.Sprintf(`lake."%s".%s`, schema, table)
	execAll(t, db, fmt.Sprintf(`CREATE TABLE %s (date DATE, ts TIMESTAMP, v BIGINT)`, ref),
		fmt.Sprintf(`ALTER TABLE %s SET PARTITIONED BY (date)`, ref))
	for i := 0; i < files; i++ {
		execAll(t, db, fmt.Sprintf(`INSERT INTO %s VALUES (DATE '2026-10-07', TIMESTAMP '2026-10-07 10:00:00', %d)`, ref, i))
	}
}

func parquetFiles(t *testing.T, data, schema, table string) []string {
	t.Helper()
	var out []string
	root := filepath.Join(data, schema, table)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".parquet") {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(out)
	return out
}

func scanGhosts(t *testing.T, db *sql.DB, data string) GhostScan {
	t.Helper()
	s, err := ScanGhostFiles(db.Query, "lake", data)
	if err != nil {
		t.Fatalf("ScanGhostFiles: %v", err)
	}
	return s
}

func TestScanGhostFilesKeepsTablesOutsideMain(t *testing.T) {
	db, data := newGhostLake(t)
	ghostTable(t, db, "main", "cpu", 3)
	ghostTable(t, db, "default", "net_flow", 5)

	s := scanGhosts(t, db, data)
	if s.FilesChecked != 8 || len(s.Ghosts) != 0 || s.Refused {
		t.Fatalf("scan = %+v, want 8 files, no ghosts", s)
	}
}

func TestScanGhostFilesReportsMissingAndCorrupt(t *testing.T) {
	db, data := newGhostLake(t)
	ghostTable(t, db, "default", "net_flow", 4)
	files := parquetFiles(t, data, "default", "net_flow")
	if len(files) != 4 {
		t.Fatalf("files = %d, want 4", len(files))
	}
	if err := os.Remove(files[0]); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files[1], []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := scanGhosts(t, db, data)
	if s.Refused || len(s.Ghosts) != 2 {
		t.Fatalf("scan = %+v, want 2 ghosts, not refused", s)
	}
	byPath := map[string]GhostFile{}
	for _, g := range s.Ghosts {
		byPath[g.Path] = g
	}
	if g, ok := byPath[files[0]]; !ok || g.Exists || g.Schema != "default" || g.Table != "net_flow" {
		t.Errorf("missing file entry = %+v (found %v)", g, ok)
	}
	if g, ok := byPath[files[1]]; !ok || !g.Exists {
		t.Errorf("corrupt file entry = %+v (found %v)", g, ok)
	}

	if err := DeleteDataFileEntry(db.Exec, "lake", byPath[files[0]].DataFileID); err != nil {
		t.Fatalf("DeleteDataFileEntry: %v", err)
	}
	if after := scanGhosts(t, db, data); after.FilesChecked != 3 || len(after.Ghosts) != 1 {
		t.Errorf("after delete: %+v, want 3 files and 1 ghost", after)
	}
}

func TestScanGhostFilesRefusesWholeTable(t *testing.T) {
	db, data := newGhostLake(t)
	ghostTable(t, db, "main", "cpu", 3)
	ghostTable(t, db, "default", "gone", 3)
	if err := os.RemoveAll(filepath.Join(data, "default")); err != nil {
		t.Fatal(err)
	}

	s := scanGhosts(t, db, data)
	if !s.Refused || !strings.Contains(s.RefuseReason, "default.gone") {
		t.Fatalf("scan = %+v, want refused for default.gone", s)
	}
}

func TestScanGhostFilesRefusesTooMany(t *testing.T) {
	db, data := newGhostLake(t)
	ghostTable(t, db, "main", "cpu", 14)
	files := parquetFiles(t, data, "main", "cpu")
	for _, f := range files[:11] {
		if err := os.Remove(f); err != nil {
			t.Fatal(err)
		}
	}

	s := scanGhosts(t, db, data)
	if !s.Refused || len(s.Ghosts) != 11 {
		t.Fatalf("scan = %+v, want 11 ghosts, refused", s)
	}
}
