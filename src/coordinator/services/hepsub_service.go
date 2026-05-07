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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sipcapture/homer-core/src/coordinator/sqlvalidator"
)

// HepsubSchema represents hepsub_mapping_schema row
type HepsubSchema struct {
	GUID     string          `json:"guid"`
	Profile  string          `json:"profile"`
	HepID    int             `json:"hepid"`
	HepAlias string          `json:"hep_alias"`
	Version  int             `json:"version"`
	Mapping  json.RawMessage `json:"mapping"`
}

type HepsubListFilters struct {
	Profile  string
	HepID    *int
	HepAlias string
	Limit    int
	Sort     string
}

// HepsubService provides access to hepsub_mapping_schema
type HepsubService struct {
	db *sql.DB
}

func NewHepsubService(db *sql.DB) *HepsubService {
	return &HepsubService{db: db}
}

func (s *HepsubService) List(ctx context.Context, filters HepsubListFilters) ([]HepsubSchema, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	where := []string{"1=1"}
	if filters.Profile != "" {
		value := "%" + sqlvalidator.SafeString(filters.Profile) + "%"
		where = append(where, fmt.Sprintf("profile ILIKE '%s'", value))
	}
	if filters.HepID != nil {
		where = append(where, fmt.Sprintf("hepid = %d", *filters.HepID))
	}
	if filters.HepAlias != "" {
		value := "%" + sqlvalidator.SafeString(filters.HepAlias) + "%"
		where = append(where, fmt.Sprintf("hep_alias ILIKE '%s'", value))
	}
	limit := filters.Limit
	if limit <= 0 {
		limit = 100
	}
	sortExpr := filters.Sort
	if sortExpr == "" {
		sortExpr = "profile ASC"
	}
	sql := fmt.Sprintf(
		`SELECT guid, profile, hepid, hep_alias, version, mapping
		 FROM hepsub_mapping_schema
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
	items := make([]HepsubSchema, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapRowToHepsub(row))
	}
	return items, nil
}

func (s *HepsubService) GetByGUID(ctx context.Context, guid string) (*HepsubSchema, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	sql := fmt.Sprintf(
		`SELECT guid, profile, hepid, hep_alias, version, mapping
		 FROM hepsub_mapping_schema
		 WHERE guid = '%s'`,
		escapeSQL(guid),
	)
	rows, err := settingsDBQuery(ctx, s.db, sql)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	item := mapRowToHepsub(rows[0])
	return &item, nil
}

func (s *HepsubService) Create(ctx context.Context, item HepsubSchema) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("settings db not available")
	}
	if item.GUID == "" {
		item.GUID = newGUID()
	}
	sql := fmt.Sprintf(
		`INSERT INTO hepsub_mapping_schema (guid, profile, hepid, hep_alias, version, mapping, create_date)
		 VALUES ('%s', '%s', %d, '%s', %d, '%s', current_timestamp)`,
		escapeSQL(item.GUID),
		escapeSQL(item.Profile),
		item.HepID,
		escapeSQL(item.HepAlias),
		item.Version,
		escapeJSONData(string(item.Mapping)),
	)
	if err := settingsDBExec(ctx, s.db, sql); err != nil {
		return "", err
	}
	return item.GUID, nil
}

func (s *HepsubService) Update(ctx context.Context, guid string, item HepsubSchema) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("settings db not available")
	}
	sql := fmt.Sprintf(
		`UPDATE hepsub_mapping_schema
		 SET profile = '%s', hepid = %d, hep_alias = '%s', version = %d, mapping = '%s'
		 WHERE guid = '%s'`,
		escapeSQL(item.Profile),
		item.HepID,
		escapeSQL(item.HepAlias),
		item.Version,
		escapeJSONData(string(item.Mapping)),
		escapeSQL(guid),
	)
	if err := settingsDBExec(ctx, s.db, sql); err != nil {
		return "", err
	}
	return guid, nil
}

func (s *HepsubService) Delete(ctx context.Context, guid string) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("settings db not available")
	}
	sql := fmt.Sprintf(
		`DELETE FROM hepsub_mapping_schema
		 WHERE guid = '%s'`,
		escapeSQL(guid),
	)
	if err := settingsDBExec(ctx, s.db, sql); err != nil {
		return false, err
	}
	return true, nil
}

func mapRowToHepsub(row map[string]interface{}) HepsubSchema {
	item := HepsubSchema{}
	if v, ok := row["guid"].(string); ok {
		item.GUID = v
	}
	if v, ok := row["profile"].(string); ok {
		item.Profile = v
	}
	if v, ok := row["hepid"]; ok {
		item.HepID = int(toInt64(v))
	}
	if v, ok := row["hep_alias"].(string); ok {
		item.HepAlias = v
	}
	if v, ok := row["version"]; ok {
		item.Version = int(toInt64(v))
	}
	if v, ok := row["mapping"]; ok {
		item.Mapping = toJSONRaw(v)
	}
	return item
}
