// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

const maxAvatarBytes = 2 << 20 // 2 MiB

// V4GetMeAvatar serves the cached SSO profile photo for the authenticated user.
// GET /api/v4/me/avatar
func (h *AuthHandler) V4GetMeAvatar(c echo.Context) error {
	claims, err := h.jwtClaimsFromContext(c)
	if err != nil {
		return v4ContextError(c, err)
	}
	if h.avatarStore == nil {
		return c.NoContent(http.StatusNotFound)
	}
	data, contentType, ok := h.avatarStore.Get(claims.Username)
	if !ok {
		return c.NoContent(http.StatusNotFound)
	}
	return c.Blob(http.StatusOK, contentType, data)
}

func oauthAvatarURLs(profile map[string]interface{}, provider *OAuthProvider) []string {
	seen := make(map[string]struct{})
	var urls []string
	add := func(u string) {
		u = strings.TrimSpace(u)
		if u == "" {
			return
		}
		if _, ok := seen[u]; ok {
			return
		}
		seen[u] = struct{}{}
		urls = append(urls, u)
	}

	add(oauthStringClaim(profile, "picture"))
	add(oauthStringClaim(profile, "avatar"))
	add(oauthStringClaim(profile, "photo"))

	if provider != nil {
		profileURL := strings.ToLower(provider.ProfileURL)
		name := strings.ToLower(provider.Name)
		if strings.Contains(profileURL, "graph.microsoft.com") || name == "azure" || name == "microsoft" {
			add("https://graph.microsoft.com/v1.0/me/photo/$value")
		}
	}
	return urls
}

func fetchOAuthProfilePhoto(ctx context.Context, client *http.Client, profile map[string]interface{}, provider *OAuthProvider) ([]byte, string, error) {
	if client == nil {
		return nil, "", fmt.Errorf("http client is nil")
	}
	var lastErr error
	for _, u := range oauthAvatarURLs(profile, provider) {
		data, ctype, err := fetchAuthenticatedImage(ctx, client, u)
		if err != nil {
			lastErr = err
			continue
		}
		if len(data) > 0 {
			return data, ctype, nil
		}
	}
	if lastErr != nil {
		return nil, "", lastErr
	}
	return nil, "", fmt.Errorf("no profile photo available")
}

func fetchAuthenticatedImage(ctx context.Context, client *http.Client, rawURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent {
		return nil, "", fmt.Errorf("photo HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("photo HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAvatarBytes))
	if err != nil {
		return nil, "", err
	}
	ctype := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if ctype == "" || !strings.HasPrefix(ctype, "image/") {
		ctype = "image/jpeg"
	}
	return body, ctype, nil
}

func cacheOAuthAvatar(h *AuthHandler, username string, profile map[string]interface{}, client *http.Client, provider *OAuthProvider) {
	if h == nil || h.avatarStore == nil || username == "" || client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	data, ctype, err := fetchOAuthProfilePhoto(ctx, client, profile, provider)
	if err != nil || len(data) == 0 {
		return
	}
	h.avatarStore.Put(username, data, ctype)
}
