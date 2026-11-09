// Copyright (C) 2026 Homer Server Contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
)

// runtimeStateStore is the shared state used by authentication.
type runtimeStateStore interface {
	Get(context.Context, string, string, bool) ([]byte, bool, error)
	Put(context.Context, string, string, []byte, time.Time) error
}

// Bound revocation lookups below the five-second fallback lifetime, so a full
// database timeout leaves a recent answer available to the current request.
const revocationLookupTimeout = time.Second

// SetRuntimeState replaces instance-local HTTP state for stateless deployments.
func (h *AuthHandler) SetRuntimeState(state *services.RuntimeState) {
	h.runtimeState = state
	h.avatarStore.shared = state
}

// A recent successful answer can bridge a brief settings outage. Entries expire
// after five seconds; failures never extend that window.
type revocationAnswer struct {
	revoked bool
	expires time.Time
}

type revocationCache struct {
	mu      sync.Mutex
	entries map[string]revocationAnswer
}

func (r *revocationCache) remember(id string, revoked bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	if r.entries == nil {
		r.entries = make(map[string]revocationAnswer)
	}
	if _, exists := r.entries[id]; !exists && len(r.entries) >= 10000 {
		// Eviction only shortens the optional outage fallback. Never scan the
		// entire cache on the authentication hot path.
		for key := range r.entries {
			delete(r.entries, key)
			break
		}
	}
	r.entries[id] = revocationAnswer{revoked: revoked, expires: now.Add(5 * time.Second)}
}

func (h *AuthHandler) sessionIsRevoked(c echo.Context, id string) (bool, error) {
	if h.runtimeState == nil {
		return h.sessionStore.IsRevoked(id), nil
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), revocationLookupTimeout)
	defer cancel()
	_, found, err := h.runtimeState.Get(ctx, "revoked-session", id, false)
	if err == nil {
		h.revocations.remember(id, found)
		return found, nil
	}
	slog.Error("shared session lookup failed", "error", err)
	h.revocations.mu.Lock()
	answer, ok := h.revocations.entries[id]
	if ok && !time.Now().Before(answer.expires) {
		delete(h.revocations.entries, id)
		ok = false
	}
	h.revocations.mu.Unlock()
	if ok {
		return answer.revoked, nil
	}
	return false, err
}

func (h *AuthHandler) revokeSession(c echo.Context, id string, expires time.Time) error {
	if h.runtimeState == nil {
		h.sessionStore.Revoke(id, expires)
		return nil
	}
	if err := h.runtimeState.Put(c.Request().Context(), "revoked-session", id, []byte{1}, expires); err != nil {
		return err
	}
	h.revocations.remember(id, true)
	return nil
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
