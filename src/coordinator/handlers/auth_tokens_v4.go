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

type AuthTokensHandler struct {
	service *services.AuthTokenService
}

func NewAuthTokensHandler(service *services.AuthTokenService) *AuthTokensHandler {
	return &AuthTokensHandler{service: service}
}

type AuthTokenItemV4 struct {
	GUID          string          `json:"guid"`
	CreatorGUID   string          `json:"creator_guid,omitempty"`
	Name          string          `json:"name"`
	UserObject    json.RawMessage `json:"user_object,omitempty"`
	IPAddress     string          `json:"ip_address,omitempty"`
	CreateDate    string          `json:"create_date,omitempty"`
	LastUsageDate string          `json:"lastusage_date,omitempty"`
	ExpireDate    string          `json:"expire_date,omitempty"`
	UsageCalls    int             `json:"usage_calls,omitempty"`
	LimitCalls    int             `json:"limit_calls,omitempty"`
	Active        bool            `json:"active"`
}

type AuthTokenCreateRequest struct {
	Name       string `json:"name"`
	ExpireDate string `json:"expire_date"`
	LimitCalls int    `json:"limit_calls"`
	Active     *bool  `json:"active"`
}

type AuthTokenUpdateRequest struct {
	Name       *string `json:"name"`
	ExpireDate *string `json:"expire_date"`
	LimitCalls *int    `json:"limit_calls"`
	Active     *bool   `json:"active"`
}

type AuthTokenResponseV4 struct {
	Data AuthTokenItemV4 `json:"data"`
	Meta Meta            `json:"meta"`
}

type AuthTokenListResponseV4 struct {
	Data struct {
		Items []AuthTokenItemV4 `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type AuthTokenCreateResponseV4 struct {
	Data struct {
		Token string `json:"token"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

func (h *AuthTokensHandler) V4AuthTokensList(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	filters, err := parseAuthTokenFilters(c)
	if err != nil {
		if httpErr, ok := err.(*echo.HTTPError); ok {
			return writeError(c, httpErr.Code, "Bad Request", httpErr.Message.(string))
		}
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid query parameters")
	}

	items, err := h.service.List(c.Request().Context(), filters)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to list tokens")
	}

	resp := AuthTokenListResponseV4{}
	resp.Data.Items = mapAuthTokens(items)
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: filters.Limit, Total: len(items)}
	return c.JSON(http.StatusOK, resp)
}

func (h *AuthTokensHandler) V4AuthTokensCreate(c echo.Context) error {
	username, err := getUsernameFromContext(c)
	if err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	var payload AuthTokenCreateRequest
	if err := c.Bind(&payload); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	if payload.Name == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "name is required")
	}

	active := true
	if payload.Active != nil {
		active = *payload.Active
	}

	userObject, _ := json.Marshal(map[string]interface{}{
		"username":   username,
		"user_group": userGroupLabel(c),
	})

	item := services.AuthTokenItem{
		CreatorGUID: username,
		Name:        payload.Name,
		UserObject:  userObject,
		IPAddress:   c.RealIP(),
		ExpireDate:  payload.ExpireDate,
		LimitCalls:  payload.LimitCalls,
		Active:      active,
	}

	created, err := h.service.Create(c.Request().Context(), item)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to create token")
	}

	resp := AuthTokenCreateResponseV4{Meta: buildMeta(c, "")}
	resp.Data.Token = created.Token
	return c.JSON(http.StatusCreated, resp)
}

func (h *AuthTokensHandler) V4AuthTokensGet(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	tokenID := c.Param("tokenId")
	if tokenID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Token id is required")
	}

	item, err := h.service.GetByGUID(c.Request().Context(), tokenID)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to get token")
	}
	if item == nil {
		return writeError(c, http.StatusNotFound, "Not Found", "Token not found")
	}

	resp := AuthTokenResponseV4{
		Data: mapAuthToken(*item),
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *AuthTokensHandler) V4AuthTokensUpdate(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	tokenID := c.Param("tokenId")
	if tokenID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Token id is required")
	}

	var payload AuthTokenUpdateRequest
	if err := c.Bind(&payload); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}

	current, err := h.service.GetByGUID(c.Request().Context(), tokenID)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to get token")
	}
	if current == nil {
		return writeError(c, http.StatusNotFound, "Not Found", "Token not found")
	}

	updated := *current
	if payload.Name != nil {
		updated.Name = *payload.Name
	}
	if payload.ExpireDate != nil {
		updated.ExpireDate = *payload.ExpireDate
	}
	if payload.LimitCalls != nil {
		updated.LimitCalls = *payload.LimitCalls
	}
	if payload.Active != nil {
		updated.Active = *payload.Active
	}

	if updated.Name == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "name is required")
	}

	result, err := h.service.Update(c.Request().Context(), tokenID, updated)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to update token")
	}
	if result == "" {
		return writeError(c, http.StatusNotFound, "Not Found", "Token not found")
	}

	resp := IdResponseV4{
		Data: result,
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *AuthTokensHandler) V4AuthTokensDelete(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	tokenID := c.Param("tokenId")
	if tokenID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Token id is required")
	}

	deleted, err := h.service.Delete(c.Request().Context(), tokenID)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to delete token")
	}
	if !deleted {
		return writeError(c, http.StatusNotFound, "Not Found", "Token not found")
	}
	return c.NoContent(http.StatusNoContent)
}

func parseAuthTokenFilters(c echo.Context) (services.AuthTokenListFilters, error) {
	filters := services.AuthTokenListFilters{
		Name: c.QueryParam("filter[name]"),
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

	if activeStr := c.QueryParam("filter[active]"); activeStr != "" {
		val, err := parseBool(activeStr)
		if err != nil {
			return filters, echo.NewHTTPError(http.StatusBadRequest, "Invalid filter[active]")
		}
		filters.Active = &val
	}

	sortParam := c.QueryParam("sort")
	if sortParam != "" {
		sortExpr, err := parseAuthTokenSort(sortParam)
		if err != nil {
			return filters, err
		}
		filters.Sort = sortExpr
	}
	return filters, nil
}

func parseAuthTokenSort(value string) (string, error) {
	desc := false
	field := value
	if strings.HasPrefix(value, "-") {
		desc = true
		field = strings.TrimPrefix(value, "-")
	}

	allowed := map[string]string{
		"guid":           "guid",
		"name":           "name",
		"create_date":    "create_date",
		"lastusage_date": "lastusage_date",
		"expire_date":    "expire_date",
		"usage_calls":    "usage_calls",
		"limit_calls":    "limit_calls",
		"active":         "active",
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

func mapAuthTokens(items []services.AuthTokenItem) []AuthTokenItemV4 {
	result := make([]AuthTokenItemV4, 0, len(items))
	for _, item := range items {
		result = append(result, mapAuthToken(item))
	}
	return result
}

func mapAuthToken(item services.AuthTokenItem) AuthTokenItemV4 {
	userObject := item.UserObject
	if userObject == nil {
		userObject = json.RawMessage(`{}`)
	}
	return AuthTokenItemV4{
		GUID:          item.GUID,
		CreatorGUID:   item.CreatorGUID,
		Name:          item.Name,
		UserObject:    userObject,
		IPAddress:     item.IPAddress,
		CreateDate:    item.CreateDate,
		LastUsageDate: item.LastUsageDate,
		ExpireDate:    item.ExpireDate,
		UsageCalls:    item.UsageCalls,
		LimitCalls:    item.LimitCalls,
		Active:        item.Active,
	}
}

func userGroupLabel(c echo.Context) string {
	if isAdmin(c) {
		return "admin"
	}
	return "user"
}
