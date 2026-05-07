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
	"time"

	"github.com/sipcapture/homer-core/src/coordinator/sqlvalidator"
)

// AuthTokenItem represents auth_token row
type AuthTokenItem struct {
	GUID          string          `json:"guid"`
	CreatorGUID   string          `json:"creator_guid"`
	Name          string          `json:"name"`
	UserObject    json.RawMessage `json:"user_object"`
	IPAddress     string          `json:"ip_address"`
	CreateDate    string          `json:"create_date"`
	LastUsageDate string          `json:"lastusage_date"`
	ExpireDate    string          `json:"expire_date"`
	UsageCalls    int             `json:"usage_calls"`
	LimitCalls    int             `json:"limit_calls"`
	Active        bool            `json:"active"`
	Token         string          `json:"-"`
}

type AuthTokenListFilters struct {
	Name   string
	Active *bool
	Limit  int
	Sort   string
}

// AuthTokenService provides access to auth_token
type AuthTokenService struct {
	db *sql.DB
}

func NewAuthTokenService(db *sql.DB) *AuthTokenService {
	return &AuthTokenService{db: db}
}

// LookupValidAPIAccessToken resolves a raw secret from the settings DuckDB auth_token table
// (same semantics as homer-app api_settings + Auth-Token header). Returns (nil, nil) if not found, inactive, expired,
// or over call limit; returns (nil, err) only on query failure.
func (s *AuthTokenService) LookupValidAPIAccessToken(ctx context.Context, raw string) (*AuthTokenItem, error) {
	if s.db == nil {
		return nil, nil
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	sql := fmt.Sprintf(
		`SELECT guid, creator_guid, name, token, user_object, ip_address, create_date,
		        lastusage_date, expire_date, usage_calls, limit_calls, active
		 FROM auth_token WHERE token = '%s'`,
		escapeSQL(raw),
	)
	rows, err := settingsDBQuery(ctx, s.db, sql)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	item := mapRowToAuthToken(rows[0])
	if v, ok := rows[0]["token"].(string); ok {
		item.Token = v
	}
	if !item.Active {
		return nil, nil
	}
	exp, err := parseAuthTokenExpire(item.ExpireDate)
	if err != nil {
		return nil, nil
	}
	if time.Now().After(exp) {
		return nil, nil
	}
	if item.LimitCalls > 0 && item.UsageCalls >= item.LimitCalls {
		return nil, nil
	}
	return &item, nil
}

func parseAuthTokenExpire(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty expire")
	}
	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.999999Z",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparsed expire_date")
}

func (s *AuthTokenService) List(ctx context.Context, filters AuthTokenListFilters) ([]AuthTokenItem, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	where := []string{"1=1"}
	if filters.Name != "" {
		value := "%" + sqlvalidator.SafeString(filters.Name) + "%"
		where = append(where, fmt.Sprintf("name ILIKE '%s'", value))
	}
	if filters.Active != nil {
		val := 0
		if *filters.Active {
			val = 1
		}
		where = append(where, fmt.Sprintf("active = %d", val))
	}
	limit := filters.Limit
	if limit <= 0 {
		limit = 100
	}
	sortExpr := filters.Sort
	if sortExpr == "" {
		sortExpr = "create_date DESC"
	}
	sql := fmt.Sprintf(
		`SELECT guid, creator_guid, name, user_object, ip_address, create_date,
		        lastusage_date, expire_date, usage_calls, limit_calls, active
		 FROM auth_token
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
	items := make([]AuthTokenItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapRowToAuthToken(row))
	}
	return items, nil
}

func (s *AuthTokenService) GetByGUID(ctx context.Context, guid string) (*AuthTokenItem, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	sql := fmt.Sprintf(
		`SELECT guid, creator_guid, name, user_object, ip_address, create_date,
		        lastusage_date, expire_date, usage_calls, limit_calls, active
		 FROM auth_token
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
	item := mapRowToAuthToken(rows[0])
	return &item, nil
}

func (s *AuthTokenService) Create(ctx context.Context, item AuthTokenItem) (AuthTokenItem, error) {
	if s.db == nil {
		return AuthTokenItem{}, fmt.Errorf("settings db not available")
	}
	if item.GUID == "" {
		item.GUID = newGUID()
	}
	if item.Token == "" {
		item.Token = newToken()
	}
	if item.UserObject == nil {
		item.UserObject = json.RawMessage(`{}`)
	}
	if item.LimitCalls <= 0 {
		item.LimitCalls = 1000
	}
	if item.ExpireDate == "" {
		item.ExpireDate = time.Now().Add(365 * 24 * time.Hour).Format(time.RFC3339)
	}
	if item.LastUsageDate == "" {
		item.LastUsageDate = time.Now().Format(time.RFC3339)
	}
	status := 0
	if item.Active {
		status = 1
	}

	sql := fmt.Sprintf(
		`INSERT INTO auth_token (
			 guid, creator_guid, name, token, user_object, ip_address,
			 create_date, lastusage_date, expire_date, usage_calls, limit_calls, active
		 )
		 VALUES (
			 '%s', '%s', '%s', '%s', '%s', '%s',
			 current_timestamp, '%s', '%s', %d, %d, %d
		 )`,
		escapeSQL(item.GUID),
		escapeSQL(item.CreatorGUID),
		escapeSQL(item.Name),
		escapeSQL(item.Token),
		escapeJSONData(string(item.UserObject)),
		escapeSQL(item.IPAddress),
		escapeSQL(item.LastUsageDate),
		escapeSQL(item.ExpireDate),
		item.UsageCalls,
		item.LimitCalls,
		status,
	)
	if err := settingsDBExec(ctx, s.db, sql); err != nil {
		return AuthTokenItem{}, err
	}
	return item, nil
}

func (s *AuthTokenService) Update(ctx context.Context, guid string, item AuthTokenItem) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("settings db not available")
	}
	if item.UserObject == nil {
		item.UserObject = json.RawMessage(`{}`)
	}
	if item.LimitCalls <= 0 {
		item.LimitCalls = 1000
	}
	status := 0
	if item.Active {
		status = 1
	}
	sql := fmt.Sprintf(
		`UPDATE auth_token
		 SET name = '%s', expire_date = '%s', limit_calls = %d, active = %d
		 WHERE guid = '%s'`,
		escapeSQL(item.Name),
		escapeSQL(item.ExpireDate),
		item.LimitCalls,
		status,
		escapeSQL(guid),
	)
	if err := settingsDBExec(ctx, s.db, sql); err != nil {
		return "", err
	}
	return guid, nil
}

func (s *AuthTokenService) Delete(ctx context.Context, guid string) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("settings db not available")
	}
	sql := fmt.Sprintf(
		`DELETE FROM auth_token
		 WHERE guid = '%s'`,
		escapeSQL(guid),
	)
	if err := settingsDBExec(ctx, s.db, sql); err != nil {
		return false, err
	}
	return true, nil
}

func mapRowToAuthToken(row map[string]interface{}) AuthTokenItem {
	item := AuthTokenItem{}
	if v, ok := row["guid"].(string); ok {
		item.GUID = v
	}
	if v, ok := row["creator_guid"].(string); ok {
		item.CreatorGUID = v
	}
	if v, ok := row["name"].(string); ok {
		item.Name = v
	}
	if v, ok := row["user_object"]; ok {
		item.UserObject = toJSONRaw(v)
	}
	if v, ok := row["ip_address"].(string); ok {
		item.IPAddress = v
	}
	if v, ok := row["create_date"]; ok {
		item.CreateDate = toTimeString(v)
	}
	if v, ok := row["lastusage_date"]; ok {
		item.LastUsageDate = toTimeString(v)
	}
	if v, ok := row["expire_date"]; ok {
		item.ExpireDate = toTimeString(v)
	}
	if v, ok := row["usage_calls"]; ok {
		item.UsageCalls = int(toInt64(v))
	}
	if v, ok := row["limit_calls"]; ok {
		item.LimitCalls = int(toInt64(v))
	}
	if v, ok := row["active"]; ok {
		item.Active = toBoolValue(v)
	}
	return item
}

func toTimeString(value interface{}) string {
	switch v := value.(type) {
	case time.Time:
		return v.Format(time.RFC3339)
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return ""
	}
}

func newToken() string {
	return newGUID() + newGUID()
}
