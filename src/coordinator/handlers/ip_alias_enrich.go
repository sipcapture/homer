// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"

	"github.com/sipcapture/homer-core/src/coordinator/services"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// enrichRowsWithIPAliases enriches each result row in place: it flattens
// data_extra.custom_headers onto the row as top-level keys (so configured custom
// SIP headers surface as selectable Results columns) and resolves src_ip/dst_ip
// into aliasSrc/aliasDst. Safe when aliasService is nil or the query fails.
func (h *SearchHandler) enrichRowsWithIPAliases(ctx context.Context, rows []map[string]interface{}) {
	if h == nil || len(rows) == 0 {
		return
	}
	// Custom-header flattening is independent of the IP-alias service, so it runs
	// for every search result regardless of alias configuration.
	for i := range rows {
		flattenRowCustomHeaders(rows[i])
	}
	if h.aliasService == nil {
		return
	}
	m, err := h.aliasService.CachedIPAliasMap(ctx)
	if err != nil {
		logger.Warn("IP alias enrichment skipped", "error", err.Error())
		return
	}
	for i := range rows {
		services.EnrichRowIPAliases(m, rows[i])
	}
}

// flattenRowCustomHeaders copies data_extra.custom_headers.<Name> onto the row as a
// top-level "<Name>" key (without overwriting existing columns) so extracted custom
// SIP headers (e.g. X-Call-Trace, X-Session-Id) become selectable Results-table columns via the
// same row-key mechanism used for aliasSrc/aliasDst.
func flattenRowCustomHeaders(row map[string]interface{}) {
	if row == nil {
		return
	}
	extra := parseDataExtraMap(row["data_extra"])
	if extra == nil {
		return
	}
	ch, ok := extra["custom_headers"].(map[string]interface{})
	if !ok {
		return
	}
	for name, val := range ch {
		if name == "" {
			continue
		}
		if _, exists := row[name]; exists {
			continue
		}
		row[name] = val
	}
}
