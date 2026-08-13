// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package node

import (
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

func TestAppendFsqlTimestampUsesArrowUnit(t *testing.T) {
	// 2026-08-12 20:34:30 UTC — the capture times that Grafana showed as 1882.
	ts := time.Unix(1786566870, 123000).UTC()
	dt := arrow.FixedWidthTypes.Timestamp_us
	mem := memory.NewGoAllocator()
	bldr := array.NewTimestampBuilder(mem, dt.(*arrow.TimestampType))
	defer bldr.Release()

	appendFsqlValue(bldr, dt, ts)
	arr := bldr.NewArray().(*array.Timestamp)
	defer arr.Release()
	if arr.Len() != 1 || arr.IsNull(0) {
		t.Fatal("expected one non-null timestamp")
	}

	got := arr.Value(0)
	want := arrow.Timestamp(ts.UnixMicro())
	if got != want {
		t.Fatalf("timestamp[us] value: got %d want %d (unixnano=%d)", got, want, ts.UnixNano())
	}

	// Influx/Grafana FlightSQL converts timestamp[us] → ns with *1000.
	// UnixNano written into timestamp[us] overflows here and becomes ~1882.
	asNs := int64(got) * 1000
	restored := time.Unix(0, asNs).UTC()
	if restored.Year() != 2026 {
		t.Fatalf("us*1000 as ns: got %s (year %d)", restored.Format(time.RFC3339Nano), restored.Year())
	}
}

func TestUnixNanoInTimestampUsOverflowsTo1882(t *testing.T) {
	ts := time.Unix(1786566870, 0).UTC()
	overflowed := ts.UnixNano() * 1000 // the old FlightSQL write, then Influx us→ns
	got := time.Unix(0, overflowed).UTC()
	if got.Year() != 1882 {
		t.Fatalf("expected the known 1882 overflow, got %s", got.Format(time.RFC3339Nano))
	}
}

func TestAppendFsqlTimestampParsesDuckDBString(t *testing.T) {
	dt := arrow.FixedWidthTypes.Timestamp_us
	mem := memory.NewGoAllocator()
	bldr := array.NewTimestampBuilder(mem, dt.(*arrow.TimestampType))
	defer bldr.Release()

	appendFsqlValue(bldr, dt, "2026-08-12 20:34:30")
	arr := bldr.NewArray().(*array.Timestamp)
	defer arr.Release()
	got := arr.Value(0).ToTime(arrow.Microsecond)
	if got.Year() != 2026 || got.Month() != time.August || got.Day() != 12 {
		t.Fatalf("parsed timestamp: %s", got.Format(time.RFC3339Nano))
	}
}

func TestDuckDBTypeToArrowTimestamps(t *testing.T) {
	cases := []struct {
		duck string
		unit arrow.TimeUnit
	}{
		{"TIMESTAMP", arrow.Microsecond},
		{"TIMESTAMP WITH TIME ZONE", arrow.Microsecond},
		{"TIMESTAMP_TZ", arrow.Microsecond},
		{"TIMESTAMP_NS", arrow.Nanosecond},
		{"TIMESTAMP_MS", arrow.Millisecond},
		{"TIMESTAMP_S", arrow.Second},
	}
	for _, tc := range cases {
		got := duckDBTypeToArrow(tc.duck)
		ts, ok := got.(*arrow.TimestampType)
		if !ok {
			t.Fatalf("%s: got %v, want timestamp", tc.duck, got)
		}
		if ts.Unit != tc.unit {
			t.Fatalf("%s: unit got %s want %s", tc.duck, ts.Unit, tc.unit)
		}
	}
}
