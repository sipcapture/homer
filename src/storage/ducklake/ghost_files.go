// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

// GhostMaxAbsolute and GhostMaxFraction bound how many ghost entries one scan
// may remove: more than both at once means the paths are wrong, not the files.
const (
	GhostMaxAbsolute = 10
	GhostMaxFraction = 0.01
)

// GhostFile is an active catalog entry whose parquet file is missing or corrupt.
type GhostFile struct {
	DataFileID int64
	Schema     string
	Table      string
	Path       string
	// Exists is true when the file is on disk but is not a valid parquet file.
	Exists bool
}

// GhostScan is the result of ScanGhostFiles.
type GhostScan struct {
	FilesChecked int
	Ghosts       []GhostFile
	// Refused is set when the ghosts look like a path resolution problem
	// rather than lost files; callers must not remove anything then.
	Refused      bool
	RefuseReason string
}

// ResolveDataFilePath returns the location of a data file the way DuckLake
// resolves it: dataPath / schema path / table path / file path, where a level
// that is not relative is used as is. An empty schema or table path falls back
// to "<name>/".
func ResolveDataFilePath(dataPath, schemaName, schemaPath string, schemaRel bool,
	tableName, tablePath string, tableRel bool, filePath string, fileRel bool) string {
	if !fileRel {
		return filePath
	}
	if schemaPath == "" {
		schemaPath = schemaName + "/"
	}
	if tablePath == "" {
		tablePath = tableName + "/"
	}
	schemaDir := schemaPath
	if schemaRel {
		schemaDir = JoinLakeDataPath(dataPath, schemaPath)
	}
	tableDir := tablePath
	if tableRel {
		tableDir = JoinLakeDataPath(schemaDir, tablePath)
	}
	return JoinLakeDataPath(tableDir, filePath)
}

type fileState int

const (
	fileOK fileState = iota
	fileMissing
	fileCorrupt
)

var parquetMagic = []byte("PAR1")

func probeParquet(path string) fileState {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileMissing
		}
		return fileOK
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() < 12 {
		return fileCorrupt
	}
	head := make([]byte, 4)
	tail := make([]byte, 4)
	if _, err := f.ReadAt(head, 0); err != nil {
		return fileCorrupt
	}
	if _, err := f.ReadAt(tail, info.Size()-4); err != nil {
		return fileCorrupt
	}
	if !bytes.Equal(head, parquetMagic) || !bytes.Equal(tail, parquetMagic) {
		return fileCorrupt
	}
	return fileOK
}

// IsUnreadableParquet reports whether path is missing or not a valid parquet file.
func IsUnreadableParquet(path string) bool {
	return probeParquet(path) != fileOK
}

// ScanGhostFiles lists active data files of every table in the lake whose
// local parquet file is missing or corrupt. A table whose files are all
// missing, or a total above GhostMaxAbsolute and GhostMaxFraction, sets
// Refused. query runs a statement against the DuckDB connection with the lake
// attached; dataPath must be local.
func ScanGhostFiles(query func(string, ...any) (*sql.Rows, error), lake, dataPath string) (GhostScan, error) {
	var out GhostScan
	if IsRemoteLakeDataPath(dataPath) {
		return out, fmt.Errorf("ghost scan needs a local data_path, got %s", dataPath)
	}
	md := lakeMetadataSchema(lake)
	rows, err := query(fmt.Sprintf(`
		SELECT f.data_file_id, s.schema_name, s.path, s.path_is_relative,
		       t.table_name, t.path, t.path_is_relative, f.path, f.path_is_relative
		FROM %[1]s.ducklake_data_file f
		JOIN %[1]s.ducklake_table t ON t.table_id = f.table_id AND t.end_snapshot IS NULL
		JOIN %[1]s.ducklake_schema s ON s.schema_id = t.schema_id AND s.end_snapshot IS NULL
		WHERE f.end_snapshot IS NULL`, md))
	if err != nil {
		return out, err
	}
	defer rows.Close()

	type tableCount struct{ files, missing int }
	perTable := map[string]*tableCount{}
	for rows.Next() {
		var (
			g                            GhostFile
			schemaPath, tablePath, file  sql.NullString
			schemaRel, tableRel, fileRel sql.NullBool
		)
		if err := rows.Scan(&g.DataFileID, &g.Schema, &schemaPath, &schemaRel,
			&g.Table, &tablePath, &tableRel, &file, &fileRel); err != nil {
			return out, err
		}
		g.Path = ResolveDataFilePath(dataPath,
			g.Schema, schemaPath.String, !schemaRel.Valid || schemaRel.Bool,
			g.Table, tablePath.String, !tableRel.Valid || tableRel.Bool,
			file.String, !fileRel.Valid || fileRel.Bool)

		key := g.Schema + "." + g.Table
		tc := perTable[key]
		if tc == nil {
			tc = &tableCount{}
			perTable[key] = tc
		}
		tc.files++
		out.FilesChecked++
		switch probeParquet(filepath.Clean(g.Path)) {
		case fileMissing:
			tc.missing++
			out.Ghosts = append(out.Ghosts, g)
		case fileCorrupt:
			g.Exists = true
			out.Ghosts = append(out.Ghosts, g)
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}

	for key, tc := range perTable {
		if tc.files > 1 && tc.missing == tc.files {
			out.Refused = true
			out.RefuseReason = fmt.Sprintf("all %d files of %s are missing", tc.files, key)
			return out, nil
		}
	}
	n := len(out.Ghosts)
	if n > GhostMaxAbsolute && float64(n) > GhostMaxFraction*float64(out.FilesChecked) {
		out.Refused = true
		out.RefuseReason = fmt.Sprintf("%d of %d files are missing or corrupt", n, out.FilesChecked)
	}
	return out, nil
}

// DeleteDataFileEntry removes one ducklake_data_file row by id.
func DeleteDataFileEntry(exec func(string, ...any) (sql.Result, error), lake string, id int64) error {
	_, err := exec(fmt.Sprintf("DELETE FROM %s.ducklake_data_file WHERE data_file_id = %d",
		lakeMetadataSchema(lake), id))
	return err
}
