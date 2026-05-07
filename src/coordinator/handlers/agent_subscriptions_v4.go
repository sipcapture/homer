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

type AgentSubscriptionsHandler struct {
	service *services.AgentSubscriptionService
}

func NewAgentSubscriptionsHandler(service *services.AgentSubscriptionService) *AgentSubscriptionsHandler {
	return &AgentSubscriptionsHandler{service: service}
}

type AgentSubV4 struct {
	UUID       string `json:"uuid"`
	GID        int    `json:"gid"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Protocol   string `json:"protocol"`
	Path       string `json:"path"`
	Node       string `json:"node"`
	Type       string `json:"type"`
	TTL        int    `json:"ttl,omitempty"`
	CreateDate string `json:"create_date,omitempty"`
	ExpireDate string `json:"expire_date,omitempty"`
	Active     int    `json:"active"`
}

type AgentSubResponseV4 struct {
	Data AgentSubV4 `json:"data"`
	Meta Meta       `json:"meta"`
}

type AgentSubListResponseV4 struct {
	Data struct {
		Items []AgentSubV4 `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type AgentTypeListResponseV4 struct {
	Data struct {
		Items []string `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type AgentSearchRequestV4 struct {
	Guid string `json:"guid"`
	Type string `json:"type"`
}

func (h *AgentSubscriptionsHandler) V4AgentSubsList(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	filters, err := parseAgentSubFilters(c)
	if err != nil {
		if httpErr, ok := err.(*echo.HTTPError); ok {
			return writeError(c, httpErr.Code, "Bad Request", httpErr.Message.(string))
		}
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid query parameters")
	}

	items, err := h.service.List(c.Request().Context(), filters)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to list subscriptions")
	}

	resp := AgentSubListResponseV4{}
	resp.Data.Items = mapAgentSubs(items)
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: filters.Limit, Total: len(items)}
	return c.JSON(http.StatusOK, resp)
}

func (h *AgentSubscriptionsHandler) V4AgentTypesList(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	limit, err := parseLimit(c)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid page[limit]")
	}
	sortExpr, err := parseAgentTypeSort(c.QueryParam("sort"))
	if err != nil {
		if httpErr, ok := err.(*echo.HTTPError); ok {
			return writeError(c, httpErr.Code, "Bad Request", httpErr.Message.(string))
		}
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid sort")
	}
	search := c.QueryParam("search")

	items, err := h.service.ListTypes(c.Request().Context(), search, limit, sortExpr)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to list agent types")
	}

	resp := AgentTypeListResponseV4{}
	resp.Data.Items = items
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: limit, Total: len(items)}
	return c.JSON(http.StatusOK, resp)
}

func (h *AgentSubscriptionsHandler) V4AgentTypeSubscriptions(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	subType := c.Param("type")
	if subType == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Type is required")
	}

	filters, err := parseAgentSubFilters(c)
	if err != nil {
		if httpErr, ok := err.(*echo.HTTPError); ok {
			return writeError(c, httpErr.Code, "Bad Request", httpErr.Message.(string))
		}
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid query parameters")
	}
	filters.Type = subType

	items, err := h.service.List(c.Request().Context(), filters)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to list subscriptions")
	}

	resp := AgentSubListResponseV4{}
	resp.Data.Items = mapAgentSubs(items)
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: filters.Limit, Total: len(items)}
	return c.JSON(http.StatusOK, resp)
}

func (h *AgentSubscriptionsHandler) V4AgentSubsSearch(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	var payload AgentSearchRequestV4
	if err := c.Bind(&payload); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	if payload.Guid == "" && payload.Type == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "guid or type is required")
	}

	items, err := h.service.Search(c.Request().Context(), payload.Guid, payload.Type)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to search subscriptions")
	}

	resp := AgentSubListResponseV4{}
	resp.Data.Items = mapAgentSubs(items)
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: len(items), Total: len(items)}
	return c.JSON(http.StatusOK, resp)
}

func (h *AgentSubscriptionsHandler) V4AgentSubsCreate(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	var payload AgentSubV4
	if err := c.Bind(&payload); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	if payload.Type == "" || payload.Host == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "type and host are required")
	}

	created, err := h.service.Create(c.Request().Context(), mapAgentSubToService(payload))
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to create subscription")
	}

	resp := IdResponseV4{
		Data: created,
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusCreated, resp)
}

func (h *AgentSubscriptionsHandler) V4AgentSubsGet(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	subscriptionID := c.Param("subscriptionId")
	if subscriptionID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Subscription id is required")
	}

	item, err := h.service.GetByUUID(c.Request().Context(), subscriptionID)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to get subscription")
	}
	if item == nil {
		return writeError(c, http.StatusNotFound, "Not Found", "Subscription not found")
	}

	resp := AgentSubResponseV4{
		Data: mapAgentSub(*item),
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *AgentSubscriptionsHandler) V4AgentSubsUpdate(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	subscriptionID := c.Param("subscriptionId")
	if subscriptionID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Subscription id is required")
	}

	var payload AgentSubV4
	if err := c.Bind(&payload); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}

	updated, err := h.service.Update(c.Request().Context(), subscriptionID, mapAgentSubToService(payload))
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to update subscription")
	}
	if updated == "" {
		return writeError(c, http.StatusNotFound, "Not Found", "Subscription not found")
	}

	resp := IdResponseV4{
		Data: updated,
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *AgentSubscriptionsHandler) V4AgentSubsDelete(c echo.Context) error {
	if _, err := getUsernameFromContext(c); err != nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}

	subscriptionID := c.Param("subscriptionId")
	if subscriptionID == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Subscription id is required")
	}

	deleted, err := h.service.Delete(c.Request().Context(), subscriptionID)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to delete subscription")
	}
	if !deleted {
		return writeError(c, http.StatusNotFound, "Not Found", "Subscription not found")
	}
	return c.NoContent(http.StatusNoContent)
}

func parseAgentSubFilters(c echo.Context) (services.AgentSubscriptionFilters, error) {
	filters := services.AgentSubscriptionFilters{
		Type: c.QueryParam("filter[type]"),
		Node: c.QueryParam("filter[node]"),
	}

	limit, err := parseLimit(c)
	if err != nil {
		return filters, echo.NewHTTPError(http.StatusBadRequest, "Invalid page[limit]")
	}
	filters.Limit = limit

	sortParam := c.QueryParam("sort")
	if sortParam != "" {
		sortExpr, err := parseAgentSubSort(sortParam)
		if err != nil {
			return filters, err
		}
		filters.Sort = sortExpr
	}
	return filters, nil
}

func parseLimit(c echo.Context) (int, error) {
	limitStr := c.QueryParam("page[limit]")
	if limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit <= 0 || limit > 1000 {
			return 0, echo.NewHTTPError(http.StatusBadRequest, "Invalid page[limit]")
		}
		return limit, nil
	}
	return 100, nil
}

func parseAgentSubSort(value string) (string, error) {
	desc := false
	field := value
	if strings.HasPrefix(value, "-") {
		desc = true
		field = strings.TrimPrefix(value, "-")
	}

	allowed := map[string]string{
		"uuid":        "guid",
		"gid":         "gid",
		"host":        "host",
		"port":        "port",
		"protocol":    "protocol",
		"path":        "path",
		"node":        "node",
		"type":        "type",
		"create_date": "create_date",
		"expire_date": "expire_date",
		"active":      "active",
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

func parseAgentTypeSort(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	desc := false
	field := value
	if strings.HasPrefix(value, "-") {
		desc = true
		field = strings.TrimPrefix(value, "-")
	}
	if field != "type" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "Invalid sort field")
	}
	order := "ASC"
	if desc {
		order = "DESC"
	}
	return "type " + order, nil
}

func mapAgentSubs(items []services.AgentSubscription) []AgentSubV4 {
	result := make([]AgentSubV4, 0, len(items))
	for _, item := range items {
		result = append(result, mapAgentSub(item))
	}
	return result
}

func mapAgentSub(item services.AgentSubscription) AgentSubV4 {
	return AgentSubV4{
		UUID:       item.UUID,
		GID:        item.GID,
		Host:       item.Host,
		Port:       item.Port,
		Protocol:   item.Protocol,
		Path:       item.Path,
		Node:       item.Node,
		Type:       item.Type,
		TTL:        item.TTL,
		CreateDate: item.CreateDate,
		ExpireDate: item.ExpireDate,
		Active:     item.Active,
	}
}

func mapAgentSubToService(item AgentSubV4) services.AgentSubscription {
	return services.AgentSubscription{
		UUID:       item.UUID,
		GID:        item.GID,
		Host:       item.Host,
		Port:       item.Port,
		Protocol:   item.Protocol,
		Path:       item.Path,
		Node:       item.Node,
		Type:       item.Type,
		TTL:        item.TTL,
		CreateDate: item.CreateDate,
		ExpireDate: item.ExpireDate,
		Active:     item.Active,
	}
}
