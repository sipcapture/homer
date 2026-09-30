// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package ducklake

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestBackupCatalogOutOfProcessArgs(t *testing.T) {
	var gotName string
	var gotArgs []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotName, gotArgs = name, args
		return []byte("some log line\ncatalog backup: /data/c.sqlite.bak-20260930T082100Z\n"), nil
	}

	dest, err := BackupCatalogOutOfProcess(context.Background(), "/usr/local/bin/homer", "/data/c.sqlite", 3, run)
	if err != nil {
		t.Fatalf("BackupCatalogOutOfProcess: %v", err)
	}
	if dest != "/data/c.sqlite.bak-20260930T082100Z" {
		t.Errorf("dest = %q", dest)
	}
	if gotName != "/usr/local/bin/homer" {
		t.Errorf("exe = %q", gotName)
	}
	want := []string{"catalog", "backup", "--catalog", "/data/c.sqlite", "--keep", "3"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Errorf("args = %q, want %q", gotArgs, want)
	}
}

func TestBackupCatalogOutOfProcessFailure(t *testing.T) {
	run := func(context.Context, string, ...string) ([]byte, error) {
		return []byte("Catalog error: vacuum into: disk full\n"), errors.New("exit status 1")
	}
	_, err := BackupCatalogOutOfProcess(context.Background(), "homer", "/data/c.sqlite", 3, run)
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("err = %v, want child output in error", err)
	}

	noDest := func(context.Context, string, ...string) ([]byte, error) { return []byte("ok\n"), nil }
	if _, err := BackupCatalogOutOfProcess(context.Background(), "homer", "/data/c.sqlite", 3, noDest); err == nil {
		t.Fatal("expected error when child does not report a backup path")
	}
}
