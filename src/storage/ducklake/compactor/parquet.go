package compactor

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/extensions"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
	"github.com/apache/arrow-go/v18/parquet/schema"
)

// mergeResult describes a freshly written compacted parquet file.
type mergeResult struct {
	recordCount   int64
	fileSizeBytes int64
	footerSize    int64
}

// planBatches groups consecutive files so each group's combined on-disk size
// stays within targetBytes. Every input file lands in exactly one group,
// including lone files and files already larger than the target: the compactor
// retires a partition with a single DELETE, so anything left out of the merged
// output would be lost. Input order is preserved.
func planBatches(sizes []int64, targetBytes int64) [][]int {
	if targetBytes <= 0 || len(sizes) == 0 {
		return nil
	}
	var batches [][]int
	var cur []int
	var curSize int64
	flush := func() {
		if len(cur) > 0 {
			batches = append(batches, cur)
		}
		cur = nil
		curSize = 0
	}
	for i, sz := range sizes {
		if len(cur) > 0 && curSize+sz > targetBytes {
			flush()
		}
		cur = append(cur, i)
		curSize += sz
	}
	flush()
	return batches
}

// planPartition decides whether a partition is worth compacting and, if so,
// returns a batching that covers all of its files.
//
// Because retirement is per partition, coverage is all-or-nothing: either every
// file is rewritten or the partition is skipped. That makes a file already at or
// above the target size pure overhead — it gets rewritten to a new path for no
// gain. The partition is therefore skipped when that overhead exceeds the bytes
// actually being consolidated, which lets small files accumulate until merging
// them is worth the rewrite. The cost of touching a large file is amortised
// instead of paid every cycle.
func planPartition(sizes []int64, targetBytes int64) ([][]int, bool) {
	if len(sizes) < 2 {
		return nil, false
	}
	batches := planBatches(sizes, targetBytes)
	if len(batches) == 0 {
		return nil, false
	}
	var consolidated, overhead int64
	for _, b := range batches {
		var total int64
		for _, i := range b {
			total += sizes[i]
		}
		if len(b) >= 2 {
			consolidated += total
		} else {
			overhead += total
		}
	}
	if consolidated == 0 || overhead > consolidated {
		return nil, false
	}
	return batches, true
}

// readParquetFooterSize returns the serialized FileMetaData length recorded in
// the 8-byte parquet trailer (<uint32 footer_len><"PAR1">). This matches the
// value DuckLake stores in ducklake_data_file.footer_size.
func readParquetFooterSize(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	if _, err := f.Seek(-8, io.SeekEnd); err != nil {
		return 0, err
	}
	var tail [8]byte
	if _, err := io.ReadFull(f, tail[:]); err != nil {
		return 0, err
	}
	if string(tail[4:]) != "PAR1" {
		return 0, fmt.Errorf("not a parquet file (bad trailer magic): %s", path)
	}
	return int64(binary.LittleEndian.Uint32(tail[:4])), nil
}

// jsonLogicalColumns reports which top-level columns of a parquet file carry the
// JSON logical type.
//
// The Arrow bridge is lossy in one direction: reading BYTE_ARRAY/JSON yields a
// plain `binary` field (unlike UUID, it does not consult the extension registry),
// and writing `binary` back emits no logical annotation. DuckLake then refuses the
// file, because the table declares the column as JSON while the file says BLOB.
// Re-declaring those fields with the arrow.json extension type restores the
// annotation on write, since pqarrow honours ParquetLogicalType().
func jsonLogicalColumns(sc *schema.Schema) map[string]bool {
	out := make(map[string]bool)
	for i := 0; i < sc.NumColumns(); i++ {
		col := sc.Column(i)
		if _, ok := col.LogicalType().(schema.JSONLogicalType); ok {
			out[col.Name()] = true
		}
	}
	return out
}

// jsonAnnotatedSchema returns schema with every field named in jsonCols redeclared
// as the arrow.json extension type, plus the indices that were changed. Binary and
// string arrays share a memory layout, so the storage type can be swapped without
// touching the data.
func jsonAnnotatedSchema(in *arrow.Schema, jsonCols map[string]bool) (*arrow.Schema, map[int]arrow.DataType, error) {
	if len(jsonCols) == 0 {
		return in, nil, nil
	}
	fields := make([]arrow.Field, len(in.Fields()))
	changed := make(map[int]arrow.DataType)
	for i, f := range in.Fields() {
		fields[i] = f
		if !jsonCols[f.Name] || f.Type.ID() != arrow.BINARY {
			continue
		}
		jt, err := extensions.NewJSONType(arrow.BinaryTypes.String)
		if err != nil {
			return nil, nil, fmt.Errorf("json extension type for %q: %w", f.Name, err)
		}
		fields[i].Type = jt
		changed[i] = jt
	}
	if len(changed) == 0 {
		return in, nil, nil
	}
	md := in.Metadata()
	return arrow.NewSchema(fields, &md), changed, nil
}

// retypeJSONColumns rebuilds tbl against outSchema, reinterpreting the binary
// arrays of the given columns as arrow.json extension arrays. Binary and string
// arrays have identical buffer layouts (validity, int32 offsets, values), so the
// underlying data is reused rather than copied.
func retypeJSONColumns(tbl arrow.Table, outSchema *arrow.Schema, changed map[int]arrow.DataType) (arrow.Table, error) {
	cols := make([]arrow.Column, tbl.NumCols())
	release := make([]func(), 0, len(changed)*2)
	cleanup := func() {
		for _, fn := range release {
			fn()
		}
	}
	for i := 0; i < int(tbl.NumCols()); i++ {
		extType, needs := changed[i]
		if !needs {
			cols[i] = *tbl.Column(i)
			continue
		}
		ext, ok := extType.(arrow.ExtensionType)
		if !ok {
			cleanup()
			return nil, fmt.Errorf("column %d: %s is not an extension type", i, extType)
		}
		src := tbl.Column(i).Data().Chunks()
		chunks := make([]arrow.Array, len(src))
		for j, a := range src {
			d := a.Data()
			strData := array.NewData(arrow.BinaryTypes.String, d.Len(), d.Buffers(), nil, d.NullN(), d.Offset())
			storage := array.MakeFromData(strData)
			strData.Release()
			chunks[j] = array.NewExtensionArrayWithStorage(ext, storage)
			storage.Release()
			release = append(release, chunks[j].Release)
		}
		chunked := arrow.NewChunked(ext, chunks)
		release = append(release, chunked.Release)
		cols[i] = *arrow.NewColumn(outSchema.Field(i), chunked)
	}
	out := array.NewTable(outSchema, cols, tbl.NumRows())
	cleanup()
	return out, nil
}

// errUnsupportedSchema means the parquet -> Arrow -> parquet round trip would not
// reproduce the source column types, so the merged file would not match the table.
type errUnsupportedSchema struct {
	column   string
	from, to string
}

func (e *errUnsupportedSchema) Error() string {
	return fmt.Sprintf("column %q would change type from %s to %s through the arrow round trip",
		e.column, e.from, e.to)
}

// verifyRoundTrip checks that writing outSchema reproduces src's column types.
//
// The Arrow bridge silently degrades types it cannot represent exactly — HUGEINT
// comes back as DOUBLE, for example — and DuckLake then rejects the merged file at
// registration. Checking up front turns that into a clean skip with a readable
// reason instead of a merge that is thrown away on every cycle. It also needs no
// allowlist, so a column type added later is handled without touching this code.
func verifyRoundTrip(src *schema.Schema, outSchema *arrow.Schema, props *parquet.WriterProperties) error {
	got, err := pqarrow.ToParquet(outSchema, props, pqarrow.DefaultWriterProps())
	if err != nil {
		return fmt.Errorf("derive output parquet schema: %w", err)
	}
	if got.NumColumns() != src.NumColumns() {
		return &errUnsupportedSchema{
			column: "<all>",
			from:   fmt.Sprintf("%d columns", src.NumColumns()),
			to:     fmt.Sprintf("%d columns", got.NumColumns()),
		}
	}
	for i := 0; i < src.NumColumns(); i++ {
		want, have := src.Column(i), got.Column(i)
		if want.PhysicalType() == have.PhysicalType() &&
			want.LogicalType().Equals(have.LogicalType()) {
			continue
		}
		return &errUnsupportedSchema{
			column: want.Name(),
			from:   fmt.Sprintf("%s/%s", want.PhysicalType(), want.LogicalType()),
			to:     fmt.Sprintf("%s/%s", have.PhysicalType(), have.LogicalType()),
		}
	}
	return nil
}

// maxRowGroupBytes returns the uncompressed size of the file's largest row group,
// which is what the merge has to hold in memory at once. Reading it costs one
// footer read, no column data.
func maxRowGroupBytes(path string) (int64, error) {
	rdr, err := file.OpenParquetFile(path, false)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer rdr.Close()
	var largest int64
	for rg := 0; rg < rdr.NumRowGroups(); rg++ {
		if b := rdr.MetaData().RowGroup(rg).TotalByteSize(); b > largest {
			largest = b
		}
	}
	return largest, nil
}

// mergeParquetFiles concatenates the row groups of srcPaths into a single new
// parquet file at outPath, copying one row group at a time so peak memory is
// bounded by the largest single row group rather than the whole partition.
//
// The output reuses the first source file's Arrow schema and column
// compression codec so DuckLake/DuckDB read it identically to native files.
func mergeParquetFiles(ctx context.Context, srcPaths []string, outPath string) (mergeResult, error) {
	if len(srcPaths) == 0 {
		return mergeResult{}, fmt.Errorf("mergeParquetFiles: no source files")
	}

	first, err := file.OpenParquetFile(srcPaths[0], false)
	if err != nil {
		return mergeResult{}, fmt.Errorf("open %s: %w", srcPaths[0], err)
	}
	codec := compress.Codecs.Snappy
	if first.NumRowGroups() > 0 {
		if cc, err := first.RowGroup(0).MetaData().ColumnChunk(0); err == nil {
			codec = cc.Compression()
		}
	}
	firstReader, err := pqarrow.NewFileReader(first, pqarrow.ArrowReadProperties{}, memory.DefaultAllocator)
	if err != nil {
		_ = first.Close()
		return mergeResult{}, fmt.Errorf("arrow reader %s: %w", srcPaths[0], err)
	}
	arrowSchema, err := firstReader.Schema()
	if err != nil {
		_ = first.Close()
		return mergeResult{}, fmt.Errorf("schema %s: %w", srcPaths[0], err)
	}
	outSchema, jsonCols, err := jsonAnnotatedSchema(arrowSchema, jsonLogicalColumns(first.MetaData().Schema))
	if err != nil {
		_ = first.Close()
		return mergeResult{}, err
	}

	writerProps := parquet.NewWriterProperties(
		parquet.WithCompression(codec),
		parquet.WithCreatedBy("homer-native-compactor"),
	)
	// Checked before anything is written, so an unsupported column costs nothing.
	if err := verifyRoundTrip(first.MetaData().Schema, outSchema, writerProps); err != nil {
		_ = first.Close()
		return mergeResult{}, err
	}

	out, err := os.Create(outPath)
	if err != nil {
		_ = first.Close()
		return mergeResult{}, fmt.Errorf("create %s: %w", outPath, err)
	}

	fw, err := pqarrow.NewFileWriter(outSchema, out, writerProps, pqarrow.DefaultWriterProps())
	if err != nil {
		_ = first.Close()
		_ = out.Close()
		_ = os.Remove(outPath)
		return mergeResult{}, fmt.Errorf("parquet writer %s: %w", outPath, err)
	}

	var totalRows int64
	fail := func(err error) (mergeResult, error) {
		_ = fw.Close()
		_ = out.Close()
		_ = os.Remove(outPath)
		return mergeResult{}, err
	}

	for idx, src := range srcPaths {
		var rdr *file.Reader
		if idx == 0 {
			rdr = first
		} else {
			rdr, err = file.OpenParquetFile(src, false)
			if err != nil {
				return fail(fmt.Errorf("open %s: %w", src, err))
			}
		}
		fr, err := pqarrow.NewFileReader(rdr, pqarrow.ArrowReadProperties{}, memory.DefaultAllocator)
		if err != nil {
			_ = rdr.Close()
			return fail(fmt.Errorf("arrow reader %s: %w", src, err))
		}
		// ReadRowGroups needs explicit leaf-column indices; a nil slice reads
		// zero columns. Build the full column set for this file.
		numCols := rdr.MetaData().Schema.NumColumns()
		cols := make([]int, numCols)
		for i := range cols {
			cols[i] = i
		}
		for rg := 0; rg < rdr.NumRowGroups(); rg++ {
			tbl, err := fr.ReadRowGroups(ctx, cols, []int{rg})
			if err != nil {
				_ = rdr.Close()
				return fail(fmt.Errorf("read row group %d of %s: %w", rg, src, err))
			}
			rows := tbl.NumRows()
			chunk := rows
			if chunk <= 0 {
				chunk = 1
			}
			toWrite := tbl
			if len(jsonCols) > 0 {
				retyped, err := retypeJSONColumns(tbl, outSchema, jsonCols)
				if err != nil {
					tbl.Release()
					_ = rdr.Close()
					return fail(fmt.Errorf("retype row group %d of %s: %w", rg, src, err))
				}
				toWrite = retyped
			}
			err = fw.WriteTable(toWrite, chunk)
			if toWrite != tbl {
				toWrite.Release()
			}
			tbl.Release()
			if err != nil {
				_ = rdr.Close()
				return fail(fmt.Errorf("write row group %d of %s: %w", rg, src, err))
			}
			totalRows += rows
		}
		_ = rdr.Close()
	}

	// fw.Close() flushes the footer and closes the underlying *os.File, so the
	// output file must not be closed again here.
	if err := fw.Close(); err != nil {
		_ = out.Close()
		_ = os.Remove(outPath)
		return mergeResult{}, fmt.Errorf("close parquet writer %s: %w", outPath, err)
	}

	info, err := os.Stat(outPath)
	if err != nil {
		return mergeResult{}, fmt.Errorf("stat %s: %w", outPath, err)
	}
	footer, err := readParquetFooterSize(outPath)
	if err != nil {
		return mergeResult{}, err
	}
	return mergeResult{
		recordCount:   totalRows,
		fileSizeBytes: info.Size(),
		footerSize:    footer,
	}, nil
}
