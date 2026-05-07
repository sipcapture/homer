// Copyright (C) 2026 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

// RunMigrateSettings copies settings tables from a homer-app PostgreSQL
// database into the homer-core DuckDB settings file.
//
// Mapping (homer-app → homer-core):
//
//	users                   → users           (preserve bcrypt password_hash)
//	global_settings         → global_settings
//	user_settings           → user_preferences
//	dashboard_settings      → dashboard_settings   (meta only; widget body wiped)
//	mapping_schema          → mapping_schema       (only with --with-mappings)
//	hepsub_mapping_schema   → hepsub_mapping_schema
//	alias                   → alias
//	auth_token              → auth_token
//	agent_location_session  → agent_location_session
func RunMigrateSettings(f MigrateFlags) error {
	duckPath, err := resolveSettingsDBPath(f)
	if err != nil {
		return err
	}

	pg, err := sql.Open("pgx", f.PgDSN)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer pg.Close()

	if err := pg.Ping(); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}

	duck, err := services.OpenSettingsDB(duckPath)
	if err != nil {
		return fmt.Errorf("open duckdb settings: %w", err)
	}
	defer duck.Close()

	if err := services.EnsureSettingsSchema(duck); err != nil {
		return fmt.Errorf("ensure duckdb schema: %w", err)
	}

	ctx := context.Background()
	stats := newMigrateStats()
	steps := []struct {
		name string
		fn   func(ctx context.Context, pg, duck *sql.DB, f MigrateFlags, s *migrateStats) error
	}{
		{"users", migrateUsers},
		{"global_settings", migrateGlobalSettings},
		{"user_preferences", migrateUserPreferences},
		{"dashboards", migrateDashboards},
		{"mapping_schema", migrateMappingSchema},
		{"hepsub_mapping_schema", migrateHEPSub},
		{"alias", migrateAlias},
		{"auth_token", migrateAuthToken},
		{"agent_location_session", migrateAgentSessions},
	}

	for _, step := range steps {
		if err := step.fn(ctx, pg, duck, f, stats); err != nil {
			return fmt.Errorf("migrate %s: %w", step.name, err)
		}
	}

	stats.report(f.DryRun)
	return nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func resolveSettingsDBPath(f MigrateFlags) (string, error) {
	if f.DuckDBPath != "" {
		return f.DuckDBPath, nil
	}
	cfg, err := config.Load(f.ConfigPath)
	if err != nil {
		return "", fmt.Errorf("--duckdb is unset and could not load config: %w", err)
	}
	if cfg.Coordinator.SettingsDBPath == "" {
		return "", fmt.Errorf("config has empty coordinator.settings_db_path; pass --duckdb explicitly")
	}
	return cfg.Coordinator.SettingsDBPath, nil
}

type migrateStats struct {
	tables map[string]struct{ scanned, inserted, skipped int }
}

func newMigrateStats() *migrateStats {
	return &migrateStats{tables: make(map[string]struct{ scanned, inserted, skipped int })}
}

func (s *migrateStats) bump(table string, scanned, inserted, skipped int) {
	cur := s.tables[table]
	cur.scanned += scanned
	cur.inserted += inserted
	cur.skipped += skipped
	s.tables[table] = cur
}

func (s *migrateStats) report(dryRun bool) {
	verb := "wrote"
	if dryRun {
		verb = "would write"
	}
	log.Printf("=== homer7 → homer11 settings migration summary ===")
	for name, c := range s.tables {
		log.Printf("  %-25s scanned=%d %s=%d skipped=%d", name, c.scanned, verb, c.inserted, c.skipped)
	}
}

// pgTableExists returns true if `<table>` is present in the source database.
// homer-app installs may not have every table (e.g. fresh install without
// auth_token), so we silently skip missing ones rather than erroring.
func pgTableExists(ctx context.Context, db *sql.DB, table string) (bool, error) {
	const q = `SELECT EXISTS (
		SELECT 1 FROM information_schema.tables
		WHERE table_schema NOT IN ('pg_catalog','information_schema')
		  AND table_name = $1
	)`
	var ok bool
	if err := db.QueryRowContext(ctx, q, table).Scan(&ok); err != nil {
		return false, err
	}
	return ok, nil
}

// duckDBHasUsername checks whether a username already exists in homer-core.
func duckDBHasUsername(ctx context.Context, duck *sql.DB, username string) (bool, error) {
	var n int
	err := duck.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// duckDBHasGUID checks whether a row with the given guid exists in `<table>`.
func duckDBHasGUID(ctx context.Context, duck *sql.DB, table, guid string) (bool, error) {
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE guid = ?", table)
	var n int
	if err := duck.QueryRowContext(ctx, q, guid).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

func nullToString(s sql.NullString) string {
	if s.Valid {
		return s.String
	}
	return ""
}

func nullToInt(i sql.NullInt64) int {
	if i.Valid {
		return int(i.Int64)
	}
	return 0
}

func nullToTime(t sql.NullTime) time.Time {
	if t.Valid {
		return t.Time
	}
	return time.Now().UTC()
}

// jsonOrNull returns "{}" for empty input so DuckDB JSON column never gets NULL.
func jsonOrNull(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "{}"
	}
	return s
}

// ---------------------------------------------------------------------------
// users
// ---------------------------------------------------------------------------

func migrateUsers(ctx context.Context, pg, duck *sql.DB, f MigrateFlags, s *migrateStats) error {
	if f.SkipUsers {
		log.Printf("users: skipped (--skip-users)")
		return nil
	}
	ok, err := pgTableExists(ctx, pg, "users")
	if err != nil {
		return err
	}
	if !ok {
		log.Printf("users: table not present in source; skipped")
		return nil
	}
	rows, err := pg.QueryContext(ctx, `
		SELECT username,
		       COALESCE(password,'')      AS password_hash,
		       COALESCE(email,'')         AS email,
		       COALESCE(firstname,'')     AS first_name,
		       COALESCE(lastname,'')      AS last_name,
		       COALESCE(is_admin, false)  AS is_admin,
		       COALESCE(is_active, true)  AS is_active,
		       COALESCE(create_date, now()) AS create_date
		FROM users`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			username, passwordHash, email, firstName, lastName string
			isAdmin, isActive                                  bool
			createDate                                         time.Time
		)
		if err := rows.Scan(&username, &passwordHash, &email, &firstName, &lastName, &isAdmin, &isActive, &createDate); err != nil {
			return err
		}
		s.bump("users", 1, 0, 0)
		if username == "" {
			s.bump("users", 0, 0, 1)
			continue
		}
		exists, err := duckDBHasUsername(ctx, duck, username)
		if err != nil {
			return err
		}
		if exists {
			if f.Verbose {
				log.Printf("users: %q already in target; skip", username)
			}
			s.bump("users", 0, 0, 1)
			continue
		}
		fullName := strings.TrimSpace(firstName + " " + lastName)

		if f.DryRun {
			s.bump("users", 0, 1, 0)
			continue
		}
		_, err = duck.ExecContext(ctx, `
			INSERT INTO users (username, password_hash, email, full_name, is_admin, is_active, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, username, passwordHash, email, fullName, isAdmin, isActive, createDate, createDate)
		if err != nil {
			return fmt.Errorf("insert user %q: %w", username, err)
		}
		s.bump("users", 0, 1, 0)
	}
	return rows.Err()
}

// ---------------------------------------------------------------------------
// global_settings
// ---------------------------------------------------------------------------

func migrateGlobalSettings(ctx context.Context, pg, duck *sql.DB, f MigrateFlags, s *migrateStats) error {
	ok, err := pgTableExists(ctx, pg, "global_settings")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	rows, err := pg.QueryContext(ctx, `
		SELECT COALESCE(guid::text,'') AS guid,
		       COALESCE(partid,1)       AS partid,
		       COALESCE(category,'')    AS category,
		       COALESCE(param,'')       AS param,
		       COALESCE(data::text,'{}')AS data,
		       COALESCE(create_date, now()) AS create_date
		FROM global_settings`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			guid, category, param, data string
			partid                      int
			createDate                  time.Time
		)
		if err := rows.Scan(&guid, &partid, &category, &param, &data, &createDate); err != nil {
			return err
		}
		s.bump("global_settings", 1, 0, 0)
		if guid == "" {
			s.bump("global_settings", 0, 0, 1)
			continue
		}
		exists, err := duckDBHasGUID(ctx, duck, "global_settings", guid)
		if err != nil {
			return err
		}
		if exists {
			s.bump("global_settings", 0, 0, 1)
			continue
		}
		if f.DryRun {
			s.bump("global_settings", 0, 1, 0)
			continue
		}
		_, err = duck.ExecContext(ctx, `
			INSERT INTO global_settings (guid, partid, category, param, data, create_date)
			VALUES (?, ?, ?, ?, ?, ?)`,
			guid, partid, category, param, jsonOrNull(data), createDate)
		if err != nil {
			return fmt.Errorf("insert global_settings %q: %w", guid, err)
		}
		s.bump("global_settings", 0, 1, 0)
	}
	return rows.Err()
}

// ---------------------------------------------------------------------------
// user_settings → user_preferences
// ---------------------------------------------------------------------------

func migrateUserPreferences(ctx context.Context, pg, duck *sql.DB, f MigrateFlags, s *migrateStats) error {
	ok, err := pgTableExists(ctx, pg, "user_settings")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	rows, err := pg.QueryContext(ctx, `
		SELECT COALESCE(guid::text,'')   AS guid,
		       COALESCE(username,'')     AS username,
		       COALESCE(partid,1)        AS partid,
		       COALESCE(category,'')     AS category,
		       COALESCE(param,'')        AS param,
		       COALESCE(data::text,'{}') AS data,
		       COALESCE(create_date, now()) AS create_date
		FROM user_settings`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			guid, username, category, param, data string
			partid                                int
			createDate                            time.Time
		)
		if err := rows.Scan(&guid, &username, &partid, &category, &param, &data, &createDate); err != nil {
			return err
		}
		s.bump("user_preferences", 1, 0, 0)
		if guid == "" {
			s.bump("user_preferences", 0, 0, 1)
			continue
		}
		exists, err := duckDBHasGUID(ctx, duck, services.UserPreferencesTable, guid)
		if err != nil {
			return err
		}
		if exists {
			s.bump("user_preferences", 0, 0, 1)
			continue
		}
		if f.DryRun {
			s.bump("user_preferences", 0, 1, 0)
			continue
		}
		_, err = duck.ExecContext(ctx, fmt.Sprintf(`
			INSERT INTO %s (guid, username, partid, category, param, data, create_date)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, services.UserPreferencesTable),
			guid, username, partid, category, param, jsonOrNull(data), createDate)
		if err != nil {
			return fmt.Errorf("insert user_preferences %q: %w", guid, err)
		}
		s.bump("user_preferences", 0, 1, 0)
	}
	return rows.Err()
}

// ---------------------------------------------------------------------------
// dashboards (metadata only — widget bodies are incompatible)
// ---------------------------------------------------------------------------

func migrateDashboards(ctx context.Context, pg, duck *sql.DB, f MigrateFlags, s *migrateStats) error {
	if !f.IncludeDashboards {
		log.Printf("dashboard_settings: skipped (--with-dashboards=false)")
		return nil
	}
	ok, err := pgTableExists(ctx, pg, "dashboard_settings")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	rows, err := pg.QueryContext(ctx, `
		SELECT COALESCE(guid::text,'')   AS guid,
		       COALESCE(username,'')     AS username,
		       COALESCE(partid,1)        AS partid,
		       COALESCE(dashboard_id,'') AS dashboard_id,
		       COALESCE(data::text,'{}') AS data,
		       COALESCE(create_date, now()) AS create_date
		FROM dashboard_settings`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			guid, username, dashboardID, data string
			partid                            int
			createDate                        time.Time
		)
		if err := rows.Scan(&guid, &username, &partid, &dashboardID, &data, &createDate); err != nil {
			return err
		}
		s.bump("dashboard_settings", 1, 0, 0)
		if guid == "" {
			s.bump("dashboard_settings", 0, 0, 1)
			continue
		}
		exists, err := duckDBHasGUID(ctx, duck, "dashboard_settings", guid)
		if err != nil {
			return err
		}
		if exists {
			s.bump("dashboard_settings", 0, 0, 1)
			continue
		}
		// Wipe widget body — incompatible between v7 and v11. Keep the
		// dashboard "shell" so users see a placeholder list and can
		// re-populate widgets.
		stub := dashboardStub(data)
		if f.DryRun {
			s.bump("dashboard_settings", 0, 1, 0)
			continue
		}
		_, err = duck.ExecContext(ctx, `
			INSERT INTO dashboard_settings (guid, username, partid, dashboard_id, data, create_date)
			VALUES (?, ?, ?, ?, ?, ?)`,
			guid, username, partid, dashboardID, stub, createDate)
		if err != nil {
			return fmt.Errorf("insert dashboard_settings %q: %w", guid, err)
		}
		s.bump("dashboard_settings", 0, 1, 0)
	}
	return rows.Err()
}

// dashboardStub keeps top-level metadata (name/type/shared/weight) but drops
// the widget tree. Records the original payload under "_homer7_legacy" so
// admins can salvage the old config manually if needed.
func dashboardStub(rawData string) string {
	var src map[string]any
	_ = json.Unmarshal([]byte(jsonOrNull(rawData)), &src)
	out := map[string]any{
		"name":              firstString(src, "name", "title"),
		"weight":            src["weight"],
		"shared":            src["shared"],
		"type":              firstString(src, "type"),
		"widgets":           []any{},
		"layouts":           map[string]any{},
		"_homer7_migrated":  true,
		"_homer7_legacy":    src,
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// mapping_schema
// ---------------------------------------------------------------------------

func migrateMappingSchema(ctx context.Context, pg, duck *sql.DB, f MigrateFlags, s *migrateStats) error {
	if !f.IncludeMappings {
		log.Printf("mapping_schema: skipped (pass --with-mappings to migrate; homer-core ships its own seeds)")
		return nil
	}
	ok, err := pgTableExists(ctx, pg, "mapping_schema")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	rows, err := pg.QueryContext(ctx, `
		SELECT COALESCE(guid::text,'')               AS guid,
		       COALESCE(profile,'')                  AS profile,
		       COALESCE(hepid,1)                     AS hepid,
		       COALESCE(hep_alias,'')                AS hep_alias,
		       COALESCE(partid,1)                    AS partid,
		       COALESCE(version,1)                   AS version,
		       COALESCE(retention,30)                AS retention,
		       COALESCE(partition_step,3600)         AS partition_step,
		       COALESCE(create_index::text,'{}')     AS create_index,
		       COALESCE(create_table,'')             AS create_table,
		       COALESCE(correlation_mapping::text,'{}') AS correlation_mapping,
		       COALESCE(fields_mapping::text,'[]')   AS fields_mapping,
		       COALESCE(mapping_settings::text,'{}') AS mapping_settings,
		       COALESCE(schema_mapping::text,'{}')   AS schema_mapping,
		       COALESCE(schema_settings::text,'{}')  AS schema_settings,
		       COALESCE(create_date, now())          AS create_date
		FROM mapping_schema`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			guid, profile, hepAlias, createTable                                                    string
			createIndex, correlationMapping, fieldsMapping, mappingSettings, schemaMapping, schemaSettings string
			hepid, partid, version, retention, partitionStep                                        int
			createDate                                                                              time.Time
		)
		if err := rows.Scan(&guid, &profile, &hepid, &hepAlias, &partid, &version, &retention,
			&partitionStep, &createIndex, &createTable, &correlationMapping, &fieldsMapping,
			&mappingSettings, &schemaMapping, &schemaSettings, &createDate); err != nil {
			return err
		}
		s.bump("mapping_schema", 1, 0, 0)
		if guid == "" {
			s.bump("mapping_schema", 0, 0, 1)
			continue
		}
		exists, err := duckDBHasGUID(ctx, duck, "mapping_schema", guid)
		if err != nil {
			return err
		}
		if exists {
			s.bump("mapping_schema", 0, 0, 1)
			continue
		}
		if f.DryRun {
			s.bump("mapping_schema", 0, 1, 0)
			continue
		}
		_, err = duck.ExecContext(ctx, `
			INSERT INTO mapping_schema (guid, profile, hepid, hep_alias, partid, version, retention,
			       partition_step, create_index, create_table, correlation_mapping, fields_mapping,
			       mapping_settings, schema_mapping, schema_settings, create_date)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			guid, profile, hepid, hepAlias, partid, version, retention, partitionStep,
			jsonOrNull(createIndex), createTable, jsonOrNull(correlationMapping),
			jsonOrNull(fieldsMapping), jsonOrNull(mappingSettings), jsonOrNull(schemaMapping),
			jsonOrNull(schemaSettings), createDate)
		if err != nil {
			return fmt.Errorf("insert mapping_schema %q: %w", guid, err)
		}
		s.bump("mapping_schema", 0, 1, 0)
	}
	return rows.Err()
}

// ---------------------------------------------------------------------------
// hepsub_mapping_schema
// ---------------------------------------------------------------------------

func migrateHEPSub(ctx context.Context, pg, duck *sql.DB, f MigrateFlags, s *migrateStats) error {
	ok, err := pgTableExists(ctx, pg, "hepsub_mapping_schema")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	rows, err := pg.QueryContext(ctx, `
		SELECT COALESCE(guid::text,'')        AS guid,
		       COALESCE(profile,'')           AS profile,
		       COALESCE(hepid,1)              AS hepid,
		       COALESCE(hep_alias,'')         AS hep_alias,
		       COALESCE(version,1)            AS version,
		       COALESCE(mapping::text,'{}')   AS mapping,
		       COALESCE(create_date, now())   AS create_date
		FROM hepsub_mapping_schema`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			guid, profile, hepAlias, mapping string
			hepid, version                   int
			createDate                       time.Time
		)
		if err := rows.Scan(&guid, &profile, &hepid, &hepAlias, &version, &mapping, &createDate); err != nil {
			return err
		}
		s.bump("hepsub_mapping_schema", 1, 0, 0)
		if guid == "" {
			s.bump("hepsub_mapping_schema", 0, 0, 1)
			continue
		}
		exists, err := duckDBHasGUID(ctx, duck, "hepsub_mapping_schema", guid)
		if err != nil {
			return err
		}
		if exists {
			s.bump("hepsub_mapping_schema", 0, 0, 1)
			continue
		}
		if f.DryRun {
			s.bump("hepsub_mapping_schema", 0, 1, 0)
			continue
		}
		_, err = duck.ExecContext(ctx, `
			INSERT INTO hepsub_mapping_schema (guid, profile, hepid, hep_alias, version, mapping, create_date)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			guid, profile, hepid, hepAlias, version, jsonOrNull(mapping), createDate)
		if err != nil {
			return fmt.Errorf("insert hepsub_mapping_schema %q: %w", guid, err)
		}
		s.bump("hepsub_mapping_schema", 0, 1, 0)
	}
	return rows.Err()
}

// ---------------------------------------------------------------------------
// alias
// ---------------------------------------------------------------------------

func migrateAlias(ctx context.Context, pg, duck *sql.DB, f MigrateFlags, s *migrateStats) error {
	ok, err := pgTableExists(ctx, pg, "alias")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	// homer-app has alias columns: guid, alias, ip, port, mask, capture_id,
	// status, create_date. Custom_image / tag1..4 are homer-core-only.
	rows, err := pg.QueryContext(ctx, `
		SELECT COALESCE(guid::text,'')    AS guid,
		       COALESCE(alias,'')         AS alias,
		       COALESCE(ip,'')            AS ip,
		       COALESCE(port,0)           AS port,
		       COALESCE(mask,32)          AS mask,
		       COALESCE(capture_id,'')    AS capture_id,
		       COALESCE(status,1)         AS status,
		       COALESCE(create_date, now()) AS create_date
		FROM alias`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			guid, alias, ip, captureID string
			port, mask, status         int
			createDate                 time.Time
		)
		if err := rows.Scan(&guid, &alias, &ip, &port, &mask, &captureID, &status, &createDate); err != nil {
			return err
		}
		s.bump("alias", 1, 0, 0)
		if guid == "" {
			s.bump("alias", 0, 0, 1)
			continue
		}
		exists, err := duckDBHasGUID(ctx, duck, "alias", guid)
		if err != nil {
			return err
		}
		if exists {
			s.bump("alias", 0, 0, 1)
			continue
		}
		if f.DryRun {
			s.bump("alias", 0, 1, 0)
			continue
		}
		_, err = duck.ExecContext(ctx, `
			INSERT INTO alias (guid, alias, ip, port, mask, capture_id, status, create_date)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			guid, alias, ip, port, mask, captureID, status, createDate)
		if err != nil {
			return fmt.Errorf("insert alias %q: %w", guid, err)
		}
		s.bump("alias", 0, 1, 0)
	}
	return rows.Err()
}

// ---------------------------------------------------------------------------
// auth_token
// ---------------------------------------------------------------------------

func migrateAuthToken(ctx context.Context, pg, duck *sql.DB, f MigrateFlags, s *migrateStats) error {
	ok, err := pgTableExists(ctx, pg, "auth_token")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	rows, err := pg.QueryContext(ctx, `
		SELECT COALESCE(guid::text,'')              AS guid,
		       COALESCE(creator_guid::text,'')      AS creator_guid,
		       COALESCE(name,'')                    AS name,
		       COALESCE(token,'')                   AS token,
		       COALESCE(user_object::text,'{}')     AS user_object,
		       COALESCE(ip_address,'')              AS ip_address,
		       COALESCE(create_date, now())         AS create_date,
		       COALESCE(lastusage_date::text,'')    AS lastusage_date,
		       COALESCE(expire_date::text,'')       AS expire_date,
		       COALESCE(usage_calls,0)              AS usage_calls,
		       COALESCE(limit_calls,0)              AS limit_calls,
		       COALESCE(active,1)                   AS active
		FROM auth_token`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			guid, creatorGUID, name, token, userObject, ipAddress, lastUsage, expire string
			usageCalls, limitCalls, active                                            int
			createDate                                                                time.Time
		)
		if err := rows.Scan(&guid, &creatorGUID, &name, &token, &userObject, &ipAddress,
			&createDate, &lastUsage, &expire, &usageCalls, &limitCalls, &active); err != nil {
			return err
		}
		s.bump("auth_token", 1, 0, 0)
		if guid == "" {
			s.bump("auth_token", 0, 0, 1)
			continue
		}
		exists, err := duckDBHasGUID(ctx, duck, "auth_token", guid)
		if err != nil {
			return err
		}
		if exists {
			s.bump("auth_token", 0, 0, 1)
			continue
		}
		if f.DryRun {
			s.bump("auth_token", 0, 1, 0)
			continue
		}
		_, err = duck.ExecContext(ctx, `
			INSERT INTO auth_token (guid, creator_guid, name, token, user_object, ip_address,
			       create_date, lastusage_date, expire_date, usage_calls, limit_calls, active)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			guid, creatorGUID, name, token, jsonOrNull(userObject), ipAddress,
			createDate, lastUsage, expire, usageCalls, limitCalls, active)
		if err != nil {
			return fmt.Errorf("insert auth_token %q: %w", guid, err)
		}
		s.bump("auth_token", 0, 1, 0)
	}
	return rows.Err()
}

// ---------------------------------------------------------------------------
// agent_location_session
// ---------------------------------------------------------------------------

func migrateAgentSessions(ctx context.Context, pg, duck *sql.DB, f MigrateFlags, s *migrateStats) error {
	ok, err := pgTableExists(ctx, pg, "agent_location_session")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	rows, err := pg.QueryContext(ctx, `
		SELECT COALESCE(guid::text,'')           AS guid,
		       COALESCE(gid,0)                   AS gid,
		       COALESCE(host,'')                 AS host,
		       COALESCE(port,0)                  AS port,
		       COALESCE(protocol,'')             AS protocol,
		       COALESCE(path,'')                 AS path,
		       COALESCE(node,'')                 AS node,
		       COALESCE(type,'')                 AS type,
		       COALESCE(create_date, now())      AS create_date,
		       COALESCE(expire_date::text,'')    AS expire_date,
		       COALESCE(active,1)                AS active
		FROM agent_location_session`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			guid, host, protocol, path, node, agentType, expire string
			gid, port, active                                   int
			createDate                                          time.Time
		)
		if err := rows.Scan(&guid, &gid, &host, &port, &protocol, &path, &node, &agentType,
			&createDate, &expire, &active); err != nil {
			return err
		}
		s.bump("agent_location_session", 1, 0, 0)
		if guid == "" {
			s.bump("agent_location_session", 0, 0, 1)
			continue
		}
		exists, err := duckDBHasGUID(ctx, duck, "agent_location_session", guid)
		if err != nil {
			return err
		}
		if exists {
			s.bump("agent_location_session", 0, 0, 1)
			continue
		}
		if f.DryRun {
			s.bump("agent_location_session", 0, 1, 0)
			continue
		}
		_, err = duck.ExecContext(ctx, `
			INSERT INTO agent_location_session (guid, gid, host, port, protocol, path, node, type,
			       create_date, expire_date, active)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			guid, gid, host, port, protocol, path, node, agentType, createDate, expire, active)
		if err != nil {
			return fmt.Errorf("insert agent_location_session %q: %w", guid, err)
		}
		s.bump("agent_location_session", 0, 1, 0)
	}
	return rows.Err()
}
