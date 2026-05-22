// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"sync"
	"time"
)

type avatarEntry struct {
	data        []byte
	contentType string
	expiresAt   time.Time
}

// AvatarStore caches SSO profile photos keyed by username (in-memory, refreshed on OAuth login).
type AvatarStore struct {
	mu    sync.RWMutex
	items map[string]avatarEntry
	ttl   time.Duration
}

func NewAvatarStore(ttl time.Duration) *AvatarStore {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &AvatarStore{
		items: make(map[string]avatarEntry),
		ttl:   ttl,
	}
}

func (s *AvatarStore) Put(username string, data []byte, contentType string) {
	if s == nil || username == "" || len(data) == 0 {
		return
	}
	if contentType == "" {
		contentType = "image/jpeg"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[username] = avatarEntry{
		data:        append([]byte(nil), data...),
		contentType: contentType,
		expiresAt:   time.Now().Add(s.ttl),
	}
}

func (s *AvatarStore) Get(username string) (data []byte, contentType string, ok bool) {
	if s == nil || username == "" {
		return nil, "", false
	}
	s.mu.RLock()
	entry, found := s.items[username]
	s.mu.RUnlock()
	if !found {
		return nil, "", false
	}
	if time.Now().After(entry.expiresAt) {
		s.mu.Lock()
		delete(s.items, username)
		s.mu.Unlock()
		return nil, "", false
	}
	return append([]byte(nil), entry.data...), entry.contentType, true
}

func (s *AvatarStore) Has(username string) bool {
	_, _, ok := s.Get(username)
	return ok
}
