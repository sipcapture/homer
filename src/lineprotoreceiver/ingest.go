// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This file is adapted from hepic-lake/src/writer/lineproto_ingest.go
// (also AGPL-3.0-or-later).

package lineprotoreceiver

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
	"github.com/sipcapture/homer-core/src/utils/metrics"
)

// defaultTablePrefix is used when NewIngester is called with cfg == nil
// (tests). Matches the config default: empty string = measurement name
// is the DuckLake table name.
const defaultTablePrefix = ""

// Ingester writes parsed Line Protocol points into DuckLake. It is safe
// for concurrent use; per-table DDL is serialised internally.
//
// One instance is created per receiver Module and shared between the
// HTTP handlers. The underlying *sql.DB and lakeName come from the
// writer's primary DuckLake shard (see writer.GetDuckLakeManager).
type Ingester struct {
	db              *sql.DB
	lakeName        string
	tablePrefix     string
	allowHepSipCall bool

	tablesMu    sync.Mutex
	tablesState map[string]map[string]string // fqTable → col → SQL type

	schemasMu      sync.Mutex
	schemasCreated map[string]bool
}

// NewIngester returns a ready-to-use Ingester. lakeName is the DuckLake
// catalog identifier ("homer_lake" by default) used to qualify table
// names. cfg may be nil (uses defaultTablePrefix); if cfg is non-nil,
// cfg.TablePrefix is used verbatim (including empty string for
// measurement-as-table-name).
func NewIngester(db *sql.DB, lakeName string, cfg *config.LineProtoConfig) *Ingester {
	tp := defaultTablePrefix
	allow := false
	if cfg != nil {
		tp = cfg.TablePrefix
		allow = cfg.AllowHepSipCall
	}
	return &Ingester{
		db:              db,
		lakeName:        lakeName,
		tablePrefix:     tp,
		allowHepSipCall: allow,
		tablesState:     make(map[string]map[string]string),
		schemasCreated:  make(map[string]bool),
	}
}

// Ingest parses body as Line Protocol, lazily creates / extends the
// target tables, and INSERTs the resulting rows. dbName is optional —
// when non-empty it routes points into a per-database DuckDB schema
// (mirrors the InfluxDB v1 / gigapi `?db=…` semantics).
//
// Returns (rowsWritten, error). On a parse error, no rows are written.
//
// When allowHepSipCall is true and every parsed point uses the same
// supported HEP measurement name (hep_proto_* tables from the lake schema,
// e.g. hep_proto_1_call, hep_proto_1_registration), rows are written with
// the fixed HEP column mapping into {lake}.main.<measurement>.
// A mix of those lines with other measurements in one request is rejected.
// You cannot mix two different hep_proto_* table names in one request.
//
// With allowHepSipCall false and an empty table_prefix, any measurement
// starting with hep_proto_ is rejected so generic LP cannot corrupt HEP
// tables.
func (i *Ingester) Ingest(ctx context.Context, dbName string, body []byte, precision LineProtoPrecision) (int, error) {
	if i == nil || i.db == nil {
		return 0, fmt.Errorf("line-proto ingester: not initialised")
	}

	pts, badIdx, err := ParseLineProtocol(body, precision)
	if err != nil {
		metrics.RecordLineProtoWriteError("parse")
		return 0, fmt.Errorf("parse error at line %d: %w", badIdx+1, err)
	}
	if len(pts) == 0 {
		return 0, nil
	}

	var hepN, otherN int
	var hepMeas string
	var hepKey ducklake.TableKey
	for li, p := range pts {
		if k, ok := hepLPTableKeyForMeasurement(p.Measurement); ok && i.allowHepSipCall {
			if hepN == 0 {
				hepMeas = p.Measurement
				hepKey = k
			} else if p.Measurement != hepMeas {
				return 0, fmt.Errorf("line protocol: point %d: cannot mix different HEP tables (%q vs %q) in one request", li+1, hepMeas, p.Measurement)
			}
			hepN++
			continue
		}
		if strings.HasPrefix(p.Measurement, "hep_proto_") && i.tablePrefix == "" {
			if !i.allowHepSipCall {
				return 0, fmt.Errorf("line protocol: point %d: measurement %q is in the HEP table namespace; enable allow_hep_sip_call for structured hep_proto_* ingest or set table_prefix to namespace LP tables", li+1, p.Measurement)
			}
			return 0, fmt.Errorf("line protocol: point %d: unknown HEP table measurement %q", li+1, p.Measurement)
		}
		otherN++
	}

	if hepN > 0 && otherN > 0 {
		return 0, fmt.Errorf("line protocol: cannot mix structured hep_proto_* ingest (%q) with other measurements in one request", hepMeas)
	}
	if hepN > 0 {
		return i.ingestHepLPFromPoints(ctx, hepKey, pts)
	}

	schema := ""
	if dbName != "" {
		schema = SanitizeIdent(dbName)
		if err := i.ensureSchema(ctx, schema); err != nil {
			metrics.RecordLineProtoWriteError("ddl_schema")
			return 0, err
		}
	}

	grouped := i.groupByTable(pts, schema)
	total := 0
	for tbl, rows := range grouped {
		n, err := i.writeRows(ctx, tbl, rows)
		total += n
		if err != nil {
			return total, fmt.Errorf("write %s: %w", tbl, err)
		}
	}
	return total, nil
}

func (i *Ingester) ingestHepLPFromPoints(ctx context.Context, key ducklake.TableKey, pts []LineProtoPoint) (int, error) {
	sc := ducklake.GetTableSchemas()[key]
	tblLabel := "hep_proto"
	if sc != nil {
		tblLabel = "hep_proto_" + sc.TableSuffix
	}
	rows := make([][]interface{}, 0, len(pts))
	for li := range pts {
		row, err := lineProtoPointToHEPRow(&pts[li], key)
		if err != nil {
			metrics.RecordLineProtoWriteError("hep_lp_map")
			return 0, fmt.Errorf("point %d: %w", li+1, err)
		}
		rows = append(rows, row)
	}
	sql, err := ducklake.BuildInsertMultiValues(i.lakeName, key, rows)
	if err != nil {
		return 0, err
	}
	if _, err := i.db.ExecContext(ctx, sql); err != nil {
		metrics.RecordLineProtoWriteError("hep_lp_write")
		return 0, fmt.Errorf("write %s: %w", tblLabel, err)
	}
	return len(rows), nil
}

// ensureSchema creates "<lakeName>.<schema>" on first use and memoises
// it. Serialised via schemasMu so concurrent writers don't race the DDL.
func (i *Ingester) ensureSchema(ctx context.Context, schema string) error {
	key := i.lakeName + "." + schema
	i.schemasMu.Lock()
	defer i.schemasMu.Unlock()
	if i.schemasCreated[key] {
		return nil
	}
	ddl := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s.%s", i.lakeName, schema)
	if _, err := i.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("create schema %s: %w", key, err)
	}
	i.schemasCreated[key] = true
	return nil
}

// groupByTable maps each point to its fully-qualified table name and
// produces a list of rows ready for insertion.
func (i *Ingester) groupByTable(pts []LineProtoPoint, schema string) map[string][]map[string]interface{} {
	out := make(map[string][]map[string]interface{}, 4)
	nowNs := time.Now().UnixNano()
	for _, p := range pts {
		tblName := i.tablePrefix + SanitizeIdent(p.Measurement)
		var fq string
		if schema != "" {
			fq = i.lakeName + "." + schema + "." + tblName
		} else {
			fq = i.lakeName + "." + tblName
		}

		row := make(map[string]interface{}, len(p.Tags)+len(p.Fields)+1)
		for k, v := range p.Tags {
			row[SanitizeIdent(k)] = v
		}
		// Fields may override a same-named tag — follow InfluxDB
		// behaviour where fields and tags share the flat namespace.
		for k, v := range p.Fields {
			row[SanitizeIdent(k)] = v
		}
		tsNs := p.TimestampNs
		if tsNs == 0 {
			tsNs = nowNs
		}
		row["ts"] = time.Unix(0, tsNs).UTC().Format(time.RFC3339Nano)

		out[fq] = append(out[fq], row)
	}
	return out
}

// writeRows ensures the table exists with all required columns, then
// INSERTs every row using prepared statements grouped by column
// signature. Returns the count of rows successfully inserted.
func (i *Ingester) writeRows(ctx context.Context, fqTable string, rows []map[string]interface{}) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}

	colTypes := make(map[string]string, 8)
	for _, r := range rows {
		for k, v := range r {
			t := colType(k, v)
			if existing, ok := colTypes[k]; ok {
				// Widen INT → DOUBLE if a later row in this batch
				// holds a float for the same column.
				if existing == "BIGINT" && t == "DOUBLE" {
					colTypes[k] = "DOUBLE"
				}
				continue
			}
			colTypes[k] = t
		}
	}

	if err := i.ensureTable(ctx, fqTable, colTypes); err != nil {
		return 0, err
	}

	// Bucket rows by their column signature so we emit one prepared
	// INSERT per distinct subset (most batches end up in a single
	// bucket; this only matters when measurements share a table but
	// have heterogeneous tag/field sets).
	buckets := make(map[string][]map[string]interface{}, 1)
	for _, r := range rows {
		buckets[colSignature(r)] = append(buckets[colSignature(r)], r)
	}

	inserted := 0
	for _, bucket := range buckets {
		if len(bucket) == 0 {
			continue
		}
		cols := sortedMapKeys(bucket[0])
		n, err := i.insertBucket(ctx, fqTable, cols, bucket)
		inserted += n
		if err != nil {
			return inserted, err
		}
	}
	return inserted, nil
}

// lpInsertChunkRows bounds how many rows go into a single multi-row INSERT
// so a large batch doesn't blow past DuckDB's bound-parameter / statement
// size limits. Chosen well below any practical limit while still collapsing
// thousands of per-row commits into a handful of statements.
const lpInsertChunkRows = 500

// insertBucket writes a homogeneous set of rows (same column subset) using
// chunked multi-row `INSERT ... VALUES (...),(...),...` statements instead of
// one Exec per row.
//
// Each per-row Exec used to be its own DuckLake transaction — a catalog
// snapshot plus a tiny Parquet (or inlined) write per row. Under sustained
// Line Protocol traffic that micro-commit storm bloats the catalog and stalls
// ingest (the same pattern fixed for OTLP/Python in the sibling ingest
// service). Bulk inserts cut the transaction/snapshot count by up to
// lpInsertChunkRows×.
func (i *Ingester) insertBucket(ctx context.Context, fqTable string, cols []string, bucket []map[string]interface{}) (int, error) {
	colList := strings.Join(cols, ", ")
	// "(?, ?, ... ?)" for one row.
	rowPlaceholder := "(" + strings.TrimSuffix(strings.Repeat("?, ", len(cols)), ", ") + ")"

	inserted := 0
	for start := 0; start < len(bucket); start += lpInsertChunkRows {
		end := start + lpInsertChunkRows
		if end > len(bucket) {
			end = len(bucket)
		}
		chunk := bucket[start:end]

		placeholders := make([]string, len(chunk))
		args := make([]interface{}, 0, len(chunk)*len(cols))
		for k, r := range chunk {
			placeholders[k] = rowPlaceholder
			for _, c := range cols {
				args = append(args, r[c])
			}
		}
		stmtSQL := fmt.Sprintf(
			"INSERT INTO %s (%s) VALUES %s",
			fqTable,
			colList,
			strings.Join(placeholders, ", "),
		)
		if _, err := i.db.ExecContext(ctx, stmtSQL, args...); err != nil {
			metrics.RecordLineProtoWriteError("insert")
			return inserted, fmt.Errorf("bulk insert: %w", err)
		}
		inserted += len(chunk)
	}
	return inserted, nil
}

// ensureTable creates the table on first use and ALTER TABLE ADD COLUMNs
// any missing columns seen later. Column types are locked on first
// insertion; we do not narrow in place.
func (i *Ingester) ensureTable(ctx context.Context, fqTable string, cols map[string]string) error {
	i.tablesMu.Lock()
	defer i.tablesMu.Unlock()

	state, exists := i.tablesState[fqTable]
	if !exists {
		// `ts` is always TIMESTAMP, regardless of what the caller put
		// in the row (groupByTable always fills it in).
		if _, ok := cols["ts"]; !ok {
			cols["ts"] = "TIMESTAMP"
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "CREATE TABLE IF NOT EXISTS %s (", fqTable)
		names := sortedStringMapKeys(cols)
		for j, n := range names {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(n)
			sb.WriteByte(' ')
			sb.WriteString(cols[n])
		}
		sb.WriteByte(')')
		if _, err := i.db.ExecContext(ctx, sb.String()); err != nil {
			metrics.RecordLineProtoWriteError("ddl_create")
			return fmt.Errorf("create table: %w", err)
		}
		state = make(map[string]string, len(cols))
		for n, t := range cols {
			state[n] = t
		}
		i.tablesState[fqTable] = state
		return nil
	}

	for n, t := range cols {
		if _, ok := state[n]; ok {
			continue
		}
		ddl := fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s", fqTable, n, t)
		if _, err := i.db.ExecContext(ctx, ddl); err != nil {
			metrics.RecordLineProtoWriteError("ddl_alter")
			return fmt.Errorf("alter add column %s: %w", n, err)
		}
		state[n] = t
	}
	return nil
}

// colType picks a DuckDB type for a given column value. "ts" and columns
// ending in "_ts" / "_at" are always TIMESTAMP. Other types are mapped
// from the Go representation produced by parseFieldValue.
func colType(col string, v interface{}) string {
	if col == "ts" || strings.HasSuffix(col, "_ts") || strings.HasSuffix(col, "_at") {
		return "TIMESTAMP"
	}
	switch v.(type) {
	case bool:
		return "BOOLEAN"
	case int64, int32, int, uint64, uint32, uint:
		return "BIGINT"
	case float64, float32:
		return "DOUBLE"
	default:
		return "VARCHAR"
	}
}

func colSignature(row map[string]interface{}) string {
	keys := sortedMapKeys(row)
	return strings.Join(keys, "|")
}

func sortedMapKeys(row map[string]interface{}) []string {
	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// SanitizeIdent returns a DuckDB-safe identifier by replacing any
// character that is not ASCII alphanumeric or underscore with '_'.
// Identifiers beginning with a digit are prefixed with '_'.
func SanitizeIdent(name string) string {
	if name == "" {
		return "_"
	}
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	s := b.String()
	if len(s) == 0 || (s[0] >= '0' && s[0] <= '9') {
		return "_" + s
	}
	return s
}

// resetForTests clears the memoised per-process DDL state. Tests use
// this to start with a clean slate between in-memory DuckDB instances.
func (i *Ingester) resetForTests() {
	i.tablesMu.Lock()
	i.tablesState = make(map[string]map[string]string)
	i.tablesMu.Unlock()
	i.schemasMu.Lock()
	i.schemasCreated = make(map[string]bool)
	i.schemasMu.Unlock()
}
