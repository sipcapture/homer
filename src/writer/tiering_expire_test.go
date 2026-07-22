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
