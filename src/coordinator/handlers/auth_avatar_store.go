// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/sipcapture/homer-core/src/coordinator/services"
)

type avatarEntry struct {
	data        []byte
	contentType string
	expiresAt   time.Time
}

// AvatarStore caches SSO profile photos keyed by username (in-memory, refreshed on OAuth login).
type AvatarStore struct {
	shared *services.RuntimeState
	mu     sync.RWMutex
	items  map[string]avatarEntry
	ttl    time.Duration
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
	if s.shared != nil {
		payload, err := json.Marshal(struct {
			Data        []byte
			ContentType string
		}{data, contentType})
		if err == nil {
			err = s.shared.Put(context.Background(), "avatar", username, payload, time.Now().Add(s.ttl))
		}
		if err != nil {
			slog.Warn("shared avatar cache write failed", "error", err)
		}
		return
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
	if s.shared != nil {
		payload, found, err := s.shared.Get(context.Background(), "avatar", username, false)
		var entry struct {
			Data        []byte
			ContentType string
		}
		if err != nil || !found || json.Unmarshal(payload, &entry) != nil {
			return nil, "", false
		}
		return entry.Data, entry.ContentType, true
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
