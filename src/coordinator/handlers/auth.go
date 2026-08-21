// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/config"
	"github.com/sipcapture/homer-core/src/coordinator/services"
	"github.com/sipcapture/homer-core/src/passwordhash"
	"golang.org/x/oauth2"
)

// AuthHandler handles authentication-related API endpoints
type AuthHandler struct {
	jwtSecret      string
	expireHours    int
	cookieEnable   bool
	cookieName     string
	cookieSameSite string
	cookieSecure   *bool
	userService    *services.UserService
	ldapAuth       *services.LDAPAuthService
	sessionStore   *SessionStore
	oneTimeStore   *OneTimeTokenStore
	avatarStore    *AvatarStore
	providers      []OAuthProvider
	oauthState     *OAuthStateStore

	authTokenSvc *services.AuthTokenService
	apiSettings  config.APISettingsConfig
	// fallbackAuthType is coordinator.auth.fallback_auth_type (internal|ldap); empty disables.
	fallbackAuthType string
	// disablePasswordLogin is coordinator.auth.disable_password_login (OAuth-only UI/API).
	disablePasswordLogin bool
}

// OAuthProvider represents a configured OAuth2 provider
type OAuthProvider struct {
	Enable        bool
	Name          string
	Position      int
	Type          string
	ProviderImage string
	ProviderName  string
	URL           string // legacy; empty for authorization code flow
	AutoRedirect  bool
	CallbackURL   string

	// Authorization code flow (non-nil OAuth2Config when enabled).
	OAuth2Config      *oauth2.Config
	ProfileURL        string
	UsePKCE           bool
	SkipAutoProvision bool
	AdminGroups       []string
	GroupClaim        string
}

// NewAuthHandlerWithUserService creates auth handler with user service.
// authTokenSvc may be nil; api_settings.enable_token_access controls Auth-Token header lookup.
// ldapAuth may be nil when LDAP is not configured.
func NewAuthHandlerWithUserService(
	secret string,
	expireHours int,
	jwtCookie config.JWTConfig,
	userService *services.UserService,
	providers []OAuthProvider,
	authTokenSvc *services.AuthTokenService,
	apiSettings config.APISettingsConfig,
	ldapAuth *services.LDAPAuthService,
	fallbackAuthType string,
	disablePasswordLogin bool,
) *AuthHandler {
	cookieEnable := true
	if jwtCookie.CookieEnable != nil {
		cookieEnable = *jwtCookie.CookieEnable
	}
	cookieName := jwtCookie.CookieName
	if cookieName == "" {
		cookieName = defaultSessionCookieName
	}
	cookieSameSite := jwtCookie.CookieSameSite
	if cookieSameSite == "" {
		cookieSameSite = "Lax"
	}
	return &AuthHandler{
		jwtSecret:            secret,
		expireHours:          expireHours,
		cookieEnable:         cookieEnable,
		cookieName:           cookieName,
		cookieSameSite:       cookieSameSite,
		cookieSecure:         jwtCookie.CookieSecure,
		userService:          userService,
		ldapAuth:             ldapAuth,
		sessionStore:         NewSessionStore(),
		oneTimeStore:         NewOneTimeTokenStore(),
		avatarStore:          NewAvatarStore(24 * time.Hour),
		oauthState:           NewOAuthStateStore(),
		providers:            providers,
		authTokenSvc:         authTokenSvc,
		apiSettings:          apiSettings,
		fallbackAuthType:     fallbackAuthType,
		disablePasswordLogin: disablePasswordLogin,
	}
}

func (h *AuthHandler) passwordLoginAllowed() bool {
	return h != nil && !h.disablePasswordLogin
}

// tryPasswordBackend authenticates with a single backend ("internal" or "ldap").
func (h *AuthHandler) tryPasswordBackend(ctx context.Context, username, password, authType string) (*services.User, error) {
	switch strings.ToLower(strings.TrimSpace(authType)) {
	case "ldap":
		if h.ldapAuth == nil || !h.ldapAuth.Enabled() {
			return nil, fmt.Errorf("ldap not available")
		}
		return h.ldapAuth.Authenticate(ctx, username, password)
	case "internal":
		if h.userService == nil {
			return nil, fmt.Errorf("internal authentication not configured")
		}
		return h.userService.Authenticate(ctx, username, password)
	default:
		return nil, fmt.Errorf("invalid authentication type")
	}
}

// authenticatePasswordWithFallback tries the client-selected backend, then coordinator.auth.fallback_auth_type if set and different.
func (h *AuthHandler) authenticatePasswordWithFallback(ctx context.Context, username, password, primaryType string) (*services.User, error) {
	pt := strings.ToLower(strings.TrimSpace(primaryType))
	if pt == "" {
		pt = "internal"
	}
	chain := []string{pt}
	fb := strings.TrimSpace(strings.ToLower(h.fallbackAuthType))
	if fb != "" && fb != pt {
		chain = append(chain, fb)
	}
	var lastErr error
	for _, t := range chain {
		u, err := h.tryPasswordBackend(ctx, username, password, t)
		if err == nil && u != nil {
			return u, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("invalid credentials")
	}
	return nil, lastErr
}

// LoginRequest represents the login request body
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	// Type selects the password backend: "internal" (default) or "ldap" when LDAP is enabled.
	// If login fails, coordinator.auth.fallback_auth_type may be tried server-side.
	Type string `json:"type"`
	// Remember, when true, sets a persistent HttpOnly session cookie (Max-Age =
	// jwt.expire_hours). When false the cookie is session-scoped. The JWT is
	// never stored in browser script-visible storage (GHSA-rqwc-fmx3-95j8).
	Remember bool `json:"remember"`
}

// LoginResponse represents the login response
type LoginResponse struct {
	Token string `json:"token"`
	Scope string `json:"scope"`
	User  struct {
		Admin              bool   `json:"admin"`
		Username           string `json:"username"`
		MustChangePassword bool   `json:"must_change_password,omitempty"`
	} `json:"user"`
}

// JWTClaims represents the JWT claims
type JWTClaims struct {
	Username           string `json:"username"`
	Admin              bool   `json:"admin"`
	MustChangePassword bool   `json:"must_change_password,omitempty"`
	jwt.RegisteredClaims
}

// Login handles POST /api/v3/auth and /api/v3/auth/login
func (h *AuthHandler) Login(c echo.Context) error {
	if !h.passwordLoginAllowed() {
		return c.JSON(http.StatusForbidden, map[string]interface{}{
			"error": "Password login is disabled",
		})
	}

	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Invalid request body",
		})
	}

	// Authenticate user (optional second backend from coordinator.auth.fallback_auth_type).
	authType := strings.ToLower(strings.TrimSpace(req.Type))
	if authType == "" {
		authType = "internal"
	}
	if authType != "internal" && authType != "ldap" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Invalid authentication type",
		})
	}

	user, authErr := h.authenticatePasswordWithFallback(c.Request().Context(), req.Username, req.Password, authType)
	if authErr != nil || user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]interface{}{
			"error": "Invalid credentials",
		})
	}

	// Generate JWT token
	token, _, err := h.generateToken(user.Username, user.IsAdmin, passwordhash.IsDisallowedDefaultHash(user.PasswordHash))
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
				Admin              bool   `json:"admin"`
				Username           string `json:"username"`
				MustChangePassword bool   `json:"must_change_password,omitempty"`
			}{
				Admin:              user.IsAdmin,
				Username:           user.Username,
				MustChangePassword: passwordhash.IsDisallowedDefaultHash(user.PasswordHash),
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
			if h.jwtSecret == "" {
				return c.JSON(http.StatusInternalServerError, map[string]interface{}{
					"error": "JWT authentication is not configured",
				})
			}

			if h.authenticateWithAuthTokenHeader(c) {
				return next(c)
			}

			token, claims, src, err := h.parseAuthToken(c)
			if err != nil {
				httpErr, ok := err.(*echo.HTTPError)
				if ok {
					return c.JSON(httpErr.Code, map[string]interface{}{
						"error": httpErr.Message,
					})
				}
				return c.JSON(http.StatusUnauthorized, map[string]interface{}{
					"error": "Invalid token",
				})
			}
			if src == authSourceCookie && isMutatingHTTPMethod(c.Request().Method) && !validateCSRFForCookieAuth(c) {
				return c.JSON(http.StatusForbidden, map[string]interface{}{
					"error": "CSRF validation failed",
				})
			}

			if claims.ID != "" && h.sessionStore != nil && h.sessionStore.IsRevoked(claims.ID) {
				return c.JSON(http.StatusUnauthorized, map[string]interface{}{
					"error": "Token revoked",
				})
			}

			c.Set("user", token)
			if passwordChangeRequiredBlocks(c, claims) {
				return c.JSON(http.StatusForbidden, map[string]interface{}{
					"error": "Password change required",
				})
			}
			return next(c)
		}
	}
}

// generateToken generates a JWT token and returns token + sessionId
func (h *AuthHandler) generateToken(username string, admin bool, mustChangePassword bool) (string, string, error) {
	sessionID := newSessionID()
	claims := &JWTClaims{
		Username:           username,
		Admin:              admin,
		MustChangePassword: mustChangePassword,
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
	UserGroup string `json:"user_group"`
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
			if strings.EqualFold(uo.UserGroup, "admin") {
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
