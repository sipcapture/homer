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

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

type UsersHandler struct {
	userService *services.UserService
}

func NewUsersHandler(userService *services.UserService) *UsersHandler {
	return &UsersHandler{
		userService: userService,
	}
}

type UserCreateRequest struct {
	Username   string `json:"username"`
	PartID     int    `json:"partid"`
	Email      string `json:"email"`
	Password   string `json:"password"`
	Name       string `json:"name"`
	Enabled    *bool  `json:"enabled"`
	FirstName  string `json:"firstname"`
	LastName   string `json:"lastname"`
	Department string `json:"department"`
	UserGroup  string `json:"user_group"`
}

type UserPatchRequest struct {
	PartID     *int    `json:"partid"`
	Email      *string `json:"email"`
	Password   *string `json:"password"`
	Name       *string `json:"name"`
	Enabled    *bool   `json:"enabled"`
	FirstName  *string `json:"firstname"`
	LastName   *string `json:"lastname"`
	Department *string `json:"department"`
	UserGroup  *string `json:"user_group"`
}

type UserV4 struct {
	GUID       string `json:"guid,omitempty"`
	Username   string `json:"username,omitempty"`
	Name       string `json:"name,omitempty"`
	PartID     int    `json:"partid,omitempty"`
	Email      string `json:"email,omitempty"`
	Enabled    bool   `json:"enabled,omitempty"`
	FirstName  string `json:"firstname,omitempty"`
	LastName   string `json:"lastname,omitempty"`
	Department string `json:"department,omitempty"`
	UserGroup  string `json:"user_group,omitempty"`
}

type UserResponseV4 struct {
	Data UserV4 `json:"data"`
	Meta Meta   `json:"meta"`
}

type UserListResponseV4 struct {
	Data struct {
		Items []UserV4 `json:"items"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type IdResponseV4 struct {
	Data string `json:"data"`
	Meta Meta   `json:"meta"`
}

func (h *UsersHandler) V4UsersList(c echo.Context) error {
	// Listing every account in the system is administrative — non-admins
	// stay on /api/v4/me for their own profile and never need to see
	// other users' usernames / emails. Refuse early with a 403 so the
	// settings UI's "Users" tab also stays hidden for ordinary users.
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	filters, err := parseUserListFilters(c)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", err.Error())
	}

	users, err := h.userService.ListUsersFiltered(c.Request().Context(), filters)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to list users")
	}

	resp := UserListResponseV4{}
	resp.Data.Items = make([]UserV4, 0, len(users))
	for _, user := range users {
		resp.Data.Items = append(resp.Data.Items, mapUserToV4(user))
	}
	resp.Meta = buildMeta(c, "")
	resp.Meta.Pagination = &Pagination{Limit: filters.Limit}

	return c.JSON(http.StatusOK, resp)
}

func (h *UsersHandler) V4UsersCreate(c echo.Context) error {
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	var req UserCreateRequest
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	if req.Username == "" || req.Password == "" || req.Email == "" || req.UserGroup == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Missing required fields")
	}

	isAdmin, ok := parseUserGroup(req.UserGroup)
	if !ok {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid user_group")
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	id, err := h.userService.CreateUser(c.Request().Context(), req.Username, req.Password, req.Email, strings.TrimSpace(req.Name), isAdmin, enabled)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to create user")
	}

	resp := IdResponseV4{
		Data: strconv.FormatInt(id, 10),
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusCreated, resp)
}

func (h *UsersHandler) V4UsersGet(c echo.Context) error {
	userID, err := parseUserID(c.Param("userId"))
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid user id")
	}

	// Non-admins may only fetch their own record. Anything else is
	// administrative; profile data for the caller is also reachable
	// via /api/v4/me without needing the numeric id.
	if !isAdmin(c) && !isSelf(c, h.userService, userID) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Insufficient permissions")
	}

	user, err := h.userService.GetUserByID(c.Request().Context(), userID)
	if err != nil || user == nil {
		return writeError(c, http.StatusNotFound, "Not Found", "User not found")
	}

	resp := UserResponseV4{
		Data: mapUserToV4(user),
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UsersHandler) V4UsersUpdate(c echo.Context) error {
	userID, err := parseUserID(c.Param("userId"))
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid user id")
	}

	if !isAdmin(c) && !isSelf(c, h.userService, userID) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Insufficient permissions")
	}

	var req UserPatchRequest
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}

	var isAdminValue *bool
	if req.UserGroup != nil {
		admin, ok := parseUserGroup(*req.UserGroup)
		if !ok {
			return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid user_group")
		}
		isAdminValue = &admin
	}

	updated, err := h.userService.UpdateUser(c.Request().Context(), userID, req.Email, req.Password, req.Name, isAdminValue, req.Enabled)
	if err != nil {
		if err.Error() == "no fields to update" {
			return writeError(c, http.StatusBadRequest, "Bad Request", "No fields to update")
		}
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to update user")
	}
	if !updated {
		return writeError(c, http.StatusNotFound, "Not Found", "User not found")
	}

	resp := IdResponseV4{
		Data: strconv.FormatInt(userID, 10),
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}

func (h *UsersHandler) V4UsersDelete(c echo.Context) error {
	if !isAdmin(c) {
		return writeError(c, http.StatusForbidden, "Forbidden", "Admin privileges required")
	}

	userID, err := parseUserID(c.Param("userId"))
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid user id")
	}

	deleted, err := h.userService.DeleteUser(c.Request().Context(), userID)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to delete user")
	}
	if !deleted {
		return writeError(c, http.StatusNotFound, "Not Found", "User not found")
	}

	return c.NoContent(http.StatusNoContent)
}

func parseUserID(id string) (int64, error) {
	return strconv.ParseInt(id, 10, 64)
}

func parseUserGroup(value string) (bool, bool) {
	switch strings.ToLower(value) {
	case "admin":
		return true, true
	case "user":
		return false, true
	default:
		return false, false
	}
}

func parseUserListFilters(c echo.Context) (services.UserListFilters, error) {
	filters := services.UserListFilters{
		Search:   c.QueryParam("search"),
		Username: c.QueryParam("filter[username]"),
		Email:    c.QueryParam("filter[email]"),
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

	enabledStr := c.QueryParam("filter[enabled]")
	if enabledStr != "" {
		val, err := parseBool(enabledStr)
		if err != nil {
			return filters, echo.NewHTTPError(http.StatusBadRequest, "Invalid filter[enabled]")
		}
		filters.Enabled = &val
	}

	userGroup := c.QueryParam("filter[user_group]")
	if userGroup != "" {
		admin, ok := parseUserGroup(userGroup)
		if !ok {
			return filters, echo.NewHTTPError(http.StatusBadRequest, "Invalid filter[user_group]")
		}
		filters.IsAdmin = &admin
	}

	sortParam := c.QueryParam("sort")
	if sortParam != "" {
		sort, err := parseUserSort(sortParam)
		if err != nil {
			return filters, echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		filters.Sort = sort
	}

	return filters, nil
}

func parseUserSort(value string) (string, error) {
	desc := false
	field := value
	if strings.HasPrefix(value, "-") {
		desc = true
		field = strings.TrimPrefix(value, "-")
	}

	allowed := map[string]string{
		"username":   "username",
		"name":       "full_name",
		"email":      "email",
		"created_at": "created_at",
		"updated_at": "updated_at",
		"id":         "id",
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

func parseBool(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	default:
		return false, echo.NewHTTPError(http.StatusBadRequest, "Invalid boolean")
	}
}

func isAdmin(c echo.Context) bool {
	user := c.Get("user")
	if user == nil {
		return false
	}
	token, ok := user.(*jwt.Token)
	if !ok {
		return false
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return false
	}
	return claims.Admin
}

func isSelf(c echo.Context, userService *services.UserService, userID int64) bool {
	user := c.Get("user")
	if user == nil {
		return false
	}
	token, ok := user.(*jwt.Token)
	if !ok {
		return false
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return false
	}

	dbUser, err := userService.GetUserByID(c.Request().Context(), userID)
	if err != nil || dbUser == nil {
		return false
	}
	return strings.EqualFold(dbUser.Username, claims.Username)
}

func mapUserToV4(user *services.User) UserV4 {
	if user == nil {
		return UserV4{}
	}
	group := "user"
	if user.IsAdmin {
		group = "admin"
	}
	return UserV4{
		GUID:      strconv.FormatInt(user.ID, 10),
		Username:  user.Username,
		Name:      user.Name,
		Email:     user.Email,
		Enabled:   user.IsActive,
		UserGroup: group,
	}
}
