package compactor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
)

func testSchema() *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "id", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "val", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil)
}

func writeTestParquet(t *testing.T, path string, ids []int64, vals []string) {
	t.Helper()
	schema := testSchema()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	props := parquet.NewWriterProperties(parquet.WithCompression(compress.Codecs.Snappy))
	w, err := pqarrow.NewFileWriter(schema, f, props, pqarrow.DefaultWriterProps())
	if err != nil {
		t.Fatal(err)
	}
	b := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	defer b.Release()
	b.Field(0).(*array.Int64Builder).AppendValues(ids, nil)
	sb := b.Field(1).(*array.StringBuilder)
	for _, v := range vals {
		sb.Append(v)
	}
	rec := b.NewRecord()
	defer rec.Release()
	if err := w.WriteBuffered(rec); err != nil {
		t.Fatal(err)
	}
	// pqarrow's Close flushes the footer and closes the underlying *os.File.
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestMergeRealHomerParquet exercises the merge against real DuckLake-written
// Homer parquet (wide varchar/json/uint32/date columns) when present locally.
// Skipped in environments without the sample data.
func TestMergeRealHomerParquet(t *testing.T) {
	srcDir := "/data/homer/parquet/main/hep_proto_1_call/date=2026-05-30"
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Skipf("no real homer data at %s: %v", srcDir, err)
	}
	var srcs []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".parquet" {
			srcs = append(srcs, filepath.Join(srcDir, e.Name()))
			if len(srcs) == 3 {
				break
			}
		}
	}
	if len(srcs) < 2 {
		t.Skip("need at least 2 real parquet files")
	}

	var wantRows int64
	for _, s := range srcs {
		rdr, err := file.OpenParquetFile(s, false)
		if err != nil {
			t.Fatal(err)
		}
		wantRows += rdr.NumRows()
		_ = rdr.Close()
	}

	out := filepath.Join(t.TempDir(), "merged.parquet")
	mr, err := mergeParquetFiles(context.Background(), srcs, out)
	if err != nil {
		t.Fatalf("merge real parquet: %v", err)
	}
	if mr.recordCount != wantRows {
		t.Errorf("merged rows = %d, want %d", mr.recordCount, wantRows)
	}
	rdr, err := file.OpenParquetFile(out, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rdr.Close()
	if rdr.NumRows() != wantRows {
		t.Errorf("reopened rows = %d, want %d", rdr.NumRows(), wantRows)
	}
	t.Logf("merged %d real files -> %d bytes, %d rows, footer %d",
		len(srcs), mr.fileSizeBytes, mr.recordCount, mr.footerSize)
}

func TestMergeParquetFiles(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.parquet")
	b := filepath.Join(dir, "b.parquet")
	out := filepath.Join(dir, "out.parquet")

	writeTestParquet(t, a, []int64{0, 1, 2}, []string{"x", "y", "z"})
	writeTestParquet(t, b, []int64{3, 4}, []string{"p", "q"})

	mr, err := mergeParquetFiles(context.Background(), []string{a, b}, out)
	if err != nil {
		t.Fatalf("mergeParquetFiles: %v", err)
	}
	if mr.recordCount != 5 {
		t.Errorf("recordCount = %d, want 5", mr.recordCount)
	}
	if mr.fileSizeBytes <= 0 {
		t.Errorf("fileSizeBytes = %d, want > 0", mr.fileSizeBytes)
	}

	// footer_size must match the trailer length recorded in the file.
	footer, err := readParquetFooterSize(out)
	if err != nil {
		t.Fatal(err)
	}
	if footer != mr.footerSize {
		t.Errorf("footerSize = %d, recomputed = %d", mr.footerSize, footer)
	}

	// Re-read the merged file and confirm all rows are present.
	rdr, err := file.OpenParquetFile(out, false)
	if err != nil {
		t.Fatal(err)
	}
	defer rdr.Close()
	if got := rdr.NumRows(); got != 5 {
		t.Errorf("merged NumRows = %d, want 5", got)
	}
}
