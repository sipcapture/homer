// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package handlers

import (
	"sync"
	"time"
)

type sessionEntry struct {
	expiresAt time.Time
}

// SessionStore keeps revoked session IDs in memory
type SessionStore struct {
	mu      sync.Mutex
	entries map[string]sessionEntry
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		entries: make(map[string]sessionEntry),
	}
}

func (s *SessionStore) Revoke(sessionID string, expiresAt time.Time) {
	if sessionID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[sessionID] = sessionEntry{expiresAt: expiresAt}
}

func (s *SessionStore) IsRevoked(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[sessionID]
	if !ok {
		return false
	}
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		delete(s.entries, sessionID)
		return false
	}
	return true
}

type oneTimeEntry struct {
	username  string
	isAdmin   bool
	expiresAt time.Time
}

// OneTimeTokenStore keeps OAuth2 one-time tokens in memory
type OneTimeTokenStore struct {
	mu      sync.Mutex
	entries map[string]oneTimeEntry
}

func NewOneTimeTokenStore() *OneTimeTokenStore {
	return &OneTimeTokenStore{
		entries: make(map[string]oneTimeEntry),
	}
}

func (s *OneTimeTokenStore) Put(token, username string, isAdmin bool, ttl time.Duration) {
	if token == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[token] = oneTimeEntry{
		username:  username,
		isAdmin:   isAdmin,
		expiresAt: time.Now().Add(ttl),
	}
}

func (s *OneTimeTokenStore) Consume(token string) (string, bool, bool) {
	if token == "" {
		return "", false, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[token]
	if !ok {
		return "", false, false
	}
	delete(s.entries, token)
	if time.Now().After(entry.expiresAt) {
		return "", false, false
	}
	return entry.username, entry.isAdmin, true
}
