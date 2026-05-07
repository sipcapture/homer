// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

// DashboardHandler handles dashboard-related API endpoints
type DashboardHandler struct {
	flightService *services.FlightService
}

// NewDashboardHandler creates a new dashboard handler
func NewDashboardHandler(fs *services.FlightService) *DashboardHandler {
	return &DashboardHandler{
		flightService: fs,
	}
}

// GetDashboardInfo handles GET /api/v3/dashboard/info
func (h *DashboardHandler) GetDashboardInfo(c echo.Context) error {
	// Get statistics from flight service
	stats := h.flightService.GetStats()

	// Build dashboard info response
	response := map[string]interface{}{
		"status": "ok",
		"data": map[string]interface{}{
			"nodes": stats,
			"storage": map[string]interface{}{
				"type": "ducklake",
			},
		},
	}

	// Try to get row counts from nodes
	ctx := c.Request().Context()
	
	// Query SIP call count
	ln := h.flightService.LakeName()
	callCountSQL := "SELECT COUNT(*) as count FROM " + ln + ".main.hep_proto_1_call"
	callResults, err := h.flightService.Query(ctx, callCountSQL)
	if err == nil && len(callResults) > 0 {
		if count, ok := callResults[0]["count"]; ok {
			response["data"].(map[string]interface{})["sip_calls"] = count
		}
	}

	// Query registration count
	regCountSQL := "SELECT COUNT(*) as count FROM " + ln + ".main.hep_proto_1_registration"
	regResults, err := h.flightService.Query(ctx, regCountSQL)
	if err == nil && len(regResults) > 0 {
		if count, ok := regResults[0]["count"]; ok {
			response["data"].(map[string]interface{})["sip_registrations"] = count
		}
	}

	return c.JSON(http.StatusOK, response)
}
