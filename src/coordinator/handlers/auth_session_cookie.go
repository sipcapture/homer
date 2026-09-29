// Copyright (C) 2025 Homer Server Contributors
//
// HttpOnly session cookie for browser UI (multi-tab) with CSRF checks when
// the JWT is sent via cookie instead of Authorization: Bearer.

package handlers

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/config"
)

const defaultSessionCookieName = "homer_session"

type authSource int

const (
	authSourceNone authSource = iota
	authSourceBearer
	authSourceCookie
	authSourceQuery
)

func (h *AuthHandler) sessionCookieName() string {
	if h != nil && h.cookieName != "" {
		return h.cookieName
	}
	return defaultSessionCookieName
}

func (h *AuthHandler) cookieAuthEnabled() bool {
	return h != nil && h.cookieEnable
}

func (h *AuthHandler) resolveCookieSecure(c echo.Context) bool {
	if h.cookieSecure != nil {
		return *h.cookieSecure
	}
	if c.Request().TLS != nil {
		return true
	}
	if strings.EqualFold(c.Request().Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return false
}

func parseSameSiteMode(raw string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func (h *AuthHandler) setSessionCookie(c echo.Context, jwtToken string, persistent bool) {
	if !h.cookieAuthEnabled() || jwtToken == "" {
		return
	}
	maxAge := 0 // session cookie (dropped when the browser closes)
	if persistent {
		maxAge = h.expireHours * 3600
		if maxAge <= 0 {
			maxAge = 24 * 3600
		}
	}
	c.SetCookie(&http.Cookie{
		Name:     h.sessionCookieName(),
		Value:    jwtToken,
		Path:     "/api/v4",
		HttpOnly: true,
		Secure:   h.resolveCookieSecure(c),
		SameSite: parseSameSiteMode(h.cookieSameSite),
		MaxAge:   maxAge,
	})
}

func (h *AuthHandler) clearSessionCookie(c echo.Context) {
	if !h.cookieAuthEnabled() {
		return
	}
	c.SetCookie(&http.Cookie{
		Name:     h.sessionCookieName(),
		Value:    "",
		Path:     "/api/v4",
		HttpOnly: true,
		Secure:   h.resolveCookieSecure(c),
		SameSite: parseSameSiteMode(h.cookieSameSite),
		MaxAge:   -1,
	})
}

func readSessionCookie(c echo.Context, name string) string {
	cookie, err := c.Cookie(name)
	if err != nil || cookie == nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func isMutatingHTTPMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func requestOrigin(c echo.Context) string {
	scheme := "http"
	if c.Request().TLS != nil || strings.EqualFold(c.Request().Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := c.Request().Host
	if host == "" {
		host = c.Request().Header.Get("Host")
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

func refererOrigin(referer string) (string, bool) {
	u, err := url.Parse(referer)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", false
	}
	origin, err := config.NormalizeOrigin(u.Scheme + "://" + u.Host)
	return origin, err == nil
}

// originAllowed reports whether a browser-supplied origin matches the request's
// own origin or one of coordinator.jwt.cookie_trusted_origins.
func (h *AuthHandler) originAllowed(origin, expected string) bool {
	if strings.EqualFold(origin, expected) {
		return true
	}
	if n, err := config.NormalizeOrigin(expected); err == nil && n == origin {
		return true
	}
	_, ok := h.trustedOrigins[origin]
	return ok
}

// validateCSRFForCookieAuth ensures cross-site POST/PUT/PATCH/DELETE cannot
// ride the browser's session cookie (SameSite=Lax is the primary guard; this
// is defense in depth for same-site subdomain issues).
func (h *AuthHandler) validateCSRFForCookieAuth(c echo.Context) bool {
	expected := requestOrigin(c)
	if expected == "" {
		return true
	}
	if raw := strings.TrimSpace(c.Request().Header.Get("Origin")); raw != "" {
		origin, err := config.NormalizeOrigin(raw)
		return err == nil && h.originAllowed(origin, expected)
	}
	if referer := strings.TrimSpace(c.Request().Header.Get("Referer")); referer != "" {
		origin, ok := refererOrigin(referer)
		return ok && h.originAllowed(origin, expected)
	}
	// Same-origin navigations and some fetch clients omit both headers.
	return true
}
