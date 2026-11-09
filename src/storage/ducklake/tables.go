// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	duckdb "github.com/duckdb/duckdb-go/v2"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"github.com/sipcapture/homer-core/src/utils/metrics"
)

// flushJob carries the buffer-slot index that needs to be flushed to DuckLake.
type flushJob struct {
	slotIdx int // 0 or 1 — which memTable to flush
}

// ProtoType constants for HEP protocols
const (
	ProtoTypeSIP      uint32 = 1
	ProtoTypeRTCPJSON uint32 = 5
	ProtoTypeRTCP     uint32 = 34
	ProtoTypeRTP      uint32 = 35
	ProtoTypeDNS      uint32 = 53
	ProtoTypeLOG      uint32 = 100
)

// SIPType constants for SIP message categories
const (
	SIPTypeCall         = "call"         // INVITE, ACK, PRACK, UPDATE, BYE, CANCEL, INFO, REFER
	SIPTypeRegistration = "registration" // REGISTER
	SIPTypeDefault      = "default"      // OPTIONS, NOTIFY, SUBSCRIBE, PUBLISH, MESSAGE
	SIPTypeSiprec       = "siprec"       // SIPREC SRS signaling captured in Homer
)

// TableKey uniquely identifies a table (proto_type + optional sub_type)
type TableKey struct {
	ProtoType uint32
	SubType   string // empty for non-SIP, "call"/"registration"/"default" for SIP
}

// String returns string representation of TableKey
func (k TableKey) String() string {
	if k.SubType != "" {
		return fmt.Sprintf("%d_%s", k.ProtoType, k.SubType)
	}
	return fmt.Sprintf("%d_default", k.ProtoType)
}

// TableSchema defines schema for a specific table
type TableSchema struct {
	ProtoType   uint32
	SubType     string // for SIP sub-types
	TableSuffix string
	CreateSQL   string
	InsertSQL   string
	Columns     []string
}

// GetSIPMethod returns the effective SIP method for routing
// For requests: returns FirstMethod
// For responses: returns CseqMethod (from CSeq header)
func GetSIPMethod(firstMethod, cseqMethod, firstResp string) string {
	// If it's a response (has response code), use CSeq method
	if firstResp != "" && cseqMethod != "" {
		return cseqMethod
	}
	// Otherwise use the request method
	return firstMethod
}

// GetSIPType returns the SIP sub-type based on method
func GetSIPType(method string) string {
	switch method {
	case "INVITE", "ACK", "PRACK", "UPDATE", "BYE", "CANCEL", "INFO", "REFER":
		return SIPTypeCall
	case "REGISTER":
		return SIPTypeRegistration
	default:
		// OPTIONS, NOTIFY, SUBSCRIBE, PUBLISH, MESSAGE, etc.
		return SIPTypeDefault
	}
}

// GetTableSchemas returns schemas for all supported table types
func GetTableSchemas() map[TableKey]*TableSchema {
	return map[TableKey]*TableSchema{
		// SIP Call - INVITE, ACK, PRACK, UPDATE, BYE, CANCEL, INFO, REFER
		{ProtoType: ProtoTypeSIP, SubType: SIPTypeCall}: {
			ProtoType:   ProtoTypeSIP,
			SubType:     SIPTypeCall,
			TableSuffix: "1_call",
			Columns: []string{
				"uuid", "timestamp", "session_id", "caller", "callee",
				"src_ip", "dst_ip", "src_port", "dst_port",
				"method", "response_code", "cseq_method",
				"protocol", "node_id", "cid", "payload", "data_extra", "isup_body",
			},
			CreateSQL: `
				uuid VARCHAR,
				date DATE,
				timestamp TIMESTAMP,
				session_id VARCHAR,
				caller VARCHAR,
				callee VARCHAR,
				src_ip VARCHAR,
				dst_ip VARCHAR,
				src_port UINTEGER,
				dst_port UINTEGER,
				method VARCHAR,
				response_code VARCHAR,
				cseq_method VARCHAR,
				protocol UINTEGER,
				node_id VARCHAR,
				cid VARCHAR,
				payload VARCHAR,
				data_extra JSON,
				isup_body BLOB
			`,
			InsertSQL: `(uuid, date, timestamp, session_id, caller, callee, src_ip, dst_ip,
				src_port, dst_port, method, response_code, cseq_method,
				protocol, node_id, cid, payload, data_extra, isup_body)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?::JSON, ?)`,
		},

		// SIP Registration - REGISTER
		{ProtoType: ProtoTypeSIP, SubType: SIPTypeRegistration}: {
			ProtoType:   ProtoTypeSIP,
			SubType:     SIPTypeRegistration,
			TableSuffix: "1_registration",
			Columns: []string{
				"uuid", "timestamp", "session_id",
				"aor", "contact", "expires", "user_agent",
				"src_ip", "dst_ip", "src_port", "dst_port",
				"method", "response_code",
				"protocol", "node_id", "payload", "data_extra", "isup_body",
			},
			CreateSQL: `
				uuid VARCHAR,
				date DATE,
				timestamp TIMESTAMP,
				session_id VARCHAR,
				aor VARCHAR,
				contact VARCHAR,
				expires VARCHAR,
				user_agent VARCHAR,
				src_ip VARCHAR,
				dst_ip VARCHAR,
				src_port UINTEGER,
				dst_port UINTEGER,
				method VARCHAR,
				response_code VARCHAR,
				protocol UINTEGER,
				node_id VARCHAR,
				payload VARCHAR,
				data_extra JSON,
				isup_body BLOB
			`,
			InsertSQL: `(uuid, date, timestamp, session_id, aor, contact, expires, user_agent,
				src_ip, dst_ip, src_port, dst_port, method, response_code,
				protocol, node_id, payload, data_extra, isup_body)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?::JSON, ?)`,
		},

		// SIPREC - signaling from in-process SRS
		{ProtoType: ProtoTypeSIP, SubType: SIPTypeSiprec}: {
			ProtoType:   ProtoTypeSIP,
			SubType:     SIPTypeSiprec,
			TableSuffix: "1_siprec",
			Columns: []string{
				"uuid", "timestamp", "session_id", "caller", "callee",
				"src_ip", "dst_ip", "src_port", "dst_port",
				"method", "response_code", "cseq_method",
				"protocol", "node_id", "cid", "payload", "data_extra", "isup_body",
			},
			CreateSQL: `
				uuid VARCHAR,
				date DATE,
				timestamp TIMESTAMP,
				session_id VARCHAR,
				caller VARCHAR,
				callee VARCHAR,
				src_ip VARCHAR,
				dst_ip VARCHAR,
				src_port UINTEGER,
				dst_port UINTEGER,
				method VARCHAR,
				response_code VARCHAR,
				cseq_method VARCHAR,
				protocol UINTEGER,
				node_id VARCHAR,
				cid VARCHAR,
				payload VARCHAR,
				data_extra JSON,
				isup_body BLOB
			`,
			InsertSQL: `(uuid, date, timestamp, session_id, caller, callee, src_ip, dst_ip,
				src_port, dst_port, method, response_code, cseq_method,
				protocol, node_id, cid, payload, data_extra, isup_body)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?::JSON, ?)`,
		},

		// SIP Default - OPTIONS, NOTIFY, SUBSCRIBE, PUBLISH, MESSAGE
		{ProtoType: ProtoTypeSIP, SubType: SIPTypeDefault}: {
			ProtoType:   ProtoTypeSIP,
			SubType:     SIPTypeDefault,
			TableSuffix: "1_default",
			Columns: []string{
				"uuid", "timestamp", "session_id",
				"src_ip", "dst_ip", "src_port", "dst_port",
				"method", "response_code",
				"protocol", "node_id", "cid", "payload", "data_extra", "isup_body",
			},
			CreateSQL: `
				uuid VARCHAR,
				date DATE,
				timestamp TIMESTAMP,
				session_id VARCHAR,
				src_ip VARCHAR,
				dst_ip VARCHAR,
				src_port UINTEGER,
				dst_port UINTEGER,
				method VARCHAR,
				response_code VARCHAR,
				protocol UINTEGER,
				node_id VARCHAR,
				cid VARCHAR,
				payload VARCHAR,
				data_extra JSON,
				isup_body BLOB
			`,
			InsertSQL: `(uuid, date, timestamp, session_id, src_ip, dst_ip,
				src_port, dst_port, method, response_code,
				protocol, node_id, cid, payload, data_extra, isup_body)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?::JSON, ?)`,
		},

		// RTCP JSON - reports with stats
		{ProtoType: ProtoTypeRTCPJSON}: {
			ProtoType:   ProtoTypeRTCPJSON,
			TableSuffix: "5_default",
			Columns: []string{
				"uuid", "timestamp", "session_id",
				"src_ip", "dst_ip", "src_port", "dst_port",
				"protocol", "node_id", "cid", "payload", "data_extra",
			},
			CreateSQL: `
				uuid VARCHAR,
				date DATE,
				timestamp TIMESTAMP,
				session_id VARCHAR,
				src_ip VARCHAR,
				dst_ip VARCHAR,
				src_port UINTEGER,
				dst_port UINTEGER,
				protocol UINTEGER,
				node_id VARCHAR,
				cid VARCHAR,
				payload VARCHAR,
				data_extra JSON
			`,
			InsertSQL: `(uuid, date, timestamp, session_id, src_ip, dst_ip,
				src_port, dst_port, protocol, node_id, cid, payload, data_extra)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?::JSON)`,
		},

		// RTCP binary
		{ProtoType: ProtoTypeRTCP}: {
			ProtoType:   ProtoTypeRTCP,
			TableSuffix: "34_default",
			Columns: []string{
				"uuid", "timestamp", "session_id",
				"src_ip", "dst_ip", "src_port", "dst_port",
				"protocol", "node_id", "cid", "payload", "data_extra",
			},
			CreateSQL: `
				uuid VARCHAR,
				date DATE,
				timestamp TIMESTAMP,
				session_id VARCHAR,
				src_ip VARCHAR,
				dst_ip VARCHAR,
				src_port UINTEGER,
				dst_port UINTEGER,
				protocol UINTEGER,
				node_id VARCHAR,
				cid VARCHAR,
				payload VARCHAR,
				data_extra JSON
			`,
			InsertSQL: `(uuid, date, timestamp, session_id, src_ip, dst_ip,
				src_port, dst_port, protocol, node_id, cid, payload, data_extra)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?::JSON)`,
		},

		// RTP
		{ProtoType: ProtoTypeRTP}: {
			ProtoType:   ProtoTypeRTP,
			TableSuffix: "35_default",
			Columns: []string{
				"uuid", "timestamp", "session_id",
				"src_ip", "dst_ip", "src_port", "dst_port",
				"protocol", "node_id", "cid", "payload", "data_extra",
			},
			CreateSQL: `
				uuid VARCHAR,
				date DATE,
				timestamp TIMESTAMP,
				session_id VARCHAR,
				src_ip VARCHAR,
				dst_ip VARCHAR,
				src_port UINTEGER,
				dst_port UINTEGER,
				protocol UINTEGER,
				node_id VARCHAR,
				cid VARCHAR,
				payload VARCHAR,
				data_extra JSON
			`,
			InsertSQL: `(uuid, date, timestamp, session_id, src_ip, dst_ip,
				src_port, dst_port, protocol, node_id, cid, payload, data_extra)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?::JSON)`,
		},

		// DNS
		{ProtoType: ProtoTypeDNS}: {
			ProtoType:   ProtoTypeDNS,
			TableSuffix: "53_default",
			Columns: []string{
				"uuid", "timestamp",
				"src_ip", "dst_ip", "src_port", "dst_port",
				"protocol", "node_id", "payload", "data_extra",
			},
			CreateSQL: `
				uuid VARCHAR,
				date DATE,
				timestamp TIMESTAMP,
				src_ip VARCHAR,
				dst_ip VARCHAR,
				src_port UINTEGER,
				dst_port UINTEGER,
				protocol UINTEGER,
				node_id VARCHAR,
				payload VARCHAR,
				data_extra JSON
			`,
			InsertSQL: `(uuid, date, timestamp, src_ip, dst_ip,
				src_port, dst_port, protocol, node_id, payload, data_extra)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?::JSON)`,
		},

		// LOG
		{ProtoType: ProtoTypeLOG}: {
			ProtoType:   ProtoTypeLOG,
			TableSuffix: "100_default",
			Columns: []string{
				"uuid", "timestamp", "session_id",
				"src_ip", "dst_ip", "node_id", "payload", "data_extra",
			},
			CreateSQL: `
				uuid VARCHAR,
				date DATE,
				timestamp TIMESTAMP,
				session_id VARCHAR,
				src_ip VARCHAR,
				dst_ip VARCHAR,
				node_id VARCHAR,
				payload VARCHAR,
				data_extra JSON
			`,
			InsertSQL: `(uuid, date, timestamp, session_id, src_ip, dst_ip, node_id, payload, data_extra)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?::JSON)`,
		},
	}
}

// GetDefaultSchema returns schema for unknown proto_types
func GetDefaultSchema(key TableKey) *TableSchema {
	return &TableSchema{
		ProtoType:   key.ProtoType,
		SubType:     key.SubType,
		TableSuffix: key.String(),
		Columns: []string{
			"uuid", "date", "timestamp", "session_id",
			"src_ip", "dst_ip", "src_port", "dst_port",
			"protocol", "node_id", "cid", "payload", "data_extra",
		},
		CreateSQL: `
			uuid VARCHAR,
			date DATE,
			timestamp TIMESTAMP,
			session_id VARCHAR,
			src_ip VARCHAR,
			dst_ip VARCHAR,
			src_port UINTEGER,
			dst_port UINTEGER,
			protocol UINTEGER,
			node_id VARCHAR,
			cid VARCHAR,
			payload VARCHAR,
			data_extra JSON
		`,
		InsertSQL: `(uuid, date, timestamp, session_id, src_ip, dst_ip,
			src_port, dst_port, protocol, node_id, cid, payload, data_extra)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?::JSON)`,
	}
}

// rowValuesPool recycles []interface{} row slices used in convertHEPToValues.
// The pool stores pointers to slices with capacity up to maxRowCols (18 columns).
// Lifecycle: alloc in build*Values → stored in tw.batch → flushBatch copies to
// DuckDB Appender → flushBatch returns slice to pool after clearing references.
const maxRowCols = 20

var rowValuesPool = sync.Pool{New: func() interface{} {
	s := make([]interface{}, 0, maxRowCols)
	return &s
}}

// getRowSlice returns a recycled or fresh []interface{} with len=n.
func getRowSlice(n int) []interface{} {
	p := rowValuesPool.Get().(*[]interface{})
	if cap(*p) >= n {
		*p = (*p)[:n]
		return *p
	}
	rowValuesPool.Put(p) // wrong size, discard
	return make([]interface{}, n)
}

// putRowSlice returns a row slice to the pool, clearing all references.
func putRowSlice(row []interface{}) {
	for i := range row {
		releaseExtraJSONCell(row[i])
		row[i] = nil
	}
	p := row[:0]
	rowValuesPool.Put(&p)
}

// TableWriter handles writes to a specific table using Go-side batch buffering,
// double-buffered DuckDB in-memory tables, and a dedicated flush goroutine.
//
// Architecture:
//   - Incoming records accumulate in a Go slice (batch). When the batch reaches
//     batchSize it is flushed to the *active* in-memory DuckDB table via the
//     DuckDB Appender API (bypasses SQL parsing entirely).
//   - Periodically the active and standby memory tables are swapped atomically.
//     The now-standby table (full of data) is sent to the flush goroutine which
//     copies rows to DuckLake and truncates the standby table under its
//     write lock. Appends only wait while that table is actually being flushed,
//     not while its flush job waits for the catalog lock.
//   - Catalog contention is handled with exponential-backoff retry.
type TableWriter struct {
	db       *sql.DB
	tableFQN string // DuckLake table: "homer_lake.hep_proto_1_call"
	schema   *TableSchema

	// Double-buffer: two memory tables, swapped atomically.
	// activeIdx 0 → memTables[0] is active (receives writes), memTables[1] is standby.
	memTables [2]string
	activeIdx atomic.Int32
	// Each table permits one appender. Appends and INSERT/TRUNCATE flushes
	// hold the exclusive slot lock; queued jobs take the catalog lock first.
	memMu [2]sync.Mutex
	// Protected by memMu. Committed rows must be cleared before another INSERT
	// or append, including after a failed TRUNCATE.
	slotCommitted [2]bool

	batchMu   sync.Mutex
	batch     [][]interface{}
	batchSize int

	// Pre-built flush/truncate SQL per buffer slot.
	flushInsertSQL   [2]string // "INSERT INTO <lakeFQN> SELECT * FROM <memTable_N>"
	flushTruncateSQL [2]string // "TRUNCATE TABLE <memTable_N>"

	// Flush queue: dedicated goroutine drains this channel.
	flushCh   chan flushJob
	flushWg   sync.WaitGroup
	catalogMu *sync.Mutex // shared catalog lock from MultiTableWriter

	// Notify the writer when a fatal flush invalidates its DuckDB instance.
	onFatalFlush func(error)
}

// NewTableWriter creates a new table writer with a DuckLake table and
// two in-memory buffer tables (double-buffer) and a dedicated flush goroutine.
// catalogMu is the shared lock from MultiTableWriter that serializes
// catalog-modifying operations (flush to DuckLake, compaction).
func NewTableWriter(db *sql.DB, lakeName string, schema *TableSchema, batchSize int, catalogMu *sync.Mutex) (*TableWriter, error) {
	tableFQN := fmt.Sprintf("%s.hep_proto_%s", lakeName, schema.TableSuffix)
	memA := fmt.Sprintf("mem_hep_proto_%s_a", schema.TableSuffix)
	memB := fmt.Sprintf("mem_hep_proto_%s_b", schema.TableSuffix)

	if batchSize <= 0 {
		batchSize = 5000
	}

	tw := &TableWriter{
		db:        db,
		tableFQN:  tableFQN,
		memTables: [2]string{memA, memB},
		schema:    schema,
		batch:     make([][]interface{}, 0, batchSize),
		batchSize: batchSize,
		flushCh:   make(chan flushJob, 2),
		catalogMu: catalogMu,
	}

	for i := 0; i < 2; i++ {
		tw.flushInsertSQL[i] = "INSERT INTO " + tableFQN + " SELECT * FROM " + tw.memTables[i]
		tw.flushTruncateSQL[i] = "TRUNCATE TABLE " + tw.memTables[i]
	}

	// Did the table already exist before this CREATE? SET PARTITIONED BY /
	// SET SORTED BY are DDL that bump the DuckLake schema_version every time
	// they run, and each new schema_version spawns another (leaked)
	// ducklake_inlined_data_* table (upstream duckdb/ducklake#1065). Re-issuing
	// them on every restart is pure churn, so only configure a freshly created
	// table.
	tableName := fmt.Sprintf("hep_proto_%s", schema.TableSuffix)
	alreadyExisted := duckLakeTableExists(db, lakeName, tableName)

	// Create DuckLake persistent table
	createSQL := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s);", tableFQN, schema.CreateSQL)
	if _, err := db.Exec(createSQL); err != nil {
		return nil, fmt.Errorf("failed to create table %s: %w", tableFQN, err)
	}

	if !alreadyExisted {
		// Set partitioning by date for efficient time-range queries
		partitionSQL := fmt.Sprintf("ALTER TABLE %s SET PARTITIONED BY (date);", tableFQN)
		if _, err := db.Exec(partitionSQL); err != nil {
			logger.Warn(fmt.Sprintf("Failed to set partitioning for %s: %v", tableFQN, err))
		}

		// Sort rows by timestamp within each file (DuckLake v1.0).
		sortSQL := fmt.Sprintf("ALTER TABLE %s SET SORTED BY (timestamp ASC);", tableFQN)
		if _, err := db.Exec(sortSQL); err != nil {
			logger.Warn(fmt.Sprintf("Failed to set sort order for %s: %v", tableFQN, err))
		}
	}

	// Create both in-memory buffer tables (plain DuckDB, not DuckLake)
	for _, mem := range tw.memTables {
		memCreateSQL := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s);", mem, schema.CreateSQL)
		if _, err := db.Exec(memCreateSQL); err != nil {
			return nil, fmt.Errorf("failed to create memory table %s: %w", mem, err)
		}
	}

	// Existing lakes were created before isup_body. ADD COLUMN leaves old
	// Parquet files readable: the new column is NULL on those rows.
	if strings.Contains(schema.CreateSQL, "isup_body") {
		for _, table := range append([]string{tableFQN}, tw.memTables[:]...) {
			alter := fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS isup_body BLOB", table)
			if _, err := db.Exec(alter); err != nil {
				return nil, fmt.Errorf("add isup_body to %s: %w", table, err)
			}
		}
	}

	// Start dedicated flush goroutine
	tw.flushWg.Add(1)
	go tw.flushWorker()

	logger.Info("DuckLake table ready (double-buffer, appender)", "table", tableFQN,
		"buf_a", memA, "buf_b", memB, "batch_size", batchSize)
	return tw, nil
}

// activeMemTable returns the name of the currently active memory table.
func (tw *TableWriter) activeMemTable() string {
	return tw.memTables[tw.activeIdx.Load()]
}

// Write buffers a record. When the batch reaches batchSize, it is flushed
// to the in-memory DuckDB table via a single multi-row INSERT.
func (tw *TableWriter) Write(values []interface{}) error {
	tw.batchMu.Lock()
	tw.batch = append(tw.batch, values)
	needFlush := len(tw.batch) >= tw.batchSize
	tw.batchMu.Unlock()

	if needFlush {
		return tw.flushBatch()
	}
	return nil
}

// driverValPool recycles []driver.Value slices to avoid per-row allocation in flushBatch.
var driverValPool sync.Pool // elements: *[]driver.Value

// batchSlicePool recycles the outer [][]interface{} slices swapped out in flushBatch.
var batchSlicePool sync.Pool // elements: *[][]interface{}

// flushBatch drains the Go-side batch into the currently active in-memory
// DuckDB table using the Appender API. This bypasses SQL parsing entirely —
// rows are written directly via DuckDB's columnar DataChunk interface.
// batchMu only protects the batch handoff. The active memory table is held
// exclusively until the appender commits; waiting leaves batchMu free.
func (tw *TableWriter) flushBatch() error {
	tw.batchMu.Lock()
	if len(tw.batch) == 0 {
		tw.batchMu.Unlock()
		return nil
	}
	// Swap batch with a recycled slice to avoid allocation on the hot path.
	// We get a recycled slice from the pool, assign it to tw.batch, and later
	// return the flushed rows (after clearing) to the pool. This keeps the pool
	// size stable without leaking the pooled pointer.
	rows := tw.batch
	tw.batch = make([][]interface{}, 0, tw.batchSize)
	if p := batchSlicePool.Get(); p != nil {
		recycled := p.(*[][]interface{})
		if cap(*recycled) >= tw.batchSize {
			tw.batch = (*recycled)[:0]
		}
	}

	tw.batchMu.Unlock()

	committed := false
	defer func() {
		if !committed {
			tw.batchMu.Lock()
			tw.batch = append(rows, tw.batch...)
			tw.batchMu.Unlock()
			return
		}
		// Only successful batches may return their rows to the pools.
		for _, row := range rows {
			putRowSlice(row)
		}
		clear(rows)
		batchSlicePool.Put(&rows)
	}()

	slot, err := tw.lockActiveMem()
	if err != nil {
		return err
	}
	defer tw.memMu[slot].Unlock()

	start := time.Now()
	nRows := len(rows)
	memTable := tw.memTables[slot]

	// Grab a reusable vals slice sized for this table's column count.
	nCols := len(rows[0])
	var vals []driver.Value
	if p := driverValPool.Get(); p != nil {
		v := p.(*[]driver.Value)
		if cap(*v) >= nCols {
			vals = (*v)[:nCols]
		} else {
			vals = make([]driver.Value, nCols)
		}
	} else {
		vals = make([]driver.Value, nCols)
	}

	defer driverValPool.Put(&vals)
	ctx := context.Background()
	sqlConn, err := tw.db.Conn(ctx)
	if err != nil {
		metrics.RecordPipelineStageError("ducklake", "batch_insert", "conn_error")
		return fmt.Errorf("appender conn for %s: %w", memTable, err)
	}
	defer sqlConn.Close()
	// Appender.Close can flush rows even after an earlier AppendRow error.
	// An explicit transaction makes the whole batch retryable without copies
	// of partially appended rows remaining in the memory table.
	tx, err := sqlConn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin append into %s: %w", memTable, err)
	}
	defer tx.Rollback()

	err = sqlConn.Raw(func(driverConn interface{}) error {
		dc, ok := driverConn.(driver.Conn)
		if !ok {
			return fmt.Errorf("raw conn does not implement driver.Conn (got %T)", driverConn)
		}
		appender, appErr := duckdb.NewAppenderFromConn(dc, "", memTable)
		if appErr != nil {
			return appErr
		}
		for _, row := range rows {
			for i, v := range row {
				vals[i] = cellToDriverValue(v)
			}
			if appErr = appender.AppendRow(vals...); appErr != nil {
				appender.Close()
				return appErr
			}
		}
		return appender.Close()
	})

	if err != nil {
		metrics.RecordPipelineStageError("ducklake", "batch_insert", "insert_error")
		return fmt.Errorf("appender INSERT into %s (%d rows): %w", memTable, nRows, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit append into %s: %w", memTable, err)
	}
	committed = true
	metrics.RecordPipelineStageDuration("ducklake", "batch_insert", time.Since(start).Seconds())

	return nil
}

// lockActiveMem returns an exclusively locked active slot, redirecting a
// waiter if a swap changed the active table. Committed rows must be cleared
// before admitting new rows, including after an earlier failed TRUNCATE.
func (tw *TableWriter) lockActiveMem() (int, error) {
	for {
		slot := int(tw.activeIdx.Load())
		tw.memMu[slot].Lock()
		if int(tw.activeIdx.Load()) != slot {
			tw.memMu[slot].Unlock()
			continue
		}
		if tw.slotCommitted[slot] {
			if err := tw.clearSlot(slot); err != nil {
				tw.memMu[slot].Unlock()
				return 0, err
			}
		}
		return slot, nil
	}
}

// Close flushes remaining records and stops the flush goroutine.
func (tw *TableWriter) Close() error {
	closeErr := tw.flushBatch()
	close(tw.flushCh)
	tw.flushWg.Wait()
	// A queued flush may have cleared committed rows that blocked the first
	// append. Retry any retained Go batch before draining both memory tables.
	if closeErr != nil {
		closeErr = errors.Join(closeErr, tw.flushBatch())
	}
	for slot := range tw.memTables {
		closeErr = errors.Join(closeErr, tw.flushSlotDirect(slot, tw.catalogMu))
	}
	return closeErr
}

// swapBuffer drains the Go batch and swaps slots without reserving the queued
// slot. Appenders can reuse it until the flush actually obtains its write lock.
func (tw *TableWriter) swapBuffer() int {
	if err := tw.flushBatch(); err != nil {
		logger.Warn(fmt.Sprintf("Failed to drain batch before swap for %s: %v", tw.tableFQN, err))
	}
	tw.batchMu.Lock()
	defer tw.batchMu.Unlock()
	old := int(tw.activeIdx.Load())
	tw.activeIdx.Store(int32(1 - old))
	return old
}

func (tw *TableWriter) SwapAndFlush() {
	tw.flushCh <- flushJob{slotIdx: tw.swapBuffer()}
}

// Flush is a synchronous convenience wrapper: swap buffers and wait for
// the flush goroutine to finish the job. Used during Stop().
func (tw *TableWriter) Flush() error {
	tw.SwapAndFlush()
	// Drain: send a second swap so the flush worker processes the first,
	// then wait for both slots to be empty.
	tw.SwapAndFlush()
	return nil
}

const (
	flushMaxRetries  = 5
	flushBaseBackoff = 50 * time.Millisecond
)

// isFlushRetriableError classifies errors that often clear with a short backoff:
// SQLite catalog contention, DuckLake transaction conflicts, and HTTP transport
// flakes from S3-compatible backends (timeouts, 429/5xx, intermittent 404).
// Permanent config/auth/bucket-missing errors are excluded.
func isFlushRetriableError(errStr string) bool {
	if errStr == "" {
		return false
	}
	if strings.Contains(errStr, "NoSuchBucket") {
		return false
	}
	if strings.Contains(errStr, "InvalidAccessKeyId") ||
		strings.Contains(errStr, "SignatureDoesNotMatch") {
		return false
	}
	if strings.Contains(errStr, "database is locked") ||
		strings.Contains(errStr, "Could not set lock") ||
		strings.Contains(errStr, "catalog") ||
		strings.Contains(errStr, "transaction conflict") {
		return true
	}
	// DuckDB httpfs surfaces remote I/O as "HTTP Error: ..."
	if strings.Contains(errStr, "HTTP Error") {
		return true
	}
	return false
}

// isDuckDBFatalError is true when DuckDB has invalidated the in-memory
// instance (typically a checkpoint/pin OOM). Further queries fail until
// the process restarts; retrying the flush cannot recover it.
func isDuckDBFatalError(errStr string) bool {
	return strings.Contains(errStr, "database has been invalidated") ||
		strings.Contains(errStr, "FATAL Error") ||
		strings.Contains(errStr, "failed to pin block")
}

func logFlushFailure(memName, tableFQN string, attempts int, err error) {
	msg := fmt.Sprintf("flush %s → %s failed after %d attempts: %v", memName, tableFQN, attempts, err)
	if err != nil && isDuckDBFatalError(err.Error()) {
		logger.Error(msg + "; DuckDB instance is dead until restart — raise storage.ducklake.tuning.memory_limit (docs/OOM.md)")
		return
	}
	logger.Error(msg)
}

// flushWorker is the dedicated goroutine that processes flush jobs.
// It reads from flushCh and for each job copies data from the standby
// memory table to DuckLake with retry on transient errors (catalog lock,
// S3 HTTP flakes).
func (tw *TableWriter) flushWorker() {
	defer tw.flushWg.Done()
	for job := range tw.flushCh {
		_ = tw.flushSlotDirect(job.slotIdx, tw.catalogMu)
	}
}

// clearSlot is called with memMu write-locked. A failed clear retains the committed
// marker, so subsequent retries do not insert the same rows again.
func (tw *TableWriter) clearSlot(slot int) error {
	if _, err := tw.db.Exec(tw.flushTruncateSQL[slot]); err != nil {
		return fmt.Errorf("clear memory table %s: %w", tw.memTables[slot], err)
	}
	tw.slotCommitted[slot] = false
	return nil
}

// flushSlotDirect copies rows from the given memory table slot to DuckLake.
// Each attempt locks the catalog before the memory table, so a queued flush
// waiting for compaction does not block appends. Failed clears retain the
// committed marker and retries only clear those rows.
func (tw *TableWriter) flushSlotDirect(slot int, catalogMu *sync.Mutex) error {
	start := time.Now()
	memName := tw.memTables[slot]

	var rowsFlushed int64
	var err error
	backoff := flushBaseBackoff

	for attempt := 0; attempt <= flushMaxRetries; attempt++ {
		var copied int64
		copied, err = tw.copyAndClearSlot(slot, catalogMu)
		rowsFlushed += copied
		if err == nil {
			break
		}
		errStr := err.Error()
		if !isFlushRetriableError(errStr) || attempt == flushMaxRetries {
			metrics.RecordPipelineStageError("ducklake", "flush", "insert_error")
			logFlushFailure(memName, tw.tableFQN, attempt+1, err)
			if tw.onFatalFlush != nil && isDuckDBFatalError(errStr) {
				tw.onFatalFlush(err)
			}
			return err
		}
		logger.Warn(fmt.Sprintf("flush %s: retriable error (attempt %d/%d), retrying in %v: %v",
			memName, attempt+1, flushMaxRetries+1, backoff, err))
		time.Sleep(backoff)
		backoff *= 2
	}

	elapsed := time.Since(start)
	elapsedSec := elapsed.Seconds()

	metrics.RecordDucklakeTableFlushDuration(tw.tableFQN, elapsedSec)
	if rowsFlushed == 0 {
		return nil
	}
	metrics.RecordDucklakeTableFlushedRows(tw.tableFQN, rowsFlushed)

	logger.Info("💾 Flushed rows", "count", rowsFlushed, "from", memName, "to", tw.tableFQN,
		"elapsed", elapsed.Round(time.Millisecond), "rec_per_sec", fmt.Sprintf("%.0f", float64(rowsFlushed)/elapsedSec))
	return nil
}

// copyAndClearSlot holds the write lock across INSERT and TRUNCATE. A
// committed marker survives a failed clear; neither retries nor appends may
// insert those rows again. The catalog mutex is released before clearing.
func (tw *TableWriter) copyAndClearSlot(slot int, catalogMu *sync.Mutex) (int64, error) {
	if catalogMu != nil {
		catalogMu.Lock()
	}
	tw.memMu[slot].Lock()
	defer tw.memMu[slot].Unlock()
	if tw.slotCommitted[slot] {
		if catalogMu != nil {
			catalogMu.Unlock()
		}
		return 0, tw.clearSlot(slot)
	}
	result, err := tw.db.Exec(tw.flushInsertSQL[slot])
	if catalogMu != nil {
		catalogMu.Unlock()
	}
	if err != nil {
		return 0, err
	}
	// Exec succeeded: even an unavailable row count must not allow reinsertion.
	tw.slotCommitted[slot] = true
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if rows == 0 {
		tw.slotCommitted[slot] = false
		return 0, nil
	}
	return rows, tw.clearSlot(slot)
}

// GetStats returns statistics for this table
func (tw *TableWriter) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	stats["table"] = tw.tableFQN
	stats["proto_type"] = tw.schema.ProtoType

	// Get row count from DuckLake
	var rowCount int64
	row := tw.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", tw.tableFQN))
	if err := row.Scan(&rowCount); err == nil {
		stats["row_count"] = rowCount
	}

	// Get time range from DuckLake
	var minTs, maxTs sql.NullInt64
	row = tw.db.QueryRow(fmt.Sprintf(
		"SELECT MIN(timestamp), MAX(timestamp) FROM %s", tw.tableFQN))
	if err := row.Scan(&minTs, &maxTs); err == nil {
		if minTs.Valid {
			stats["min_timestamp"] = minTs.Int64
			stats["oldest_data"] = time.Unix(0, minTs.Int64).Format(time.RFC3339)
		}
		if maxTs.Valid {
			stats["max_timestamp"] = maxTs.Int64
			stats["newest_data"] = time.Unix(0, maxTs.Int64).Format(time.RFC3339)
		}
	}

	// Get unflushed buffer size from both memory tables
	var bufSize int64
	for _, mem := range tw.memTables {
		var cnt int64
		row = tw.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", mem))
		if err := row.Scan(&cnt); err == nil {
			bufSize += cnt
		}
	}
	stats["buffer_size"] = bufSize

	return stats, nil
}

// GetBufferStats returns only the in-memory buffer row count (cheap, no lake scan).
func (tw *TableWriter) GetBufferStats() int64 {
	var bufSize int64
	for _, mem := range tw.memTables {
		var cnt int64
		row := tw.db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", mem))
		if err := row.Scan(&cnt); err == nil {
			bufSize += cnt
		}
	}
	return bufSize
}

// TableFQN returns the fully qualified DuckLake table name
func (tw *TableWriter) TableFQN() string {
	return tw.tableFQN
}

// MemTableNames returns both in-memory buffer table names (for UNION ALL queries).
func (tw *TableWriter) MemTableNames() [2]string {
	return tw.memTables
}

// GetSchema returns the table schema
func (tw *TableWriter) GetSchema() *TableSchema {
	return tw.schema
}
