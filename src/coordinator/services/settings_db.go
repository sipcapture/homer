// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package services

import (
	"database/sql"
	"fmt"
)

// OpenSettingsDB opens the DuckDB file for users/settings.
func OpenSettingsDB(path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("settings_db_path is empty")
	}
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// EnsureSettingsSchema creates tables for users/settings if missing.
func EnsureSettingsSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("settings db not available")
	}
	statements := []string{
		`CREATE SEQUENCE IF NOT EXISTS users_id_seq`,
		`CREATE TABLE IF NOT EXISTS users (
			id BIGINT PRIMARY KEY DEFAULT nextval('users_id_seq'),
			username VARCHAR,
			password_hash VARCHAR,
			email VARCHAR,
			full_name VARCHAR,
			is_admin BOOLEAN,
			is_active BOOLEAN,
			created_at TIMESTAMP,
			updated_at TIMESTAMP
		)`,
		fmt.Sprintf(`CREATE SEQUENCE IF NOT EXISTS %s`, userPreferencesSeq),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id BIGINT PRIMARY KEY DEFAULT nextval('%s'),
			guid VARCHAR,
			username VARCHAR,
			partid INTEGER,
			category VARCHAR,
			param VARCHAR,
			data JSON,
			create_date TIMESTAMP
		)`, UserPreferencesTable, userPreferencesSeq),
		`CREATE SEQUENCE IF NOT EXISTS global_settings_id_seq`,
		`CREATE TABLE IF NOT EXISTS global_settings (
			id BIGINT PRIMARY KEY DEFAULT nextval('global_settings_id_seq'),
			guid VARCHAR,
			partid INTEGER,
			category VARCHAR,
			param VARCHAR,
			data JSON,
			create_date TIMESTAMP
		)`,
		`CREATE SEQUENCE IF NOT EXISTS dashboard_settings_id_seq`,
		`CREATE TABLE IF NOT EXISTS dashboard_settings (
			id BIGINT PRIMARY KEY DEFAULT nextval('dashboard_settings_id_seq'),
			guid VARCHAR UNIQUE,
			username VARCHAR,
			partid INTEGER,
			dashboard_id VARCHAR,
			data JSON,
			create_date TIMESTAMP
		)`,
		`CREATE SEQUENCE IF NOT EXISTS user_mapping_settings_id_seq`,
		`CREATE TABLE IF NOT EXISTS user_mapping_settings (
			id BIGINT PRIMARY KEY DEFAULT nextval('user_mapping_settings_id_seq'),
			guid VARCHAR UNIQUE,
			username VARCHAR,
			partid INTEGER,
			mapping_key VARCHAR,
			data JSON,
			create_date TIMESTAMP
		)`,
		`CREATE SEQUENCE IF NOT EXISTS dashboard_alerts_id_seq`,
		`CREATE TABLE IF NOT EXISTS dashboard_alerts (
			id BIGINT PRIMARY KEY DEFAULT nextval('dashboard_alerts_id_seq'),
			severity VARCHAR,
			title VARCHAR,
			message VARCHAR,
			payload JSON,
			created_at TIMESTAMP DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS export_share_links (
			id VARCHAR PRIMARY KEY,
			owner_username VARCHAR NOT NULL,
			payload VARCHAR NOT NULL,
			created_at TIMESTAMP DEFAULT current_timestamp,
			expires_at TIMESTAMP NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS transaction_view_tokens (
			id VARCHAR PRIMARY KEY,
			owner_username VARCHAR NOT NULL,
			payload VARCHAR NOT NULL,
			created_at TIMESTAMP DEFAULT current_timestamp,
			expires_at TIMESTAMP NOT NULL,
			max_opens INTEGER NOT NULL DEFAULT 3,
			open_count INTEGER NOT NULL DEFAULT 0
		)`,
		// Coordinator metadata (mapping, aliases, static API tokens, HEP subs, agent sessions).
		// Canonical DDL — keep in sync with MappingService / AliasService / etc.
		`CREATE TABLE IF NOT EXISTS mapping_schema (
			guid VARCHAR,
			profile VARCHAR,
			hepid INTEGER,
			hep_alias VARCHAR,
			partid INTEGER,
			version INTEGER,
			retention INTEGER,
			partition_step INTEGER,
			create_index JSON,
			create_table VARCHAR,
			correlation_mapping JSON,
			fields_mapping JSON,
			mapping_settings JSON,
			schema_mapping JSON,
			schema_settings JSON,
			create_date TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS hepsub_mapping_schema (
			guid VARCHAR,
			profile VARCHAR,
			hepid INTEGER,
			hep_alias VARCHAR,
			version INTEGER,
			mapping JSON,
			create_date TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS alias (
			guid VARCHAR,
			alias VARCHAR,
			ip VARCHAR,
			port INTEGER,
			mask INTEGER,
			capture_id VARCHAR,
			status INTEGER,
			custom_image VARCHAR,
			tag1 VARCHAR,
			tag2 VARCHAR,
			tag3 VARCHAR,
			tag4 VARCHAR,
			create_date TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS auth_token (
			guid VARCHAR,
			creator_guid VARCHAR,
			name VARCHAR,
			token VARCHAR,
			user_object JSON,
			ip_address VARCHAR,
			create_date TIMESTAMP,
			lastusage_date VARCHAR,
			expire_date VARCHAR,
			usage_calls INTEGER,
			limit_calls INTEGER,
			active INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS agent_location_session (
			guid VARCHAR,
			gid INTEGER,
			host VARCHAR,
			port INTEGER,
			protocol VARCHAR,
			path VARCHAR,
			node VARCHAR,
			type VARCHAR,
			create_date TIMESTAMP,
			expire_date VARCHAR,
			active INTEGER
		)`,
	}
	for _, sqlStmt := range statements {
		if _, err := db.Exec(sqlStmt); err != nil {
			return err
		}
	}

	scriptsDDL := []string{
		fmt.Sprintf(`CREATE SEQUENCE IF NOT EXISTS %s`, correlationScriptsSeq),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id BIGINT PRIMARY KEY DEFAULT nextval('%s'),
			guid VARCHAR,
			profile VARCHAR,
			hep_alias VARCHAR,
			type VARCHAR,
			hepid INTEGER,
			status BOOLEAN,
			script VARCHAR,
			create_date TIMESTAMP
		)`, CorrelationScriptsTable, correlationScriptsSeq),
	}
	for _, sqlStmt := range scriptsDDL {
		if _, err := db.Exec(sqlStmt); err != nil {
			return err
		}
	}

	// Existing installs: add optional display columns (custom image + free-form tags).
	for _, sqlStmt := range settingsAliasOptionalColumns() {
		if _, err := db.Exec(sqlStmt); err != nil {
			return err
		}
	}

	// Existing installs: users.full_name (display name).
	for _, sqlStmt := range settingsUsersOptionalColumns() {
		if _, err := db.Exec(sqlStmt); err != nil {
			return err
		}
	}

	return nil
}

// settingsUsersOptionalColumns migrates legacy users tables forward.
func settingsUsersOptionalColumns() []string {
	return []string{
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS full_name VARCHAR`,
	}
}

// settingsAliasOptionalColumns migrates legacy alias tables forward.
func settingsAliasOptionalColumns() []string {
	return []string{
		`ALTER TABLE alias ADD COLUMN IF NOT EXISTS custom_image VARCHAR`,
		`ALTER TABLE alias ADD COLUMN IF NOT EXISTS tag1 VARCHAR`,
		`ALTER TABLE alias ADD COLUMN IF NOT EXISTS tag2 VARCHAR`,
		`ALTER TABLE alias ADD COLUMN IF NOT EXISTS tag3 VARCHAR`,
		`ALTER TABLE alias ADD COLUMN IF NOT EXISTS tag4 VARCHAR`,
	}
}
