package mover

import "testing"

func TestJoinLakeLocalAndS3(t *testing.T) {
	got := joinLake("/data/hot", "main", "hep_proto_1_call", "date=2026-07-18/a.parquet")
	want := "/data/hot/main/hep_proto_1_call/date=2026-07-18/a.parquet"
	if got != want {
		t.Fatalf("local: got %q want %q", got, want)
	}
	got = destAbs("s3://bucket/cold/", "hep_proto_1_call", "date=2026-07-18/a.parquet")
	want = "s3://bucket/cold/main/hep_proto_1_call/date=2026-07-18/a.parquet"
	if got != want {
		t.Fatalf("s3: got %q want %q", got, want)
	}
}

// TestJoinLakeAz and TestJoinLakeAzure: PR review should-fix
// (github.com/sipcapture/homer/pull/983) — mirrors TestJoinLakeLocalAndS3's
// S3 case for both Azure scheme aliases; filepath.Join collapses "az://"/
// "azure://" to "az:/"/"azure:/" on Unix if the isAzurePath guard in
// joinLake is ever removed or missed.
func TestJoinLakeAz(t *testing.T) {
	got := destAbs("az://container/cold/", "hep_proto_1_call", "date=2026-07-18/a.parquet")
	want := "az://container/cold/main/hep_proto_1_call/date=2026-07-18/a.parquet"
	if got != want {
		t.Fatalf("az: got %q want %q", got, want)
	}
}

func TestJoinLakeAzure(t *testing.T) {
	got := destAbs("azure://container/cold/", "hep_proto_1_call", "date=2026-07-18/a.parquet")
	want := "azure://container/cold/main/hep_proto_1_call/date=2026-07-18/a.parquet"
	if got != want {
		t.Fatalf("azure: got %q want %q", got, want)
	}
}

func TestDestRelPathKeepsHiveDir(t *testing.T) {
	rel := destRelPath("date=2026-07-18/ducklake-x.parquet", "", "/data/hot", "calls", 1)
	if rel != "date=2026-07-18/ducklake-x.parquet" {
		t.Fatalf("got %q", rel)
	}
}

func TestDestRelPathStripsAbsolutePrefix(t *testing.T) {
	abs := "/data/hot/main/calls/date=2026-07-18/f.parquet"
	rel := destRelPath("", abs, "/data/hot", "calls", 0)
	if rel != "date=2026-07-18/f.parquet" {
		t.Fatalf("got %q", rel)
	}
}

func TestSplitS3URL(t *testing.T) {
	b, k, ok := splitS3URL("s3://bucket/homer/cold/main/t/date=2026-01-01/f.parquet")
	if !ok || b != "bucket" || k != "homer/cold/main/t/date=2026-01-01/f.parquet" {
		t.Fatalf("got bucket=%q key=%q ok=%v", b, k, ok)
	}
	if _, _, ok := splitS3URL("/local/path"); ok {
		t.Fatal("local path should not parse as s3")
	}
}

func TestFileAbsRelativeVsAbsolute(t *testing.T) {
	if got := fileAbs("/data", "calls", "/abs/file.parquet", 0); got != "/abs/file.parquet" {
		t.Fatalf("absolute catalog path: %q", got)
	}
	got := fileAbs("/data", "calls", "date=x/f.parquet", 1)
	if got != "/data/main/calls/date=x/f.parquet" {
		t.Fatalf("relative: %q", got)
	}
}
