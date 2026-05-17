// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// DashboardService provides dashboard operations backed by dashboard_settings.
type DashboardService struct {
	db *sql.DB
}

func NewDashboardService(db *sql.DB) *DashboardService {
	return &DashboardService{db: db}
}

func (s *DashboardService) ListDashboards(ctx context.Context, username string) ([]UserSetting, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	query := `SELECT id, guid, username, partid, dashboard_id, data
			FROM dashboard_settings
			ORDER BY id`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	settings := make([]UserSetting, 0)
	for rows.Next() {
		setting, err := scanDashboardSettingRow(rows)
		if err != nil {
			return nil, err
		}
		if strings.EqualFold(setting.UserName, username) || isDashboardShared(setting.Data) {
			settings = append(settings, setting)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return settings, nil
}

func (s *DashboardService) GetDashboard(ctx context.Context, username, dashboardID string) (*UserSetting, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	query := fmt.Sprintf(
		`SELECT id, guid, username, partid, dashboard_id, data
		 FROM dashboard_settings
		 WHERE dashboard_id = '%s'`,
		escapeSQL(dashboardID),
	)
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shared *UserSetting
	for rows.Next() {
		setting, err := scanDashboardSettingRow(rows)
		if err != nil {
			return nil, err
		}
		if strings.EqualFold(setting.UserName, username) {
			return &setting, nil
		}
		if shared == nil && isDashboardShared(setting.Data) {
			shared = &setting
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return shared, nil
}

func (s *DashboardService) CreateDashboard(ctx context.Context, username, dashboardID string, data json.RawMessage) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("settings db not available")
	}

	sqlCheck := fmt.Sprintf(
		`SELECT username FROM dashboard_settings
		 WHERE dashboard_id = '%s' LIMIT 1`,
		escapeSQL(dashboardID),
	)
	row := s.db.QueryRowContext(ctx, sqlCheck)
	var owner sql.NullString
	if err := row.Scan(&owner); err != nil && err != sql.ErrNoRows {
		return "", err
	}
	if owner.Valid && !strings.EqualFold(owner.String, username) {
		return "", fmt.Errorf("dashboard is owned by another user")
	}

	guid := newGUID()
	sqlInsert := fmt.Sprintf(
		`INSERT INTO dashboard_settings (guid, username, partid, dashboard_id, data, create_date)
		 VALUES ('%s', '%s', %d, '%s', '%s', current_timestamp)
		 RETURNING guid`,
		escapeSQL(guid),
		escapeSQL(username),
		10,
		escapeSQL(dashboardID),
		escapeJSONData(string(data)),
	)
	var inserted string
	if err := s.db.QueryRowContext(ctx, sqlInsert).Scan(&inserted); err != nil {
		return guid, err
	}
	if inserted != "" {
		return inserted, nil
	}
	return guid, nil
}

func (s *DashboardService) UpdateDashboard(ctx context.Context, username, dashboardID string, data json.RawMessage) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("settings db not available")
	}

	sqlUpdate := fmt.Sprintf(
		`UPDATE dashboard_settings
		 SET data = '%s'
		 WHERE username = '%s' AND dashboard_id = '%s'
		 RETURNING guid`,
		escapeJSONData(string(data)),
		escapeSQL(username),
		escapeSQL(dashboardID),
	)
	var guid string
	if err := s.db.QueryRowContext(ctx, sqlUpdate).Scan(&guid); err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return guid, nil
}

func (s *DashboardService) DeleteDashboard(ctx context.Context, username, dashboardID string) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("settings db not available")
	}
	query := fmt.Sprintf(
		`DELETE FROM dashboard_settings
		 WHERE username = '%s' AND dashboard_id = '%s'
		 RETURNING id`,
		escapeSQL(username),
		escapeSQL(dashboardID),
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

func (s *DashboardService) ResetDashboards(ctx context.Context, username string) error {
	if s.db == nil {
		return fmt.Errorf("settings db not available")
	}
	sqlDelete := fmt.Sprintf(
		`DELETE FROM dashboard_settings
		 WHERE username = '%s'`,
		escapeSQL(username),
	)
	if _, err := s.db.ExecContext(ctx, sqlDelete); err != nil {
		return err
	}

	// Default "Home": SIP Call Search + Results Table + Clock
	homeData, _ := json.Marshal(map[string]interface{}{
		"name": "Home", "param": "home", "shared": false, "type": 0, "weight": 10,
		"config": map[string]interface{}{"columns": 12, "grid_type": "fit", "locked": false},
		"widgets": []map[string]interface{}{
			{
				"id": "search-1", "type": "search", "x": 0, "y": 0, "w": 3, "h": 14, "title": "SIP Call Search",
				"config": map[string]interface{}{
					"preset":       "sip_call",
					"targetWidget": "results-1",
				},
			},
			{"id": "results-1", "type": "results", "x": 3, "y": 0, "w": 6, "h": 14, "title": "Results Table"},
			{"id": "clock-1", "type": "clock", "x": 9, "y": 0, "w": 3, "h": 14, "title": "Clock"},
		},
	})
	if _, err := s.CreateDashboard(ctx, username, "home", homeData); err != nil {
		return err
	}

	// Default "Smart Search" dashboard with search + results + chart
	searchData, _ := json.Marshal(map[string]interface{}{
		"name": "Smart Search", "param": "smartsearch", "shared": false, "type": 1, "weight": 20,
		"config": map[string]interface{}{"columns": 12, "grid_type": "fit", "locked": false},
		"widgets": []map[string]interface{}{
			{"id": "search-1", "type": "search", "x": 0, "y": 0, "w": 3, "h": 14, "title": "Protocol Search"},
			{"id": "results-1", "type": "results", "x": 3, "y": 0, "w": 9, "h": 10, "title": "Results"},
			{"id": "chart-1", "type": "chart", "x": 3, "y": 10, "w": 9, "h": 4, "title": "Time Chart"},
		},
	})
	if _, err := s.CreateDashboard(ctx, username, "smartsearch", searchData); err != nil {
		return err
	}

	// Default "Games" dashboard with all single-player game widgets pre-laid out
	// on the 12-col grid. Players land here and try every game without having
	// to use the Add Widget dialog.
	gamesData, _ := json.Marshal(map[string]interface{}{
		"name": "Games", "param": "games", "shared": false, "type": 2, "weight": 30,
		"config": map[string]interface{}{"columns": 12, "grid_type": "fit", "locked": false},
		"widgets": []map[string]interface{}{
			{"id": "packet-defender-1", "type": "packet_defender", "x": 0, "y": 0, "w": 6, "h": 10, "title": "Packet Defender"},
			{"id": "sip-dialog-master-1", "type": "sip_dialog_master", "x": 6, "y": 0, "w": 6, "h": 10, "title": "SIP Dialog Master"},
			{"id": "jitter-buffer-hero-1", "type": "jitter_buffer_hero", "x": 0, "y": 10, "w": 6, "h": 10, "title": "Jitter Buffer Hero"},
			{"id": "sipetris-1", "type": "sipetris", "x": 6, "y": 10, "w": 6, "h": 12, "title": "SIPetris"},
			{"id": "chess-1", "type": "chess", "x": 0, "y": 22, "w": 8, "h": 12, "title": "Chess"},
		},
	})
	if _, err := s.CreateDashboard(ctx, username, "games", gamesData); err != nil {
		return err
	}

	// Default "NetGames" dashboard with the multiplayer game widgets. They
	// require coordinator hubs (netris, netchess), so we surface them in a
	// dedicated tab to avoid surprising single-user installs. NetChess
	// matches the single-player Chess widget footprint (8x12) rather than
	// the full 12-wide arena Netris needs.
	netGamesData, _ := json.Marshal(map[string]interface{}{
		"name": "NetGames", "param": "netgames", "shared": false, "type": 3, "weight": 40,
		"config": map[string]interface{}{"columns": 12, "grid_type": "fit", "locked": false},
		"widgets": []map[string]interface{}{
			{"id": "netris-1", "type": "netris", "x": 0, "y": 0, "w": 12, "h": 14, "title": "Netris"},
			{"id": "netchess-1", "type": "netchess", "x": 0, "y": 14, "w": 8, "h": 12, "title": "NetChess"},
		},
	})
	if _, err := s.CreateDashboard(ctx, username, "netgames", netGamesData); err != nil {
		return err
	}
	return nil
}

func isDashboardShared(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return false
	}
	val, ok := data["shared"]
	if !ok {
		return false
	}
	switch v := val.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true") || v == "1"
	case float64:
		return v != 0
	default:
		return false
	}
}

// scanDashboardSettingRow maps a dashboard_settings row to UserSetting so existing API handlers stay unchanged.
func scanDashboardSettingRow(rows *sql.Rows) (UserSetting, error) {
	var (
		setting     UserSetting
		id          sql.NullInt64
		guid        sql.NullString
		username    sql.NullString
		partid      sql.NullInt64
		dashboardID sql.NullString
		data        interface{}
	)
	if err := rows.Scan(&id, &guid, &username, &partid, &dashboardID, &data); err != nil {
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
	setting.Category = "dashboard"
	if dashboardID.Valid {
		setting.Param = dashboardID.String
	}
	setting.Data = rawMessageFromSQLValue(data)
	return setting, nil
}
