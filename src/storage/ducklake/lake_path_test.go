// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import "testing"

func TestJoinLakeDataPath_PreservesS3Scheme(t *testing.T) {
	base := "s3://homer-data/lake/"
	got := JoinLakeDataPath(base, "main", "hep_proto_1_call", "date=2026-05-12", "x.parquet")
	want := "s3://homer-data/lake/main/hep_proto_1_call/date=2026-05-12/x.parquet"
	if got != want {
		t.Fatalf("JoinLakeDataPath: got %q want %q", got, want)
	}
}

func TestJoinLakeDataPath_LocalUsesFilepath(t *testing.T) {
	got := JoinLakeDataPath("/data/homer", "main", "t", "f.parquet")
	if got == "" {
		t.Fatal("empty result")
	}
	if got[0] != '/' {
		t.Fatalf("expected absolute local path, got %q", got)
	}
}
