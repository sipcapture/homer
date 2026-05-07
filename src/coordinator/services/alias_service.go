// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package services

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sipcapture/homer-core/src/coordinator/sqlvalidator"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// AliasItem represents alias row
type AliasItem struct {
	ID          int64  `json:"id"`
	GUID        string `json:"guid"`
	Alias       string `json:"alias"`
	IP          string `json:"ip"`
	Port        int    `json:"port"`
	Mask        int    `json:"mask"`
	CaptureID   string `json:"capture_id"`
	Status      bool   `json:"status"`
	CustomImage string `json:"custom_image,omitempty"`
	Tag1        string `json:"tag1,omitempty"`
	Tag2        string `json:"tag2,omitempty"`
	Tag3        string `json:"tag3,omitempty"`
	Tag4        string `json:"tag4,omitempty"`
}

type AliasListFilters struct {
	Alias     string
	IP        string
	CaptureID string
	Limit     int
	Sort      string
}

// AliasService provides access to alias table.
// Mutations and reads use the coordinator settings DuckDB (single file).
type AliasService struct {
	db *sql.DB

	ipMapMu     sync.RWMutex
	ipMapCache  *IPAliasMap
	ipMapLoaded time.Time
	// ipMapTTL is how long a built IPAliasMap is reused for row enrichment.
	ipMapTTL time.Duration
}

// NewAliasService constructs the alias CRUD service and IP enrichment cache.
// ipAliasCacheTTL is how long CachedIPAliasMap may reuse the LPM table; if <= 0, 30s is used.
func NewAliasService(db *sql.DB, ipAliasCacheTTL time.Duration) *AliasService {
	if ipAliasCacheTTL <= 0 {
		ipAliasCacheTTL = 30 * time.Second
	}
	return &AliasService{db: db, ipMapTTL: ipAliasCacheTTL}
}

// invalidateIPAliasMapCache clears the cached LPM table (call after alias CRUD).
func (s *AliasService) invalidateIPAliasMapCache() {
	if s == nil {
		return
	}
	s.ipMapMu.Lock()
	s.ipMapCache, s.ipMapLoaded = nil, time.Time{}
	s.ipMapMu.Unlock()
}

// CachedIPAliasMap returns a shared IPAliasMap for hot paths (search enrich).
// Rebuilds from ListActive at most every ipMapTTL; errors return (nil, err).
func (s *AliasService) CachedIPAliasMap(ctx context.Context) (*IPAliasMap, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	s.ipMapMu.RLock()
	cached, at := s.ipMapCache, s.ipMapLoaded
	if cached != nil && time.Since(at) < s.ipMapTTL {
		s.ipMapMu.RUnlock()
		return cached, nil
	}
	s.ipMapMu.RUnlock()

	items, err := s.ListActive(ctx, 200000)
	if err != nil {
		return nil, err
	}
	m := NewIPAliasMap(items)

	// Surface the actual count of active alias rows the LPM table was
	// built from. Operators kept asking "are my aliases even loaded?"
	// when row enrichment seemed silent; this single line answers that
	// question directly, without needing a separate diagnostic call.
	loaded := 0
	if m != nil {
		loaded = m.Size()
	}
	logger.Info("IPAliasMap rebuilt", "active_rows", len(items), "loaded_prefixes", loaded, "ttl", s.ipMapTTL.String())

	s.ipMapMu.Lock()
	defer s.ipMapMu.Unlock()
	if s.ipMapCache != nil && time.Since(s.ipMapLoaded) < s.ipMapTTL {
		return s.ipMapCache, nil
	}
	s.ipMapCache, s.ipMapLoaded = m, time.Now()
	return m, nil
}

// ListActive returns enabled aliases only (status = 1), ordered by alias.
// Used for IP→alias enrichment on transaction rows (matches homer-app GetAllActive).
func (s *AliasService) ListActive(ctx context.Context, limit int) ([]AliasItem, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	if limit <= 0 {
		limit = 100000
	}
	sql := fmt.Sprintf(
		`SELECT guid, alias, ip, port, mask, capture_id, status, custom_image, tag1, tag2, tag3, tag4
		 FROM alias
		 WHERE status = 1
		 ORDER BY alias ASC
		 LIMIT %d`,
		limit,
	)
	rows, err := settingsDBQuery(ctx, s.db, sql)
	if err != nil {
		return nil, err
	}
	items := make([]AliasItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapRowToAlias(row))
	}
	return items, nil
}

func (s *AliasService) List(ctx context.Context, filters AliasListFilters) ([]AliasItem, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	where := []string{"1=1"}
	if filters.Alias != "" {
		value := "%" + sqlvalidator.SafeString(filters.Alias) + "%"
		where = append(where, fmt.Sprintf("alias ILIKE '%s'", value))
	}
	if filters.IP != "" {
		value := "%" + sqlvalidator.SafeString(filters.IP) + "%"
		where = append(where, fmt.Sprintf("ip ILIKE '%s'", value))
	}
	if filters.CaptureID != "" {
		value := "%" + sqlvalidator.SafeString(filters.CaptureID) + "%"
		where = append(where, fmt.Sprintf("capture_id ILIKE '%s'", value))
	}
	limit := filters.Limit
	if limit <= 0 {
		limit = 100
	}
	sortExpr := filters.Sort
	if sortExpr == "" {
		sortExpr = "alias ASC"
	}
	// Table alias (EnsureSettingsSchema) has no `id` column — only guid, alias, ip, …
	sql := fmt.Sprintf(
		`SELECT guid, alias, ip, port, mask, capture_id, status, custom_image, tag1, tag2, tag3, tag4
		 FROM alias
		 WHERE %s
		 ORDER BY %s
		 LIMIT %d`,
		strings.Join(where, " AND "),
		sortExpr,
		limit,
	)
	rows, err := settingsDBQuery(ctx, s.db, sql)
	if err != nil {
		return nil, err
	}
	items := make([]AliasItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapRowToAlias(row))
	}
	return items, nil
}

func (s *AliasService) Create(ctx context.Context, item AliasItem) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("settings db not available")
	}
	if item.GUID == "" {
		item.GUID = newGUID()
	}
	status := 0
	if item.Status {
		status = 1
	}
	sql := fmt.Sprintf(
		`INSERT INTO alias (guid, alias, ip, port, mask, capture_id, status, custom_image, tag1, tag2, tag3, tag4, create_date)
		 VALUES ('%s', '%s', '%s', %d, %d, '%s', %d, '%s', '%s', '%s', '%s', '%s', current_timestamp)`,
		escapeSQL(item.GUID),
		escapeSQL(item.Alias),
		escapeSQL(item.IP),
		item.Port,
		item.Mask,
		escapeSQL(item.CaptureID),
		status,
		escapeSQL(item.CustomImage),
		escapeSQL(item.Tag1),
		escapeSQL(item.Tag2),
		escapeSQL(item.Tag3),
		escapeSQL(item.Tag4),
	)
	if err := settingsDBExec(ctx, s.db, sql); err != nil {
		return "", err
	}
	s.invalidateIPAliasMapCache()
	return item.GUID, nil
}

func (s *AliasService) Update(ctx context.Context, guid string, item AliasItem) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("settings db not available")
	}
	status := 0
	if item.Status {
		status = 1
	}
	sql := fmt.Sprintf(
		`UPDATE alias
		 SET alias = '%s', ip = '%s', port = %d, mask = %d, capture_id = '%s', status = %d,
			 custom_image = '%s', tag1 = '%s', tag2 = '%s', tag3 = '%s', tag4 = '%s'
		 WHERE guid = '%s'`,
		escapeSQL(item.Alias),
		escapeSQL(item.IP),
		item.Port,
		item.Mask,
		escapeSQL(item.CaptureID),
		status,
		escapeSQL(item.CustomImage),
		escapeSQL(item.Tag1),
		escapeSQL(item.Tag2),
		escapeSQL(item.Tag3),
		escapeSQL(item.Tag4),
		escapeSQL(guid),
	)
	if err := settingsDBExec(ctx, s.db, sql); err != nil {
		return "", err
	}
	s.invalidateIPAliasMapCache()
	return guid, nil
}

func (s *AliasService) Delete(ctx context.Context, guid string) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("settings db not available")
	}
	sql := fmt.Sprintf(
		`DELETE FROM alias
		 WHERE guid = '%s'`,
		escapeSQL(guid),
	)
	if err := settingsDBExec(ctx, s.db, sql); err != nil {
		return false, err
	}
	s.invalidateIPAliasMapCache()
	return true, nil
}

func mapRowToAlias(row map[string]interface{}) AliasItem {
	item := AliasItem{}
	if v, ok := rowGetCI(row, "id"); ok {
		item.ID = toInt64(v)
	}
	if v, ok := rowGetStringCI(row, "guid"); ok {
		item.GUID = v
	}
	if v, ok := rowGetStringCI(row, "alias"); ok {
		item.Alias = v
	}
	if v, ok := rowGetStringCI(row, "ip"); ok {
		item.IP = v
	}
	if v, ok := rowGetCI(row, "port"); ok {
		item.Port = int(toInt64(v))
	}
	if v, ok := rowGetCI(row, "mask"); ok {
		item.Mask = int(toInt64(v))
	}
	if v, ok := rowGetStringCI(row, "capture_id"); ok {
		item.CaptureID = v
	}
	if v, ok := rowGetCI(row, "status"); ok {
		item.Status = toBoolValue(v)
	}
	if v, ok := rowGetStringCI(row, "custom_image"); ok {
		item.CustomImage = v
	}
	if v, ok := rowGetStringCI(row, "tag1"); ok {
		item.Tag1 = v
	}
	if v, ok := rowGetStringCI(row, "tag2"); ok {
		item.Tag2 = v
	}
	if v, ok := rowGetStringCI(row, "tag3"); ok {
		item.Tag3 = v
	}
	if v, ok := rowGetStringCI(row, "tag4"); ok {
		item.Tag4 = v
	}
	return item
}

// rowGetCI fetches a value with case-insensitive key (DuckDB/JSON may return "GUID" etc.)
func rowGetCI(row map[string]interface{}, want string) (interface{}, bool) {
	lw := strings.ToLower(want)
	for k, v := range row {
		if strings.ToLower(k) == lw {
			return v, true
		}
	}
	return nil, false
}

func rowGetStringCI(row map[string]interface{}, want string) (string, bool) {
	v, ok := rowGetCI(row, want)
	if !ok {
		return "", false
	}
	switch s := v.(type) {
	case string:
		return s, true
	default:
		return fmt.Sprint(v), true
	}
}

// toBoolValue coerces whatever scalar the duckdb driver decides to
// hand back for an INTEGER/BOOLEAN-shaped column into a Go bool.
//
// History: previously only int/int64/float64 were covered. The
// duckdb-go driver returns INTEGER columns as **int32** in our
// settings DB (alias.status, mapping_schema.partid, etc.), which
// silently fell through to the default case and yielded false.
// That single missing case in 11.0.122 caused the canonical
// "active_rows=1 loaded_prefixes=0" symptom: ListActive's SQL
// `WHERE status = 1` returned the row, but mapRowToAlias mapped
// Status to false, and NewIPAliasMap's `if !row.Status` skipped
// it silently (no Warn emitted because the row never reached the
// prefix-parse / empty-name branches).
//
// Cover every scalar shape the driver might emit so this never
// recurs: all signed/unsigned int widths, both float widths, and
// the canonical string forms (case-insensitive) used by JSON-style
// payloads.
func toBoolValue(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case int:
		return v != 0
	case int8:
		return v != 0
	case int16:
		return v != 0
	case int32:
		return v != 0
	case int64:
		return v != 0
	case uint:
		return v != 0
	case uint8:
		return v != 0
	case uint16:
		return v != 0
	case uint32:
		return v != 0
	case uint64:
		return v != 0
	case float32:
		return v != 0
	case float64:
		return v != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "t", "true", "y", "yes", "on":
			return true
		default:
			return false
		}
	default:
		return false
	}
}
