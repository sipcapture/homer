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
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

type LoginResponseV4 struct {
	Data struct {
		Token string `json:"token"`
		Scope string `json:"scope"`
		User  struct {
			Admin bool `json:"admin"`
		} `json:"user"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type ProvidersResponseV4 struct {
	Data struct {
		Internal struct {
			Enable   bool   `json:"enable"`
			Name     string `json:"name"`
			Position int    `json:"position"`
			Type     string `json:"type"`
		} `json:"internal"`
		Ldap struct {
			Enable   bool   `json:"enable"`
			Name     string `json:"name"`
			Position int    `json:"position"`
			Type     string `json:"type"`
		} `json:"ldap"`
		Oauth2 []struct {
			Enable        bool   `json:"enable"`
			Name          string `json:"name"`
			Position      int    `json:"position"`
			Type          string `json:"type"`
			ProviderImage string `json:"provider_image,omitempty"`
			ProviderName  string `json:"provider_name,omitempty"`
			URL           string `json:"url,omitempty"`
			AutoRedirect  bool   `json:"auto_redirect"`
		} `json:"oauth2"`
	} `json:"data"`
	Meta Meta `json:"meta"`
}

type OAuth2TokenExchangeRequest struct {
	Token string `json:"token"`
}

type UserProfileV4 struct {
	GUID            string `json:"guid,omitempty"`
	Username        string `json:"username,omitempty"`
	DisplayName     string `json:"display_name,omitempty"`
	Avatar          string `json:"avatar,omitempty"`
	Group           string `json:"group,omitempty"`
	Admin           bool   `json:"admin,omitempty"`
	ExternalAuth    bool   `json:"external_auth,omitempty"`
	ExternalProfile string `json:"external_profile,omitempty"`
}

type UserProfileResponseV4 struct {
	Data UserProfileV4 `json:"data"`
	Meta Meta          `json:"meta"`
}

// V4CreateSession handles POST /api/v4/auth/sessions
func (h *AuthHandler) V4CreateSession(c echo.Context) error {
	if !h.passwordLoginAllowed() {
		return writeError(c, http.StatusForbidden, "Forbidden", "Password login is disabled")
	}

	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}

	if req.Username == "" || req.Password == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Username and password are required")
	}

	authType := strings.ToLower(strings.TrimSpace(req.Type))
	if authType == "" {
		authType = "internal"
	}
	if authType != "internal" && authType != "ldap" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid authentication type")
	}

	user, authErr := h.authenticatePasswordWithFallback(c.Request().Context(), req.Username, req.Password, authType)
	if authErr != nil || user == nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Invalid credentials")
	}

	token, _, err := h.generateToken(user.Username, user.IsAdmin)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to generate token")
	}

	scope := "user"
	if user.IsAdmin {
		scope = "admin"
	}

	var resp LoginResponseV4
	resp.Data.Token = token
	resp.Data.Scope = scope
	resp.Data.User.Admin = user.IsAdmin
	resp.Meta = buildMeta(c, "")

	return c.JSON(http.StatusCreated, resp)
}

// V4DeleteSession handles DELETE /api/v4/auth/sessions/{sessionId}
func (h *AuthHandler) V4DeleteSession(c echo.Context) error {
	sessionID := c.Param("sessionId")
	_, claims, err := h.parseBearerToken(c)
	if err != nil {
		httpErr, ok := err.(*echo.HTTPError)
		if ok {
			return writeError(c, httpErr.Code, "Unauthorized", httpErr.Message.(string))
		}
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Invalid token")
	}

	if claims.ID == "" || sessionID == "" || claims.ID != sessionID {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid session id")
	}

	if h.sessionStore != nil {
		exp := time.Now().Add(time.Duration(h.expireHours) * time.Hour)
		if claims.ExpiresAt != nil {
			exp = claims.ExpiresAt.Time
		}
		h.sessionStore.Revoke(sessionID, exp)
	}

	return c.NoContent(http.StatusNoContent)
}

// V4ListProviders handles GET /api/v4/auth/providers
func (h *AuthHandler) V4ListProviders(c echo.Context) error {
	var resp ProvidersResponseV4
	passwordLogin := h.passwordLoginAllowed()
	resp.Data.Internal.Enable = passwordLogin
	resp.Data.Internal.Name = "internal"
	resp.Data.Internal.Position = 0
	resp.Data.Internal.Type = "internal"

	resp.Data.Ldap.Name = "LDAP"
	resp.Data.Ldap.Type = "ldap"
	resp.Data.Ldap.Position = 1
	resp.Data.Ldap.Enable = passwordLogin && h.ldapAuth != nil && h.ldapAuth.Enabled()

	resp.Data.Oauth2 = make([]struct {
		Enable        bool   `json:"enable"`
		Name          string `json:"name"`
		Position      int    `json:"position"`
		Type          string `json:"type"`
		ProviderImage string `json:"provider_image,omitempty"`
		ProviderName  string `json:"provider_name,omitempty"`
		URL           string `json:"url,omitempty"`
		AutoRedirect  bool   `json:"auto_redirect"`
	}, 0, len(h.providers))

	for _, provider := range h.providers {
		if !provider.Enable {
			continue
		}
		item := struct {
			Enable        bool   `json:"enable"`
			Name          string `json:"name"`
			Position      int    `json:"position"`
			Type          string `json:"type"`
			ProviderImage string `json:"provider_image,omitempty"`
			ProviderName  string `json:"provider_name,omitempty"`
			URL           string `json:"url,omitempty"`
			AutoRedirect  bool   `json:"auto_redirect"`
		}{
			Enable:        provider.Enable,
			Name:          provider.Name,
			Position:      provider.Position,
			Type:          provider.Type,
			ProviderImage: provider.ProviderImage,
			ProviderName:  provider.ProviderName,
			URL:           provider.URL,
			AutoRedirect:  provider.AutoRedirect,
		}
		resp.Data.Oauth2 = append(resp.Data.Oauth2, item)
	}

	resp.Meta = buildMeta(c, "")
	return c.JSON(http.StatusOK, resp)
}

// V4OAuth2TokenExchange handles POST /api/v4/auth/oauth2/token
func (h *AuthHandler) V4OAuth2TokenExchange(c echo.Context) error {
	var req OAuth2TokenExchangeRequest
	if err := c.Bind(&req); err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid request body")
	}
	if req.Token == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Token is required")
	}

	username, isAdmin, ok := h.oneTimeStore.Consume(req.Token)
	if !ok {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Invalid or expired token")
	}

	jwtToken, _, err := h.generateToken(username, isAdmin)
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to generate token")
	}

	scope := "user"
	if isAdmin {
		scope = "admin"
	}

	var resp LoginResponseV4
	resp.Data.Token = jwtToken
	resp.Data.Scope = scope
	resp.Data.User.Admin = isAdmin
	resp.Meta = buildMeta(c, "")

	return c.JSON(http.StatusCreated, resp)
}

// V4GetMe handles GET /api/v4/me
func (h *AuthHandler) V4GetMe(c echo.Context) error {
	user := c.Get("user")
	if user == nil {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Not authenticated")
	}
	token, ok := user.(*jwt.Token)
	if !ok {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Invalid token")
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return writeError(c, http.StatusUnauthorized, "Unauthorized", "Invalid claims")
	}

	profile := UserProfileV4{
		Username:    claims.Username,
		DisplayName: claims.Username,
		Admin:       claims.Admin,
		Group:       "user",
	}
	if claims.Admin {
		profile.Group = "admin"
	}

	if h.userService != nil {
		dbUser, err := h.userService.GetUserByUsername(c.Request().Context(), claims.Username)
		if err == nil && dbUser != nil {
			profile.GUID = strconv.FormatInt(dbUser.ID, 10)
			if dbUser.Name != "" {
				profile.DisplayName = dbUser.Name
			}
			if dbUser.Email != "" {
				profile.ExternalProfile = dbUser.Email
			}
		}
	}

	resp := UserProfileResponseV4{
		Data: profile,
		Meta: buildMeta(c, ""),
	}
	return c.JSON(http.StatusOK, resp)
}
