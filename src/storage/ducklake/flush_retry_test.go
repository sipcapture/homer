// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import "testing"

func TestIsFlushRetriableError(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"HTTP Error: Unable to connect to URL s3://b/x.parquet: Not Found (HTTP code 404)", true},
		{"HTTP Error: SlowDown", true},
		{"database is locked", true},
		{"transaction conflict", true},
		{"NoSuchBucket: missing", false},
		{"InvalidAccessKeyId", false},
		{"SignatureDoesNotMatch", false},
		{"syntax error near unexpected", false},
	}
	for _, tc := range cases {
		if got := isFlushRetriableError(tc.msg); got != tc.want {
			t.Errorf("isFlushRetriableError(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
}
