package writer

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The merge error handler drops a data file from the catalog, and from the
// disk, when extractBrokenParquetPath returns its path. "Cannot open file"
// means a missing file only with "No such file or directory": every other
// reason is about a file that may still be there and valid.
func TestExtractBrokenParquetPathOnlyMissingOrCorrupt(t *testing.T) {
	const path = "/data/parquet/main/hep_proto_1_call/date=2026-11-04/ducklake-1.parquet"
	cases := []struct {
		err  string
		want string
	}{
		{`IO Error: Cannot open file "` + path + `": No such file or directory`, path},
		{`Invalid Input Error: Invalid footer length provided for file '` + path + `'`, path},
		{`IO Error: Cannot open file "` + path + `": Permission denied`, ""},
		{`IO Error: Cannot open file "` + path + `": Too many open files`, ""},
		{`IO Error: Cannot open file "` + path + `": Input/output error`, ""},
		{`IO Error: Cannot open file "` + path + `": Too many levels of symbolic links`, ""},
		{`HTTP Error: Unable to connect to URL s3://b/x.parquet: Not Found (HTTP code 404)`, ""},
	}
	for _, tc := range cases {
		if got := extractBrokenParquetPath(errors.New(tc.err)); got != tc.want {
			t.Errorf("extractBrokenParquetPath(%q) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

// End to end through the merge: a data file that exists but cannot be opened
// keeps its catalog entry and stays on disk, while a deleted one is still
// removed from the catalog so the merge can go on.
func TestMergeKeepsParquetThatFailsToOpen(t *testing.T) {
	for _, tc := range []struct {
		name     string
		deleted  bool // the file is gone; otherwise it only fails to open
		wantRows int
	}{
		{"open fails", false, 4},
		{"file deleted", true, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			victim := files[1]
			saved := victim + ".saved"
			if tc.deleted {
				if err := os.Remove(victim); err != nil {
					t.Fatal(err)
				}
			} else {
				// A symlink loop makes open(2) fail with ELOOP even when the test
				// runs as root, where chmod 000 would not stop the read. Like
				// Permission denied or Too many open files, it says nothing about
				// the file being gone.
				if err := os.Rename(victim, saved); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(victim, victim); err != nil {
					t.Fatal(err)
				}
			}

			if err := svc.mergeTablesDuckDB([]string{"lake.cpu"}); err != nil {
				t.Fatal(err)
			}

			if !tc.deleted {
				if _, err := os.Lstat(victim); err != nil {
					t.Fatalf("file that failed to open was removed from disk: %v", err)
				}
				if err := os.Remove(victim); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(saved, victim); err != nil {
					t.Fatal(err)
				}
			}
			if got := rowCount(t, db, "lake.cpu"); got != tc.wantRows {
				t.Fatalf("rows after merge = %d, want %d", got, tc.wantRows)
			}
		})
	}
}
