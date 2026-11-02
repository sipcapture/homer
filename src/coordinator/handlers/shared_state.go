// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

// SetRuntimeState replaces instance-local HTTP state for stateless deployments.
func (h *AuthHandler) SetRuntimeState(state *services.RuntimeState) {
	h.runtimeState = state
	h.avatarStore.shared = state
}

func (h *AuthHandler) sessionIsRevoked(c echo.Context, id string) bool {
	if h.runtimeState == nil {
		return h.sessionStore.IsRevoked(id)
	}
	_, found, err := h.runtimeState.Get(c.Request().Context(), "revoked-session", id, false)
	if err != nil {
		slog.Error("shared session lookup failed", "error", err)
		return true
	}
	return found
}

func (h *AuthHandler) revokeSession(c echo.Context, id string, expires time.Time) error {
	if h.runtimeState == nil {
		h.sessionStore.Revoke(id, expires)
		return nil
	}
	return h.runtimeState.Put(c.Request().Context(), "revoked-session", id, []byte{1}, expires)
}

func (h *AuthHandler) putOAuthState(c echo.Context, state, provider, verifier string) error {
	if h.runtimeState == nil {
		h.oauthState.Put(state, provider, verifier, 15*time.Minute)
		return nil
	}
	entry := oauthStateEntry{ProviderName: provider, PKCEVerifier: verifier, ExpiresAt: time.Now().Add(15 * time.Minute)}
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return h.runtimeState.Put(c.Request().Context(), "oauth-state", state, payload, entry.ExpiresAt)
}

func (h *AuthHandler) takeOAuthState(c echo.Context, state string) (oauthStateEntry, bool) {
	if h.runtimeState == nil {
		return h.oauthState.Take(state)
	}
	payload, found, err := h.runtimeState.Get(c.Request().Context(), "oauth-state", state, true)
	var entry oauthStateEntry
	if err != nil {
		slog.Error("shared OAuth state lookup failed", "error", err)
	}
	if err != nil || !found || json.Unmarshal(payload, &entry) != nil {
		return oauthStateEntry{}, false
	}
	return entry, true
}

type sharedLogin struct {
	Username string
	Admin    bool
}

func (h *AuthHandler) putOneTimeToken(c echo.Context, token, username string, admin bool) error {
	if h.runtimeState == nil {
		h.oneTimeStore.Put(token, username, admin, 10*time.Minute)
		return nil
	}
	payload, err := json.Marshal(sharedLogin{Username: username, Admin: admin})
	if err != nil {
		return err
	}
	return h.runtimeState.Put(c.Request().Context(), "oauth-login", token, payload, time.Now().Add(10*time.Minute))
}

func (h *AuthHandler) consumeOneTimeToken(c echo.Context, token string) (string, bool, bool) {
	if h.runtimeState == nil {
		return h.oneTimeStore.Consume(token)
	}
	payload, found, err := h.runtimeState.Get(c.Request().Context(), "oauth-login", token, true)
	if err != nil {
		slog.Error("shared OAuth login lookup failed", "error", err)
	}
	var login sharedLogin
	if err != nil || !found || json.Unmarshal(payload, &login) != nil {
		return "", false, false
	}
	return login.Username, login.Admin, true
}
