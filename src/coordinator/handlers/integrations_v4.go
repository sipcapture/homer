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
)

type IntegrationsHandler struct{}

func NewIntegrationsHandler() *IntegrationsHandler {
	return &IntegrationsHandler{}
}

type GrafanaStatusResponseV4 struct {
	Data map[string]interface{} `json:"data"`
	Meta Meta                   `json:"meta"`
}

type GrafanaFoldersResponseV4 struct {
	Data struct {
		Items []map[string]interface{} `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type GrafanaDashboardResponseV4 struct {
	Data map[string]interface{} `json:"data"`
	Meta Meta                   `json:"meta"`
}

func (h *IntegrationsHandler) V4GrafanaStatus(c echo.Context) error {
	resp := GrafanaStatusResponseV4{
		Data: map[string]interface{}{},
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *IntegrationsHandler) V4GrafanaFolders(c echo.Context) error {
	limit, err := parseLimit(c)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid page[limit]")
	}

	resp := GrafanaFoldersResponseV4{}
	resp.Data.Items = []map[string]interface{}{}
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: limit, Total: 0, HasMore: false}
	return c.JSON(http.StatusOK, resp)
}

func (h *IntegrationsHandler) V4GrafanaDashboard(c echo.Context) error {
	resp := GrafanaDashboardResponseV4{
		Data: map[string]interface{}{},
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}
