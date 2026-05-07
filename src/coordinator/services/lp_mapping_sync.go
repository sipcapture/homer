// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // not for security; only for stable name->UUID hashing
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// LPVirtualHepID is the synthetic hepid the Proto Search widget uses to
// address Line Protocol tables. It does not collide with the real HEP
// types we ship (1=SIP, 5=RTCP, 53=DNS, 100=LOG) or with the OTLP
// virtual hepids (200/201/202).
//
// Every dynamic lp_<measurement> table gets one mapping_schema row with
// this hepid; the table's actual identity (schema + name) is encoded
// into the `profile` column. See LPProfileFor / SplitLPProfile below
// for the encoding.
const LPVirtualHepID = 300

// LPProfileSeparator is the marker that splits "<schema>__<table>" in
// the encoded profile. Using "__" means a schema or table containing a
// single underscore (e.g. "lp_cpu") still round-trips cleanly.
const LPProfileSeparator = "__"

// LPMappingSyncService keeps the mapping_schema settings table in sync
// with the live set of lp_<measurement> tables in DuckLake.
//
// Why a sync loop and not on-demand seeding:
//
//   - LP tables are created lazily by the receiver on first ingest.
//   - Their column set evolves over time (ALTER TABLE ADD COLUMN).
//   - The Proto Search widget reads mapping_schema once at boot and
//     refreshes from the cache; users expect new measurements to
//     "just appear" without restarting either side.
//
// The loop is conservative — read-only on the data plane, idempotent
// on the settings DB, and deliberately tolerant of node disconnects
// (one query per tick, errors logged not fatal).
type LPMappingSyncService struct {
	db       *sql.DB
	flight   *FlightService
	prefix   string
	interval time.Duration

	cancel context.CancelFunc
	done   chan struct{}
}

// NewLPMappingSyncService constructs the sync service. db is the
// coordinator's settings DuckDB (where mapping_schema lives), flight
// queries the data-plane nodes via the existing /query API. prefix
// defaults to "lp_" when empty; interval defaults to 60s.
func NewLPMappingSyncService(db *sql.DB, flight *FlightService, prefix string, interval time.Duration) *LPMappingSyncService {
	if prefix == "" {
		prefix = "lp_"
	}
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &LPMappingSyncService{
		db:       db,
		flight:   flight,
		prefix:   prefix,
		interval: interval,
		done:     make(chan struct{}),
	}
}

// Start launches the sync goroutine. It runs one tick immediately so
// the UI sees current tables on the first request after coordinator
// boot; subsequent ticks fire every `interval`. Safe to call once;
// calling again is a no-op until Stop has been called.
func (s *LPMappingSyncService) Start(ctx context.Context) {
	if s == nil || s.db == nil || s.flight == nil {
		return
	}
	if s.cancel != nil {
		return
	}
	c, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	go s.loop(c)
}

// Stop signals the goroutine to exit and blocks until it has returned
// (or until 5s elapses, to keep coordinator shutdown bounded).
func (s *LPMappingSyncService) Stop() {
	if s == nil || s.cancel == nil {
		return
	}
	s.cancel()
	s.cancel = nil
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		logger.Warn("LPMappingSync: stop timed out after 5s")
	}
}

func (s *LPMappingSyncService) loop(ctx context.Context) {
	defer close(s.done)

	// First tick fires immediately. Errors are logged but never bring
	// the loop down — we'd rather retry on the next tick than leave
	// the UI permanently empty after a transient flight error.
	if err := s.SyncOnce(ctx); err != nil {
		logger.Warn("LPMappingSync: initial sync failed", "err", err.Error())
	}

	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.SyncOnce(ctx); err != nil {
				logger.Warn("LPMappingSync: tick failed", "err", err.Error())
			}
		}
	}
}

// SyncOnce performs a single discovery + upsert pass. Exposed so tests
// can drive the loop deterministically without waiting for a tick.
func (s *LPMappingSyncService) SyncOnce(ctx context.Context) error {
	tables, err := s.discoverTables(ctx)
	if err != nil {
		return fmt.Errorf("discover lp tables: %w", err)
	}
	if len(tables) == 0 {
		return nil
	}
	if err := s.attachColumns(ctx, tables); err != nil {
		return fmt.Errorf("attach columns: %w", err)
	}
	upserted := 0
	for _, t := range tables {
		// Skip tables whose schema we couldn't read — without a
		// column list we'd publish a useless empty mapping.
		if len(t.Columns) == 0 {
			continue
		}
		changed, err := s.upsertMapping(ctx, t)
		if err != nil {
			logger.Warn("LPMappingSync: upsert failed",
				"schema", t.Schema, "table", t.Name, "err", err.Error())
			continue
		}
		if changed {
			upserted++
		}
	}
	if upserted > 0 {
		logger.Info("LPMappingSync: synced",
			"discovered", len(tables), "changed", upserted, "prefix", s.prefix)
	}
	return nil
}

// discoveredLPTable mirrors handlers.LineProtoTable but lives in
// services/ to avoid an import cycle. The handler can keep its own
// shape; this struct is internal.
type discoveredLPTable struct {
	Catalog string
	Schema  string
	Name    string
	Columns []LPColumn
}

func (s *LPMappingSyncService) discoverTables(ctx context.Context) ([]discoveredLPTable, error) {
	sql := "SELECT table_catalog, table_schema, table_name FROM information_schema.tables WHERE table_name LIKE '" +
		escapeSQL(s.prefix) + "%' ORDER BY table_catalog, table_schema, table_name"
	rows, err := s.flight.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	out := make([]discoveredLPTable, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		t := discoveredLPTable{
			Catalog: stringFieldRow(r, "table_catalog"),
			Schema:  stringFieldRow(r, "table_schema"),
			Name:    stringFieldRow(r, "table_name"),
		}
		if t.Name == "" || t.Schema == "" {
			continue
		}
		// Multi-node fan-out can return the same (schema, name) pair
		// multiple times — keep only the first occurrence so
		// upsertMapping isn't called redundantly.
		key := t.Schema + "." + t.Name
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, t)
	}
	return out, nil
}

func (s *LPMappingSyncService) attachColumns(ctx context.Context, tables []discoveredLPTable) error {
	if len(tables) == 0 {
		return nil
	}
	preds := make([]string, 0, len(tables))
	for _, t := range tables {
		preds = append(preds,
			"(table_schema = '"+escapeSQL(t.Schema)+"' AND table_name = '"+escapeSQL(t.Name)+"')")
	}
	sql := "SELECT table_schema, table_name, column_name, data_type, ordinal_position " +
		"FROM information_schema.columns WHERE " + strings.Join(preds, " OR ") +
		" ORDER BY table_schema, table_name, ordinal_position"
	rows, err := s.flight.Query(ctx, sql)
	if err != nil {
		return err
	}
	idx := make(map[string]int, len(tables))
	for i, t := range tables {
		idx[t.Schema+"."+t.Name] = i
	}
	// Some flight backends return the same column row from multiple
	// nodes (replicated catalog) — track (table, column) pairs we
	// already accepted so a measurement isn't published with duplicate
	// columns in its fields_mapping JSON.
	seen := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		key := stringFieldRow(r, "table_schema") + "." + stringFieldRow(r, "table_name")
		i, ok := idx[key]
		if !ok {
			continue
		}
		col := LPColumn{
			Name:     stringFieldRow(r, "column_name"),
			DataType: stringFieldRow(r, "data_type"),
			Position: intFieldRow(r, "ordinal_position"),
		}
		dedup := key + "::" + col.Name
		if _, dup := seen[dedup]; dup {
			continue
		}
		seen[dedup] = struct{}{}
		tables[i].Columns = append(tables[i].Columns, col)
	}
	return nil
}

// upsertMapping writes (or updates) the mapping_schema row for the
// given table. Returns changed=true iff the row was inserted or its
// fields_mapping JSON differs from what is already on disk — that lets
// SyncOnce log an honest "changed" count instead of always reporting
// the full discovered set as touched.
func (s *LPMappingSyncService) upsertMapping(ctx context.Context, t discoveredLPTable) (bool, error) {
	// Stable column order — INFORMATION_SCHEMA already ORDERed by
	// ordinal_position, but harmless to enforce.
	sort.SliceStable(t.Columns, func(i, j int) bool {
		return t.Columns[i].Position < t.Columns[j].Position
	})
	fields, err := BuildLPFieldsMapping(t.Columns)
	if err != nil {
		return false, fmt.Errorf("build fields mapping: %w", err)
	}

	guid := LPMappingGUID(t.Schema, t.Name)
	profile := LPProfileFor(t.Schema, t.Name)
	hepAlias := LPHepAlias(t.Name)

	// Check whether we already have a row with this guid AND if so,
	// whether the fields_mapping payload is logically identical (we
	// never rewrite it just to update the timestamp — keeps history
	// clean).  CAST the JSON column to VARCHAR so the duckdb driver
	// hands us a string we can compare; the raw column type is JSON
	// which the driver returns as []interface{}/map[string]interface{},
	// neither of which Scan into []byte.
	var existingFields string
	row := s.db.QueryRowContext(ctx,
		`SELECT CAST(fields_mapping AS VARCHAR) FROM mapping_schema WHERE guid = '`+escapeSQL(guid)+`'`)
	switch err := row.Scan(&existingFields); {
	case err == nil:
		if jsonEqual(existingFields, string(fields)) {
			return false, nil
		}
		// UPDATE existing row, leaving other operator-curated columns
		// (retention, partition_step, etc.) untouched.
		q := `UPDATE mapping_schema SET fields_mapping = '` + escapeJSONData(string(fields)) + `'` +
			` WHERE guid = '` + escapeSQL(guid) + `'`
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return false, fmt.Errorf("update fields_mapping: %w", err)
		}
		return true, nil
	case errors.Is(err, sql.ErrNoRows):
		// Fall through to INSERT.
	default:
		return false, fmt.Errorf("lookup existing mapping: %w", err)
	}

	q := fmt.Sprintf(`INSERT INTO mapping_schema (
		guid, profile, hepid, hep_alias, partid, version, retention, partition_step,
		create_index, create_table, correlation_mapping, fields_mapping, mapping_settings,
		schema_mapping, schema_settings, create_date
	) VALUES (
		'%s', '%s', %d, '%s', 10, 1, 14, 3600,
		'{}',
		'%s',
		'%s',
		'%s',
		'%s',
		'%s',
		'%s',
		current_timestamp
	)`,
		escapeSQL(guid),
		escapeSQL(profile),
		LPVirtualHepID,
		escapeSQL(hepAlias),
		escapeSQL(defaultMappingCreateTable),
		escapeJSONData(correlationMappingEmpty),
		escapeJSONData(string(fields)),
		escapeJSONData("{}"),
		escapeJSONData("{}"),
		escapeJSONData("{}"),
	)
	if _, err := s.db.ExecContext(ctx, q); err != nil {
		return false, fmt.Errorf("insert lp mapping schema=%s table=%s: %w", t.Schema, t.Name, err)
	}
	return true, nil
}

// LPMappingGUID returns a deterministic UUIDv5-like identifier for a
// given (schema, table) pair. Stable across restarts and across nodes
// so the per-row idempotent seed/upsert logic stays correct.
func LPMappingGUID(schema, table string) string {
	h := sha1.New() //nolint:gosec
	_, _ = h.Write([]byte("lp:"))
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(schema))))
	_, _ = h.Write([]byte(":"))
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(table))))
	sum := h.Sum(nil)
	hexStr := hex.EncodeToString(sum[:16])
	// Format as canonical UUID 8-4-4-4-12.
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hexStr[0:8], hexStr[8:12], hexStr[12:16], hexStr[16:20], hexStr[20:32])
}

// LPProfileFor encodes (schema, table) into the profile string used by
// mapping_schema. The encoding survives a round-trip via
// SplitLPProfile so getTableName can recover both halves at search time.
func LPProfileFor(schema, table string) string {
	return strings.TrimSpace(strings.ToLower(schema)) +
		LPProfileSeparator +
		strings.TrimSpace(strings.ToLower(table))
}

// SplitLPProfile is the inverse of LPProfileFor. Returns (schema,
// table, ok). When the encoding is missing or malformed it returns
// ("", "", false) so the caller can fall back to a sensible default
// (typically "main" + the bare profile string).
func SplitLPProfile(profile string) (string, string, bool) {
	idx := strings.Index(profile, LPProfileSeparator)
	if idx <= 0 || idx+len(LPProfileSeparator) >= len(profile) {
		return "", "", false
	}
	schema := profile[:idx]
	table := profile[idx+len(LPProfileSeparator):]
	if schema == "" || table == "" {
		return "", "", false
	}
	return schema, table, true
}

// LPHepAlias returns the user-facing label shown in Settings →
// Mappings and the Proto Search picker for the given table name.
// The schema is intentionally omitted — most deployments use a single
// schema and the alias should read like a measurement name, not a
// fully-qualified identifier.
func LPHepAlias(table string) string {
	t := strings.ToUpper(strings.TrimSpace(table))
	return "LP_" + strings.TrimPrefix(t, "LP_")
}

// jsonEqual compares two JSON documents for semantic equality so the
// "did fields_mapping change" decision in upsertMapping is robust to
// formatting drift between what we INSERTed and what DuckDB hands
// back via CAST(... AS VARCHAR) (whitespace, key order in objects,
// integer vs float reflection, …). Falls back to byte equality when
// either side fails to parse.
func jsonEqual(a, b string) bool {
	if a == b {
		return true
	}
	var av, bv any
	if err := json.Unmarshal([]byte(a), &av); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(b), &bv); err != nil {
		return false
	}
	an, _ := json.Marshal(av)
	bn, _ := json.Marshal(bv)
	return bytes.Equal(an, bn)
}

// stringFieldRow / intFieldRow are local copies of the helpers in
// handlers/lineproto_v4.go. We don't import the handlers package from
// services to keep the dependency direction clean
// (handlers → services → config).
func stringFieldRow(row map[string]interface{}, key string) string {
	v, ok := row[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func intFieldRow(row map[string]interface{}, key string) int {
	v, ok := row[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	var i int
	_, _ = fmt.Sscanf(fmt.Sprint(v), "%d", &i)
	return i
}
