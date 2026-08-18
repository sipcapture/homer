package cli

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func writeCatalogTestConfig(t *testing.T, catalogPath string) string {
	t.Helper()
	cfg := filepath.Join(filepath.Dir(catalogPath), "homer.json")
	body := `{
  "storage": {
    "enable": true,
    "ducklake": {
      "catalog_path": "` + catalogPath + `"
    }
  }
}
`
	if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func seedCLICatalog(t *testing.T, path string, rows int) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE ducklake_snapshot(snapshot_id BIGINT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < rows; i++ {
		if _, err := db.Exec(`INSERT INTO ducklake_snapshot VALUES (?)`, i); err != nil {
			t.Fatal(err)
		}
	}
}

func countCLISnapshots(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ducklake_snapshot`).Scan(&n); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return n
}

func TestRunCatalogCmdBackupRestoreList(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "homer_catalog.sqlite")
	seedCLICatalog(t, catalog, 5)
	cfg := writeCatalogTestConfig(t, catalog)

	if err := RunCatalogCmd(CatalogFlags{Action: "backup", ConfigPath: cfg, Keep: 3}); err != nil {
		t.Fatalf("backup: %v", err)
	}

	db, err := sql.Open("sqlite", "file:"+catalog)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ducklake_snapshot VALUES (99)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if got := countCLISnapshots(t, catalog); got != 6 {
		t.Fatalf("mutated catalog has %d rows, want 6", got)
	}

	if err := RunCatalogCmd(CatalogFlags{Action: "restore", ConfigPath: cfg}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := countCLISnapshots(t, catalog); got != 5 {
		t.Fatalf("restored catalog has %d rows, want 5", got)
	}

	if err := RunCatalogCmd(CatalogFlags{Action: "list", ConfigPath: cfg}); err != nil {
		t.Fatalf("list: %v", err)
	}
}

func TestRunCatalogCmdBackupToOut(t *testing.T) {
	dir := t.TempDir()
	catalog := filepath.Join(dir, "homer_catalog.sqlite")
	seedCLICatalog(t, catalog, 2)
	cfg := writeCatalogTestConfig(t, catalog)
	out := filepath.Join(dir, "offsite", "catalog.sqlite")

	if err := RunCatalogCmd(CatalogFlags{Action: "backup", ConfigPath: cfg, Out: out}); err != nil {
		t.Fatalf("backup --out: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected --out file: %v", err)
	}
	if got := countCLISnapshots(t, out); got != 2 {
		t.Fatalf("out catalog has %d rows, want 2", got)
	}
}

func TestRunCatalogCmdUnknownAction(t *testing.T) {
	err := RunCatalogCmd(CatalogFlags{Action: "explode"})
	if err == nil || !strings.Contains(err.Error(), "unknown catalog action") {
		t.Fatalf("got %v", err)
	}
	err = RunCatalogCmd(CatalogFlags{})
	if err == nil || !strings.Contains(err.Error(), "no catalog action") {
		t.Fatalf("got %v", err)
	}
}

func TestParseCatalogFlags(t *testing.T) {
	fs, refs := RegisterCatalogFlags()
	if err := fs.Parse([]string{"--config-path", "/etc/homer/homer.json", "--keep", "0", "--from", "snap.sqlite"}); err != nil {
		t.Fatal(err)
	}
	f := ParseCatalogFlags(refs)
	if f.ConfigPath != "/etc/homer/homer.json" || f.Keep != 0 || f.From != "snap.sqlite" {
		t.Fatalf("parsed flags: %+v", f)
	}
}
