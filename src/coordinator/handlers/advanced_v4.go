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

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

type AdvancedHandler struct {
	service *services.GlobalSettingsService
}

func NewAdvancedHandler(service *services.GlobalSettingsService) *AdvancedHandler {
	return &AdvancedHandler{service: service}
}

type GlobalSettingV4 struct {
	ID       int64           `json:"id,omitempty"`
	GUID     string          `json:"guid,omitempty"`
	PartID   int             `json:"partid,omitempty"`
	Category string          `json:"category"`
	Param    string          `json:"param"`
	Data     json.RawMessage `json:"data"`
}

type GlobalSettingListResponseV4 struct {
	Data struct {
		Items []GlobalSettingV4 `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type GlobalSettingResponseV4 struct {
	Data GlobalSettingV4 `json:"data"`
	Meta Meta            `json:"meta"`
}

func (h *AdvancedHandler) V4AdvancedList(c echo.Context) error {
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

	filters := services.GlobalSettingsListFilters{
		Category: c.QueryParam("filter[category]"),
		Param:    c.QueryParam("filter[param]"),
		Limit:    limit,
	}

	items, err := h.service.List(c.Request().Context(), filters)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to list advanced settings")
	}

	resp := GlobalSettingListResponseV4{}
	resp.Data.Items = make([]GlobalSettingV4, 0, len(items))
	for _, item := range items {
		resp.Data.Items = append(resp.Data.Items, mapGlobalSetting(item))
	}
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: limit, Total: len(items)}
	return c.JSON(http.StatusOK, resp)
}

func (h *AdvancedHandler) V4AdvancedGet(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	guid := c.Param("guid")
	if guid == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "guid is required")
	}

	item, err := h.service.GetByGUID(c.Request().Context(), guid)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to get advanced setting")
	}
	if item == nil {
		return writeError(c, http.StatusNotFound, "Not Found", "Advanced setting not found")
	}

	resp := GlobalSettingResponseV4{
		Data: mapGlobalSetting(*item),
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *AdvancedHandler) V4AdvancedCreate(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	var payload GlobalSettingV4
	if err := c.Bind(&payload); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	if payload.Category == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "category is required")
	}

	guid, err := h.service.Create(c.Request().Context(), mapGlobalSettingToService(payload))
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to create advanced setting")
	}

	resp := IdResponseV4{Data: guid, Meta: buildMeta(c, "")}
	return c.JSON(http.StatusCreated, resp)
}

func (h *AdvancedHandler) V4AdvancedUpdate(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	guid := c.Param("guid")
	if guid == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "guid is required")
	}

	var payload GlobalSettingV4
	if err := c.Bind(&payload); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}

	updated, err := h.service.Update(c.Request().Context(), guid, mapGlobalSettingToService(payload))
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to update advanced setting")
	}
	if updated == "" {
		return writeError(c, http.StatusNotFound, "Not Found", "Advanced setting not found")
	}

	resp := IdResponseV4{Data: updated, Meta: buildMeta(c, "")}
	return c.JSON(http.StatusOK, resp)
}

func (h *AdvancedHandler) V4AdvancedDelete(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	guid := c.Param("guid")
	if guid == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "guid is required")
	}

	deleted, err := h.service.Delete(c.Request().Context(), guid)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to delete advanced setting")
	}
	if !deleted {
		return writeError(c, http.StatusNotFound, "Not Found", "Advanced setting not found")
	}
	return c.NoContent(http.StatusNoContent)
}

func mapGlobalSetting(item services.GlobalSetting) GlobalSettingV4 {
	data := item.Data
	if len(data) == 0 {
		data = json.RawMessage("{}")
	}
	return GlobalSettingV4{
		ID:       item.ID,
		GUID:     item.GUID,
		PartID:   item.PartID,
		Category: item.Category,
		Param:    item.Param,
		Data:     data,
	}
}

func mapGlobalSettingToService(item GlobalSettingV4) services.GlobalSetting {
	data := item.Data
	if len(data) == 0 {
		data = json.RawMessage("{}")
	}
	return services.GlobalSetting{
		GUID:     item.GUID,
		PartID:   item.PartID,
		Category: item.Category,
		Param:    item.Param,
		Data:     data,
	}
}
