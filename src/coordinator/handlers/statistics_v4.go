// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

type StatisticsHandler struct {
	flightService *services.FlightService
}

func NewStatisticsHandler(fs *services.FlightService) *StatisticsHandler {
	return &StatisticsHandler{flightService: fs}
}

type StatisticResponseV4 struct {
	Data  map[string]interface{} `json:"data"`
	Total int                    `json:"total"`
	Meta  Meta                   `json:"meta"`
}

type StatisticDbResponseV4 struct {
	Data   map[string]interface{} `json:"data"`
	Status string                 `json:"status"`
	Total  int                    `json:"total"`
	Meta   Meta                   `json:"meta"`
}

type StatisticMeasurementsResponseV4 struct {
	Data struct {
		Items []string `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type StatisticMetricsResponseV4 struct {
	Data struct {
		Items []string `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type StatisticTagsResponseV4 struct {
	Data struct {
		Items []string `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type StatisticQueryRequestV4 struct {
	Param struct {
		Query []struct {
			RawQuery string `json:"rawquery"`
		} `json:"query"`
	} `json:"param"`
}

type StatisticQueryResultV4 struct {
	Query string                   `json:"query"`
	Items []map[string]interface{} `json:"items"`
}

func (h *StatisticsHandler) V4StatisticsQuery(c echo.Context) error {
	var req StatisticQueryRequestV4
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}

	if len(req.Param.Query) == 0 {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Query list is required")
	}

	results := make([]StatisticQueryResultV4, 0, len(req.Param.Query))
	total := 0
	for _, query := range req.Param.Query {
		if strings.TrimSpace(query.RawQuery) == "" {
			return writeError(c, http.StatusBadRequest, "Bad Request", "rawquery is required")
		}
		items, err := h.flightService.Query(c.Request().Context(), query.RawQuery)
		if err != nil {
			return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to execute query")
		}
		results = append(results, StatisticQueryResultV4{
			Query: query.RawQuery,
			Items: items,
		})
		total += len(items)
	}

	resp := StatisticResponseV4{
		Data:  map[string]interface{}{"results": results},
		Total: total,
		Meta:  buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *StatisticsHandler) V4StatisticsDatabases(c echo.Context) error {
	limit, err := parseLimit(c)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid page[limit]")
	}

	items, err := h.querySingleColumn(c, "SELECT DISTINCT table_catalog AS name FROM information_schema.tables ORDER BY name", "name")
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to list databases")
	}
	if len(items) > limit {
		items = items[:limit]
	}

	resp := StatisticDbResponseV4{
		Data:   map[string]interface{}{"items": items},
		Status: "ok",
		Total:  len(items),
		Meta:   buildMeta(c, ""),
	}
	resp.Meta.Pagination = &Pagination{Limit: limit, Total: len(items), HasMore: false}
	return c.JSON(http.StatusOK, resp)
}

func (h *StatisticsHandler) V4StatisticsMeasurements(c echo.Context) error {
	limit, err := parseLimit(c)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid page[limit]")
	}

	db := strings.TrimSpace(c.QueryParam("db"))
	if db == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "db is required")
	}

	sql := "SELECT DISTINCT table_name AS name FROM information_schema.tables WHERE table_catalog = '" + escapeLiteral(db) + "' ORDER BY name"
	items, err := h.querySingleColumn(c, sql, "name")
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to list measurements")
	}
	if len(items) > limit {
		items = items[:limit]
	}

	resp := StatisticMeasurementsResponseV4{}
	resp.Data.Items = items
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: limit, Total: len(items), HasMore: false}
	return c.JSON(http.StatusOK, resp)
}

func (h *StatisticsHandler) V4StatisticsMetrics(c echo.Context) error {
	limit, err := parseLimit(c)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid page[limit]")
	}

	db := strings.TrimSpace(c.QueryParam("db"))
	if db == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "db is required")
	}

	sql := "SELECT DISTINCT column_name AS name FROM information_schema.columns WHERE table_catalog = '" + escapeLiteral(db) + "' ORDER BY name"
	items, err := h.querySingleColumn(c, sql, "name")
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to list metrics")
	}
	if len(items) > limit {
		items = items[:limit]
	}

	resp := StatisticMetricsResponseV4{}
	resp.Data.Items = items
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: limit, Total: len(items), HasMore: false}
	return c.JSON(http.StatusOK, resp)
}

func (h *StatisticsHandler) V4StatisticsTags(c echo.Context) error {
	limit, err := parseLimit(c)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid page[limit]")
	}

	db := strings.TrimSpace(c.QueryParam("db"))
	if db == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "db is required")
	}

	sql := "SELECT DISTINCT column_name AS name FROM information_schema.columns WHERE table_catalog = '" + escapeLiteral(db) + "' ORDER BY name"
	items, err := h.querySingleColumn(c, sql, "name")
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to list tags")
	}
	if len(items) > limit {
		items = items[:limit]
	}

	resp := StatisticTagsResponseV4{}
	resp.Data.Items = items
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: limit, Total: len(items), HasMore: false}
	return c.JSON(http.StatusOK, resp)
}

func (h *StatisticsHandler) querySingleColumn(c echo.Context, sql, column string) ([]string, error) {
	rows, err := h.flightService.Query(c.Request().Context(), sql)
	if err != nil {
		return nil, err
	}
	items := make([]string, 0, len(rows))
	for _, row := range rows {
		if value, ok := row[column]; ok && value != nil {
			items = append(items, strings.TrimSpace(fmt.Sprint(value)))
		}
	}
	return items, nil
}

func escapeLiteral(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
