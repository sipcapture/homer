// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

// AuthHandler handles authentication-related API endpoints
type AuthHandler struct {
	jwtSecret    string
	expireHours  int
	userService  *services.UserService
	ldapAuth     *services.LDAPAuthService
	sessionStore *SessionStore
	oneTimeStore *OneTimeTokenStore
	providers    []OAuthProvider

	authTokenSvc *services.AuthTokenService
	apiSettings  config.APISettingsConfig
}

// OAuthProvider represents a configured OAuth2 provider
type OAuthProvider struct {
	Enable        bool
	Name          string
	Position      int
	Type          string
	ProviderImage string
	ProviderName  string
	URL           string
	AutoRedirect  bool
	CallbackURL   string
}

// NewAuthHandlerWithUserService creates auth handler with user service.
// authTokenSvc may be nil; api_settings.enable_token_access controls Auth-Token header lookup.
// ldapAuth may be nil when LDAP is not configured.
func NewAuthHandlerWithUserService(
	secret string,
	expireHours int,
	userService *services.UserService,
	providers []OAuthProvider,
	authTokenSvc *services.AuthTokenService,
	apiSettings config.APISettingsConfig,
	ldapAuth *services.LDAPAuthService,
) *AuthHandler {
	return &AuthHandler{
		jwtSecret:    secret,
		expireHours:  expireHours,
		userService:  userService,
		ldapAuth:     ldapAuth,
		sessionStore: NewSessionStore(),
		oneTimeStore: NewOneTimeTokenStore(),
		providers:    providers,
		authTokenSvc: authTokenSvc,
		apiSettings:  apiSettings,
	}
}

// LoginRequest represents the login request body
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	// Type selects the password backend: "internal" (default) or "ldap" when LDAP is enabled.
	Type string `json:"type"`
}

// LoginResponse represents the login response
type LoginResponse struct {
	Token string `json:"token"`
	Scope string `json:"scope"`
	User  struct {
		Admin    bool   `json:"admin"`
		Username string `json:"username"`
	} `json:"user"`
}

// JWTClaims represents the JWT claims
type JWTClaims struct {
	Username string `json:"username"`
	Admin    bool   `json:"admin"`
	jwt.RegisteredClaims
}

// Login handles POST /api/v3/auth and /api/v3/auth/login
func (h *AuthHandler) Login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Invalid request body",
		})
	}

	// Authenticate user
	var user *services.User
	var err error

	authType := strings.ToLower(strings.TrimSpace(req.Type))
	if authType == "" {
		authType = "internal"
	}

	if authType == "ldap" {
		if h.ldapAuth == nil || !h.ldapAuth.Enabled() {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{
				"error": "LDAP authentication is not enabled on this server",
			})
		}
		user, err = h.ldapAuth.Authenticate(c.Request().Context(), req.Username, req.Password)
	} else if authType == "internal" {
		if h.userService == nil {
			return c.JSON(http.StatusUnauthorized, map[string]interface{}{
				"error": "Authentication service not configured",
			})
		}
		user, err = h.userService.Authenticate(c.Request().Context(), req.Username, req.Password)
	} else {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Invalid authentication type",
		})
	}
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]interface{}{
			"error": "Invalid credentials",
		})
	}

	// Generate JWT token
	token, _, err := h.generateToken(user.Username, user.IsAdmin)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"error": "Failed to generate token",
		})
	}

	scope := "user"
	if user.IsAdmin {
		scope = "admin"
	}

	// Wrap in data field for UI compatibility
	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": LoginResponse{
			Token: token,
			Scope: scope,
			User: struct {
				Admin    bool   `json:"admin"`
				Username string `json:"username"`
			}{
				Admin:    user.IsAdmin,
				Username: user.Username,
			},
		},
	})
}

// GetProfile handles GET /api/v3/auth/me
func (h *AuthHandler) GetProfile(c echo.Context) error {
	// Get user from context (set by JWT middleware)
	user := c.Get("user")
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]interface{}{
			"error": "Not authenticated",
		})
	}

	token, ok := user.(*jwt.Token)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]interface{}{
			"error": "Invalid token",
		})
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]interface{}{
			"error": "Invalid claims",
		})
	}

	role := "user"
	if claims.Admin {
		role = "admin"
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"data": map[string]interface{}{
			"username": claims.Username,
			"admin":    claims.Admin,
			"role":     role,
		},
	})
}

// JWTMiddleware returns the JWT middleware function
func (h *AuthHandler) JWTMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip if no secret configured
			if h.jwtSecret == "" {
				return next(c)
			}

			if h.authenticateWithAuthTokenHeader(c) {
				return next(c)
			}

			// Get token from header
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				return c.JSON(http.StatusUnauthorized, map[string]interface{}{
					"error": "Missing authorization header",
				})
			}

			// Parse Bearer token
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				return c.JSON(http.StatusUnauthorized, map[string]interface{}{
					"error": "Invalid authorization header",
				})
			}

			tokenString := parts[1]

			// Parse and validate token
			token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
				return []byte(h.jwtSecret), nil
			})

			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]interface{}{
					"error": "Invalid token",
				})
			}

			if !token.Valid {
				return c.JSON(http.StatusUnauthorized, map[string]interface{}{
					"error": "Token expired",
				})
			}

			claims, ok := token.Claims.(*JWTClaims)
			if !ok {
				return c.JSON(http.StatusUnauthorized, map[string]interface{}{
					"error": "Invalid claims",
				})
			}

			if claims.ID != "" && h.sessionStore != nil && h.sessionStore.IsRevoked(claims.ID) {
				return c.JSON(http.StatusUnauthorized, map[string]interface{}{
					"error": "Token revoked",
				})
			}

			// Set user in context
			c.Set("user", token)
			return next(c)
		}
	}
}

// generateToken generates a JWT token and returns token + sessionId
func (h *AuthHandler) generateToken(username string, admin bool) (string, string, error) {
	sessionID := newSessionID()
	claims := &JWTClaims{
		Username: username,
		Admin:    admin,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        sessionID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(h.expireHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "homer-core",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(h.jwtSecret))
	if err != nil {
		return "", "", err
	}
	return signed, sessionID, nil
}

func newSessionID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(buf)
}

// authenticateWithAuthTokenHeader validates Auth-Token (or api_settings.auth_token_header)
// against the coordinator settings DuckDB auth_token table and sets context "user" to a synthetic JWT (homer-app parity).
func (h *AuthHandler) authenticateWithAuthTokenHeader(c echo.Context) bool {
	if !h.apiSettings.EnableTokenAccess || h.authTokenSvc == nil {
		return false
	}
	hdr := strings.TrimSpace(h.apiSettings.AuthTokenHeader)
	if hdr == "" {
		hdr = "Auth-Token"
	}
	raw := strings.TrimSpace(c.Request().Header.Get(hdr))
	if raw == "" {
		return false
	}
	item, err := h.authTokenSvc.LookupValidAPIAccessToken(c.Request().Context(), raw)
	if err != nil || item == nil {
		return false
	}
	c.Set("user", h.jwtTokenFromAuthTokenItem(item))
	return true
}

type authTokenUserObject struct {
	Username  string `json:"username"`
	Usergroup string `json:"usergroup"`
}

func (h *AuthHandler) jwtTokenFromAuthTokenItem(item *services.AuthTokenItem) *jwt.Token {
	userName := "guest"
	isAdmin := false
	if len(item.UserObject) > 0 {
		var uo authTokenUserObject
		if err := json.Unmarshal(item.UserObject, &uo); err == nil {
			if uo.Username != "" {
				userName = uo.Username
			}
			if strings.EqualFold(uo.Usergroup, "admin") {
				isAdmin = true
			}
		}
	}
	claims := &JWTClaims{
		Username: userName,
		Admin:    isAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "auth-token:" + item.GUID,
		},
	}
	return &jwt.Token{
		Valid:  true,
		Claims: claims,
	}
}

func (h *AuthHandler) findProvider(name string) *OAuthProvider {
	for i := range h.providers {
		if strings.EqualFold(h.providers[i].Name, name) {
			return &h.providers[i]
		}
	}
	return nil
}
