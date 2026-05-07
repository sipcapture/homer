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

// MappingSchema represents mapping_schema row
type MappingSchema struct {
	GUID           string          `json:"guid"`
	Profile        string          `json:"profile"`
	HepID          int             `json:"hepid"`
	HepAlias       string          `json:"hep_alias"`
	PartID         int             `json:"partid"`
	Version        int             `json:"version"`
	Retention      int             `json:"retention"`
	PartitionStep  int             `json:"partition_step"`
	CreateIndex    json.RawMessage `json:"create_index"`
	CreateTable    string          `json:"create_table"`
	CorrelationMap json.RawMessage `json:"correlation_mapping"`
	FieldsMapping  json.RawMessage `json:"fields_mapping"`
	FieldsSettings json.RawMessage `json:"fields_settings"`
	SchemaMapping  json.RawMessage `json:"schema_mapping"`
	SchemaSettings json.RawMessage `json:"schema_settings"`
}

type MappingListFilters struct {
	Profile  string
	HepID    *int
	HepAlias string
	Limit    int
	Sort     string
}

// MappingService provides access to mapping_schema
type MappingService struct {
	db *sql.DB
}

func NewMappingService(db *sql.DB) *MappingService {
	return &MappingService{db: db}
}

func (s *MappingService) ListMappings(ctx context.Context, filters MappingListFilters) ([]MappingSchema, error) {
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
		`SELECT guid, profile, hepid, hep_alias, partid, version, retention, partition_step,
		        create_index, create_table, correlation_mapping, fields_mapping, mapping_settings,
		        schema_mapping, schema_settings
		 FROM mapping_schema
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
	items := make([]MappingSchema, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapRowToMapping(row))
	}
	return items, nil
}

func (s *MappingService) GetMappingByGUID(ctx context.Context, guid string) (*MappingSchema, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	sql := fmt.Sprintf(
		`SELECT guid, profile, hepid, hep_alias, partid, version, retention, partition_step,
		        create_index, create_table, correlation_mapping, fields_mapping, mapping_settings,
		        schema_mapping, schema_settings
		 FROM mapping_schema
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
	mapping := mapRowToMapping(rows[0])
	return &mapping, nil
}

func (s *MappingService) GetMappingByProfile(ctx context.Context, hepid int, profile string) (*MappingSchema, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	sql := fmt.Sprintf(
		`SELECT guid, profile, hepid, hep_alias, partid, version, retention, partition_step,
		        create_index, create_table, correlation_mapping, fields_mapping, mapping_settings,
		        schema_mapping, schema_settings
		 FROM mapping_schema
		 WHERE hepid = %d AND profile = '%s'
		 LIMIT 1`,
		hepid,
		escapeSQL(profile),
	)
	rows, err := settingsDBQuery(ctx, s.db, sql)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	mapping := mapRowToMapping(rows[0])
	return &mapping, nil
}

func (s *MappingService) CreateMapping(ctx context.Context, mapping MappingSchema) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("settings db not available")
	}
	if mapping.GUID == "" {
		mapping.GUID = newGUID()
	}
	if mapping.PartID <= 0 {
		mapping.PartID = 10
	}
	sql := fmt.Sprintf(
		`INSERT INTO mapping_schema (
			 guid, profile, hepid, hep_alias, partid, version, retention, partition_step,
			 create_index, create_table, correlation_mapping, fields_mapping, mapping_settings,
			 schema_mapping, schema_settings, create_date
		 )
		 VALUES (
			 '%s', '%s', %d, '%s', %d, %d, %d, %d,
			 '%s', '%s', '%s', '%s', '%s', '%s', '%s', current_timestamp
		 )`,
		escapeSQL(mapping.GUID),
		escapeSQL(mapping.Profile),
		mapping.HepID,
		escapeSQL(mapping.HepAlias),
		mapping.PartID,
		mapping.Version,
		mapping.Retention,
		mapping.PartitionStep,
		escapeJSONData(string(mapping.CreateIndex)),
		escapeSQL(mapping.CreateTable),
		escapeJSONData(string(mapping.CorrelationMap)),
		escapeJSONData(string(mapping.FieldsMapping)),
		escapeJSONData(string(mapping.FieldsSettings)),
		escapeJSONData(string(mapping.SchemaMapping)),
		escapeJSONData(string(mapping.SchemaSettings)),
	)
	if err := settingsDBExec(ctx, s.db, sql); err != nil {
		return "", err
	}
	return mapping.GUID, nil
}

func (s *MappingService) UpdateMapping(ctx context.Context, guid string, mapping MappingSchema) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("settings db not available")
	}
	if mapping.PartID <= 0 {
		mapping.PartID = 10
	}
	sql := fmt.Sprintf(
		`UPDATE mapping_schema
		 SET profile = '%s', hepid = %d, hep_alias = '%s', partid = %d, version = %d,
		     retention = %d, partition_step = %d, create_index = '%s', create_table = '%s',
		     correlation_mapping = '%s', fields_mapping = '%s', mapping_settings = '%s',
		     schema_mapping = '%s', schema_settings = '%s'
		 WHERE guid = '%s'`,
		escapeSQL(mapping.Profile),
		mapping.HepID,
		escapeSQL(mapping.HepAlias),
		mapping.PartID,
		mapping.Version,
		mapping.Retention,
		mapping.PartitionStep,
		escapeJSONData(string(mapping.CreateIndex)),
		escapeSQL(mapping.CreateTable),
		escapeJSONData(string(mapping.CorrelationMap)),
		escapeJSONData(string(mapping.FieldsMapping)),
		escapeJSONData(string(mapping.FieldsSettings)),
		escapeJSONData(string(mapping.SchemaMapping)),
		escapeJSONData(string(mapping.SchemaSettings)),
		escapeSQL(guid),
	)
	if err := settingsDBExec(ctx, s.db, sql); err != nil {
		return "", err
	}
	return guid, nil
}

func (s *MappingService) DeleteMapping(ctx context.Context, guid string) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("settings db not available")
	}
	sql := fmt.Sprintf(
		`DELETE FROM mapping_schema
		 WHERE guid = '%s'`,
		escapeSQL(guid),
	)
	if err := settingsDBExec(ctx, s.db, sql); err != nil {
		return false, err
	}
	return true, nil
}

func mapRowToMapping(row map[string]interface{}) MappingSchema {
	mapping := MappingSchema{
		PartID: 10,
	}
	if v, ok := row["guid"].(string); ok {
		mapping.GUID = v
	}
	if v, ok := row["profile"].(string); ok {
		mapping.Profile = v
	}
	if v, ok := row["hepid"]; ok {
		mapping.HepID = int(toInt64(v))
	}
	if v, ok := row["hep_alias"].(string); ok {
		mapping.HepAlias = v
	}
	if v, ok := row["partid"]; ok {
		mapping.PartID = int(toInt64(v))
	}
	if v, ok := row["version"]; ok {
		mapping.Version = int(toInt64(v))
	}
	if v, ok := row["retention"]; ok {
		mapping.Retention = int(toInt64(v))
	}
	if v, ok := row["partition_step"]; ok {
		mapping.PartitionStep = int(toInt64(v))
	}
	if v, ok := row["create_index"]; ok {
		mapping.CreateIndex = toJSONRaw(v)
	}
	if v, ok := row["create_table"].(string); ok {
		mapping.CreateTable = v
	}
	if v, ok := row["correlation_mapping"]; ok {
		mapping.CorrelationMap = toJSONRaw(v)
	}
	if v, ok := row["fields_mapping"]; ok {
		mapping.FieldsMapping = toJSONRaw(v)
	}
	if v, ok := row["mapping_settings"]; ok {
		mapping.FieldsSettings = toJSONRaw(v)
	}
	if v, ok := row["schema_mapping"]; ok {
		mapping.SchemaMapping = toJSONRaw(v)
	}
	if v, ok := row["schema_settings"]; ok {
		mapping.SchemaSettings = toJSONRaw(v)
	}
	return mapping
}
