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

type MappingsHandler struct {
	service        *services.MappingService
	userMappingSvc *services.UserMappingService
}

func NewMappingsHandler(service *services.MappingService, userMappingSvc *services.UserMappingService) *MappingsHandler {
	return &MappingsHandler{service: service, userMappingSvc: userMappingSvc}
}

// MappingSchemaWithUserV4 extends MappingSchemaV4 with per-user field overrides.
// UserMapping, when non-nil, contains the user's saved field ordering/selection
// for this protocol; the client should merge it with FieldsMapping before rendering.
type MappingSchemaWithUserV4 struct {
	MappingSchemaV4
	UserMapping json.RawMessage `json:"user_mapping,omitempty"`
}

type MappingMergedListResponseV4 struct {
	Data struct {
		Items []MappingSchemaWithUserV4 `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type MappingWidgetFieldsResponseV4 struct {
	Data struct {
		Items json.RawMessage `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type MappingSchemaV4 struct {
	GUID           string          `json:"guid"`
	Profile        string          `json:"profile"`
	HepID          int             `json:"hepid"`
	HepAlias       string          `json:"hep_alias"`
	PartID         int             `json:"partid"`
	Version        int             `json:"version"`
	Retention      int             `json:"retention"`
	PartitionStep  int             `json:"partition_step"`
	CreateIndex    json.RawMessage `json:"create_index"`
	CreateTable    string          `json:"create_table"`
	CorrelationMap json.RawMessage `json:"correlation_mapping"`
	FieldsMapping  json.RawMessage `json:"fields_mapping"`
	FieldsSettings json.RawMessage `json:"fields_settings"`
	SchemaMapping  json.RawMessage `json:"schema_mapping"`
	SchemaSettings json.RawMessage `json:"schema_settings"`
}

type MappingResponseV4 struct {
	Data MappingSchemaV4 `json:"data"`
	Meta Meta            `json:"meta"`
}

type MappingListResponseV4 struct {
	Data struct {
		Items []MappingSchemaV4 `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type SmartSearchFieldV4 struct {
	Category string `json:"category"`
	Name     string `json:"name"`
	Value    string `json:"value"`
}

type MappingFieldsResponseV4 struct {
	Data struct {
		Items []SmartSearchFieldV4 `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

func (h *MappingsHandler) V4MappingsList(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	filters, err := parseMappingFilters(c)
	if err != nil {
		if httpErr, ok := err.(*echo.HTTPError); ok {
			return writeError(c, httpErr.Code, "Bad Request", httpErr.Message.(string))
		}
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid query parameters")
	}

	items, err := h.service.ListMappings(c.Request().Context(), filters)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to list mappings")
	}

	resp := MappingListResponseV4{}
	resp.Data.Items = mapMappings(items)
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: filters.Limit, Total: len(items)}
	return c.JSON(http.StatusOK, resp)
}

func (h *MappingsHandler) V4MappingsCreate(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	var payload MappingSchemaV4
	if err := c.Bind(&payload); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	if payload.Profile == "" || payload.HepID == 0 {
		return writeError(c, http.StatusBadRequest, "Bad Request", "profile and hepid are required")
	}

	guid, err := h.service.CreateMapping(c.Request().Context(), mapMappingToService(payload))
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to create mapping")
	}

	resp := IdResponseV4{
		Data: guid,
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusCreated, resp)
}

func (h *MappingsHandler) V4MappingsReset(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}
	if err := h.service.ResetMappings(c.Request().Context()); err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to reset mappings")
	}
	resp := MessageResponseV4{
		Data: map[string]interface{}{
			"message": "reset",
		},
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *MappingsHandler) V4MappingsGet(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	mappingID := c.Param("mappingId")
	if mappingID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Mapping id is required")
	}

	mapping, err := h.service.GetMappingByGUID(c.Request().Context(), mappingID)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to get mapping")
	}
	if mapping == nil {
		return writeError(c, http.StatusNotFound, "Not Found", "Mapping not found")
	}

	resp := MappingResponseV4{
		Data: mapMapping(*mapping),
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *MappingsHandler) V4MappingsUpdate(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	mappingID := c.Param("mappingId")
	if mappingID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Mapping id is required")
	}

	var payload MappingSchemaV4
	if err := c.Bind(&payload); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}

	updated, err := h.service.UpdateMapping(c.Request().Context(), mappingID, mapMappingToService(payload))
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to update mapping")
	}
	if updated == "" {
		return writeError(c, http.StatusNotFound, "Not Found", "Mapping not found")
	}

	resp := IdResponseV4{
		Data: updated,
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *MappingsHandler) V4MappingsDelete(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	mappingID := c.Param("mappingId")
	if mappingID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Mapping id is required")
	}

	deleted, err := h.service.DeleteMapping(c.Request().Context(), mappingID)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to delete mapping")
	}
	if !deleted {
		return writeError(c, http.StatusNotFound, "Not Found", "Mapping not found")
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *MappingsHandler) V4MappingsFields(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	hepidStr := c.QueryParam("hepid")
	profile := c.QueryParam("profile")
	if hepidStr == "" || profile == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "hepid and profile are required")
	}
	hepid, err := strconv.Atoi(hepidStr)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid hepid")
	}

	mapping, err := h.service.GetMappingByProfile(c.Request().Context(), hepid, profile)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to get mapping")
	}
	if mapping == nil {
		return writeError(c, http.StatusNotFound, "Not Found", "Mapping not found")
	}

	fields := parseFieldsMapping(mapping.FieldsMapping)
	resp := MappingFieldsResponseV4{}
	resp.Data.Items = fields
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: len(fields), Total: len(fields)}
	return c.JSON(http.StatusOK, resp)
}

// V4MappingsWidgetFields returns the full fields_mapping array for a protocol,
// preserving all field attributes needed by the drag-drop widget UI
// (id, name, alias, form_type, type, category, position, hide, skip, etc.).
func (h *MappingsHandler) V4MappingsWidgetFields(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	hepidStr := c.QueryParam("hepid")
	profile := c.QueryParam("profile")
	if hepidStr == "" || profile == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "hepid and profile are required")
	}
	hepid, err := strconv.Atoi(hepidStr)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid hepid")
	}

	mapping, err := h.service.GetMappingByProfile(c.Request().Context(), hepid, profile)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to get mapping")
	}
	if mapping == nil {
		return writeError(c, http.StatusNotFound, "Not Found", "Mapping not found")
	}

	raw := mapping.FieldsMapping
	if len(raw) == 0 {
		raw = json.RawMessage(`[]`)
	}
	resp := MappingWidgetFieldsResponseV4{}
	resp.Data.Items = raw
	resp.Meta = buildMeta(c, "")
	return c.JSON(http.StatusOK, resp)
}

// V4MappingsMerged returns all protocol mappings enriched with the current user's
// field overrides (user_mapping). The client can use user_mapping directly or
// merge it with the base fields_mapping for the drag-drop widget configuration.
func (h *MappingsHandler) V4MappingsMerged(c echo.Context) error {
	username, err := getUsernameFromContext(c)
	if err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	filters := services.MappingListFilters{Limit: 1000}
	items, err := h.service.ListMappings(c.Request().Context(), filters)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to list mappings")
	}

	// Build a lookup of user overrides keyed by "hepid_profile"
	userMappings := map[string]json.RawMessage{}
	if h.userMappingSvc != nil {
		overrides, err := h.userMappingSvc.ListForUser(c.Request().Context(), username)
		if err == nil {
			for _, s := range overrides {
				userMappings[s.Param] = s.Data
			}
		}
	}

	result := make([]MappingSchemaWithUserV4, 0, len(items))
	for _, item := range items {
		entry := MappingSchemaWithUserV4{MappingSchemaV4: mapMapping(item)}
		key := strings.Join([]string{strconv.Itoa(item.HepID), item.Profile}, "_")
		if um, ok := userMappings[key]; ok {
			entry.UserMapping = um
		}
		result = append(result, entry)
	}

	resp := MappingMergedListResponseV4{}
	resp.Data.Items = result
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: filters.Limit, Total: len(result)}
	return c.JSON(http.StatusOK, resp)
}

func parseMappingFilters(c echo.Context) (services.MappingListFilters, error) {
	filters := services.MappingListFilters{
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
		sortExpr, err := parseMappingSort(sortParam)
		if err != nil {
			return filters, err
		}
		filters.Sort = sortExpr
	}
	return filters, nil
}

func parseMappingSort(value string) (string, error) {
	desc := false
	field := value
	if strings.HasPrefix(value, "-") {
		desc = true
		field = strings.TrimPrefix(value, "-")
	}

	allowed := map[string]string{
		"guid":           "guid",
		"profile":        "profile",
		"hepid":          "hepid",
		"hep_alias":      "hep_alias",
		"partid":         "partid",
		"version":        "version",
		"retention":      "retention",
		"partition_step": "partition_step",
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

func mapMappings(items []services.MappingSchema) []MappingSchemaV4 {
	result := make([]MappingSchemaV4, 0, len(items))
	for _, item := range items {
		result = append(result, mapMapping(item))
	}
	return result
}

func mapMapping(item services.MappingSchema) MappingSchemaV4 {
	return MappingSchemaV4{
		GUID:           item.GUID,
		Profile:        item.Profile,
		HepID:          item.HepID,
		HepAlias:       item.HepAlias,
		PartID:         item.PartID,
		Version:        item.Version,
		Retention:      item.Retention,
		PartitionStep:  item.PartitionStep,
		CreateIndex:    item.CreateIndex,
		CreateTable:    item.CreateTable,
		CorrelationMap: item.CorrelationMap,
		FieldsMapping:  item.FieldsMapping,
		FieldsSettings: item.FieldsSettings,
		SchemaMapping:  item.SchemaMapping,
		SchemaSettings: item.SchemaSettings,
	}
}

func mapMappingToService(item MappingSchemaV4) services.MappingSchema {
	return services.MappingSchema{
		GUID:           item.GUID,
		Profile:        item.Profile,
		HepID:          item.HepID,
		HepAlias:       item.HepAlias,
		PartID:         item.PartID,
		Version:        item.Version,
		Retention:      item.Retention,
		PartitionStep:  item.PartitionStep,
		CreateIndex:    normalizeJSON(item.CreateIndex),
		CreateTable:    item.CreateTable,
		CorrelationMap: normalizeJSON(item.CorrelationMap),
		FieldsMapping:  normalizeJSON(item.FieldsMapping),
		FieldsSettings: normalizeJSON(item.FieldsSettings),
		SchemaMapping:  normalizeJSON(item.SchemaMapping),
		SchemaSettings: normalizeJSON(item.SchemaSettings),
	}
}

func normalizeJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

func parseFieldsMapping(raw json.RawMessage) []SmartSearchFieldV4 {
	fields := make([]SmartSearchFieldV4, 0)
	if len(raw) == 0 {
		return fields
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal(raw, &arr); err == nil {
		for _, entry := range arr {
			field := mapField(entry)
			if field.Value != "" {
				fields = append(fields, field)
			}
		}
		return fields
	}

	var wrapper map[string]interface{}
	if err := json.Unmarshal(raw, &wrapper); err == nil {
		if data, ok := wrapper["data"].([]interface{}); ok {
			for _, item := range data {
				if entry, ok := item.(map[string]interface{}); ok {
					field := mapField(entry)
					if field.Value != "" {
						fields = append(fields, field)
					}
				}
			}
		}
	}
	return fields
}

func mapField(entry map[string]interface{}) SmartSearchFieldV4 {
	field := SmartSearchFieldV4{}
	if v, ok := entry["category"].(string); ok {
		field.Category = v
	}
	if v, ok := entry["name"].(string); ok {
		field.Name = v
	}
	if v, ok := entry["id"].(string); ok {
		field.Value = v
		if field.Name == "" {
			field.Name = v
		}
	}
	return field
}
