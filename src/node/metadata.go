// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package node

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

// quoteIdent wraps a DuckDB identifier in double quotes, escaping embedded quotes.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// nodeTimeRange returns the min/max timestamp (nanoseconds since epoch) across
// all hep_proto_* tables visible to this node's DuckDB connection, spanning any
// attached DuckLake volumes. Returns 0,0 when the node holds no data.
func nodeTimeRange(db *sql.DB) (minNs, maxNs int64, err error) {
	// duckdb_tables() spans every attached catalog, so tiered volumes are
	// covered automatically.
	rows, err := db.Query(`SELECT database_name, schema_name, table_name
		FROM duckdb_tables() WHERE table_name LIKE 'hep_proto_%'`)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	var union []string
	for rows.Next() {
		var dbName, schema, table string
		if err := rows.Scan(&dbName, &schema, &table); err != nil {
			return 0, 0, err
		}
		fqn := quoteIdent(dbName) + "." + quoteIdent(schema) + "." + quoteIdent(table)
		// Apply epoch_ns to the result of min/max (not min/max of epoch_ns):
		// epoch_ns is monotonic so the values are identical, but min(timestamp)/
		// max(timestamp) operate on the bare column, letting DuckDB answer from
		// Parquet zone-map stats instead of scanning. The epoch conversion still
		// happens in-DB (BIGINT ns, timezone-independent) — important since these
		// stats gate node pruning; no Go-side time.Time/timezone conversion.
		union = append(union, fmt.Sprintf(
			`SELECT epoch_ns(min(timestamp)) AS mn, epoch_ns(max(timestamp)) AS mx FROM %s`, fqn))
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	if len(union) == 0 {
		return 0, 0, nil
	}

	q := "SELECT min(mn), max(mx) FROM (" + strings.Join(union, " UNION ALL ") + ")"
	var mn, mx sql.NullInt64
	if err := db.QueryRow(q).Scan(&mn, &mx); err != nil {
		return 0, 0, err
	}
	if mn.Valid {
		minNs = mn.Int64
	}
	if mx.Valid {
		maxNs = mx.Int64
	}
	return minNs, maxNs, nil
}

// handleMetadataStats handles GET /metadata/stats.
func (n *Node) handleMetadataStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if n.db == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "database not initialized",
		})
		return
	}
	minNs, maxNs, err := nodeTimeRange(n.db)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"min_ts": minNs,
		"max_ts": maxNs,
	})
}
