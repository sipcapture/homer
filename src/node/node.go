// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// Package node provides the FlightSQL node module for Homer Server.
// It serves data from DuckLake storage via FlightSQL protocol.
package node

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	airport "github.com/hugr-lab/airport-go"
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/storage/ducklake"
	"github.com/sipcapture/homer-core/src/stream/hepstream"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
	"google.golang.org/grpc"

	_ "github.com/duckdb/duckdb-go/v2"
)

// VolumeInfo represents an attached storage volume
type VolumeInfo struct {
	Name     string // Volume name (e.g., "hot", "cold")
	LakeName string // DuckLake name (e.g., "homer_lake_hot")
	Path     string // Data path
}

// Node is the FlightSQL node module
type Node struct {
	config        *config.NodeConfig
	grpcServer    *grpc.Server
	httpServer    *http.Server
	catalog       *DuckLakeCatalog
	listener      net.Listener
	db            *sql.DB
	sharedDB      *sql.DB // shared with writer module for real-time visibility
	tieredQueryDB *sql.DB // writer TieredStorageManager DuckDB (homer_lake_hot / _cold)
	mu            sync.RWMutex
	running       bool
	volumes       []VolumeInfo // Attached storage volumes for tiered storage

	// fsql is the optional Apache Arrow FlightSQL server (Grafana / InfluxDB FlightSQL).
	fsql *fsqlServer

	// broker is optional: wired from main.go when the ingest module is
	// running in the same process and ingest.hep_stream.enable is true.
	// When nil the /stream endpoint responds with 503 so the coordinator
	// can cleanly skip this node during fan-out.
	broker *hepstream.Broker
}

// SetBroker wires the live-stream broker into the node so handleStream
// can subscribe to it. Called once by main.go before Start(); passing
// nil disables the feature on this node.
func (n *Node) SetBroker(b *hepstream.Broker) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.broker = b
}

// New creates a new Node module
func New(cfg *config.NodeConfig) (*Node, error) {
	// Connect to DuckLake database
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return nil, fmt.Errorf("failed to open DuckDB: %w", err)
	}
	// Use single connection to ensure ATTACH catalogs are always visible
	db.SetMaxOpenConns(1)

	// Configure DuckDB for DuckLake and get attached volumes
	volumes, err := configureDuckLake(db, cfg)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to configure DuckLake: %w", err)
	}

	// Create DuckLake catalog with volume support
	catalog := NewDuckLakeCatalog(db, cfg.DuckLake.LakeName, volumes)

	// Build airport config
	airportConfig := airport.ServerConfig{
		Catalog:        catalog,
		MaxMessageSize: cfg.FlightServer.MaxMessageSize,
	}

	// Add authentication if configured
	if cfg.FlightServer.AuthToken != "" {
		airportConfig.Auth = airport.BearerAuth(func(token string) (string, error) {
			if token == cfg.FlightServer.AuthToken {
				return "homer-user", nil
			}
			return "", airport.ErrUnauthorized
		})
	}

	// Create gRPC server with airport options
	opts := airport.ServerOptions(airportConfig)
	grpcServer := grpc.NewServer(opts...)

	// Register airport Flight service
	airport.NewServer(grpcServer, airportConfig)

	n := &Node{
		config:     cfg,
		grpcServer: grpcServer,
		catalog:    catalog,
		db:         db,
		volumes:    volumes,
	}
	if cfg.FlightSQLServer.Enable {
		n.fsql = newFsqlServer(n, cfg.FlightSQLServer, cfg.DuckLake.LakeName)
	}
	return n, nil
}

// Start starts the node module
func (n *Node) Start() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.running {
		return fmt.Errorf("node already running")
	}

	// Refresh catalog: DETACH + re-ATTACH so we see tables created by the
	// storage module that started before us but used a separate DuckDB instance.
	n.refreshCatalog()

	// Start gRPC server for FlightSQL (Airport protocol)
	grpcAddr := fmt.Sprintf("%s:%d", n.config.FlightServer.Host, n.config.FlightServer.Port)
	listener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	n.listener = listener
	n.running = true

	go func() {
		logger.Info("Node: FlightSQL server started", "addr", grpcAddr)
		if err := n.grpcServer.Serve(listener); err != nil {
			logger.Error(fmt.Sprintf("Node: FlightSQL server error: %v", err))
		}
	}()

	// Start HTTP server for SQL queries (used by coordinator)
	httpPort := n.config.FlightServer.Port + 1 // HTTP on next port
	httpAddr := fmt.Sprintf("%s:%d", n.config.FlightServer.Host, httpPort)

	mux := http.NewServeMux()
	mux.HandleFunc("/query", n.handleQuery)
	mux.HandleFunc("/health", n.handleHealth)
	mux.HandleFunc("/vacuum", n.handleVacuum)
	mux.HandleFunc("/stream", n.handleStream)

	n.httpServer = &http.Server{
		Addr:         httpAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		logger.Info("Node: HTTP API started", "addr", httpAddr)
		if err := n.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error(fmt.Sprintf("Node: HTTP server error: %v", err))
		}
	}()

	if n.fsql != nil {
		if n.sharedDB == nil {
			n.fsql.withCatalogRefresher(func() { n.refreshCatalog() })
		}
		if err := n.fsql.Start(); err != nil {
			return fmt.Errorf("FlightSQL: %w", err)
		}
	}

	return nil
}

// refreshCatalog detaches and re-attaches all DuckLake volumes so that
// the node sees any tables created by the storage module (which runs
// its own DuckDB instance sharing the same catalog file).
func (n *Node) refreshCatalog() {
	for _, vol := range n.volumes {
		detachSQL := fmt.Sprintf("DETACH %s;", vol.LakeName)
		if _, err := n.db.Exec(detachSQL); err != nil {
			logger.Warn(fmt.Sprintf("Node: refreshCatalog: DETACH %s failed: %v", vol.LakeName, err))
		}
	}

	// Re-attach volumes using the original config
	newVolumes, err := configureDuckLake(n.db, n.config)
	if err != nil {
		logger.Error(fmt.Sprintf("Node: refreshCatalog: re-attach failed: %v", err))
		return
	}

	n.volumes = newVolumes
	n.catalog = NewDuckLakeCatalog(n.db, n.config.DuckLake.LakeName, newVolumes)
	logger.Info("Node: catalog refreshed", "volumes", len(newVolumes))
}

// Stop stops the node module
func (n *Node) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.running {
		return nil
	}

	// Stop HTTP server
	if n.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		n.httpServer.Shutdown(ctx)
	}

	if n.fsql != nil {
		n.fsql.Stop()
	}

	// GracefulStop waits for all in-flight RPCs to finish. Guard with a timeout
	// so that stale client connections do not block the entire shutdown sequence.
	grpcStopped := make(chan struct{})
	go func() {
		n.grpcServer.GracefulStop()
		close(grpcStopped)
	}()
	select {
	case <-grpcStopped:
	case <-time.After(5 * time.Second):
		logger.Warn("Node: gRPC server did not stop gracefully in 5s, forcing stop")
		n.grpcServer.Stop()
	}

	n.running = false

	if n.db != nil {
		n.db.Close()
	}

	logger.Info("Node: FlightSQL server stopped")
	return nil
}

// QueryRequest represents a SQL query request
type QueryRequest struct {
	SQL string `json:"sql"`
}

// QueryResponse represents a SQL query response
type QueryResponse struct {
	Success bool                     `json:"success"`
	Data    []map[string]interface{} `json:"data,omitempty"`
	Count   int                      `json:"count,omitempty"`
	Error   string                   `json:"error,omitempty"`
}

// shouldUseSQLQuery returns true for statements that return row sets and must
// use QueryContext. INSERT/UPDATE/DELETE without RETURNING should use ExecContext
// so DuckDB applies mutations reliably (Query-only INSERTs could be invisible
// to subsequent SELECTs on some paths).
func shouldUseSQLQuery(sql string) bool {
	u := strings.TrimSpace(strings.ToUpper(sql))
	if strings.Contains(u, "RETURNING") {
		return true
	}
	switch {
	case strings.HasPrefix(u, "SELECT"):
		return true
	case strings.HasPrefix(u, "WITH"):
		return true
	case strings.HasPrefix(u, "SHOW"):
		return true
	case strings.HasPrefix(u, "DESCRIBE"):
		return true
	case strings.HasPrefix(u, "PRAGMA"):
		return true
	case strings.HasPrefix(u, "EXPLAIN"):
		return true
	default:
		return false
	}
}

func sqlStringLiteral(value string) string {
	escaped := strings.ReplaceAll(value, "'", "''")
	return "'" + escaped + "'"
}

func addStorageColumnsToQuery(sql, lakeName, volumeName string) string {
	upperSQL := strings.ToUpper(sql)
	selectIdx := strings.Index(upperSQL, "SELECT")
	fromIdx := strings.Index(upperSQL, "FROM")
	if selectIdx == -1 || fromIdx == -1 || fromIdx < selectIdx {
		return sql
	}

	selectPart := sql[selectIdx+len("SELECT") : fromIdx]
	selectPartLower := strings.ToLower(selectPart)
	if strings.Contains(selectPartLower, "storage_lake") || strings.Contains(selectPartLower, "storage_volume") {
		return sql
	}

	trimmedSelect := strings.TrimSpace(selectPart)
	if trimmedSelect == "" {
		return sql
	}

	extraCols := fmt.Sprintf(
		"%s AS storage_lake, %s AS storage_volume",
		sqlStringLiteral(lakeName),
		sqlStringLiteral(volumeName),
	)
	newSelect := trimmedSelect + ", " + extraCols

	return sql[:selectIdx+len("SELECT")] + " " + newSelect + " " + sql[fromIdx:]
}

var sqlLimitRegexp = regexp.MustCompile(`(?is)\s+LIMIT\s+(\d+)\s*$`)
var (
	sqlTimestampFromRegexp = regexp.MustCompile(`(?is)\btimestamp\s*>=\s*\(to_timestamp\((\d+)\s*/\s*1000(?:\.0)?\)\s*AT\s+TIME\s+ZONE\s+'UTC'\)`)
	sqlTimestampToRegexp   = regexp.MustCompile(`(?is)\btimestamp\s*(?:<=|<)\s*\(to_timestamp\((\d+)\s*/\s*1000(?:\.0)?\)\s*AT\s+TIME\s+ZONE\s+'UTC'\)`)
	sqlGroupByRegexp       = regexp.MustCompile(`(?is)\bGROUP\s+BY\b`)
	sqlDistinctRegexp      = regexp.MustCompile(`(?is)\bSELECT\s+DISTINCT\b`)
	sqlAggregateRegexp     = regexp.MustCompile(`(?is)\b(COUNT|SUM|AVG|MIN|MAX)\s*\(`)
)

const memorySplitThresholdMs = int64(time.Hour / time.Millisecond)

func extractSQLLimit(sql string) int {
	m := sqlLimitRegexp.FindStringSubmatch(sql)
	if len(m) < 2 {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func scanAllSQLRows(rows *sql.Rows) ([]map[string]interface{}, []string, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	var out []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		ptrs := make([]interface{}, len(columns))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			continue
		}
		row := make(map[string]interface{}, len(columns))
		for i, c := range columns {
			row[c] = values[i]
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return out, columns, nil
}

func mergeColumnOrder(a, b []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, c := range a {
		if c != "" && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	for _, c := range b {
		if c != "" && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

func rowDedupKey(m map[string]interface{}) string {
	if u, ok := m["uuid"]; ok && u != nil {
		return "u:" + fmt.Sprint(u)
	}
	return "f:" + fmt.Sprint(m["session_id"]) + "|" + fmt.Sprint(m["timestamp"])
}

func rowTimestampSortKey(m map[string]interface{}) int64 {
	v, ok := m["timestamp"]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case time.Time:
		return t.UnixNano()
	case []byte:
		ts, err := time.Parse("2006-01-02 15:04:05.999999999", string(t))
		if err != nil {
			ts, err = time.Parse("2006-01-02 15:04:05", string(t))
		}
		if err == nil {
			return ts.UnixNano()
		}
	}
	return 0
}

func mergeSelectResults(a, b []map[string]interface{}, colsA, colsB []string, limit int) ([]map[string]interface{}, []string) {
	cols := mergeColumnOrder(colsA, colsB)
	byKey := make(map[string]map[string]interface{})
	for _, row := range a {
		k := rowDedupKey(row)
		byKey[k] = row
	}
	for _, row := range b {
		k := rowDedupKey(row)
		if ex, ok := byKey[k]; ok {
			if rowTimestampSortKey(row) > rowTimestampSortKey(ex) {
				byKey[k] = row
			}
		} else {
			byKey[k] = row
		}
	}
	merged := make([]map[string]interface{}, 0, len(byKey))
	for _, row := range byKey {
		nr := make(map[string]interface{}, len(cols))
		for _, c := range cols {
			if v, ok := row[c]; ok {
				nr[c] = v
			} else {
				nr[c] = nil
			}
		}
		merged = append(merged, nr)
	}
	sort.Slice(merged, func(i, j int) bool {
		return rowTimestampSortKey(merged[i]) > rowTimestampSortKey(merged[j])
	})
	if limit > 0 && len(merged) > limit {
		merged = merged[:limit]
	}
	return merged, cols
}

func runSelectQuery(ctx context.Context, db *sql.DB, query string) ([]map[string]interface{}, []string, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	return scanAllSQLRows(rows)
}

type memoryUnionQueries struct {
	lakeSQL     string
	memSQL      string
	combinedSQL string
	ok          bool
}

type sharedQueryPlanLog struct {
	mode        string
	lakeSQL     string
	memSQL      string
	combinedSQL string
}

func normalizeSQLWhitespace(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

func memorySplitRangeMs(sql string) (int64, bool) {
	fromM := sqlTimestampFromRegexp.FindStringSubmatch(sql)
	toM := sqlTimestampToRegexp.FindStringSubmatch(sql)
	if len(fromM) < 2 || len(toM) < 2 {
		return 0, false
	}
	fromMs, err := strconv.ParseInt(fromM[1], 10, 64)
	if err != nil {
		return 0, false
	}
	toMs, err := strconv.ParseInt(toM[1], 10, 64)
	if err != nil || toMs <= fromMs {
		return 0, false
	}
	return toMs - fromMs, true
}

func sqlSplitSafeForTimestampTopN(sql, baseSQL, orderByClause, limitClause string) bool {
	if strings.TrimSpace(limitClause) == "" {
		return false
	}
	if normalizeSQLWhitespace(orderByClause) != "order by timestamp desc" {
		return false
	}
	upper := strings.ToUpper(baseSQL)
	if sqlGroupByRegexp.MatchString(upper) || sqlDistinctRegexp.MatchString(upper) || sqlAggregateRegexp.MatchString(upper) {
		return false
	}
	selectIdx := strings.Index(upper, "SELECT")
	fromIdx := strings.Index(upper, "FROM")
	if selectIdx == -1 || fromIdx == -1 || fromIdx <= selectIdx+len("SELECT") {
		return false
	}
	selectList := strings.TrimSpace(baseSQL[selectIdx+len("SELECT") : fromIdx])
	return strings.HasPrefix(selectList, "*")
}

func (n *Node) buildMemoryUnionQueries(sql string) memoryUnionQueries {
	out := memoryUnionQueries{lakeSQL: sql}
	if n.sharedDB == nil {
		return out
	}
	upper := strings.ToUpper(sql)
	if !strings.HasPrefix(strings.TrimSpace(upper), "SELECT") {
		return out
	}

	marker := ".main.hep_proto_"
	markerIdx := strings.Index(sql, marker)
	if markerIdx == -1 {
		return out
	}

	lakeStart := markerIdx
	for lakeStart > 0 {
		ch := sql[lakeStart-1]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '(' || ch == ',' {
			break
		}
		lakeStart--
	}
	lakeName := sql[lakeStart:markerIdx]
	suffixStart := markerIdx + len(marker)
	suffixEnd := suffixStart
	for suffixEnd < len(sql) {
		ch := sql[suffixEnd]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == ',' || ch == ')' || ch == ';' {
			break
		}
		suffixEnd++
	}
	tableSuffix := sql[suffixStart:suffixEnd]
	memTableA := "mem_hep_proto_" + tableSuffix + "_a"
	memTableB := "mem_hep_proto_" + tableSuffix + "_b"
	lakeTableFQN := sql[lakeStart:suffixEnd]

	orderByClause, limitClause, baseSQL := extractOrderLimit(sql)
	memSQLA := strings.Replace(baseSQL, lakeTableFQN, memTableA, 1)
	memSQLB := strings.Replace(baseSQL, lakeTableFQN, memTableB, 1)
	for _, memSQL := range []*string{&memSQLA, &memSQLB} {
		*memSQL = strings.Replace(*memSQL, "'"+lakeName+"' AS storage_lake", "'memory' AS storage_lake", 1)
		for _, vol := range n.volumes {
			*memSQL = strings.Replace(*memSQL, "'"+vol.Name+"' AS storage_volume", "'buffer' AS storage_volume", 1)
		}
	}

	memUnion := "SELECT * FROM (" + memSQLA + " UNION ALL " + memSQLB + ") _m"
	combined := "SELECT * FROM (" + baseSQL + " UNION ALL " + memSQLA + " UNION ALL " + memSQLB + ") _u"
	if orderByClause != "" {
		memUnion += " " + orderByClause
		combined += " " + orderByClause
	}
	if limitClause != "" {
		memUnion += " " + limitClause
		combined += " " + limitClause
	}
	out.memSQL = memUnion
	out.combinedSQL = combined
	out.ok = true
	return out
}

func shouldSplitLakeAndMem(sql, baseSQL, orderByClause, limitClause string) bool {
	if !sqlSplitSafeForTimestampTopN(sql, baseSQL, orderByClause, limitClause) {
		return false
	}
	rangeMs, ok := memorySplitRangeMs(sql)
	return ok && rangeMs > memorySplitThresholdMs
}

func (n *Node) runSharedSelectWithMemoryPolicy(ctx context.Context, db *sql.DB, sql string) ([]map[string]interface{}, []string, sharedQueryPlanLog, error) {
	plan := sharedQueryPlanLog{
		mode:        "no_mem_union",
		lakeSQL:     sql,
		combinedSQL: sql,
	}
	mq := n.buildMemoryUnionQueries(sql)
	if !mq.ok {
		data, cols, err := runSelectQuery(ctx, db, sql)
		return data, cols, plan, err
	}
	orderByClause, limitClause, baseSQL := extractOrderLimit(sql)
	if shouldSplitLakeAndMem(sql, baseSQL, orderByClause, limitClause) {
		plan.mode = "split_lake_and_mem"
		plan.lakeSQL = mq.lakeSQL
		plan.memSQL = mq.memSQL
		plan.combinedSQL = ""
		lakeData, lakeCols, err := runSelectQuery(ctx, db, mq.lakeSQL)
		if err != nil {
			return nil, nil, plan, fmt.Errorf("lake query: %w", err)
		}
		memData, memCols, err := runSelectQuery(ctx, db, mq.memSQL)
		if err != nil {
			return nil, nil, plan, fmt.Errorf("memory query: %w", err)
		}
		limit := extractSQLLimit(sql)
		merged, cols := mergeSelectResults(lakeData, memData, lakeCols, memCols, limit)
		return merged, cols, plan, nil
	}
	plan.mode = "single_union"
	plan.lakeSQL = mq.lakeSQL
	plan.memSQL = mq.memSQL
	plan.combinedSQL = mq.combinedSQL
	data, cols, err := runSelectQuery(ctx, db, mq.combinedSQL)
	return data, cols, plan, err
}

func (n *Node) querySelectMerged(ctx context.Context, sharedSQL string, sharedDB *sql.DB, tieredSQL string, tieredDB *sql.DB, originalSQL string) ([]map[string]interface{}, []string, error) {
	rows1, err := sharedDB.QueryContext(ctx, sharedSQL)
	if err != nil {
		return nil, nil, fmt.Errorf("shared query: %w", err)
	}
	defer rows1.Close()
	data1, cols1, err := scanAllSQLRows(rows1)
	if err != nil {
		return nil, nil, err
	}

	rows2, err := tieredDB.QueryContext(ctx, tieredSQL)
	if err != nil {
		return nil, nil, fmt.Errorf("tiered query: %w", err)
	}
	defer rows2.Close()
	data2, cols2, err := scanAllSQLRows(rows2)
	if err != nil {
		return nil, nil, err
	}

	limit := extractSQLLimit(originalSQL)
	merged, cols := mergeSelectResults(data1, data2, cols1, cols2, limit)
	return merged, cols, nil
}

func (n *Node) tieredQueryDBForRead() *sql.DB {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.tieredQueryDB
}

// handleQuery handles POST /query requests
func (n *Node) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, QueryResponse{
			Success: false,
			Error:   "Invalid request body",
		})
		return
	}

	if req.SQL == "" {
		writeJSON(w, http.StatusBadRequest, QueryResponse{
			Success: false,
			Error:   "SQL query is required",
		})
		return
	}

	// Rewrite query for tiered storage (UNION ALL across volumes).
	rewrittenSQL := n.rewriteQueryForVolumes(req.SQL)
	logger.Info("Node: handleQuery", "sql_chars", len(req.SQL))
	logger.Debug("Node: handleQuery SQL", "sql", req.SQL, "rewritten", rewrittenSQL)

	db := n.queryDB()

	if !shouldUseSQLQuery(rewrittenSQL) {
		res, err := db.ExecContext(r.Context(), rewrittenSQL)
		if err != nil {
			logger.Error("Node: Exec failed", "sql", req.SQL, "rewritten", rewrittenSQL, "error", err)
			writeJSON(w, http.StatusOK, QueryResponse{
				Success: false,
				Error:   err.Error(),
			})
			return
		}
		nAff, _ := res.RowsAffected()
		logger.Info("Node: handleQuery exec OK", "rows_affected", nAff)
		logger.Debug("Node: handleQuery exec OK", "sql", req.SQL, "rows_affected", nAff)
		writeJSON(w, http.StatusOK, QueryResponse{
			Success: true,
			Data:    []map[string]interface{}{},
			Count:   0,
		})
		return
	}

	sharedResults, sharedCols, sharedPlan, err := n.runSharedSelectWithMemoryPolicy(r.Context(), db, rewrittenSQL)
	logger.Info("Node: handleQuery execution plan", "mode", sharedPlan.mode)
	logger.Debug("Node: handleQuery execution plan SQL",
		"lake_sql", sharedPlan.lakeSQL,
		"mem_sql", sharedPlan.memSQL,
		"combined_sql", sharedPlan.combinedSQL,
	)
	if err != nil {
		logger.Error("Node: Query failed",
			"sql", req.SQL,
			"mode", sharedPlan.mode,
			"lake_sql", sharedPlan.lakeSQL,
			"mem_sql", sharedPlan.memSQL,
			"combined_sql", sharedPlan.combinedSQL,
			"error", err)
		writeJSON(w, http.StatusOK, QueryResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	results := sharedResults
	columns := sharedCols
	tieredUnion, tieredOk := n.tryBuildVolumeUnionSQL(req.SQL)
	tieredDB := n.tieredQueryDBForRead()
	if tieredOk && tieredDB != nil && n.sharedDB != nil {
		tieredResults, tieredCols, terr := runSelectQuery(r.Context(), tieredDB, tieredUnion)
		if terr == nil {
			limit := extractSQLLimit(req.SQL)
			results, columns = mergeSelectResults(results, tieredResults, columns, tieredCols, limit)
			logger.Info("Node: handleQuery: returning merged results", "rows", len(results), "columns", fmt.Sprintf("%v", columns))
			writeJSON(w, http.StatusOK, QueryResponse{
				Success: true,
				Data:    results,
				Count:   len(results),
			})
			return
		}
		logger.Warn("Node: merged writer+tiered query failed, falling back to writer-only", "error", terr)
	}

	logger.Info("Node: handleQuery: returning results", "rows", len(results), "columns", fmt.Sprintf("%v", columns))

	writeJSON(w, http.StatusOK, QueryResponse{
		Success: true,
		Data:    results,
		Count:   len(results),
	})
}

// prepareFlightSQLDataSQL applies node query rewrites (tiered volumes + memory union) after Grafana/sqlrewrite.
func (n *Node) prepareFlightSQLDataSQL(q string) string {
	q = n.rewriteQueryForVolumes(q)
	mq := n.buildMemoryUnionQueries(q)
	if !mq.ok {
		return q
	}
	orderByClause, limitClause, baseSQL := extractOrderLimit(q)
	if shouldSplitLakeAndMem(q, baseSQL, orderByClause, limitClause) {
		// FlightSQL prepares a single SQL statement; for wide ranges avoid
		// the huge lake+mem UNION and keep a lake-only query.
		return q
	}
	return mq.combinedSQL
}

// duckLakeCatalogForQuery returns the DuckLake catalog name used in rewritten SQL FROM clauses.
// When the node shares the writer's DuckDB (SetSharedDB in main.go), queries run on the writer
// connection, which attaches exactly node.ducklake.lake_name (the writer's DuckLake manager).
// configureDuckLake on the node's own DB may still register baseLake+"_"+volume.Name for
// non-"default" volume labels, so vol.LakeName can differ from the catalog on sharedDB.
func (n *Node) duckLakeCatalogForQuery(vol VolumeInfo) string {
	if n.sharedDB != nil && len(n.volumes) == 1 {
		return n.config.DuckLake.LakeName
	}
	return vol.LakeName
}

// rewriteQueryForVolumes rewrites SQL query to use UNION ALL across all tiered storage volumes
// Example: "SELECT * FROM homer_lake.main.hep_proto_1 WHERE date = '2026-01-30'"
// Becomes: "SELECT * FROM homer_lake_hot.main.hep_proto_1 WHERE date = '2026-01-30'
//
//	UNION ALL SELECT * FROM homer_lake_cold.main.hep_proto_1 WHERE date = '2026-01-30'"
//
// When SetSharedDB is used (writer connection), this returns sql unchanged; the
// tiered UNION is executed separately on tieredQueryDB — see handleQuery.
func (n *Node) rewriteQueryForVolumes(sql string) string {
	// Skip if single volume or no volumes configured
	if len(n.volumes) <= 1 {
		if len(n.volumes) == 1 {
			baseLakeName := n.config.DuckLake.LakeName
			if strings.Contains(sql, baseLakeName+".main.") {
				cat := n.duckLakeCatalogForQuery(n.volumes[0])
				rewritten := strings.ReplaceAll(sql, baseLakeName+".main.", cat+".main.")
				return addStorageColumnsToQuery(rewritten, cat, n.volumes[0].Name)
			}
		}
		return sql
	}

	// Writer shares ducklakeManager's DuckDB with the node (SetSharedDB). That
	// connection only attaches storage.ducklake.lake_name (e.g. homer_lake).
	// Tiered volume catalogs (homer_lake_hot, homer_lake_cold) live on a
	// separate sql.DB inside TieredStorageManager — rewriting to suffixed
	// names here causes Binder Error: catalog does not exist.
	if n.sharedDB != nil {
		return sql
	}

	if u, ok := n.tryBuildVolumeUnionSQL(sql); ok {
		return u
	}
	return sql
}

// duckDBForTieredTablePresence returns the DuckDB connection on which
// tryBuildVolumeUnionSQL output will be executed for information_schema checks:
// merged writer+tiered path uses tieredQueryDB; standalone multi-volume rewrites use n.db.
func (n *Node) duckDBForTieredTablePresence() *sql.DB {
	if n.sharedDB != nil {
		if td := n.tieredQueryDBForRead(); td != nil {
			return td
		}
	}
	return n.db
}

func tieredSQLIdentLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// tieredTableExists reports whether main.<tableName> exists in lake <lakeName> on db.
func tieredTableExists(db *sql.DB, lakeName, tableName string) bool {
	if db == nil {
		return false
	}
	q := fmt.Sprintf(`
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_catalog = '%s'
		  AND table_schema = 'main'
		  AND table_name = '%s'
	`, tieredSQLIdentLiteral(lakeName), tieredSQLIdentLiteral(tableName))
	var count int
	if err := db.QueryRow(q).Scan(&count); err != nil {
		return false
	}
	return count > 0
}

// tryBuildVolumeUnionSQL builds SELECT ... UNION ALL ... across configured volumes
// for execution on the TieredStorageManager DuckDB (hot + cold catalogs).
// Volumes where the table is missing in that DuckDB instance are skipped so a cold
// lake without a given hep_proto_* table does not break the whole UNION.
func (n *Node) tryBuildVolumeUnionSQL(sql string) (string, bool) {
	if len(n.volumes) <= 1 {
		return "", false
	}

	baseLakeName := n.config.DuckLake.LakeName
	if !strings.Contains(sql, baseLakeName+".main.") {
		return "", false
	}

	upperSQL := strings.ToUpper(sql)
	if !strings.HasPrefix(strings.TrimSpace(upperSQL), "SELECT") {
		return "", false
	}

	fromIdx := strings.Index(upperSQL, "FROM")
	if fromIdx == -1 {
		return "", false
	}

	tablePattern := baseLakeName + ".main."
	tableStart := strings.Index(sql, tablePattern)
	if tableStart == -1 {
		return "", false
	}

	tableEnd := tableStart + len(tablePattern)
	for tableEnd < len(sql) {
		ch := sql[tableEnd]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == ',' || ch == ')' || ch == ';' {
			break
		}
		tableEnd++
	}

	tableFQN := sql[tableStart:tableEnd]
	tableName := tableFQN[len(tablePattern):]

	presenceDB := n.duckDBForTieredTablePresence()
	var unionParts []string
	for _, vol := range n.volumes {
		cat := n.duckLakeCatalogForQuery(vol)
		if presenceDB != nil && !tieredTableExists(presenceDB, cat, tableName) {
			continue
		}
		volTableFQN := cat + ".main." + tableName
		rewrittenPart := strings.Replace(sql, tableFQN, volTableFQN, 1)
		rewrittenPart = addStorageColumnsToQuery(rewrittenPart, cat, vol.Name)
		unionParts = append(unionParts, "("+rewrittenPart+")")
	}

	if len(unionParts) == 0 {
		return "", false
	}

	return strings.Join(unionParts, " UNION ALL "), true
}

// addMemoryUnion rewrites a SELECT query to include unflushed rows from the
// in-memory buffer table via UNION ALL. This gives real-time visibility into
// data that has been received but not yet flushed to DuckLake Parquet files.
//
// Only active when the node uses the writer's shared DuckDB connection
// (sharedDB != nil), because the memory tables live in that DuckDB instance.
//
// Input:  SELECT *, ... FROM homer_lake.main.hep_proto_1_call WHERE <cond> ORDER BY ts DESC LIMIT 50
// Output: SELECT * FROM (
//
//	SELECT *, ... FROM homer_lake.main.hep_proto_1_call WHERE <cond>
//	UNION ALL
//	SELECT *, 'memory' AS storage_lake, 'buffer' AS storage_volume FROM mem_hep_proto_1_call_a WHERE <cond>
//	UNION ALL
//	SELECT *, 'memory' AS storage_lake, 'buffer' AS storage_volume FROM mem_hep_proto_1_call_b WHERE <cond>
//
// ) _u ORDER BY ts DESC LIMIT 50
func (n *Node) addMemoryUnion(sql string) string {
	mq := n.buildMemoryUnionQueries(sql)
	if !mq.ok {
		return sql
	}
	return mq.combinedSQL
}

// extractOrderLimit splits a SQL query into the base query, ORDER BY clause,
// and LIMIT clause. It handles the common pattern used by buildSearchSQLV4.
func extractOrderLimit(sql string) (orderBy, limit, base string) {
	upper := strings.ToUpper(sql)

	// Find last ORDER BY (case-insensitive)
	orderIdx := strings.LastIndex(upper, "ORDER BY")

	if orderIdx == -1 {
		// No ORDER BY — check for standalone LIMIT
		limitIdx := strings.LastIndex(upper, "LIMIT")
		if limitIdx == -1 {
			return "", "", sql
		}
		base = strings.TrimSpace(sql[:limitIdx])
		limit = strings.TrimSpace(sql[limitIdx:])
		return "", limit, base
	}

	// Everything before ORDER BY is the base
	base = strings.TrimSpace(sql[:orderIdx])
	tail := sql[orderIdx:] // "ORDER BY timestamp DESC LIMIT 50"
	tailUpper := strings.ToUpper(tail)

	// Find LIMIT in the tail
	limitIdx := strings.LastIndex(tailUpper, "LIMIT")
	if limitIdx == -1 {
		return strings.TrimSpace(tail), "", base
	}

	orderBy = strings.TrimSpace(tail[:limitIdx])
	limit = strings.TrimSpace(tail[limitIdx:])
	return orderBy, limit, base
}

// handleHealth handles GET /health requests
func (n *Node) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"module": "node",
	})
}

// handleVacuum runs DuckLake maintenance: expire snapshots, merge files, cleanup
func (n *Node) handleVacuum(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if n.db == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "database not initialized",
		})
		return
	}

	// Get optional parameters
	expireOlderThan := r.URL.Query().Get("expire_older_than") // e.g., "1 hour", "1 day"
	if expireOlderThan == "" {
		expireOlderThan = "1 hour" // default: expire snapshots older than 1 hour
	}

	var allResults []map[string]interface{}
	start := time.Now()

	// Run maintenance on all volumes
	for _, vol := range n.volumes {
		volResults := n.vacuumVolume(vol.LakeName, expireOlderThan)
		allResults = append(allResults, map[string]interface{}{
			"volume":  vol.Name,
			"lake":    vol.LakeName,
			"results": volResults,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":           true,
		"volumes_count":     len(n.volumes),
		"expire_older_than": expireOlderThan,
		"duration_ms":       time.Since(start).Milliseconds(),
		"results":           allResults,
	})
}

// vacuumVolume runs maintenance on a single volume
func (n *Node) vacuumVolume(lakeName, expireOlderThan string) []map[string]interface{} {
	var results []map[string]interface{}

	// 1. Expire old snapshots
	expireSQL := fmt.Sprintf("CALL ducklake_expire_snapshots('%s', older_than => INTERVAL '%s')", lakeName, expireOlderThan)
	if _, err := n.db.Exec(expireSQL); err != nil {
		slog.Warn("ducklake_expire_snapshots failed", "lake", lakeName, "error", err)
		results = append(results, map[string]interface{}{"step": "expire_snapshots", "error": err.Error()})
	} else {
		results = append(results, map[string]interface{}{"step": "expire_snapshots", "status": "ok"})
	}

	// 2. Merge adjacent small files
	mergeSQL := fmt.Sprintf("CALL ducklake_merge_adjacent_files('%s')", lakeName)
	if _, err := n.db.Exec(mergeSQL); err != nil {
		slog.Warn("ducklake_merge_adjacent_files failed", "lake", lakeName, "error", err)
		results = append(results, map[string]interface{}{"step": "merge_adjacent_files", "error": err.Error()})
	} else {
		results = append(results, map[string]interface{}{"step": "merge_adjacent_files", "status": "ok"})
	}

	// 3. Cleanup old files
	cleanupSQL := fmt.Sprintf("CALL ducklake_cleanup_old_files('%s', cleanup_all => true)", lakeName)
	if _, err := n.db.Exec(cleanupSQL); err != nil {
		slog.Warn("ducklake_cleanup_old_files failed", "lake", lakeName, "error", err)
		results = append(results, map[string]interface{}{"step": "cleanup_old_files", "error": err.Error()})
	} else {
		results = append(results, map[string]interface{}{"step": "cleanup_old_files", "status": "ok"})
	}

	// 4. Delete orphaned files
	orphanSQL := fmt.Sprintf("CALL ducklake_delete_orphaned_files('%s', cleanup_all => true)", lakeName)
	if _, err := n.db.Exec(orphanSQL); err != nil {
		slog.Warn("ducklake_delete_orphaned_files failed", "lake", lakeName, "error", err)
		results = append(results, map[string]interface{}{"step": "delete_orphaned_files", "error": err.Error()})
	} else {
		results = append(results, map[string]interface{}{"step": "delete_orphaned_files", "status": "ok"})
	}

	// 5. Remove empty directories left after file deletion.
	// Find the volume's data path from n.volumes.
	for _, vol := range n.volumes {
		if vol.LakeName == lakeName && vol.Path != "" {
			removed := removeEmptyDirs(filepath.Join(vol.Path, "main"))
			results = append(results, map[string]interface{}{
				"step":         "cleanup_empty_dirs",
				"status":       "ok",
				"dirs_removed": removed,
			})
			break
		}
	}

	return results
}

// removeEmptyDirs removes empty leaf directories under root (bottom-up).
// Returns the number of directories removed.
func removeEmptyDirs(root string) int {
	removed := 0

	var dirs []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() || path == root {
			return nil
		}
		dirs = append(dirs, path)
		return nil
	})

	// Process deepest first so parents are evaluated after children.
	for i := len(dirs) - 1; i >= 0; i-- {
		entries, err := os.ReadDir(dirs[i])
		if err != nil {
			continue
		}
		if len(entries) == 0 {
			if err := os.Remove(dirs[i]); err == nil {
				removed++
			}
		}
	}

	return removed
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// SetSharedDB sets an external DuckDB connection (from the writer module).
// When set, handleQuery uses this connection instead of the node's own DB,
// ensuring that newly flushed data is immediately visible to queries.
func (n *Node) SetSharedDB(db *sql.DB) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sharedDB = db
	logger.Info("Node: using shared DuckDB connection from writer module")
}

// SetTieredQueryDB sets the TieredStorageManager DuckDB used for hot/cold
// catalogs when running alongside the writer. Optional; when nil, only the
// shared writer connection is queried.
func (n *Node) SetTieredQueryDB(db *sql.DB) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.tieredQueryDB = db
	if db != nil {
		logger.Info("Node: tiered storage DuckDB wired for merged search (hot+cold)")
	}
}

// queryDB returns the best DuckDB connection for queries.
// Prefers the shared writer DB (real-time visibility) over the node's own DB.
func (n *Node) queryDB() *sql.DB {
	if n.sharedDB != nil {
		return n.sharedDB
	}
	return n.db
}

// IsRunning returns true if the node is running
func (n *Node) IsRunning() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.running
}

// GetListenAddr returns the actual listen address
func (n *Node) GetListenAddr() string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.listener != nil {
		return n.listener.Addr().String()
	}
	return fmt.Sprintf("%s:%d", n.config.FlightServer.Host, n.config.FlightServer.Port)
}

// configureDuckLake configures DuckDB to use DuckLake extension
// Returns list of attached volumes for tiered storage support
func configureDuckLake(db *sql.DB, cfg *config.NodeConfig) ([]VolumeInfo, error) {
	// Apply DuckDB engine tuning before loading extensions so the
	// reader-side process honours the operator-configured memory cap
	// even during DuckLake bring-up. Same knobs as the writer side.
	// When the operator hasn't set tuning, apply sensible defaults
	// so DuckDB doesn't oversubscribe the host.
	nodeThreads := cfg.DuckLake.Tuning.Threads
	nodeMemLimit := cfg.DuckLake.Tuning.MemoryLimit
	if nodeThreads == 0 {
		nodeThreads = runtime.NumCPU() / 2
		if nodeThreads < 1 {
			nodeThreads = 1
		}
		if nodeThreads > 4 {
			nodeThreads = 4
		}
	}
	if strings.TrimSpace(nodeMemLimit) == "" {
		nodeMemLimit = "2GB"
	}
	// Same rationale as the writer: the node DB is in-memory, and in-memory
	// DuckDB cannot spill to disk without an explicit temp_directory —
	// long-range searches then die with Out of Memory at memory_limit.
	nodeTempDir := cfg.DuckLake.Tuning.TempDirectory
	if strings.TrimSpace(nodeTempDir) == "" {
		nodeTempDir = ducklake.DefaultSpillDirectory(cfg.DuckLake.CatalogPath)
	}
	ducklake.ApplyDuckDBTuning(
		db,
		nodeThreads,
		nodeMemLimit,
		nodeTempDir,
		"node",
	)
	ducklake.ApplyDuckDBMemorySafety(db, "node")

	// Install and load DuckLake extension
	_, err := db.Exec("INSTALL ducklake; LOAD ducklake;")
	if err != nil {
		return nil, fmt.Errorf("failed to install DuckLake: %w", err)
	}

	// Global S3 client (same rules as writer): path-style + trimmed endpoint for
	// s3:// default volume and any path that does not use per-volume secrets.
	if err := ducklake.ApplyDuckDBS3ClientSettings(db,
		cfg.DuckLake.S3.Region,
		cfg.DuckLake.S3.AccessKeyID,
		cfg.DuckLake.S3.SecretAccessKey,
		cfg.DuckLake.S3.Endpoint,
		cfg.DuckLake.S3.UseSSL,
	); err != nil {
		return nil, fmt.Errorf("failed to configure S3: %w", err)
	}

	// Get volumes config
	volumeConfigs := cfg.DuckLake.Volumes
	if len(volumeConfigs) == 0 {
		return nil, fmt.Errorf("no volumes configured in node.ducklake.volumes")
	}

	var volumes []VolumeInfo
	for _, vol := range volumeConfigs {
		volInfo, err := attachVolume(db, cfg.DuckLake.LakeName, vol)
		if err != nil {
			logger.Warn(fmt.Sprintf("Node: Failed to attach volume %s: %v", vol.Name, err))
			continue
		}
		volumes = append(volumes, volInfo)
		logger.Info("Node: Volume attached", "name", vol.Name, "lake", volInfo.LakeName, "path", vol.Path)
	}

	if len(volumes) == 0 {
		return nil, fmt.Errorf("no volumes could be attached")
	}

	// Verify attached catalogs are visible
	rows, err := db.Query("SELECT catalog_name FROM information_schema.schemata WHERE catalog_name != 'memory' AND catalog_name != 'system' AND catalog_name != 'temp'")
	if err == nil {
		defer rows.Close()
		var catalogs []string
		for rows.Next() {
			var name string
			if rows.Scan(&name) == nil {
				catalogs = append(catalogs, name)
			}
		}
		logger.Info("Node: DuckDB attached catalogs", "catalogs", fmt.Sprintf("%v", catalogs))
	}

	logger.Info("Node: volumes attached", "count", len(volumes))
	return volumes, nil
}

// attachVolume attaches a single storage volume as DuckLake
func attachVolume(db *sql.DB, baseLakeName string, vol config.VolumeConfig) (VolumeInfo, error) {
	// Use base lake name directly for "default" volume so queries
	// referencing homer_lake.main.table work without rewriting
	lakeName := baseLakeName
	if vol.Name != "default" {
		lakeName = baseLakeName + "_" + vol.Name
	}

	// Configure S3 secret for this volume if needed
	if vol.Type == "s3" && vol.S3AccessKeyID != "" {
		secretName := fmt.Sprintf("s3_secret_%s", vol.Name)

		// Drop existing secret if any
		db.Exec(fmt.Sprintf("DROP SECRET IF EXISTS %s;", secretName))

		// Build endpoint
		endpoint := vol.S3Endpoint
		if endpoint != "" {
			endpoint = strings.TrimPrefix(endpoint, "http://")
			endpoint = strings.TrimPrefix(endpoint, "https://")
		}
		region := strings.TrimSpace(vol.S3Region)
		if region == "" && endpoint != "" {
			region = "us-east-1"
		}

		// Create secret
		var createSecret string
		if endpoint != "" {
			createSecret = fmt.Sprintf(`
				CREATE SECRET %s (
					TYPE S3,
					KEY_ID '%s',
					SECRET '%s',
					REGION '%s',
					ENDPOINT '%s',
					URL_STYLE 'path',
					USE_SSL %t
				);
			`, secretName, vol.S3AccessKeyID, vol.S3SecretKey, region, endpoint, vol.S3UseSSL)
		} else {
			createSecret = fmt.Sprintf(`
				CREATE SECRET %s (
					TYPE S3,
					KEY_ID '%s',
					SECRET '%s',
					REGION '%s'
				);
			`, secretName, vol.S3AccessKeyID, vol.S3SecretKey, region)
		}

		if _, err := db.Exec(createSecret); err != nil {
			return VolumeInfo{}, fmt.Errorf("failed to create S3 secret: %w", err)
		}
	}

	// Use catalog_path from volume config
	catalogPath := vol.CatalogPath
	if catalogPath == "" {
		return VolumeInfo{}, fmt.Errorf("catalog_path is required for volume %s", vol.Name)
	}

	// Determine catalog type (from volume config or default to sqlite)
	catalogType := vol.CatalogType
	if catalogType == "" {
		catalogType = "sqlite"
	}
	if _, err := ducklake.NormalizeSQLiteCatalog(ducklake.CatalogType(catalogType)); err != nil {
		return VolumeInfo{}, fmt.Errorf("volume %s: %w", vol.Name, err)
	}

	// Enable WAL mode for SQLite catalog
	if err := ducklake.EnableSQLiteWALMode(catalogPath); err != nil {
		logger.Warn(fmt.Sprintf("Failed to enable WAL mode for catalog %s: %v", catalogPath, err))
	}

	// Build attach statement (SQLite catalog only)
	overrideOpt := ""
	if vol.OverrideDataPath {
		overrideOpt = ", OVERRIDE_DATA_PATH TRUE"
	}
	attachSQL := fmt.Sprintf(
		"ATTACH 'ducklake:sqlite:%s' AS %s (DATA_PATH '%s', AUTOMATIC_MIGRATION TRUE%s);",
		catalogPath, lakeName, vol.Path, overrideOpt,
	)
	if _, err := db.Exec(attachSQL); err != nil {
		return VolumeInfo{}, fmt.Errorf("failed to attach: %w", err)
	}

	return VolumeInfo{
		Name:     vol.Name,
		LakeName: lakeName,
		Path:     vol.Path,
	}, nil
}

// GetStats returns node statistics
func (n *Node) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})
	stats["running"] = n.IsRunning()
	stats["address"] = n.GetListenAddr()
	stats["volumes_count"] = len(n.volumes)

	// Volume info
	volumeNames := make([]string, len(n.volumes))
	for i, v := range n.volumes {
		volumeNames[i] = v.Name
	}
	stats["volumes"] = volumeNames

	if n.catalog != nil {
		stats["catalog"] = n.catalog.GetStats()
	}

	return stats
}
