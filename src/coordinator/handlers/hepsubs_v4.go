// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

type HepsubsHandler struct {
	service *services.HepsubService
}

func NewHepsubsHandler(service *services.HepsubService) *HepsubsHandler {
	return &HepsubsHandler{service: service}
}

type HepsubSchemaV4 struct {
	GUID     string          `json:"guid"`
	Profile  string          `json:"profile"`
	HepID    int             `json:"hepid"`
	HepAlias string          `json:"hep_alias"`
	Version  int             `json:"version"`
	Mapping  json.RawMessage `json:"mapping"`
}

type HepsubResponseV4 struct {
	Data HepsubSchemaV4 `json:"data"`
	Meta Meta           `json:"meta"`
}

type HepsubListResponseV4 struct {
	Data struct {
		Items []HepsubSchemaV4 `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

func (h *HepsubsHandler) V4HepsubsList(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	filters, err := parseHepsubFilters(c)
	if err != nil {
		if httpErr, ok := err.(*echo.HTTPError); ok {
			return writeError(c, httpErr.Code, "Bad Request", httpErr.Message.(string))
		}
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid query parameters")
	}

	items, err := h.service.List(c.Request().Context(), filters)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to list hepsubs")
	}

	resp := HepsubListResponseV4{}
	resp.Data.Items = mapHepsubs(items)
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: filters.Limit, Total: len(items)}
	return c.JSON(http.StatusOK, resp)
}

func (h *HepsubsHandler) V4HepsubsCreate(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	var payload HepsubSchemaV4
	if err := c.Bind(&payload); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	if payload.Profile == "" || payload.HepID == 0 {
		return writeError(c, http.StatusBadRequest, "Bad Request", "profile and hepid are required")
	}

	guid, err := h.service.Create(c.Request().Context(), mapHepsubToService(payload))
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to create hepsub")
	}

	resp := IdResponseV4{
		Data: guid,
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusCreated, resp)
}

func (h *HepsubsHandler) V4HepsubsGet(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	hepsubID := c.Param("hepsubId")
	if hepsubID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Hepsub id is required")
	}

	item, err := h.service.GetByGUID(c.Request().Context(), hepsubID)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to get hepsub")
	}
	if item == nil {
		return writeError(c, http.StatusNotFound, "Not Found", "Hepsub not found")
	}

	resp := HepsubResponseV4{
		Data: mapHepsub(*item),
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *HepsubsHandler) V4HepsubsUpdate(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	hepsubID := c.Param("hepsubId")
	if hepsubID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Hepsub id is required")
	}

	var payload HepsubSchemaV4
	if err := c.Bind(&payload); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}

	updated, err := h.service.Update(c.Request().Context(), hepsubID, mapHepsubToService(payload))
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to update hepsub")
	}
	if updated == "" {
		return writeError(c, http.StatusNotFound, "Not Found", "Hepsub not found")
	}

	resp := IdResponseV4{
		Data: updated,
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *HepsubsHandler) V4HepsubsDelete(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	hepsubID := c.Param("hepsubId")
	if hepsubID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Hepsub id is required")
	}

	deleted, err := h.service.Delete(c.Request().Context(), hepsubID)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to delete hepsub")
	}
	if !deleted {
		return writeError(c, http.StatusNotFound, "Not Found", "Hepsub not found")
	}
	return c.NoContent(http.StatusNoContent)
}

func parseHepsubFilters(c echo.Context) (services.HepsubListFilters, error) {
	filters := services.HepsubListFilters{
		Profile:  c.QueryParam("filter[profile]"),
		HepAlias: c.QueryParam("filter[hep_alias]"),
	}

	limitStr := c.QueryParam("page[limit]")
	if limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 || limit > 1000 {
			return filters, echo.NewHTTPError(http.StatusBadRequest, "Invalid page[limit]")
		}
		filters.Limit = limit
	} else {
		filters.Limit = 100
	}

	if hepidStr := c.QueryParam("filter[hepid]"); hepidStr != "" {
		hepid, err := strconv.Atoi(hepidStr)
		if err != nil {
			return filters, echo.NewHTTPError(http.StatusBadRequest, "Invalid filter[hepid]")
		}
		filters.HepID = &hepid
	}

	sortParam := c.QueryParam("sort")
	if sortParam != "" {
		sortExpr, err := parseHepsubSort(sortParam)
		if err != nil {
			return filters, err
		}
		filters.Sort = sortExpr
	}
	return filters, nil
}

func parseHepsubSort(value string) (string, error) {
	desc := false
	field := value
	if strings.HasPrefix(value, "-") {
		desc = true
		field = strings.TrimPrefix(value, "-")
	}

	allowed := map[string]string{
		"guid":      "guid",
		"profile":   "profile",
		"hepid":     "hepid",
		"hep_alias": "hep_alias",
		"version":   "version",
	}
	column, ok := allowed[field]
	if !ok {
		return "", echo.NewHTTPError(http.StatusBadRequest, "Invalid sort field")
	}

	order := "ASC"
	if desc {
		order = "DESC"
	}
	return column + " " + order, nil
}

func mapHepsubs(items []services.HepsubSchema) []HepsubSchemaV4 {
	result := make([]HepsubSchemaV4, 0, len(items))
	for _, item := range items {
		result = append(result, mapHepsub(item))
	}
	return result
}

func mapHepsub(item services.HepsubSchema) HepsubSchemaV4 {
	return HepsubSchemaV4{
		GUID:     item.GUID,
		Profile:  item.Profile,
		HepID:    item.HepID,
		HepAlias: item.HepAlias,
		Version:  item.Version,
		Mapping:  item.Mapping,
	}
}

func mapHepsubToService(item HepsubSchemaV4) services.HepsubSchema {
	return services.HepsubSchema{
		GUID:     item.GUID,
		Profile:  item.Profile,
		HepID:    item.HepID,
		HepAlias: item.HepAlias,
		Version:  item.Version,
		Mapping:  normalizeJSON(item.Mapping),
	}
}
