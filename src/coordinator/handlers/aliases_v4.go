// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

type AliasesHandler struct {
	service *services.AliasService
}

func NewAliasesHandler(service *services.AliasService) *AliasesHandler {
	return &AliasesHandler{service: service}
}

type AliasV4 struct {
	ID          int64  `json:"id,omitempty"`
	GUID        string `json:"guid,omitempty"`
	Alias       string `json:"alias,omitempty"`
	IP          string `json:"ip,omitempty"`
	Port        int    `json:"port,omitempty"`
	Mask        int    `json:"mask,omitempty"`
	CaptureID   string `json:"capture_id,omitempty"`
	Status      bool   `json:"status,omitempty"`
	CustomImage string `json:"custom_image,omitempty"`
	Tag1        string `json:"tag1,omitempty"`
	Tag2        string `json:"tag2,omitempty"`
	Tag3        string `json:"tag3,omitempty"`
	Tag4        string `json:"tag4,omitempty"`
}

type AliasListResponseV4 struct {
	Data struct {
		Items []AliasV4 `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

// AliasLookupResponseV4 is the diagnostic envelope for V4AliasesLookup.
// `Hit=false` means the configured aliases did not match the requested
// (ip, port[, capture_id]) tuple. `LoadedPrefixes` exposes the size of
// the in-memory LPM table — when it is zero, no aliases were loaded
// at all and row enrichment is silently a no-op.
type AliasLookupResponseV4 struct {
	Data struct {
		IP             string `json:"ip"`
		Port           int    `json:"port"`
		CaptureID      string `json:"capture_id,omitempty"`
		Alias          string `json:"alias,omitempty"`
		CustomImage    string `json:"custom_image,omitempty"`
		Tag1           string `json:"tag1,omitempty"`
		Tag2           string `json:"tag2,omitempty"`
		Tag3           string `json:"tag3,omitempty"`
		Tag4           string `json:"tag4,omitempty"`
		Hit            bool   `json:"hit"`
		LoadedPrefixes int    `json:"loaded_prefixes"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

// V4AliasesLookup answers GET /api/v4/aliases/lookup?ip=&port=&capture_id=
// with the result of a single LPM lookup against the cached IPAliasMap.
//
// This is the diagnostic counterpart of the silent enrichment that
// happens on every search/transactions response — when row enrichment
// does not produce aliasSrc/aliasDst, hit this endpoint with the same
// tuple to learn whether (a) the LPM table is empty (loaded_prefixes=0),
// (b) the prefix is missing, or (c) the port/capture_id rules excluded it.
func (h *AliasesHandler) V4AliasesLookup(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	ip := strings.TrimSpace(c.QueryParam("ip"))
	if ip == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "ip is required")
	}
	port := 0
	if p := strings.TrimSpace(c.QueryParam("port")); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 65535 {
			return writeError(c, http.StatusBadRequest, "Bad Request", "port must be an integer in [0, 65535]")
		}
		port = n
	}
	captureID := strings.TrimSpace(c.QueryParam("capture_id"))

	m, err := h.service.CachedIPAliasMap(c.Request().Context())
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to load alias map: "+err.Error())
	}

	entry, ok := services.ResolveAliasEntry(m, ip, port, captureID)

	resp := AliasLookupResponseV4{}
	resp.Data.IP = ip
	resp.Data.Port = port
	resp.Data.CaptureID = captureID
	resp.Data.Alias = entry.Name
	resp.Data.CustomImage = entry.CustomImage
	resp.Data.Tag1 = entry.Tag1
	resp.Data.Tag2 = entry.Tag2
	resp.Data.Tag3 = entry.Tag3
	resp.Data.Tag4 = entry.Tag4
	resp.Data.Hit = ok
	resp.Data.LoadedPrefixes = m.Size()
	resp.Meta = buildMeta(c, "")
	return c.JSON(http.StatusOK, resp)
}

func (h *AliasesHandler) V4AliasesList(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	filters, err := parseAliasFilters(c)
	if err != nil {
		if httpErr, ok := err.(*echo.HTTPError); ok {
			return writeError(c, httpErr.Code, "Bad Request", httpErr.Message.(string))
		}
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid query parameters")
	}

	items, err := h.service.List(c.Request().Context(), filters)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to list aliases")
	}

	resp := AliasListResponseV4{}
	resp.Data.Items = mapAliases(items)
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: filters.Limit, Total: len(items)}
	return c.JSON(http.StatusOK, resp)
}

func (h *AliasesHandler) V4AliasesCreate(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	var payload AliasV4
	if err := c.Bind(&payload); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	if payload.Alias == "" || payload.IP == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "alias and ip are required")
	}

	guid, err := h.service.Create(c.Request().Context(), mapAliasToService(payload))
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to create alias")
	}

	resp := IdResponseV4{
		Data: guid,
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusCreated, resp)
}

func (h *AliasesHandler) V4AliasesUpdate(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	aliasID := c.Param("aliasId")
	if aliasID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Alias id is required")
	}

	var payload AliasV4
	if err := c.Bind(&payload); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}

	updated, err := h.service.Update(c.Request().Context(), aliasID, mapAliasToService(payload))
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to update alias")
	}
	if updated == "" {
		return writeError(c, http.StatusNotFound, "Not Found", "Alias not found")
	}

	resp := IdResponseV4{
		Data: updated,
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *AliasesHandler) V4AliasesDelete(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	aliasID := c.Param("aliasId")
	if aliasID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Alias id is required")
	}

	deleted, err := h.service.Delete(c.Request().Context(), aliasID)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to delete alias")
	}
	if !deleted {
		return writeError(c, http.StatusNotFound, "Not Found", "Alias not found")
	}
	return c.NoContent(http.StatusNoContent)
}

func parseAliasFilters(c echo.Context) (services.AliasListFilters, error) {
	filters := services.AliasListFilters{
		Alias:     c.QueryParam("filter[alias]"),
		IP:        c.QueryParam("filter[ip]"),
		CaptureID: c.QueryParam("filter[capture_id]"),
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

	sortParam := c.QueryParam("sort")
	if sortParam != "" {
		sortExpr, err := parseAliasSort(sortParam)
		if err != nil {
			return filters, err
		}
		filters.Sort = sortExpr
	}
	return filters, nil
}

func parseAliasSort(value string) (string, error) {
	desc := false
	field := value
	if strings.HasPrefix(value, "-") {
		desc = true
		field = strings.TrimPrefix(value, "-")
	}

	allowed := map[string]string{
		"id": "guid", // lake table has no id column
		"guid":          "guid",
		"alias":         "alias",
		"ip":            "ip",
		"port":          "port",
		"mask":          "mask",
		"capture_id":    "capture_id",
		"status":        "status",
		"custom_image":  "custom_image",
		"tag1":          "tag1",
		"tag2":          "tag2",
		"tag3":          "tag3",
		"tag4":          "tag4",
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

func mapAliases(items []services.AliasItem) []AliasV4 {
	result := make([]AliasV4, 0, len(items))
	for _, item := range items {
		result = append(result, mapAlias(item))
	}
	return result
}

func mapAlias(item services.AliasItem) AliasV4 {
	return AliasV4{
		ID:          item.ID,
		GUID:        item.GUID,
		Alias:       item.Alias,
		IP:          item.IP,
		Port:        item.Port,
		Mask:        item.Mask,
		CaptureID:   item.CaptureID,
		Status:      item.Status,
		CustomImage: item.CustomImage,
		Tag1:        item.Tag1,
		Tag2:        item.Tag2,
		Tag3:        item.Tag3,
		Tag4:        item.Tag4,
	}
}

func mapAliasToService(item AliasV4) services.AliasItem {
	return services.AliasItem{
		ID:          item.ID,
		GUID:        item.GUID,
		Alias:       item.Alias,
		IP:          item.IP,
		Port:        item.Port,
		Mask:        item.Mask,
		CaptureID:   item.CaptureID,
		Status:      item.Status,
		CustomImage: item.CustomImage,
		Tag1:        item.Tag1,
		Tag2:        item.Tag2,
		Tag3:        item.Tag3,
		Tag4:        item.Tag4,
	}
}
