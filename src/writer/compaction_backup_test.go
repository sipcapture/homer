// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package writer

import (
	"context"
	"errors"
	"testing"
)

func TestBackupBeforeNativeMerge(t *testing.T) {
	tests := []struct {
		name        string
		catalogPath string
		backupErr   error
		want        bool
		wantCalled  bool
	}{
		{name: "no catalog configured", catalogPath: "", want: true},
		{name: "backup succeeds", catalogPath: "/data/c.sqlite", want: true, wantCalled: true},
		{name: "backup fails", catalogPath: "/data/c.sqlite", backupErr: errors.New("disk full"), want: false, wantCalled: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var called bool
			var gotPath string
			svc := &CompactionService{
				lakeName:    "lake",
				catalogPath: tc.catalogPath,
				backupCatalog: func(_ context.Context, p string) (string, error) {
					called, gotPath = true, p
					if tc.backupErr != nil {
						return "", tc.backupErr
					}
					return p + ".bak-x", nil
				},
			}
			if got := svc.backupBeforeNativeMerge(); got != tc.want {
				t.Errorf("backupBeforeNativeMerge() = %v, want %v", got, tc.want)
			}
			if called != tc.wantCalled {
				t.Errorf("backup called = %v, want %v", called, tc.wantCalled)
			}
			if called && gotPath != tc.catalogPath {
				t.Errorf("backup path = %q, want %q", gotPath, tc.catalogPath)
			}
		})
	}
}
