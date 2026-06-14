package compactor

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
)

// mergeResult describes a freshly written compacted parquet file.
type mergeResult struct {
	recordCount   int64
	fileSizeBytes int64
	footerSize    int64
}

// planBatches groups consecutive files so each group's combined on-disk size
// stays within targetBytes. Only groups with at least two files are returned —
// a lone file (or one already larger than the target) is left untouched, since
// rewriting it yields no merge benefit. Input order is preserved.
func planBatches(sizes []int64, targetBytes int64) [][]int {
	if targetBytes <= 0 {
		return nil
	}
	var batches [][]int
	var cur []int
	var curSize int64
	flush := func() {
		if len(cur) >= 2 {
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
	schema, err := firstReader.Schema()
	if err != nil {
		_ = first.Close()
		return mergeResult{}, fmt.Errorf("schema %s: %w", srcPaths[0], err)
	}

	out, err := os.Create(outPath)
	if err != nil {
		_ = first.Close()
		return mergeResult{}, fmt.Errorf("create %s: %w", outPath, err)
	}

	writerProps := parquet.NewWriterProperties(
		parquet.WithCompression(codec),
		parquet.WithCreatedBy("homer-native-compactor"),
	)
	fw, err := pqarrow.NewFileWriter(schema, out, writerProps, pqarrow.DefaultWriterProps())
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
			if err := fw.WriteTable(tbl, chunk); err != nil {
				tbl.Release()
				_ = rdr.Close()
				return fail(fmt.Errorf("write row group %d of %s: %w", rg, src, err))
			}
			totalRows += rows
			tbl.Release()
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
