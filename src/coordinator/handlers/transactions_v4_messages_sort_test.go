// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import "testing"

func TestSortTransactionMessageRowsByTimestampAsc_ordersOldestFirst(t *testing.T) {
	rows := []map[string]interface{}{
		{"uuid": "b", "timestamp": "2020-01-02T00:00:00Z"},
		{"uuid": "a", "timestamp": "2020-01-01T00:00:00Z"},
		{"uuid": "c", "timestamp": "2020-01-01T12:00:00Z"},
	}
	sortTransactionMessageRowsByTimestampAsc(rows)
	if got := rows[0]["uuid"]; got != "a" {
		t.Fatalf("row0 uuid = %v want a", got)
	}
	if got := rows[1]["uuid"]; got != "c" {
		t.Fatalf("row1 uuid = %v want c", got)
	}
	if got := rows[2]["uuid"]; got != "b" {
		t.Fatalf("row2 uuid = %v want b", got)
	}
}

func TestSortTransactionMessageRowsByTimestampAsc_missingTimestampLast(t *testing.T) {
	rows := []map[string]interface{}{
		{"uuid": "z", "timestamp": "2020-01-02T00:00:00Z"},
		{"uuid": "no-ts"},
		{"uuid": "a", "timestamp": "2020-01-01T00:00:00Z"},
	}
	sortTransactionMessageRowsByTimestampAsc(rows)
	if got := rows[0]["uuid"]; got != "a" {
		t.Fatalf("row0 uuid = %v want a", got)
	}
	if got := rows[1]["uuid"]; got != "z" {
		t.Fatalf("row1 uuid = %v want z", got)
	}
	if got := rows[2]["uuid"]; got != "no-ts" {
		t.Fatalf("row2 uuid = %v want no-ts", got)
	}
}
