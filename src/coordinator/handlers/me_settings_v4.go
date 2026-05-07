// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

type UserSettingsHandler struct {
	service     *services.UserSettingsService
	userMapping *services.UserMappingService
}

func NewUserSettingsHandler(service *services.UserSettingsService, userMapping *services.UserMappingService) *UserSettingsHandler {
	return &UserSettingsHandler{service: service, userMapping: userMapping}
}

type UserSettingItemV4 struct {
	ID       int64           `json:"id,omitempty"`
	GUID     string          `json:"guid,omitempty"`
	UserName string          `json:"user_name,omitempty"`
	PartID   int             `json:"partid,omitempty"`
	Category string          `json:"category,omitempty"`
	Param    string          `json:"param,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
}

type UserSettingListResponseV4 struct {
	Data struct {
		Items []UserSettingItemV4 `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type UserSettingUpsertRequest struct {
	Param string          `json:"param"`
	Data  json.RawMessage `json:"data"`
}

func (h *UserSettingsHandler) V4UserSettingsList(c echo.Context) error {
	username, err := getUsernameFromContext(c)
	if err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	settings, err := h.service.ListByUser(c.Request().Context(), username)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to load settings")
	}

	all := settings
	if h.userMapping != nil {
		mappings, err := h.userMapping.ListForUser(c.Request().Context(), username)
		if err != nil {
			return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to load settings")
		}
		all = append(make([]services.UserSetting, 0, len(settings)+len(mappings)), settings...)
		all = append(all, mappings...)
	}

	resp := UserSettingListResponseV4{}
	resp.Data.Items = make([]UserSettingItemV4, 0, len(all))
	for _, setting := range all {
		resp.Data.Items = append(resp.Data.Items, mapSettingToV4(setting))
	}
	resp.Meta = buildMeta(c, "")

	return c.JSON(http.StatusOK, resp)
}

func (h *UserSettingsHandler) V4UserSettingsUpsert(c echo.Context) error {
	username, err := getUsernameFromContext(c)
	if err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	category := c.Param("category")
	if category == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Category is required")
	}

	var req UserSettingUpsertRequest
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	if req.Param == "" || len(req.Data) == 0 {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Param and data are required")
	}

	guid, err := h.service.UpsertByCategory(c.Request().Context(), username, category, req.Param, req.Data, 10)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to update settings")
	}

	resp := IdResponseV4{
		Data: guid,
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UserSettingsHandler) V4UserSettingsDelete(c echo.Context) error {
	username, err := getUsernameFromContext(c)
	if err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	category := c.Param("category")
	if category == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Category is required")
	}

	deleted, err := h.service.DeleteByCategory(c.Request().Context(), username, category)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to delete settings")
	}
	if !deleted {
		return writeError(c, http.StatusNotFound, "Not Found", "Settings not found")
	}

	return c.NoContent(http.StatusNoContent)
}

func getUsernameFromContext(c echo.Context) (string, error) {
	user := c.Get("user")
	if user == nil {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "Missing user context")
	}
	token, ok := user.(*jwt.Token)
	if !ok {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "Invalid token")
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok || claims.Username == "" {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "Invalid claims")
	}
	return claims.Username, nil
}

// V4UserMappingsList returns all protocol field overrides saved by the current user.
// Each item includes the protocol key (hepid_profile) and the saved field array.
func (h *UserSettingsHandler) V4UserMappingsList(c echo.Context) error {
	username, err := getUsernameFromContext(c)
	if err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	if h.userMapping == nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "User mappings not available")
	}
	settings, err := h.userMapping.ListForUser(c.Request().Context(), username)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to load user mappings")
	}

	resp := UserSettingListResponseV4{}
	resp.Data.Items = make([]UserSettingItemV4, 0, len(settings))
	for _, s := range settings {
		resp.Data.Items = append(resp.Data.Items, mapSettingToV4(s))
	}
	resp.Meta = buildMeta(c, "")
	return c.JSON(http.StatusOK, resp)
}

// V4UserMappingsGet returns the field overrides saved by the current user for a specific
// protocol identified by hepid and profile path parameters.
func (h *UserSettingsHandler) V4UserMappingsGet(c echo.Context) error {
	username, err := getUsernameFromContext(c)
	if err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	param, err := parseUserMappingParam(c)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", err.Error())
	}

	if h.userMapping == nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "User mappings not available")
	}
	setting, err := h.userMapping.Get(c.Request().Context(), username, param)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to get user mapping")
	}
	if setting == nil {
		return writeError(c, http.StatusNotFound, "Not Found", "User mapping not found")
	}

	resp := struct {
		Data UserSettingItemV4 `json:"data"`
		Meta Meta              `json:"meta"`
	}{
		Data: mapSettingToV4(*setting),
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

// V4UserMappingsUpsert saves the current user's field selection/ordering for a protocol.
// The request body must contain a JSON array of field objects (matching the structure
// of fields_mapping entries) representing the desired active fields and their order.
func (h *UserSettingsHandler) V4UserMappingsUpsert(c echo.Context) error {
	username, err := getUsernameFromContext(c)
	if err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	param, err := parseUserMappingParam(c)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", err.Error())
	}

	var body json.RawMessage
	if err := c.Bind(&body); err != nil || len(body) == 0 {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Request body must be a JSON array of field objects")
	}

	if h.userMapping == nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "User mappings not available")
	}
	guid, err := h.userMapping.Upsert(c.Request().Context(), username, param, body, 10)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to save user mapping")
	}

	resp := IdResponseV4{
		Data: guid,
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

// V4UserMappingsDelete removes the user's field overrides for a protocol,
// effectively resetting the widget to the base fields_mapping.
func (h *UserSettingsHandler) V4UserMappingsDelete(c echo.Context) error {
	username, err := getUsernameFromContext(c)
	if err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	param, err := parseUserMappingParam(c)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", err.Error())
	}

	if h.userMapping == nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "User mappings not available")
	}
	deleted, err := h.userMapping.Delete(c.Request().Context(), username, param)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to delete user mapping")
	}
	if !deleted {
		return writeError(c, http.StatusNotFound, "Not Found", "User mapping not found")
	}
	return c.NoContent(http.StatusNoContent)
}

// parseUserMappingParam builds the storage key "{hepid}_{profile}" from path params.
func parseUserMappingParam(c echo.Context) (string, error) {
	hepidStr := c.Param("hepid")
	profile := c.Param("profile")
	if hepidStr == "" || profile == "" {
		return "", fmt.Errorf("hepid and profile are required")
	}
	if _, err := strconv.Atoi(hepidStr); err != nil {
		return "", fmt.Errorf("invalid hepid")
	}
	return hepidStr + "_" + profile, nil
}

func mapSettingToV4(setting services.UserSetting) UserSettingItemV4 {
	return UserSettingItemV4{
		ID:       setting.ID,
		GUID:     setting.GUID,
		UserName: setting.UserName,
		PartID:   setting.PartID,
		Category: setting.Category,
		Param:    setting.Param,
		Data:     setting.Data,
	}
}
