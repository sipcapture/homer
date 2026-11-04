package writer

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// The merge error only selects a candidate file. "Cannot open file" counts
// only with "No such file or directory", and a missing S3 object is reported
// by its HTTP URL, which maps back to an s3:// URL under the lake's bucket.
// The S3 forms are the ones DuckDB returned when merging against a deleted and
// a corrupt object, with path-style and virtual-hosted endpoints.
func TestExtractBrokenParquet(t *testing.T) {
	const (
		local  = "/data/parquet/main/hep_proto_1_call/date=2026-11-04/ducklake-1.parquet"
		lake   = "s3://homer-data/lake/"
		object = "s3://homer-data/lake/main/hep_proto_1_call/date=2026-11-04/ducklake-1.parquet"
		key    = "lake/main/hep_proto_1_call/date%3D2026-11-04/ducklake-1.parquet"
	)
	cases := []struct {
		err      string
		dataPath string
		want     brokenParquet
		ok       bool
	}{
		{`IO Error: Cannot open file "` + local + `": No such file or directory`, "/data/parquet", brokenParquet{local, true}, true},
		{`Invalid Input Error: Invalid footer length provided for file '` + local + `'`, "/data/parquet", brokenParquet{local, false}, true},
		{`Invalid Input Error: No magic bytes found at end of file '` + local + `'`, "/data/parquet", brokenParquet{local, false}, true},
		{`IO Error: Cannot open file "` + local + `": Permission denied`, "/data/parquet", brokenParquet{}, false},
		{`IO Error: Cannot open file "` + local + `": Too many open files`, "/data/parquet", brokenParquet{}, false},
		{`IO Error: Cannot open file "` + local + `": Input/output error`, "/data/parquet", brokenParquet{}, false},
		{`IO Error: Cannot open file "` + local + `": Too many levels of symbolic links`, "/data/parquet", brokenParquet{}, false},
		{`HTTP Error: HTTP GET error reading 'http://s3.local:9000/homer-data/` + key + `' in region 'us-east-1' (HTTP 404 Not Found)`, lake, brokenParquet{object, true}, true},
		{`HTTP Error: HTTP GET error reading 'https://homer-data.s3.us-east-1.amazonaws.com/` + key + `' in region 'us-east-1' (HTTP 404 Not Found)`, lake, brokenParquet{object, true}, true},
		{`HTTP Error: HTTP GET error reading 'http://s3.local:9000/other-bucket/` + key + `' in region 'us-east-1' (HTTP 404 Not Found)`, lake, brokenParquet{}, false},
		{`HTTP Error: HTTP GET error reading 'http://s3.local:9000/homer-data/other/x.parquet' in region 'us-east-1' (HTTP 404 Not Found)`, lake, brokenParquet{}, false},
		// A bucket named like the endpoint's first label: only the path-style
		// reading leaves the key under the lake's data path.
		{`HTTP Error: HTTP GET error reading 'https://s3.amazonaws.com/s3/lake/x.parquet' in region 'us-east-1' (HTTP 404 Not Found)`, "s3://s3/lake/", brokenParquet{"s3://s3/lake/x.parquet", true}, true},
		{`HTTP Error: HTTP GET error reading 'http://s3.local:9000/homer-data/` + key + `' in region 'us-east-1' (HTTP 404 Not Found)`, "/data/parquet", brokenParquet{}, false},
		{`HTTP Error: HTTP GET error reading 'http://s3.local:9000/homer-data/` + key + `' in region 'us-east-1' (HTTP 416 Range Not Satisfiable)`, lake, brokenParquet{}, false},
		{`HTTP Error: Content-Length from server mismatches requested range, server may not support range requests.`, lake, brokenParquet{}, false},
		{`Invalid Input Error: No magic bytes found at end of file '` + object + `'`, lake, brokenParquet{object, false}, true},
		{`IO Error: Cannot open file "` + object + `": Too many open files`, lake, brokenParquet{}, false},
	}
	for _, tc := range cases {
		got, ok := extractBrokenParquet(errors.New(tc.err), tc.dataPath)
		if got != tc.want || ok != tc.ok {
			t.Errorf("extractBrokenParquet(%q, %q) = %+v, %v; want %+v, %v", tc.err, tc.dataPath, got, ok, tc.want, tc.ok)
		}
	}
	if _, ok := extractBrokenParquet(nil, lake); ok {
		t.Error("extractBrokenParquet(nil) reported a file")
	}
}

// brokenFixture is a lake.cpu table with four data files, one of them chosen
// as the file a merge error names.
type brokenFixture struct {
	svc    *CompactionService
	db     *sql.DB
	victim string // absolute path of the chosen data file
	rel    string // its catalog path, relative to the table directory
}

func newBrokenFixture(t *testing.T) brokenFixture {
	t.Helper()
	svc, db := newLPCompactionFixture(t, CompactionConfig{Enable: true}, 1)
	ts := time.Now().UTC().Format("2006-01-02 15:04:05")
	for i := 1; i <= 3; i++ {
		stmt := fmt.Sprintf(`INSERT INTO lake.cpu VALUES (DATE '%s', 'h', TIMESTAMP '%s', %d)`, ts[:10], ts, 100+i)
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	var files []string
	_ = filepath.WalkDir(svc.dataPath, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".parquet") {
			files = append(files, p)
		}
		return nil
	})
	if len(files) != 4 {
		t.Fatalf("parquet files = %d, want 4", len(files))
	}
	sort.Strings(files)
	f := brokenFixture{svc: svc, db: db, victim: files[1]}
	f.rel = strings.TrimPrefix(f.victim, filepath.Join(svc.dataPath, "main", "cpu")+"/")
	if !f.inCatalog(t) {
		t.Fatalf("catalog has no row for %s", f.rel)
	}
	return f
}

// inCatalog reports whether the chosen file still has its catalog row. A nil
// merge result proves nothing: a non-fatal merge error is logged, not returned.
func (f brokenFixture) inCatalog(t *testing.T) bool {
	t.Helper()
	var n int
	if err := f.db.QueryRow(`SELECT count(*) FROM __ducklake_metadata_lake.ducklake_data_file WHERE path = ?`, f.rel).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// A merge error that names a local file removes the catalog row, and the file,
// only as far as the file confirms it.
func TestBrokenParquetLocalFileDecides(t *testing.T) {
	t.Run("missing text, file present and valid", func(t *testing.T) {
		f := newBrokenFixture(t)
		err := f.svc.clearBrokenParquet("cpu", errors.New(`IO Error: Cannot open file "`+f.victim+`": No such file or directory`))
		if err == nil {
			t.Error("merge error cleared")
		}
		if !f.inCatalog(t) || !exists(f.victim) {
			t.Fatalf("catalog row %v, file %v; want both kept", f.inCatalog(t), exists(f.victim))
		}
	})
	t.Run("invalid footer text, file is valid parquet", func(t *testing.T) {
		f := newBrokenFixture(t)
		err := f.svc.clearBrokenParquet("cpu", errors.New(`Invalid Input Error: Invalid footer length provided for file '`+f.victim+`'`))
		if err == nil {
			t.Error("merge error cleared")
		}
		if !f.inCatalog(t) || !exists(f.victim) {
			t.Fatalf("catalog row %v, file %v; want both kept", f.inCatalog(t), exists(f.victim))
		}
	})
	t.Run("invalid footer text, file is gone", func(t *testing.T) {
		f := newBrokenFixture(t)
		if err := os.Remove(f.victim); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.clearBrokenParquet("cpu", errors.New(`Invalid Input Error: Invalid footer length provided for file '`+f.victim+`'`)); err != nil {
			t.Errorf("merge error kept: %v", err)
		}
		if f.inCatalog(t) {
			t.Fatal("catalog row kept for a missing file")
		}
	})

	// Through the merge itself, with the errors DuckDB raises.
	for _, tc := range []struct {
		name     string
		damage   func(t *testing.T, path string)
		wantRow  bool
		wantFile bool
		wantRows int
	}{
		{"file deleted", func(t *testing.T, p string) {
			if err := os.Remove(p); err != nil {
				t.Fatal(err)
			}
		}, false, false, 3},
		{"short non-parquet file", func(t *testing.T, p string) {
			if err := os.WriteFile(p, []byte("not a parquet file\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, false, false, 3},
		{"large non-parquet file", func(t *testing.T, p string) {
			if err := os.WriteFile(p, []byte(strings.Repeat("x", 65536)), 0o644); err != nil {
				t.Fatal(err)
			}
		}, false, false, 3},
		// A symlink loop makes open(2) fail with ELOOP even when the test runs
		// as root, where chmod 000 would not stop the read. Like Permission
		// denied or Too many open files, it says nothing about the file.
		{"open fails", func(t *testing.T, p string) {
			if err := os.Rename(p, p+".saved"); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(p, p); err != nil {
				t.Fatal(err)
			}
		}, true, true, 4},
	} {
		t.Run("merge: "+tc.name, func(t *testing.T) {
			f := newBrokenFixture(t)
			tc.damage(t, f.victim)
			if err := f.svc.mergeTablesDuckDB([]string{"lake.cpu"}); err != nil {
				t.Fatal(err)
			}
			if got := f.inCatalog(t); got != tc.wantRow {
				t.Errorf("catalog row present = %v, want %v", got, tc.wantRow)
			}
			if got := exists(f.victim); got != tc.wantFile {
				t.Errorf("file present = %v, want %v", got, tc.wantFile)
			}
			if exists(f.victim + ".saved") {
				if err := os.Remove(f.victim); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(f.victim+".saved", f.victim); err != nil {
					t.Fatal(err)
				}
			}
			if got := rowCount(t, f.db, "lake.cpu"); got != tc.wantRows {
				t.Fatalf("rows after merge = %d, want %d", got, tc.wantRows)
			}
		})
	}
}

// On S3 the error text decides alone, because nothing local can check an
// object, and the object is never unlinked. The working directory holds a
// file at the path os.Remove would resolve the s3:// URL to, so an unlink
// attempt is visible.
func TestBrokenParquetRemoteObjectNeverUnlinked(t *testing.T) {
	const lake = "s3://homer-data/lake/"
	for _, tc := range []struct {
		name    string
		err     func(rel string) string
		wantRow bool
	}{
		{"too many open files", func(rel string) string {
			return `IO Error: Cannot open file "` + lake + "main/cpu/" + rel + `": Too many open files`
		}, true},
		{"missing object", func(rel string) string {
			return `HTTP Error: HTTP GET error reading 'http://s3.local:9000/homer-data/lake/main/cpu/` +
				strings.ReplaceAll(rel, "=", "%3D") + `' in region 'us-east-1' (HTTP 404 Not Found)`
		}, false},
		{"no magic bytes", func(rel string) string {
			return `Invalid Input Error: No magic bytes found at end of file '` + lake + "main/cpu/" + rel + `'`
		}, false},
		{"invalid footer", func(rel string) string {
			return `Invalid Input Error: Invalid footer length provided for file '` + lake + "main/cpu/" + rel + `'`
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBrokenFixture(t)
			remote := NewCompactionService(f.db, "lake", lake, f.svc.catalogPath, CompactionConfig{Enable: true}, nil, nil, nil)
			dir := t.TempDir()
			t.Chdir(dir)
			decoy := filepath.Join(dir, "s3:", "homer-data", "lake", "main", "cpu", f.rel)
			if err := os.MkdirAll(filepath.Dir(decoy), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(decoy, nil, 0o644); err != nil {
				t.Fatal(err)
			}

			err := remote.clearBrokenParquet("cpu", errors.New(tc.err(f.rel)))

			if (err == nil) == tc.wantRow {
				t.Errorf("merge error = %v, want kept = %v", err, tc.wantRow)
			}
			if got := f.inCatalog(t); got != tc.wantRow {
				t.Errorf("catalog row present = %v, want %v", got, tc.wantRow)
			}
			if !exists(decoy) || !exists(f.victim) {
				t.Fatalf("unlink attempted: decoy %v, local file %v", exists(decoy), exists(f.victim))
			}
		})
	}
}
