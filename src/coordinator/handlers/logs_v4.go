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

type LogsHandler struct{}

func NewLogsHandler() *LogsHandler {
	return &LogsHandler{}
}

type LokiLabelsResponseV4 struct {
	Data struct {
		Items []string `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type LokiValuesResponseV4 struct {
	Data struct {
		Items []string `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type LokiQueryItemV4 struct {
	Custom1 string `json:"custom_1,omitempty"`
	Custom2 string `json:"custom_2,omitempty"`
	ID      int    `json:"id,omitempty"`
	MicroTS int64  `json:"micro_ts,omitempty"`
}

type LokiQueryResponseV4 struct {
	Data  []LokiQueryItemV4 `json:"data"`
	Total int               `json:"total"`
	Meta  Meta              `json:"meta"`
}

func (h *LogsHandler) V4LokiLabels(c echo.Context) error {
	limit, err := parseLimit(c)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid page[limit]")
	}

	resp := LokiLabelsResponseV4{}
	resp.Data.Items = []string{}
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: limit, Total: 0, HasMore: false}
	return c.JSON(http.StatusOK, resp)
}

func (h *LogsHandler) V4LokiValues(c echo.Context) error {
	limit, err := parseLimit(c)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid page[limit]")
	}

	resp := LokiValuesResponseV4{}
	resp.Data.Items = []string{}
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: limit, Total: 0, HasMore: false}
	return c.JSON(http.StatusOK, resp)
}

func (h *LogsHandler) V4LokiQuery(c echo.Context) error {
	var req map[string]interface{}
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}

	resp := LokiQueryResponseV4{
		Data:  []LokiQueryItemV4{},
		Total: 0,
		Meta:  buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}
