// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package services

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// UserPreferencesTable is the settings-DuckDB table for generic per-user rows
// served by /api/v4/me/settings (category/param JSON blobs).
const UserPreferencesTable = "user_preferences"

const userPreferencesSeq = "user_preferences_id_seq"

// UserSetting represents a user settings row
type UserSetting struct {
	ID       int64           `json:"id"`
	GUID     string          `json:"guid"`
	UserName string          `json:"user_name"`
	PartID   int             `json:"partid"`
	Category string          `json:"category"`
	Param    string          `json:"param"`
	Data     json.RawMessage `json:"data"`
}

// UserSettingsService provides access to UserPreferencesTable.
type UserSettingsService struct {
	db *sql.DB
}

func NewUserSettingsService(db *sql.DB) *UserSettingsService {
	return &UserSettingsService{db: db}
}

func (s *UserSettingsService) ListByUser(ctx context.Context, username string) ([]UserSetting, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	query := fmt.Sprintf(
		`SELECT id, guid, username, partid, category, param, data
		 FROM %s
		 WHERE username = '%s'
		 ORDER BY id`,
		UserPreferencesTable,
		escapeSQL(username),
	)
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	settings := make([]UserSetting, 0)
	for rows.Next() {
		setting, err := scanUserSettingRows(rows)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return settings, nil
}

// UpsertByCategory updates existing or inserts new setting by username+category+param
func (s *UserSettingsService) UpsertByCategory(ctx context.Context, username, category, param string, data json.RawMessage, partid int) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("settings db not available")
	}
	if partid <= 0 {
		partid = 10
	}

	dataStr := string(data)
	updateSQL := fmt.Sprintf(
		`UPDATE %s
		 SET param = '%s', data = '%s', partid = %d
		 WHERE username = '%s' AND category = '%s' AND param = '%s'
		 RETURNING guid`,
		UserPreferencesTable,
		escapeSQL(param),
		escapeJSONData(dataStr),
		partid,
		escapeSQL(username),
		escapeSQL(category),
		escapeSQL(param),
	)
	var updatedGUID string
	if err := s.db.QueryRowContext(ctx, updateSQL).Scan(&updatedGUID); err == nil {
		return updatedGUID, nil
	} else if err != sql.ErrNoRows {
		return "", err
	}

	guid := newGUID()
	insertSQL := fmt.Sprintf(
		`INSERT INTO %s (guid, username, partid, category, param, data, create_date)
		 VALUES ('%s', '%s', %d, '%s', '%s', '%s', current_timestamp)
		 RETURNING guid`,
		UserPreferencesTable,
		escapeSQL(guid),
		escapeSQL(username),
		partid,
		escapeSQL(category),
		escapeSQL(param),
		escapeJSONData(dataStr),
	)
	var inserted string
	if err := s.db.QueryRowContext(ctx, insertSQL).Scan(&inserted); err != nil {
		return guid, err
	}
	if inserted != "" {
		return inserted, nil
	}
	return guid, nil
}

// ListByCategory returns all settings for a user filtered by category
func (s *UserSettingsService) ListByCategory(ctx context.Context, username, category string) ([]UserSetting, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	query := fmt.Sprintf(
		`SELECT id, guid, username, partid, category, param, data
		 FROM %s
		 WHERE username = '%s' AND category = '%s'
		 ORDER BY id`,
		UserPreferencesTable,
		escapeSQL(username),
		escapeSQL(category),
	)
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	settings := make([]UserSetting, 0)
	for rows.Next() {
		setting, err := scanUserSettingRows(rows)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return settings, nil
}

// GetByCategoryAndParam returns a single setting for a user by category and param
func (s *UserSettingsService) GetByCategoryAndParam(ctx context.Context, username, category, param string) (*UserSetting, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	query := fmt.Sprintf(
		`SELECT id, guid, username, partid, category, param, data
		 FROM %s
		 WHERE username = '%s' AND category = '%s' AND param = '%s'
		 LIMIT 1`,
		UserPreferencesTable,
		escapeSQL(username),
		escapeSQL(category),
		escapeSQL(param),
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
	setting, err := scanUserSettingRows(rows)
	if err != nil {
		return nil, err
	}
	return &setting, nil
}

// DeleteByCategoryAndParam deletes a specific setting by username+category+param
func (s *UserSettingsService) DeleteByCategoryAndParam(ctx context.Context, username, category, param string) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("settings db not available")
	}
	query := fmt.Sprintf(
		`DELETE FROM %s
		 WHERE username = '%s' AND category = '%s' AND param = '%s'
		 RETURNING id`,
		UserPreferencesTable,
		escapeSQL(username),
		escapeSQL(category),
		escapeSQL(param),
	)
	var deletedID int64
	if err := s.db.QueryRowContext(ctx, query).Scan(&deletedID); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *UserSettingsService) DeleteByCategory(ctx context.Context, username, category string) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("settings db not available")
	}
	query := fmt.Sprintf(
		`DELETE FROM %s
		 WHERE username = '%s' AND category = '%s'
		 RETURNING id`,
		UserPreferencesTable,
		escapeSQL(username),
		escapeSQL(category),
	)
	var deletedID int64
	if err := s.db.QueryRowContext(ctx, query).Scan(&deletedID); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func scanUserSettingRows(rows *sql.Rows) (UserSetting, error) {
	var (
		setting  UserSetting
		id       sql.NullInt64
		guid     sql.NullString
		username sql.NullString
		partid   sql.NullInt64
		category sql.NullString
		param    sql.NullString
		data     interface{}
	)
	if err := rows.Scan(&id, &guid, &username, &partid, &category, &param, &data); err != nil {
		return setting, err
	}
	if id.Valid {
		setting.ID = id.Int64
	}
	if guid.Valid {
		setting.GUID = guid.String
	}
	if username.Valid {
		setting.UserName = username.String
	}
	if partid.Valid {
		setting.PartID = int(partid.Int64)
	} else {
		setting.PartID = 10
	}
	if category.Valid {
		setting.Category = category.String
	}
	if param.Valid {
		setting.Param = param.String
	}

	setting.Data = rawMessageFromSQLValue(data)
	return setting, nil
}

// rawMessageFromSQLValue converts DuckDB JSON column values to json.RawMessage.
func rawMessageFromSQLValue(data interface{}) json.RawMessage {
	switch v := data.(type) {
	case []byte:
		if len(v) > 0 {
			return json.RawMessage(append([]byte(nil), v...))
		}
	case string:
		if v != "" {
			return json.RawMessage(v)
		}
	case map[string]interface{}, []interface{}:
		if b, err := json.Marshal(v); err == nil {
			return json.RawMessage(b)
		}
	case nil:
	default:
		if b, err := json.Marshal(v); err == nil {
			return json.RawMessage(b)
		}
	}
	return nil
}

func newGUID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	hexStr := hex.EncodeToString(buf)
	return strings.Join([]string{hexStr[0:8], hexStr[8:12], hexStr[12:16], hexStr[16:20], hexStr[20:]}, "-")
}
