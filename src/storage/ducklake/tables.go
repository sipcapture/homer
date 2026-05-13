// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package ducklake

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
	SIPTypeCall         = "call"         // INVITE, ACK, PRACK, UPDATE, BYE, CANCEL, INFO
	SIPTypeRegistration = "registration" // REGISTER
	SIPTypeDefault      = "default"      // OPTIONS, NOTIFY, SUBSCRIBE, PUBLISH, MESSAGE, REFER
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
	case "INVITE", "ACK", "PRACK", "UPDATE", "BYE", "CANCEL", "INFO":
		return SIPTypeCall
	case "REGISTER":
		return SIPTypeRegistration
	default:
		// OPTIONS, NOTIFY, SUBSCRIBE, PUBLISH, MESSAGE, REFER, etc.
		return SIPTypeDefault
	}
}

// GetTableSchemas returns schemas for all supported table types
func GetTableSchemas() map[TableKey]*TableSchema {
	return map[TableKey]*TableSchema{
		// SIP Call - INVITE, ACK, PRACK, UPDATE, BYE, CANCEL, INFO
		{ProtoType: ProtoTypeSIP, SubType: SIPTypeCall}: {
			ProtoType:   ProtoTypeSIP,
			SubType:     SIPTypeCall,
			TableSuffix: "1_call",
			Columns: []string{
				"uuid", "timestamp", "session_id", "caller", "callee",
				"src_ip", "dst_ip", "src_port", "dst_port",
				"method", "response_code", "cseq_method",
				"protocol", "node_id", "cid", "payload", "data_extra",
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
				data_extra JSON
			`,
			InsertSQL: `(uuid, date, timestamp, session_id, caller, callee, src_ip, dst_ip,
				src_port, dst_port, method, response_code, cseq_method,
				protocol, node_id, cid, payload, data_extra)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?::JSON)`,
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
				"protocol", "node_id", "payload", "data_extra",
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
				data_extra JSON
			`,
			InsertSQL: `(uuid, date, timestamp, session_id, aor, contact, expires, user_agent,
				src_ip, dst_ip, src_port, dst_port, method, response_code,
				protocol, node_id, payload, data_extra)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?::JSON)`,
		},

		// SIP Default - OPTIONS, NOTIFY, SUBSCRIBE, PUBLISH, MESSAGE, REFER
		{ProtoType: ProtoTypeSIP, SubType: SIPTypeDefault}: {
			ProtoType:   ProtoTypeSIP,
			SubType:     SIPTypeDefault,
			TableSuffix: "1_default",
			Columns: []string{
				"uuid", "timestamp", "session_id",
				"src_ip", "dst_ip", "src_port", "dst_port",
				"method", "response_code",
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
				method VARCHAR,
				response_code VARCHAR,
				protocol UINTEGER,
				node_id VARCHAR,
				cid VARCHAR,
				payload VARCHAR,
				data_extra JSON
			`,
			InsertSQL: `(uuid, date, timestamp, session_id, src_ip, dst_ip,
				src_port, dst_port, method, response_code,
				protocol, node_id, cid, payload, data_extra)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?::JSON)`,
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

// TableWriter handles writes to a specific table using Go-side batch buffering,
// double-buffered DuckDB in-memory tables, and a dedicated flush goroutine.
//
// Architecture:
//   - Incoming records accumulate in a Go slice (batch). When the batch reaches
//     batchSize it is flushed to the *active* in-memory DuckDB table via a single
//     multi-row INSERT.
//   - Periodically the active and standby memory tables are swapped atomically.
//     The now-standby table (full of data) is sent to the flush goroutine which
//     copies rows to DuckLake and truncates the standby table — all without
//     blocking new writes that continue into the new active table.
//   - Catalog contention is handled with exponential-backoff retry.
type TableWriter struct {
	db        *sql.DB
	tableFQN  string // DuckLake table: "homer_lake.hep_proto_1_call"
	schema    *TableSchema

	// Double-buffer: two memory tables, swapped atomically.
	// activeIdx 0 → memTables[0] is active (receives writes), memTables[1] is standby.
	memTables [2]string
	activeIdx atomic.Int32

	batchMu   sync.Mutex
	batch     [][]interface{}
	batchSize int
	numCols   int    // number of columns per row
	colNames  string // pre-built "(col1, col2, ...)" for INSERT
	oneRowPH  string // pre-built "(?, ?, ...)" placeholder for one row

	// Pre-built full SQL for the common case (exactly batchSize rows).
	// One per buffer slot so the INSERT targets the correct memory table.
	fullBatchSQL [2]string
	argsPool     sync.Pool // recycled []interface{} slices for batch INSERT

	// Pre-built flush/truncate SQL per buffer slot.
	flushInsertSQL   [2]string // "INSERT INTO <lakeFQN> SELECT * FROM <memTable_N>"
	flushTruncateSQL [2]string // "TRUNCATE TABLE <memTable_N>"

	// Flush queue: dedicated goroutine drains this channel.
	flushCh   chan flushJob
	flushWg   sync.WaitGroup
	catalogMu *sync.Mutex // shared catalog lock from MultiTableWriter
}

// NewTableWriter creates a new table writer with a DuckLake table and
// two in-memory buffer tables (double-buffer). It also starts a dedicated
// flush goroutine that processes swap+flush jobs without blocking writers.
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

	tw.numCols = tw.countInsertCols(schema.InsertSQL)
	tw.colNames = tw.extractColNames(schema.InsertSQL)
	tw.oneRowPH = tw.buildOneRowPlaceholder(schema.InsertSQL)

	for i := 0; i < 2; i++ {
		tw.fullBatchSQL[i] = tw.buildFullBatchSQLFor(batchSize, tw.memTables[i])
		tw.flushInsertSQL[i] = "INSERT INTO " + tableFQN + " SELECT * FROM " + tw.memTables[i]
		tw.flushTruncateSQL[i] = "TRUNCATE TABLE " + tw.memTables[i]
	}

	tw.argsPool = sync.Pool{New: func() interface{} {
		s := make([]interface{}, 0, batchSize*tw.numCols)
		return &s
	}}

	// Create DuckLake persistent table
	createSQL := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s);", tableFQN, schema.CreateSQL)
	if _, err := db.Exec(createSQL); err != nil {
		return nil, fmt.Errorf("failed to create table %s: %w", tableFQN, err)
	}

	// Set partitioning by date for efficient time-range queries
	partitionSQL := fmt.Sprintf("ALTER TABLE %s SET PARTITIONED BY (date);", tableFQN)
	if _, err := db.Exec(partitionSQL); err != nil {
		logger.Warn(fmt.Sprintf("Failed to set partitioning for %s: %v", tableFQN, err))
	}

	// Sort rows by timestamp within each file (DuckLake v1.0).
	// Enables row-group and file-level pruning for time-range queries,
	// so DuckLake skips Parquet files whose min/max timestamp range
	// does not overlap the query window.
	sortSQL := fmt.Sprintf("ALTER TABLE %s SET SORTED BY (timestamp ASC);", tableFQN)
	if _, err := db.Exec(sortSQL); err != nil {
		logger.Warn(fmt.Sprintf("Failed to set sort order for %s: %v", tableFQN, err))
	}

	// Create both in-memory buffer tables (plain DuckDB, not DuckLake)
	for _, mem := range tw.memTables {
		memCreateSQL := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s);", mem, schema.CreateSQL)
		if _, err := db.Exec(memCreateSQL); err != nil {
			return nil, fmt.Errorf("failed to create memory table %s: %w", mem, err)
		}
	}

	// Start dedicated flush goroutine
	tw.flushWg.Add(1)
	go tw.flushWorker()

	logger.Info("DuckLake table ready (double-buffer)", "table", tableFQN,
		"buf_a", memA, "buf_b", memB, "batch_size", batchSize)
	return tw, nil
}

// extractColNames extracts the column-name list from InsertSQL, e.g. "(uuid, date, ...)"
func (tw *TableWriter) extractColNames(insertSQL string) string {
	idx := strings.Index(insertSQL, "VALUES")
	if idx < 0 {
		idx = strings.Index(insertSQL, "values")
	}
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(insertSQL[:idx])
}

// buildOneRowPlaceholder extracts the VALUES placeholder from InsertSQL, e.g. "(?, ?, ?)"
// Type casts like ?::JSON are stripped because the memory table columns already
// have the correct type and DuckDB performs implicit casting.
func (tw *TableWriter) buildOneRowPlaceholder(insertSQL string) string {
	idx := strings.Index(insertSQL, "VALUES")
	if idx < 0 {
		idx = strings.Index(insertSQL, "values")
	}
	if idx < 0 {
		return ""
	}
	ph := strings.TrimSpace(insertSQL[idx+len("VALUES"):])
	ph = strings.ReplaceAll(ph, "?::JSON", "?")
	return ph
}

// countInsertCols counts the number of ? placeholders in InsertSQL
func (tw *TableWriter) countInsertCols(insertSQL string) int {
	return strings.Count(insertSQL, "?")
}

// buildFullBatchSQLFor pre-builds the complete INSERT statement for exactly n rows
// targeting the given memory table.
func (tw *TableWriter) buildFullBatchSQLFor(n int, memTable string) string {
	var sb strings.Builder
	sb.Grow(len("INSERT INTO ") + len(memTable) + 1 + len(tw.colNames) + len(" VALUES ") + n*(len(tw.oneRowPH)+2))
	sb.WriteString("INSERT INTO ")
	sb.WriteString(memTable)
	sb.WriteString(" ")
	sb.WriteString(tw.colNames)
	sb.WriteString(" VALUES ")
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(tw.oneRowPH)
	}
	return sb.String()
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

// flushBatch drains the Go-side batch into the currently active in-memory
// DuckDB table using a single multi-row INSERT statement. The active slot
// is read atomically so this never conflicts with the flush goroutine that
// operates on the standby slot.
func (tw *TableWriter) flushBatch() error {
	tw.batchMu.Lock()
	if len(tw.batch) == 0 {
		tw.batchMu.Unlock()
		return nil
	}
	rows := tw.batch
	tw.batch = make([][]interface{}, 0, tw.batchSize)
	slot := int(tw.activeIdx.Load())
	tw.batchMu.Unlock()

	start := time.Now()
	nRows := len(rows)

	sqlStr := tw.fullBatchSQL[slot]
	if nRows != tw.batchSize {
		sqlStr = tw.buildFullBatchSQLFor(nRows, tw.memTables[slot])
	}

	argsPtr := tw.argsPool.Get().(*[]interface{})
	args := (*argsPtr)[:0]
	for _, row := range rows {
		args = append(args, row...)
	}

	_, err := tw.db.Exec(sqlStr, args...)

	*argsPtr = args[:0]
	tw.argsPool.Put(argsPtr)

	if err != nil {
		metrics.RecordPipelineStageError("ducklake", "batch_insert", "insert_error")
		return fmt.Errorf("batch INSERT into %s (%d rows): %w", tw.memTables[slot], nRows, err)
	}

	metrics.RecordPipelineStageDuration("ducklake", "batch_insert", time.Since(start).Seconds())
	return nil
}

// Close flushes remaining records and stops the flush goroutine.
func (tw *TableWriter) Close() error {
	if err := tw.flushBatch(); err != nil {
		logger.Warn(fmt.Sprintf("flushBatch on close for %s: %v", tw.tableFQN, err))
	}
	close(tw.flushCh)
	tw.flushWg.Wait()
	return nil
}

// SwapAndFlush drains Go-side batches into the active memory table, then
// atomically swaps active↔standby and enqueues the now-standby slot for
// asynchronous flush to DuckLake. Writers are blocked only for the brief
// swap (a single atomic store), not for the DuckLake INSERT.
func (tw *TableWriter) SwapAndFlush() {
	if err := tw.flushBatch(); err != nil {
		logger.Warn(fmt.Sprintf("Failed to drain batch before swap for %s: %v", tw.tableFQN, err))
	}

	// Swap: the slot that was active becomes standby (to be flushed).
	old := int(tw.activeIdx.Load())
	next := 1 - old
	tw.activeIdx.Store(int32(next))

	tw.flushCh <- flushJob{slotIdx: old}
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

// flushWorker is the dedicated goroutine that processes flush jobs.
// It reads from flushCh and for each job copies data from the standby
// memory table to DuckLake with retry on transient errors (catalog lock,
// S3 HTTP flakes).
func (tw *TableWriter) flushWorker() {
	defer tw.flushWg.Done()
	for job := range tw.flushCh {
		tw.flushSlotToDuckLake(job.slotIdx)
	}
}

// flushSlotToDuckLake copies all rows from the given memory table slot to
// the DuckLake persistent table, then truncates the memory table asynchronously.
// Acquires catalogMu for the catalog-modifying INSERT and retries with
// exponential backoff on transient errors (catalog contention, S3 HTTP).
func (tw *TableWriter) flushSlotToDuckLake(slot int) {
	start := time.Now()
	memName := tw.memTables[slot]

	var result sql.Result
	var err error
	backoff := flushBaseBackoff

	for attempt := 0; attempt <= flushMaxRetries; attempt++ {
		tw.catalogMu.Lock()
		result, err = tw.db.Exec(tw.flushInsertSQL[slot])
		tw.catalogMu.Unlock()
		if err == nil {
			break
		}
		errStr := err.Error()
		if !isFlushRetriableError(errStr) || attempt == flushMaxRetries {
			metrics.RecordPipelineStageError("ducklake", "flush", "insert_error")
			logger.Error(fmt.Sprintf("flush %s → %s failed after %d attempts: %v",
				memName, tw.tableFQN, attempt+1, err))
			return
		}
		logger.Warn(fmt.Sprintf("flush %s: retriable error (attempt %d/%d), retrying in %v: %v",
			memName, attempt+1, flushMaxRetries+1, backoff, err))
		time.Sleep(backoff)
		backoff *= 2
	}

	rowsFlushed, _ := result.RowsAffected()
	elapsed := time.Since(start)
	elapsedSec := elapsed.Seconds()

	metrics.RecordDucklakeTableFlushDuration(tw.tableFQN, elapsedSec)
	if rowsFlushed == 0 {
		return
	}
	metrics.RecordDucklakeTableFlushedRows(tw.tableFQN, rowsFlushed)

	// Async truncate: fire and forget — the slot won't be written to until
	// the next swap, and the next flush will see it empty.
	go func(truncSQL, name string) {
		if _, err := tw.db.Exec(truncSQL); err != nil {
			logger.Warn(fmt.Sprintf("Failed to clear memory table %s: %v", name, err))
		}
	}(tw.flushTruncateSQL[slot], memName)

	logger.Info("💾 Flushed rows", "count", rowsFlushed, "from", memName, "to", tw.tableFQN,
		"elapsed", elapsed.Round(time.Millisecond), "rec_per_sec", fmt.Sprintf("%.0f", float64(rowsFlushed)/elapsedSec))
}

// flushSlotDirect copies rows from the given memory table slot to DuckLake.
// When catalogMu is non-nil, it is locked only around each db.Exec attempt (not
// during backoff sleep), matching flushSlotToDuckLake and allowing compaction
// CatalogLock to run between retries on the same shared *sql.DB.
func (tw *TableWriter) flushSlotDirect(slot int, catalogMu *sync.Mutex) {
	start := time.Now()
	memName := tw.memTables[slot]

	var result sql.Result
	var err error
	backoff := flushBaseBackoff

	for attempt := 0; attempt <= flushMaxRetries; attempt++ {
		if catalogMu != nil {
			catalogMu.Lock()
		}
		result, err = tw.db.Exec(tw.flushInsertSQL[slot])
		if catalogMu != nil {
			catalogMu.Unlock()
		}
		if err == nil {
			break
		}
		errStr := err.Error()
		if !isFlushRetriableError(errStr) || attempt == flushMaxRetries {
			metrics.RecordPipelineStageError("ducklake", "flush", "insert_error")
			logger.Error(fmt.Sprintf("flush %s → %s failed after %d attempts: %v",
				memName, tw.tableFQN, attempt+1, err))
			return
		}
		logger.Warn(fmt.Sprintf("flush %s: retriable error (attempt %d/%d), retrying in %v: %v",
			memName, attempt+1, flushMaxRetries+1, backoff, err))
		time.Sleep(backoff)
		backoff *= 2
	}

	rowsFlushed, _ := result.RowsAffected()
	elapsed := time.Since(start)
	elapsedSec := elapsed.Seconds()

	metrics.RecordDucklakeTableFlushDuration(tw.tableFQN, elapsedSec)
	if rowsFlushed == 0 {
		return
	}
	metrics.RecordDucklakeTableFlushedRows(tw.tableFQN, rowsFlushed)

	go func(truncSQL, name string) {
		if _, err := tw.db.Exec(truncSQL); err != nil {
			logger.Warn(fmt.Sprintf("Failed to clear memory table %s: %v", name, err))
		}
	}(tw.flushTruncateSQL[slot], memName)

	logger.Info("💾 Flushed rows", "count", rowsFlushed, "from", memName, "to", tw.tableFQN,
		"elapsed", elapsed.Round(time.Millisecond), "rec_per_sec", fmt.Sprintf("%.0f", float64(rowsFlushed)/elapsedSec))
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
