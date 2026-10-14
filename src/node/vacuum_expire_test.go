// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package node

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestParseExpireOlderThan(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		label   string
		seconds int64
		wantErr bool
	}{
		{name: "default", raw: "", label: "1 hour", seconds: 3600},
		{name: "one hour", raw: "1 hour", label: "1 hour", seconds: 3600},
		{name: "seven days", raw: "7 days", label: "7 days", seconds: 7 * 86400},
		{name: "two hours", raw: "2 hours", label: "2 hours", seconds: 7200},
		{name: "thirty minutes", raw: "30 minutes", label: "30 minutes", seconds: 1800},
		{name: "go duration", raw: "90m", label: "90m", seconds: 5400},
		{name: "injection", raw: "1 hour'); DROP TABLE homer_lake.main.hep_proto_1_call; --", wantErr: true},
		{name: "quote", raw: "1 hour'", wantErr: true},
		{name: "semicolon", raw: "1 hour; SELECT 1", wantErr: true},
		{name: "zero", raw: "0 hours", wantErr: true},
		{name: "negative go duration", raw: "-1h", wantErr: true},
		{name: "bare number", raw: "3600", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, seconds, err := parseExpireOlderThan(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q (%d)", label, seconds)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if label != tt.label || seconds != tt.seconds {
				t.Fatalf("got label=%q seconds=%d, want label=%q seconds=%d", label, seconds, tt.label, tt.seconds)
			}
		})
	}
}

func TestExpireSnapshotsSQLUsesIntegerSeconds(t *testing.T) {
	sql := expireSnapshotsSQL("homer_lake", 3600)
	want := "CALL ducklake_expire_snapshots('homer_lake', older_than => CAST(NOW() - INTERVAL '3600 seconds' AS TIMESTAMPTZ))"
	if sql != want {
		t.Fatalf("sql=%s", sql)
	}
	if strings.Contains(sql, "DROP") || strings.Contains(sql, ";") {
		t.Fatalf("sql must be a single call, got %s", sql)
	}

	quoted := expireSnapshotsSQL("lake'x", 1)
	if strings.Contains(quoted, "lake'x") || !strings.Contains(quoted, "lake''x") {
		t.Fatalf("lake name was not quoted: %s", quoted)
	}
}

func TestHandleVacuumRejectsExpireInjection(t *testing.T) {
	payload := "1 hour'); DROP TABLE homer_lake.main.hep_proto_1_call; CALL ducklake_expire_snapshots('homer_lake', older_than => INTERVAL '1 hour"
	req := httptest.NewRequest(http.MethodPost, "/vacuum?expire_older_than="+url.QueryEscape(payload), nil)
	rr := httptest.NewRecorder()
	(&Node{}).handleVacuum(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "DROP TABLE") && strings.Contains(rr.Body.String(), "success\":true") {
		t.Fatalf("injection must not succeed: %s", rr.Body.String())
	}
}
