// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"sync"
	"time"
)

// oauthStateEntry holds CSRF state for the authorization code flow (and optional PKCE verifier).
type oauthStateEntry struct {
	ProviderName string
	PKCEVerifier string
	ExpiresAt    time.Time
}

// OAuthStateStore maps OAuth2 "state" query values to pending login metadata (in-memory).
type OAuthStateStore struct {
	mu sync.Mutex
	m  map[string]oauthStateEntry
}

func NewOAuthStateStore() *OAuthStateStore {
	return &OAuthStateStore{m: make(map[string]oauthStateEntry)}
}

func (s *OAuthStateStore) Put(state, providerName, pkceVerifier string, ttl time.Duration) {
	if state == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[state] = oauthStateEntry{
		ProviderName: providerName,
		PKCEVerifier: pkceVerifier,
		ExpiresAt:    time.Now().Add(ttl),
	}
}

// Take consumes a state entry if present and valid.
func (s *OAuthStateStore) Take(state string) (oauthStateEntry, bool) {
	if state == "" {
		return oauthStateEntry{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[state]
	if !ok {
		return oauthStateEntry{}, false
	}
	delete(s.m, state)
	if time.Now().After(e.ExpiresAt) {
		return oauthStateEntry{}, false
	}
	return e, true
}
