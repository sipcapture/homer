// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package node

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/hugr-lab/airport-go/catalog"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// DuckLakeCatalog implements the airport catalog interface for DuckLake data
type DuckLakeCatalog struct {
	db       *sql.DB
	lakeName string
	volumes  []VolumeInfo // All attached volumes for tiered storage
	schemas  map[string]*DuckLakeSchema
	mu       sync.RWMutex
}

// NewDuckLakeCatalog creates a new DuckLake catalog with volume support
func NewDuckLakeCatalog(db *sql.DB, lakeName string, volumes []VolumeInfo) *DuckLakeCatalog {
	c := &DuckLakeCatalog{
		db:       db,
		lakeName: lakeName,
		volumes:  volumes,
		schemas:  make(map[string]*DuckLakeSchema),
	}

	// Create main schema pointing to the lake with volume support
	mainSchema := NewDuckLakeSchema(db, lakeName, "main", volumes)
	c.schemas["main"] = mainSchema

	// Also expose the lake name as a schema
	c.schemas[lakeName] = mainSchema

	return c
}

// replaceDB swaps the underlying DuckDB handle and rebuilds schema wrappers
// after a catalog reconnect. Must be called while holding the Node refresh
// write lock so no Airport query still uses the previous *sql.DB.
func (c *DuckLakeCatalog) replaceDB(db *sql.DB, volumes []VolumeInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.db = db
	c.volumes = volumes
	c.schemas = make(map[string]*DuckLakeSchema)
	mainSchema := NewDuckLakeSchema(db, c.lakeName, "main", volumes)
	c.schemas["main"] = mainSchema
	c.schemas[c.lakeName] = mainSchema
}

// Name returns the catalog name
func (c *DuckLakeCatalog) Name() string {
	return "homer"
}

// Schemas returns all schemas in the catalog
func (c *DuckLakeCatalog) Schemas(ctx context.Context) ([]catalog.Schema, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	schemas := make([]catalog.Schema, 0, len(c.schemas))
	for _, s := range c.schemas {
		schemas = append(schemas, s)
	}
	return schemas, nil
}

// Schema returns a schema by name
func (c *DuckLakeCatalog) Schema(ctx context.Context, name string) (catalog.Schema, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if s, ok := c.schemas[name]; ok {
		return s, nil
	}
	return nil, catalog.ErrNotFound
}

// GetStats returns statistics about the catalog
func (c *DuckLakeCatalog) GetStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := make(map[string]interface{})
	for name, schema := range c.schemas {
		stats[name] = schema.GetStats()
	}
	return stats
}

// DuckLakeSchema implements the catalog.Schema interface
type DuckLakeSchema struct {
	db        *sql.DB
	lakeName  string
	name      string
	volumes   []VolumeInfo // All attached volumes
	tables    map[string]*DuckLakeTable
	mu        sync.RWMutex
	allocator memory.Allocator
}

// NewDuckLakeSchema creates a new DuckLake schema with volume support
func NewDuckLakeSchema(db *sql.DB, lakeName string, name string, volumes []VolumeInfo) *DuckLakeSchema {
	return &DuckLakeSchema{
		db:        db,
		lakeName:  lakeName,
		name:      name,
		volumes:   volumes,
		tables:    make(map[string]*DuckLakeTable),
		allocator: memory.DefaultAllocator,
	}
}

// Name returns the schema name
func (s *DuckLakeSchema) Name() string {
	return s.name
}

// Comment returns the schema description
func (s *DuckLakeSchema) Comment() string {
	return fmt.Sprintf("HEP packet data from DuckLake (%s)", s.lakeName)
}

// Tables returns all tables in the schema
func (s *DuckLakeSchema) Tables(ctx context.Context) ([]catalog.Table, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Discover tables from DuckLake
	if err := s.discoverTables(); err != nil {
		logger.Error(fmt.Sprintf("Node: Failed to discover tables: %v", err))
	}

	tables := make([]catalog.Table, 0, len(s.tables))
	for _, t := range s.tables {
		tables = append(tables, t)
	}
	return tables, nil
}

// Table returns a table by name
func (s *DuckLakeSchema) Table(ctx context.Context, name string) (catalog.Table, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Discover tables if not already done
	if len(s.tables) == 0 {
		if err := s.discoverTables(); err != nil {
			return nil, err
		}
	}

	if t, ok := s.tables[name]; ok {
		return t, nil
	}
	return nil, catalog.ErrNotFound
}

// ScalarFunctions returns all scalar functions
func (s *DuckLakeSchema) ScalarFunctions(ctx context.Context) ([]catalog.ScalarFunction, error) {
	return nil, nil
}

// TableFunctions returns all table functions
func (s *DuckLakeSchema) TableFunctions(ctx context.Context) ([]catalog.TableFunction, error) {
	return nil, nil
}

// TableFunctionsInOut returns all table functions with row input
func (s *DuckLakeSchema) TableFunctionsInOut(ctx context.Context) ([]catalog.TableFunctionInOut, error) {
	return nil, nil
}

// discoverTables discovers tables from all DuckLake volumes
func (s *DuckLakeSchema) discoverTables() error {
	// Collect unique table names from all volumes
	tableNames := make(map[string]bool)

	for _, vol := range s.volumes {
		query := fmt.Sprintf(`
			SELECT table_name 
			FROM information_schema.tables 
			WHERE table_catalog = '%s' 
			  AND table_schema = 'main'
		`, vol.LakeName)

		rows, err := s.db.Query(query)
		if err != nil {
			logger.Warn(fmt.Sprintf("Node: Failed to query tables from volume %s: %v", vol.Name, err))
			continue
		}

		for rows.Next() {
			var tableName string
			if err := rows.Scan(&tableName); err != nil {
				continue
			}
			tableNames[tableName] = true
		}
		rows.Close()
	}

	// Create tables with volume support
	for tableName := range tableNames {
		if _, ok := s.tables[tableName]; !ok {
			// Build list of FQNs for this table across all volumes
			var volumeFQNs []VolumeFQN
			for _, vol := range s.volumes {
				volumeFQNs = append(volumeFQNs, VolumeFQN{
					VolumeName: vol.Name,
					LakeName:   vol.LakeName,
					FQN:        fmt.Sprintf("%s.main.%s", vol.LakeName, tableName),
				})
			}
			s.tables[tableName] = NewDuckLakeTable(s.db, tableName, volumeFQNs, s.allocator)
		}
	}

	return nil
}

// GetStats returns statistics about the schema
func (s *DuckLakeSchema) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["tables_count"] = len(s.tables)

	tableStats := make(map[string]interface{})
	for name, table := range s.tables {
		tableStats[name] = table.GetStats()
	}
	stats["tables"] = tableStats

	return stats
}

// VolumeFQN represents a fully-qualified table name in a specific volume
type VolumeFQN struct {
	VolumeName string // Volume name (e.g., "hot", "cold")
	LakeName   string // DuckLake name (e.g., "homer_lake_hot")
	FQN        string // Full table name (e.g., "homer_lake_hot.main.hep_proto_1")
}

// DuckLakeTable implements the catalog.Table interface with tiered storage support
type DuckLakeTable struct {
	db         *sql.DB
	name       string
	volumeFQNs []VolumeFQN // Table FQNs across all volumes
	schema     *arrow.Schema
	allocator  memory.Allocator
	mu         sync.RWMutex
	queryCount int64
}

// NewDuckLakeTable creates a new DuckLake table with volume support
func NewDuckLakeTable(db *sql.DB, name string, volumeFQNs []VolumeFQN, allocator memory.Allocator) *DuckLakeTable {
	return &DuckLakeTable{
		db:         db,
		name:       name,
		volumeFQNs: volumeFQNs,
		allocator:  allocator,
	}
}

// getPrimaryFQN returns the FQN for the first volume (used for schema discovery)
func (t *DuckLakeTable) getPrimaryFQN() string {
	if len(t.volumeFQNs) > 0 {
		return t.volumeFQNs[0].FQN
	}
	return ""
}

// buildUnionQuery builds a UNION ALL query across all volumes
func (t *DuckLakeTable) buildUnionQuery(selectClause string, limit int) string {
	if len(t.volumeFQNs) == 0 {
		return ""
	}

	// Single volume - no UNION needed
	if len(t.volumeFQNs) == 1 {
		query := fmt.Sprintf("SELECT %s FROM %s", selectClause, t.volumeFQNs[0].FQN)
		if limit > 0 {
			query += fmt.Sprintf(" LIMIT %d", limit)
		}
		return query
	}

	// Multiple volumes - build UNION ALL
	var parts []string
	for _, vfqn := range t.volumeFQNs {
		part := fmt.Sprintf("SELECT %s FROM %s", selectClause, vfqn.FQN)
		parts = append(parts, part)
	}

	query := "(" + strings.Join(parts, " UNION ALL ") + ")"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	return query
}

// Name returns the table name
func (t *DuckLakeTable) Name() string {
	return t.name
}

// Comment returns the table description
func (t *DuckLakeTable) Comment() string {
	return fmt.Sprintf("HEP data table: %s", t.name)
}

// ArrowSchema returns the Arrow schema with optional column projection
func (t *DuckLakeTable) ArrowSchema(columns []string) *arrow.Schema {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Discover schema if not cached
	if t.schema == nil {
		schema, err := t.discoverSchema()
		if err != nil {
			logger.Error(fmt.Sprintf("Node: Failed to discover schema for %s: %v", t.name, err))
			return defaultHEPSchema()
		}
		t.schema = schema
	}

	// No column projection requested
	if len(columns) == 0 {
		return t.schema
	}

	// Project columns
	fields := make([]arrow.Field, 0, len(columns))
	for _, col := range columns {
		for _, f := range t.schema.Fields() {
			if f.Name == col {
				fields = append(fields, f)
				break
			}
		}
	}
	return arrow.NewSchema(fields, nil)
}

// discoverSchema discovers the table schema from DuckDB (uses first available volume)
func (t *DuckLakeTable) discoverSchema() (*arrow.Schema, error) {
	fqn := t.getPrimaryFQN()
	if fqn == "" {
		return defaultHEPSchema(), nil
	}

	// Query column info
	query := fmt.Sprintf("DESCRIBE %s", fqn)
	rows, err := t.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to describe table: %w", err)
	}
	defer rows.Close()

	var fields []arrow.Field
	for rows.Next() {
		var colName, colType string
		var nullable string
		var key, defaultVal, extra interface{}

		if err := rows.Scan(&colName, &colType, &nullable, &key, &defaultVal, &extra); err != nil {
			continue
		}

		arrowType := duckDBTypeToArrow(colType)
		isNullable := nullable == "YES"
		fields = append(fields, arrow.Field{Name: colName, Type: arrowType, Nullable: isNullable})
	}

	if len(fields) == 0 {
		// Return default HEP schema if can't discover
		return defaultHEPSchema(), nil
	}

	return arrow.NewSchema(fields, nil), nil
}

// Scan executes a query across all volumes using UNION ALL and returns Arrow record reader
func (t *DuckLakeTable) Scan(ctx context.Context, opts *catalog.ScanOptions) (array.RecordReader, error) {
	t.mu.Lock()
	t.queryCount++
	t.mu.Unlock()

	// Build UNION ALL query across all volumes
	limit := 0
	if opts != nil && opts.Limit > 0 {
		limit = int(opts.Limit)
	}
	query := t.buildUnionQuery("*", limit)

	if query == "" {
		return nil, fmt.Errorf("no volumes available for table %s", t.name)
	}

	// Execute query
	rows, err := t.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	// Get schema
	schema := t.ArrowSchema(nil)

	// Build record from rows
	records, err := t.rowsToRecords(rows, schema)
	if err != nil {
		return nil, err
	}

	return array.NewRecordReader(schema, records)
}

// rowsToRecords converts SQL rows to Arrow records
func (t *DuckLakeTable) rowsToRecords(rows *sql.Rows, schema *arrow.Schema) ([]arrow.Record, error) {
	defer rows.Close()

	builder := array.NewRecordBuilder(t.allocator, schema)
	defer builder.Release()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	// Map column names to builder indices
	colToIdx := make(map[string]int)
	for i, f := range schema.Fields() {
		colToIdx[f.Name] = i
	}

	rowCount := 0
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		// Append to builders
		for i, col := range columns {
			if idx, ok := colToIdx[col]; ok {
				appendToBuilder(builder.Field(idx), values[i])
			}
		}
		rowCount++

		// Create batches of 10000 rows
		if rowCount >= 10000 {
			break
		}
	}

	if rowCount == 0 {
		return nil, nil
	}

	record := builder.NewRecord()
	record.Retain()
	return []arrow.Record{record}, nil
}

// GetStats returns table statistics across all volumes
func (t *DuckLakeTable) GetStats() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["name"] = t.name
	stats["query_count"] = t.queryCount
	stats["volumes_count"] = len(t.volumeFQNs)

	// Get row count from all volumes
	var totalCount int64
	volumeStats := make(map[string]int64)
	for _, vfqn := range t.volumeFQNs {
		var count int64
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", vfqn.FQN)
		if err := t.db.QueryRow(query).Scan(&count); err == nil {
			volumeStats[vfqn.VolumeName] = count
			totalCount += count
		}
	}
	stats["row_count"] = totalCount
	stats["volume_row_counts"] = volumeStats

	return stats
}

// duckDBTypeToArrow converts DuckDB type to Arrow type
func duckDBTypeToArrow(duckType string) arrow.DataType {
	duckType = strings.ToUpper(duckType)

	switch {
	case strings.HasPrefix(duckType, "VARCHAR"), strings.HasPrefix(duckType, "TEXT"):
		return arrow.BinaryTypes.String
	case strings.HasPrefix(duckType, "BIGINT"), strings.HasPrefix(duckType, "INT64"):
		return arrow.PrimitiveTypes.Int64
	case strings.HasPrefix(duckType, "INTEGER"), strings.HasPrefix(duckType, "INT32"), duckType == "INT":
		return arrow.PrimitiveTypes.Int32
	case strings.HasPrefix(duckType, "SMALLINT"), strings.HasPrefix(duckType, "INT16"):
		return arrow.PrimitiveTypes.Int16
	case strings.HasPrefix(duckType, "TINYINT"), strings.HasPrefix(duckType, "INT8"):
		return arrow.PrimitiveTypes.Int8
	case strings.HasPrefix(duckType, "UBIGINT"), strings.HasPrefix(duckType, "UINT64"):
		return arrow.PrimitiveTypes.Uint64
	case strings.HasPrefix(duckType, "UINTEGER"), strings.HasPrefix(duckType, "UINT32"):
		return arrow.PrimitiveTypes.Uint32
	case strings.HasPrefix(duckType, "USMALLINT"), strings.HasPrefix(duckType, "UINT16"):
		return arrow.PrimitiveTypes.Uint16
	case strings.HasPrefix(duckType, "UTINYINT"), strings.HasPrefix(duckType, "UINT8"):
		return arrow.PrimitiveTypes.Uint8
	case strings.HasPrefix(duckType, "DOUBLE"), strings.HasPrefix(duckType, "FLOAT8"):
		return arrow.PrimitiveTypes.Float64
	case strings.HasPrefix(duckType, "FLOAT"), strings.HasPrefix(duckType, "REAL"), strings.HasPrefix(duckType, "FLOAT4"):
		return arrow.PrimitiveTypes.Float32
	case strings.HasPrefix(duckType, "BOOLEAN"), strings.HasPrefix(duckType, "BOOL"):
		return arrow.FixedWidthTypes.Boolean
	// More specific TIMESTAMP_* prefixes must be checked before TIMESTAMP.
	case strings.HasPrefix(duckType, "TIMESTAMP_NS"):
		return arrow.FixedWidthTypes.Timestamp_ns
	case strings.HasPrefix(duckType, "TIMESTAMP_MS"):
		return arrow.FixedWidthTypes.Timestamp_ms
	case strings.HasPrefix(duckType, "TIMESTAMP_S"):
		return arrow.FixedWidthTypes.Timestamp_s
	case strings.HasPrefix(duckType, "TIMESTAMP"):
		return arrow.FixedWidthTypes.Timestamp_us
	case strings.HasPrefix(duckType, "DATE"):
		return arrow.FixedWidthTypes.Date32
	case strings.HasPrefix(duckType, "TIME"):
		return arrow.FixedWidthTypes.Time64us
	case strings.HasPrefix(duckType, "BLOB"), strings.HasPrefix(duckType, "BYTEA"):
		return arrow.BinaryTypes.Binary
	case strings.HasPrefix(duckType, "UUID"):
		return arrow.BinaryTypes.String // UUID as string
	default:
		return arrow.BinaryTypes.String // Default to string
	}
}

// appendToBuilder appends a value to an Arrow builder
func appendToBuilder(b array.Builder, v interface{}) {
	if v == nil {
		b.AppendNull()
		return
	}

	switch builder := b.(type) {
	case *array.StringBuilder:
		switch val := v.(type) {
		case string:
			builder.Append(val)
		case []byte:
			builder.Append(string(val))
		default:
			builder.Append(fmt.Sprintf("%v", v))
		}
	case *array.Int64Builder:
		switch val := v.(type) {
		case int64:
			builder.Append(val)
		case int:
			builder.Append(int64(val))
		case int32:
			builder.Append(int64(val))
		default:
			builder.AppendNull()
		}
	case *array.Int32Builder:
		switch val := v.(type) {
		case int32:
			builder.Append(val)
		case int:
			builder.Append(int32(val))
		case int64:
			builder.Append(int32(val))
		default:
			builder.AppendNull()
		}
	case *array.Int16Builder:
		switch val := v.(type) {
		case int16:
			builder.Append(val)
		case int:
			builder.Append(int16(val))
		default:
			builder.AppendNull()
		}
	case *array.Uint32Builder:
		switch val := v.(type) {
		case uint32:
			builder.Append(val)
		case uint:
			builder.Append(uint32(val))
		case int:
			builder.Append(uint32(val))
		case int64:
			builder.Append(uint32(val))
		default:
			builder.AppendNull()
		}
	case *array.Uint16Builder:
		switch val := v.(type) {
		case uint16:
			builder.Append(val)
		case int:
			builder.Append(uint16(val))
		default:
			builder.AppendNull()
		}
	case *array.Float64Builder:
		switch val := v.(type) {
		case float64:
			builder.Append(val)
		case float32:
			builder.Append(float64(val))
		default:
			builder.AppendNull()
		}
	case *array.BooleanBuilder:
		switch val := v.(type) {
		case bool:
			builder.Append(val)
		default:
			builder.AppendNull()
		}
	case *array.BinaryBuilder:
		switch val := v.(type) {
		case []byte:
			builder.Append(val)
		case string:
			builder.Append([]byte(val))
		default:
			builder.AppendNull()
		}
	default:
		b.AppendNull()
	}
}

// defaultHEPSchema returns the default HEP schema
func defaultHEPSchema() *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "uuid", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "timestamp", Type: arrow.PrimitiveTypes.Int64, Nullable: false},
		{Name: "session_id", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "caller", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "callee", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "src_ip", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "dst_ip", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "src_port", Type: arrow.PrimitiveTypes.Uint16, Nullable: true},
		{Name: "dst_port", Type: arrow.PrimitiveTypes.Uint16, Nullable: true},
		{Name: "event", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "proto_type", Type: arrow.PrimitiveTypes.Uint32, Nullable: false},
		{Name: "protocol", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "node_id", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "cid", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "payload", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "data_extra", Type: arrow.BinaryTypes.String, Nullable: true},
	}, nil)
}
