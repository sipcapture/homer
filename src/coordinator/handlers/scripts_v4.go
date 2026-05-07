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

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

type ScriptsHandler struct {
	service *services.ScriptsService
}

func NewScriptsHandler(service *services.ScriptsService) *ScriptsHandler {
	return &ScriptsHandler{service: service}
}

type HepScriptV4 struct {
	ID       int64  `json:"id,omitempty"`
	GUID     string `json:"guid,omitempty"`
	Profile  string `json:"profile"`
	HepAlias string `json:"hep_alias"`
	Type     string `json:"type"`
	HepID    int    `json:"hepid"`
	Status   bool   `json:"status"`
	Script   string `json:"script"`
}

type HepScriptListResponseV4 struct {
	Data struct {
		Items []HepScriptV4 `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type HepScriptResponseV4 struct {
	Data HepScriptV4 `json:"data"`
	Meta Meta        `json:"meta"`
}

func (h *ScriptsHandler) V4ScriptsList(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	limitStr := c.QueryParam("page[limit]")
	limit := 100
	if limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 && v <= 1000 {
			limit = v
		}
	}

	filters := services.ScriptsListFilters{
		Profile: c.QueryParam("filter[profile]"),
		Type:    c.QueryParam("filter[type]"),
		Limit:   limit,
	}

	items, err := h.service.List(c.Request().Context(), filters)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to list scripts")
	}

	resp := HepScriptListResponseV4{}
	resp.Data.Items = make([]HepScriptV4, 0, len(items))
	for _, item := range items {
		resp.Data.Items = append(resp.Data.Items, mapHepScript(item))
	}
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: limit, Total: len(items)}
	return c.JSON(http.StatusOK, resp)
}

func (h *ScriptsHandler) V4ScriptsGet(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	scriptID := c.Param("scriptId")
	if scriptID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "scriptId is required")
	}

	item, err := h.service.GetByGUID(c.Request().Context(), scriptID)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to get script")
	}
	if item == nil {
		return writeError(c, http.StatusNotFound, "Not Found", "Script not found")
	}

	resp := HepScriptResponseV4{
		Data: mapHepScript(*item),
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *ScriptsHandler) V4ScriptsCreate(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	var payload HepScriptV4
	if err := c.Bind(&payload); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}

	guid, err := h.service.Create(c.Request().Context(), mapHepScriptToService(payload))
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to create script")
	}

	resp := IdResponseV4{Data: guid, Meta: buildMeta(c, "")}
	return c.JSON(http.StatusCreated, resp)
}

func (h *ScriptsHandler) V4ScriptsUpdate(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	scriptID := c.Param("scriptId")
	if scriptID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "scriptId is required")
	}

	var payload HepScriptV4
	if err := c.Bind(&payload); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}

	updated, err := h.service.Update(c.Request().Context(), scriptID, mapHepScriptToService(payload))
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to update script")
	}
	if updated == "" {
		return writeError(c, http.StatusNotFound, "Not Found", "Script not found")
	}

	resp := IdResponseV4{Data: updated, Meta: buildMeta(c, "")}
	return c.JSON(http.StatusOK, resp)
}

func (h *ScriptsHandler) V4ScriptsDelete(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	scriptID := c.Param("scriptId")
	if scriptID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "scriptId is required")
	}

	deleted, err := h.service.Delete(c.Request().Context(), scriptID)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to delete script")
	}
	if !deleted {
		return writeError(c, http.StatusNotFound, "Not Found", "Script not found")
	}
	return c.NoContent(http.StatusNoContent)
}

func mapHepScript(item services.HepScript) HepScriptV4 {
	return HepScriptV4{
		ID:       item.ID,
		GUID:     item.GUID,
		Profile:  item.Profile,
		HepAlias: item.HepAlias,
		Type:     item.Type,
		HepID:    item.HepID,
		Status:   item.Status,
		Script:   item.Script,
	}
}

func mapHepScriptToService(item HepScriptV4) services.HepScript {
	return services.HepScript{
		GUID:     item.GUID,
		Profile:  item.Profile,
		HepAlias: item.HepAlias,
		Type:     item.Type,
		HepID:    item.HepID,
		Status:   item.Status,
		Script:   item.Script,
	}
}
