// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Type   string       `json:"type"`
	Title  string       `json:"title"`
	Status int          `json:"status"`
	Detail string       `json:"detail,omitempty"`
	Errors []FieldError `json:"errors,omitempty"`
}

type FieldError struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message,omitempty"`
}

// TimeRangeMeta is the inferred time range from MCP/NL query, for UI sync.
type TimeRangeMeta struct {
	From int64 `json:"from,omitempty"`
	To   int64 `json:"to,omitempty"`
}

type Meta struct {
	RequestID  string         `json:"request_id,omitempty"`
	TraceID    string         `json:"trace_id,omitempty"`
	Message    string         `json:"message,omitempty"`
	Pagination *Pagination    `json:"pagination,omitempty"`
	TimeRange  *TimeRangeMeta `json:"time_range,omitempty"`
	// Parser is populated by MCP endpoints to disclose which NL parser was
	// actually used (regex / llm / llm-fallback) and any LLM diagnostics.
	// Always omitempty so it never leaks into non-MCP responses.
	Parser *ParserMeta `json:"parser,omitempty"`
}

// ParserMeta surfaces NL-parser diagnostics for the MCP endpoints. It mirrors
// the fields the stdio MCP server returns (see src/mcp/mcp.go) so UIs can
// reuse a single rendering for both transports.
type ParserMeta struct {
	// Used is one of: "llm", "regex", "regex_fallback".
	Used string `json:"used,omitempty"`
	// Requested is the parser hint sent by the client ("auto" | "llm" | "regex"),
	// echoed back so the UI can show "auto → llm" transitions.
	Requested string `json:"requested,omitempty"`
	// Model is the LLM model name (when Used involves LLM).
	Model string `json:"model,omitempty"`
	// LatencyMS is the LLM round-trip latency in milliseconds (0 when no LLM call).
	LatencyMS int64 `json:"latency_ms,omitempty"`
	// Error is the LLM failure reason when fallback to regex happened.
	Error string `json:"error,omitempty"`
}

type Pagination struct {
	Limit   int  `json:"limit,omitempty"`
	Total   int  `json:"total,omitempty"`
	HasMore bool `json:"has_more,omitempty"`
}

func buildMeta(c echo.Context, message string) Meta {
	return Meta{
		RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
		Message:   message,
	}
}

func errorTypeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "https://api.homer/errors/invalid_argument"
	case http.StatusUnauthorized:
		return "https://api.homer/errors/unauthorized"
	case http.StatusForbidden:
		return "https://api.homer/errors/forbidden"
	case http.StatusNotFound:
		return "https://api.homer/errors/not_found"
	default:
		return "https://api.homer/errors/internal"
	}
}

func writeError(c echo.Context, status int, title, detail string) error {
	resp := ErrorResponse{
		Error: ErrorDetail{
			Type:   errorTypeForStatus(status),
			Title:  title,
			Status: status,
			Detail: detail,
		},
	}
	return c.JSON(status, resp)
}

func (h *AuthHandler) parseJWTString(tokenString string) (*jwt.Token, *JWTClaims, error) {
	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" {
		return nil, nil, echo.NewHTTPError(http.StatusUnauthorized, "Missing authorization header")
	}
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(h.jwtSecret), nil
	})
	if err != nil {
		return nil, nil, echo.NewHTTPError(http.StatusUnauthorized, "Invalid token")
	}
	if !token.Valid {
		return nil, nil, echo.NewHTTPError(http.StatusUnauthorized, "Token expired")
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return nil, nil, echo.NewHTTPError(http.StatusUnauthorized, "Invalid claims")
	}
	return token, claims, nil
}

func (h *AuthHandler) parseAuthToken(c echo.Context) (*jwt.Token, *JWTClaims, authSource, error) {
	authHeader := c.Request().Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			return nil, nil, authSourceNone, echo.NewHTTPError(http.StatusUnauthorized, "Invalid authorization header")
		}
		token, claims, err := h.parseJWTString(parts[1])
		if err != nil {
			return nil, nil, authSourceNone, err
		}
		return token, claims, authSourceBearer, nil
	}

	if h.cookieAuthEnabled() {
		if cookieToken := readSessionCookie(c, h.sessionCookieName()); cookieToken != "" {
			token, claims, err := h.parseJWTString(cookieToken)
			if err != nil {
				return nil, nil, authSourceNone, err
			}
			return token, claims, authSourceCookie, nil
		}
	}

	// Fallback for clients that can't set custom headers — most
	// importantly browsers opening a WebSocket (the handshake
	// does not let the app code add Authorization). We look for
	// the OAuth-standard `access_token` query parameter so the
	// mechanism is discoverable and familiar.
	if tokenString := c.QueryParam("access_token"); tokenString != "" {
		token, claims, err := h.parseJWTString(tokenString)
		if err != nil {
			return nil, nil, authSourceNone, err
		}
		return token, claims, authSourceQuery, nil
	}

	return nil, nil, authSourceNone, echo.NewHTTPError(http.StatusUnauthorized, "Missing authorization header")
}

func (h *AuthHandler) parseBearerToken(c echo.Context) (*jwt.Token, *JWTClaims, error) {
	token, claims, _, err := h.parseAuthToken(c)
	return token, claims, err
}

// JWTMiddlewareV4 is a JWT middleware that returns v4 error envelopes.
func (h *AuthHandler) JWTMiddlewareV4() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if h.jwtSecret == "" {
				return next(c)
			}
			if h.authenticateWithAuthTokenHeader(c) {
				return next(c)
			}
			token, claims, src, err := h.parseAuthToken(c)
			if err != nil {
				httpErr, ok := err.(*echo.HTTPError)
				if ok {
					return writeError(c, httpErr.Code, "Unauthorized", httpErr.Message.(string))
				}
				return writeError(c, http.StatusUnauthorized, "Unauthorized", "Invalid token")
			}
			if src == authSourceCookie && isMutatingHTTPMethod(c.Request().Method) && !validateCSRFForCookieAuth(c) {
				return writeError(c, http.StatusForbidden, "Forbidden", "CSRF validation failed")
			}
			if claims.ID != "" && h.sessionStore != nil && h.sessionStore.IsRevoked(claims.ID) {
				return writeError(c, http.StatusUnauthorized, "Unauthorized", "Token revoked")
			}
			c.Set("user", token)
			c.Set("auth_source", src)
			return next(c)
		}
	}
}
