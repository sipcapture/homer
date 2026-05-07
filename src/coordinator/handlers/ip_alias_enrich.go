// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"

	"github.com/sipcapture/homer-core/src/coordinator/services"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// enrichRowsWithIPAliases resolves src_ip/dst_ip into aliasSrc/aliasDst on each row.
// Safe when aliasService is nil or the query fails (rows unchanged except skipped enrich).
func (h *SearchHandler) enrichRowsWithIPAliases(ctx context.Context, rows []map[string]interface{}) {
	if h == nil || h.aliasService == nil || len(rows) == 0 {
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
