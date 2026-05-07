// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

type ImportsHandler struct {
	flight *services.FlightService
}

func NewImportsHandler(flight *services.FlightService) *ImportsHandler {
	return &ImportsHandler{flight: flight}
}

type ImportResponseV4 struct {
	Data struct {
		Inserted int `json:"inserted"`
		Rejected int `json:"rejected"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

func (h *ImportsHandler) V4ImportsPcap(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if h.flight == nil {
		return writeError(c, http.StatusServiceUnavailable, "Service Unavailable", "flight service not configured")
	}

	file, err := c.FormFile("file")
	if err != nil || file == nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "file is required")
	}
	src, err := file.Open()
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Failed to read file")
	}
	raw, readErr := io.ReadAll(src)
	src.Close()
	if readErr != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Failed to read file body")
	}

	ov := c.FormValue("override_to_current_time")
	override := strings.EqualFold(ov, "true") || ov == "1" || strings.EqualFold(ov, "on")

	forceRaw := c.FormValue("force_sip_table")
	if forceRaw == "" {
		forceRaw = c.FormValue("force_table")
	}
	forceSub, ferr := parseForceSIPTable(forceRaw)
	if ferr != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", ferr.Error())
	}

	inserted, rejected, err := importPcapSIP(c.Request().Context(), h.flight, h.flight.LakeName(), raw, pcapImportOptions{
		OverrideToCurrentTime: override,
		ForceSIPSubtype:       forceSub,
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no connected storage") {
			return writeError(c, http.StatusServiceUnavailable, "Service Unavailable", err.Error())
		}
		return writeError(c, http.StatusBadRequest, "Bad Request", err.Error())
	}

	resp := ImportResponseV4{Meta: buildMeta(c, "")}
	resp.Data.Inserted = inserted
	resp.Data.Rejected = rejected
	return c.JSON(http.StatusCreated, resp)
}
