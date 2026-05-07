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

	"github.com/sipcapture/homer-core/src/coordinator/sqlvalidator"
)

// CorrelationScriptsTable is the DuckDB table name in the settings database for
// correlation scripts and rows managed via /api/v4/scripts (formerly hep_scripts).
const CorrelationScriptsTable = "correlation_scripts"

const correlationScriptsSeq = "correlation_scripts_id_seq"

// HepScript represents a row from CorrelationScriptsTable.
type HepScript struct {
	ID       int64  `json:"id"`
	GUID     string `json:"guid"`
	Profile  string `json:"profile"`
	HepAlias string `json:"hep_alias"`
	Type     string `json:"type"`
	HepID    int    `json:"hepid"`
	Status   bool   `json:"status"`
	Script   string `json:"script"`
}

type ScriptsListFilters struct {
	Profile string
	Type    string
	Limit   int
}

// ScriptsService provides CRUD access to CorrelationScriptsTable.
type ScriptsService struct {
	db *sql.DB
}

func NewScriptsService(db *sql.DB) *ScriptsService {
	return &ScriptsService{db: db}
}

func (s *ScriptsService) List(ctx context.Context, filters ScriptsListFilters) ([]HepScript, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	where := "1=1"
	if filters.Profile != "" {
		value := "%" + sqlvalidator.SafeString(filters.Profile) + "%"
		where += fmt.Sprintf(" AND profile ILIKE '%s'", value)
	}
	if filters.Type != "" {
		value := "%" + sqlvalidator.SafeString(filters.Type) + "%"
		where += fmt.Sprintf(" AND type ILIKE '%s'", value)
	}
	limit := filters.Limit
	if limit <= 0 {
		limit = 100
	}
	query := fmt.Sprintf(
		`SELECT id, guid, profile, hep_alias, type, hepid, status, script
		 FROM %s
		 WHERE %s
		 ORDER BY id
		 LIMIT %d`,
		CorrelationScriptsTable, where, limit,
	)
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]HepScript, 0)
	for rows.Next() {
		item, err := scanHepScript(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *ScriptsService) GetByGUID(ctx context.Context, guid string) (*HepScript, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	query := fmt.Sprintf(
		`SELECT id, guid, profile, hep_alias, type, hepid, status, script
		 FROM %s WHERE guid = '%s' LIMIT 1`,
		CorrelationScriptsTable, escapeSQL(guid),
	)
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	item, err := scanHepScript(rows)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *ScriptsService) Create(ctx context.Context, item HepScript) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("settings db not available")
	}
	if item.GUID == "" {
		item.GUID = newGUID()
	}
	// Script body can be large Lua source; escapeSQL (SafeString) truncates at 1k — use long-text embedding.
	insertSQL := fmt.Sprintf(
		`INSERT INTO %s (guid, profile, hep_alias, type, hepid, status, script, create_date)
		 VALUES ('%s', '%s', '%s', '%s', %d, %t, '%s', current_timestamp)
		 RETURNING guid`,
		CorrelationScriptsTable,
		escapeSQL(item.GUID),
		escapeSQL(item.Profile),
		escapeSQL(item.HepAlias),
		escapeSQL(item.Type),
		item.HepID,
		item.Status,
		escapeJSONData(item.Script),
	)
	var guid string
	if err := s.db.QueryRowContext(ctx, insertSQL).Scan(&guid); err != nil {
		return item.GUID, err
	}
	return guid, nil
}

func (s *ScriptsService) Update(ctx context.Context, guid string, item HepScript) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("settings db not available")
	}
	updateSQL := fmt.Sprintf(
		`UPDATE %s
		 SET profile = '%s', hep_alias = '%s', type = '%s', hepid = %d, status = %t, script = '%s'
		 WHERE guid = '%s'
		 RETURNING guid`,
		CorrelationScriptsTable,
		escapeSQL(item.Profile),
		escapeSQL(item.HepAlias),
		escapeSQL(item.Type),
		item.HepID,
		item.Status,
		escapeJSONData(item.Script),
		escapeSQL(guid),
	)
	var updated string
	if err := s.db.QueryRowContext(ctx, updateSQL).Scan(&updated); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return updated, nil
}

func (s *ScriptsService) Delete(ctx context.Context, guid string) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("settings db not available")
	}
	deleteSQL := fmt.Sprintf(
		`DELETE FROM %s WHERE guid = '%s' RETURNING id`,
		CorrelationScriptsTable, escapeSQL(guid),
	)
	var id int64
	if err := s.db.QueryRowContext(ctx, deleteSQL).Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func scanHepScript(rows *sql.Rows) (HepScript, error) {
	var (
		item     HepScript
		id       sql.NullInt64
		guid     sql.NullString
		profile  sql.NullString
		hepAlias sql.NullString
		typ      sql.NullString
		hepid    sql.NullInt64
		status   sql.NullBool
		script   sql.NullString
	)
	if err := rows.Scan(&id, &guid, &profile, &hepAlias, &typ, &hepid, &status, &script); err != nil {
		return item, err
	}
	if id.Valid {
		item.ID = id.Int64
	}
	if guid.Valid {
		item.GUID = guid.String
	}
	if profile.Valid {
		item.Profile = profile.String
	}
	if hepAlias.Valid {
		item.HepAlias = hepAlias.String
	}
	if typ.Valid {
		item.Type = typ.String
	}
	if hepid.Valid {
		item.HepID = int(hepid.Int64)
	}
	if status.Valid {
		item.Status = status.Bool
	}
	if script.Valid {
		item.Script = script.String
	}
	return item, nil
}
