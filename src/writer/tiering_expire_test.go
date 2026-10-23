// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package writer

import (
	"testing"
	"time"

	"github.com/sipcapture/homer-core/src/storage/ducklake"
)

func TestFinalVolumeForExpire(t *testing.T) {
	hot := &ducklake.Volume{Name: "hot", MaxDataAgeDays: 2}
	coldDisabled := &ducklake.Volume{Name: "cold", MaxDataAgeDays: 0}
	coldEnabled := &ducklake.Volume{Name: "cold", MaxDataAgeDays: 5}
	archive := &ducklake.Volume{Name: "archive", MaxDataAgeDays: 30}

	tests := []struct {
		name    string
		volumes []*ducklake.Volume
		want    string // volume name, or "" if nil
	}{
		{name: "empty", volumes: nil, want: ""},
		{name: "hot only with TTL", volumes: []*ducklake.Volume{hot}, want: "hot"},
		{name: "hot only zero TTL", volumes: []*ducklake.Volume{coldDisabled}, want: ""},
		{name: "two volumes cold disabled", volumes: []*ducklake.Volume{hot, coldDisabled}, want: ""},
		{name: "two volumes cold enabled", volumes: []*ducklake.Volume{hot, coldEnabled}, want: "cold"},
		{name: "three volumes last enabled", volumes: []*ducklake.Volume{hot, coldDisabled, archive}, want: "archive"},
		{name: "intermediate TTL ignored for expire", volumes: []*ducklake.Volume{
			{Name: "hot", MaxDataAgeDays: 2},
			{Name: "warm", MaxDataAgeDays: 7},
			{Name: "cold", MaxDataAgeDays: 0},
		}, want: ""},
		{name: "nil last entry", volumes: []*ducklake.Volume{hot, nil}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := finalVolumeForExpire(tt.volumes)
			if tt.want == "" {
				if got != nil {
					t.Fatalf("expected nil, got %q", got.Name)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected volume %q, got nil", tt.want)
			}
			if got.Name != tt.want {
				t.Fatalf("got %q, want %q", got.Name, tt.want)
			}
		})
	}
}

func TestPartitionStillOpen(t *testing.T) {
	now := time.Date(2026, 10, 20, 15, 4, 0, 0, time.FixedZone("UTC+2", 2*60*60))
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "rfc3339 today utc", raw: "2026-10-20T00:00:00Z", want: true},
		{name: "date only today", raw: "2026-10-20", want: true},
		{name: "space separated today", raw: "2026-10-20 00:00:00", want: true},
		{name: "yesterday", raw: "2026-10-19T00:00:00Z", want: false},
		{name: "future", raw: "2026-10-21", want: true},
		{name: "unparseable is not skipped", raw: "not-a-date", want: false},
		{name: "empty", raw: "", want: false},
		{name: "yesterday while local afternoon", raw: "2026-10-19T00:00:00Z", want: false},
	}
	// 2026-10-20 00:30 in UTC+2 is still 2026-10-19 UTC, so the 19th is open.
	early := time.Date(2026, 10, 20, 0, 30, 0, 0, time.FixedZone("UTC+2", 2*60*60))
	if !partitionStillOpen("2026-10-19T00:00:00Z", early) {
		t.Fatal("partition for the current UTC day must stay open")
	}
	if partitionStillOpen("2026-10-18T00:00:00Z", early) {
		t.Fatal("partition before the current UTC day must be movable")
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := partitionStillOpen(tc.raw, now); got != tc.want {
				t.Fatalf("partitionStillOpen(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestRecordMoveFailureSpeaksTwice(t *testing.T) {
	ts := NewTieringService(TieringConfig{}, nil)
	if got := ts.recordMoveFailure("hep_proto_1_default", "2026-10-13T00:00:00Z"); got != moveFailFirst {
		t.Fatalf("first failure = %v, want first", got)
	}
	if got := ts.recordMoveFailure("hep_proto_1_default", "2026-10-13T00:00:00Z"); got != moveFailRepeat {
		t.Fatalf("second failure = %v, want repeat", got)
	}
	if got := ts.recordMoveFailure("hep_proto_1_default", "2026-10-13T00:00:00Z"); got != moveFailSilent {
		t.Fatalf("third failure = %v, want silent", got)
	}
	// A different partition is reported on its own.
	if got := ts.recordMoveFailure("hep_proto_1_call", "2026-10-13T00:00:00Z"); got != moveFailFirst {
		t.Fatalf("other partition = %v, want first", got)
	}
	ts.clearMoveFailure("hep_proto_1_default", "2026-10-13T00:00:00Z")
	if got := ts.recordMoveFailure("hep_proto_1_default", "2026-10-13T00:00:00Z"); got != moveFailFirst {
		t.Fatalf("after success = %v, want first", got)
	}
}

func TestPartitionDateCutoffInclusive(t *testing.T) {
	// Mirrors expireOldPartitions / moveOldPartitions cutoff:
	// calendar(today) - N days, inclusive selection via GetPartitionsOlderThan.
	now := time.Date(2026, 7, 22, 15, 0, 0, 0, time.Local)
	got := now.AddDate(0, 0, -5).Format("2006-01-02")
	want := "2026-07-17"
	if got != want {
		t.Fatalf("cutoff = %q, want %q", got, want)
	}
}
