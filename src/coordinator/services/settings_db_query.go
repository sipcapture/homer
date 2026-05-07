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
)

// settingsDBQuery runs a SELECT and returns rows as []map[string]interface{}
// for parity with FlightService.Query.
func settingsDBQuery(ctx context.Context, db *sql.DB, query string) ([]map[string]interface{}, error) {
	if db == nil {
		return nil, fmt.Errorf("settings db not available")
	}
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var out []map[string]interface{}
	for rows.Next() {
		raw := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			val := raw[i]
			switch v := val.(type) {
			case []byte:
				row[col] = string(v)
			default:
				row[col] = val
			}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func settingsDBExec(ctx context.Context, db *sql.DB, query string) error {
	if db == nil {
		return fmt.Errorf("settings db not available")
	}
	_, err := db.ExecContext(ctx, query)
	return err
}
