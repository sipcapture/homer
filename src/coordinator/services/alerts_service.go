// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// DashboardAlert is one row from dashboard_alerts.
type DashboardAlert struct {
	ID        int64           `json:"id"`
	Severity  string          `json:"severity,omitempty"`
	Title     string          `json:"title,omitempty"`
	Message   string          `json:"message,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// AlertsService provides access to dashboard_alerts in the settings DuckDB.
type AlertsService struct {
	db *sql.DB
}

func NewAlertsService(db *sql.DB) *AlertsService {
	return &AlertsService{db: db}
}

func (s *AlertsService) List(ctx context.Context, limit int) ([]DashboardAlert, error) {
	if s.db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	q := fmt.Sprintf(`SELECT id, severity, title, message, payload, created_at
		FROM dashboard_alerts
		ORDER BY id DESC
		LIMIT %d`, limit)
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DashboardAlert
	for rows.Next() {
		var a DashboardAlert
		var sev, tit, msg, payload sql.NullString
		if err := rows.Scan(&a.ID, &sev, &tit, &msg, &payload, &a.CreatedAt); err != nil {
			return nil, err
		}
		if sev.Valid {
			a.Severity = sev.String
		}
		if tit.Valid {
			a.Title = tit.String
		}
		if msg.Valid {
			a.Message = msg.String
		}
		if payload.Valid && payload.String != "" {
			a.Payload = json.RawMessage(payload.String)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *AlertsService) DeleteAll(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("settings db not available")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM dashboard_alerts`)
	return err
}

// Insert creates a new alert row (for integrations / POST API).
func (s *AlertsService) Insert(ctx context.Context, severity, title, message string, payload json.RawMessage) (int64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("settings db not available")
	}
	var payloadArg interface{}
	if len(payload) > 0 {
		payloadArg = string(payload)
	}
	row := s.db.QueryRowContext(ctx,
		`INSERT INTO dashboard_alerts (severity, title, message, payload) VALUES (?, ?, ?, ?) RETURNING id`,
		nullIfEmpty(severity), nullIfEmpty(title), nullIfEmpty(message), payloadArg,
	)
	var id int64
	if err := row.Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
