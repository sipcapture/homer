// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

type DashboardsHandler struct {
	service *services.DashboardService
}

func NewDashboardsHandler(service *services.DashboardService) *DashboardsHandler {
	return &DashboardsHandler{service: service}
}

type DashboardElementV4 struct {
	CssClass string              `json:"cssclass,omitempty"`
	Href     string              `json:"href,omitempty"`
	Id       string              `json:"id,omitempty"`
	Owner    string              `json:"owner,omitempty"`
	Name     string              `json:"name,omitempty"`
	Param    string              `json:"param,omitempty"`
	Shared   bool                `json:"shared,omitempty"`
	Type     int                 `json:"type,omitempty"`
	Weight   float64             `json:"weight,omitempty"`
	Config   *DashboardConfigV4  `json:"config,omitempty"`
	Widgets  []DashboardWidgetV4 `json:"widgets,omitempty"`
}

type DashboardConfigV4 struct {
	Columns  int    `json:"columns,omitempty"`
	GridType string `json:"grid_type,omitempty"`
	Locked   bool   `json:"locked"`
}

type DashboardWidgetV4 struct {
	Id     string                 `json:"id"`
	Type   string                 `json:"type"`
	X      int                    `json:"x"`
	Y      int                    `json:"y"`
	W      int                    `json:"w"`
	H      int                    `json:"h"`
	Title  string                 `json:"title,omitempty"`
	Config map[string]interface{} `json:"config,omitempty"`
}

type DashboardListResponseV4 struct {
	Data struct {
		Items []DashboardElementV4 `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type DashboardResponseV4 struct {
	Data DashboardElementV4 `json:"data"`
	Meta Meta               `json:"meta"`
}

func (h *DashboardsHandler) V4DashboardsList(c echo.Context) error {
	username, err := getUsernameFromContext(c)
	if err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	settings, err := h.service.ListDashboards(c.Request().Context(), username)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to list dashboards: "+err.Error())
	}

	items := make([]DashboardElementV4, 0, len(settings))
	for _, setting := range settings {
		items = append(items, mapDashboardElement(setting))
	}
	const defaultDashWeight = 10.0
	sort.Slice(items, func(i, j int) bool {
		wi, wj := items[i].Weight, items[j].Weight
		if wi == 0 {
			wi = defaultDashWeight
		}
		if wj == 0 {
			wj = defaultDashWeight
		}
		if wi != wj {
			return wi < wj
		}
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		return items[i].Id < items[j].Id
	})

	resp := DashboardListResponseV4{}
	resp.Data.Items = items
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: len(items), Total: len(items)}
	return c.JSON(http.StatusOK, resp)
}

func (h *DashboardsHandler) V4DashboardsCreate(c echo.Context) error {
	username, err := getUsernameFromContext(c)
	if err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	var element DashboardElementV4
	if err := c.Bind(&element); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}

	dashboardID := element.Id
	if dashboardID == "" {
		dashboardID = element.Param
	}
	if dashboardID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Dashboard id is required")
	}
	element.Id = dashboardID
	if element.Param == "" {
		element.Param = dashboardID
	}
	if element.Owner == "" {
		element.Owner = username
	}

	payload, err := json.Marshal(element)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid dashboard payload")
	}

	guid, err := h.service.CreateDashboard(c.Request().Context(), username, dashboardID, payload)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", err.Error())
	}

	resp := IdResponseV4{
		Data: guid,
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusCreated, resp)
}

func (h *DashboardsHandler) V4DashboardsGet(c echo.Context) error {
	username, err := getUsernameFromContext(c)
	if err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	dashboardID := c.Param("dashboardId")
	if dashboardID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Dashboard id is required")
	}

	setting, err := h.service.GetDashboard(c.Request().Context(), username, dashboardID)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to get dashboard")
	}
	if setting == nil {
		return writeError(c, http.StatusNotFound, "Not Found", "Dashboard not found")
	}

	resp := DashboardResponseV4{
		Data: mapDashboardElement(*setting),
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *DashboardsHandler) V4DashboardsUpdate(c echo.Context) error {
	username, err := getUsernameFromContext(c)
	if err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	dashboardID := c.Param("dashboardId")
	if dashboardID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Dashboard id is required")
	}

	bodyBytes, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}

	setting, err := h.service.GetDashboard(c.Request().Context(), username, dashboardID)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to load dashboard")
	}
	if setting == nil {
		return writeError(c, http.StatusNotFound, "Not Found", "Dashboard not found")
	}

	merged := map[string]interface{}{}
	if len(setting.Data) > 0 {
		if err := json.Unmarshal(setting.Data, &merged); err != nil {
			return writeError(c, http.StatusInternalServerError, "Server Error", "Invalid stored dashboard data")
		}
	}

	patch := map[string]interface{}{}
	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &patch); err != nil {
			return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid JSON")
		}
	}
	for k, v := range patch {
		merged[k] = v
	}
	merged["id"] = dashboardID
	merged["param"] = dashboardID
	if owner, ok := merged["owner"].(string); !ok || owner == "" {
		merged["owner"] = username
	}

	payload, err := json.Marshal(merged)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid dashboard payload")
	}

	guid, err := h.service.UpdateDashboard(c.Request().Context(), username, dashboardID, payload)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to update dashboard")
	}
	if guid == "" {
		return writeError(c, http.StatusNotFound, "Not Found", "Dashboard not found")
	}

	resp := IdResponseV4{
		Data: guid,
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *DashboardsHandler) V4DashboardsDelete(c echo.Context) error {
	username, err := getUsernameFromContext(c)
	if err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	dashboardID := c.Param("dashboardId")
	if dashboardID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Dashboard id is required")
	}

	deleted, err := h.service.DeleteDashboard(c.Request().Context(), username, dashboardID)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to delete dashboard")
	}
	if !deleted {
		return writeError(c, http.StatusNotFound, "Not Found", "Dashboard not found")
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *DashboardsHandler) V4DashboardsReset(c echo.Context) error {
	username, err := getUsernameFromContext(c)
	if err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	if err := h.service.ResetDashboards(c.Request().Context(), username); err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to reset dashboards")
	}

	resp := MessageResponseV4{
		Data: map[string]interface{}{
			"message": "reset",
		},
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func mapDashboardElement(setting services.UserSetting) DashboardElementV4 {
	element := DashboardElementV4{
		CssClass: "fa",
		Href:     setting.Param,
		Id:       setting.Param,
		Name:     "undefined",
		Param:    setting.Param,
		Owner:    setting.UserName,
		Shared:   false,
		Type:     0,
		Weight:   10,
	}

	var data map[string]interface{}
	if err := json.Unmarshal(setting.Data, &data); err == nil {
		if v, ok := data["cssclass"].(string); ok {
			element.CssClass = v
		}
		if v, ok := data["href"].(string); ok {
			element.Href = v
		}
		if v, ok := data["id"].(string); ok {
			element.Id = v
		}
		if v, ok := data["owner"].(string); ok {
			element.Owner = v
		}
		if v, ok := data["name"].(string); ok {
			element.Name = v
		}
		if v, ok := data["param"].(string); ok {
			element.Param = v
		}
		if v, ok := data["shared"]; ok {
			element.Shared = toBool(v)
		}
		if v, ok := data["type"]; ok {
			element.Type = int(toFloat(v))
		}
		if v, ok := data["weight"]; ok {
			element.Weight = toFloat(v)
		}

		// Parse dashboard grid config
		if cfgRaw, ok := data["config"]; ok {
			if cfgBytes, err := json.Marshal(cfgRaw); err == nil {
				var cfg DashboardConfigV4
				if json.Unmarshal(cfgBytes, &cfg) == nil {
					element.Config = &cfg
				}
			}
		}

		// Parse widget list
		if wRaw, ok := data["widgets"]; ok {
			if wBytes, err := json.Marshal(wRaw); err == nil {
				var widgets []DashboardWidgetV4
				if json.Unmarshal(wBytes, &widgets) == nil {
					element.Widgets = widgets
				}
			}
		}
	}
	return element
}

func toBool(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "1"
	case float64:
		return v != 0
	default:
		return false
	}
}

func toFloat(value interface{}) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	default:
		return 0
	}
}
