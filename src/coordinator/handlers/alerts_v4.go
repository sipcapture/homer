// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

type AlertsHandler struct {
	svc *services.AlertsService
}

func NewAlertsHandler(svc *services.AlertsService) *AlertsHandler {
	return &AlertsHandler{svc: svc}
}

type DashboardAlertsListResponseV4 struct {
	Data struct {
		Items []services.DashboardAlert `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type DashboardAlertCreateRequestV4 struct {
	Severity string          `json:"severity"`
	Title    string          `json:"title"`
	Message  string          `json:"message"`
	Payload  json.RawMessage `json:"payload"`
}

type DashboardAlertCreateResponseV4 struct {
	Data struct {
		ID int64 `json:"id"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

func (h *AlertsHandler) V4AlertsList(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	limit, err := parseLimit(c)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid page[limit]")
	}
	items, err := h.svc.List(c.Request().Context(), limit)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", err.Error())
	}
	resp := DashboardAlertsListResponseV4{}
	resp.Data.Items = items
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: limit, Total: len(items), HasMore: false}
	return c.JSON(http.StatusOK, resp)
}

func (h *AlertsHandler) V4AlertsDeleteAll(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if err := h.svc.DeleteAll(c.Request().Context()); err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *AlertsHandler) V4AlertsCreate(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	var req DashboardAlertCreateRequestV4
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid JSON body")
	}
	if req.Title == "" && req.Message == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "title or message is required")
	}
	id, err := h.svc.Insert(c.Request().Context(), req.Severity, req.Title, req.Message, req.Payload)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", err.Error())
	}
	out := DashboardAlertCreateResponseV4{}
	out.Data.ID = id
	out.Meta = buildMeta(c, "")
	return c.JSON(http.StatusCreated, out)
}
