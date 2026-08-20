// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Well-known hosts that serve OAuth profile photos for common IdPs.
// Matched exactly or as a DNS suffix (e.g. lh3.googleusercontent.com).
var defaultAvatarHostSuffixes = []string{
	"graph.microsoft.com",
	"googleusercontent.com",
	"googleapis.com",
	"gstatic.com",
	"githubusercontent.com",
	"gravatar.com",
}

func validateAvatarURL(raw string, provider *OAuthProvider) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("empty avatar URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse avatar URL: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("avatar URL scheme %q is not https", u.Scheme)
	}
	if u.User != nil {
		return fmt.Errorf("avatar URL must not contain userinfo")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" {
		return fmt.Errorf("avatar URL host is empty")
	}
	if ip := net.ParseIP(host); ip != nil && forbiddenAvatarIP(ip) && !providerOwnsHost(provider, host) {
		return fmt.Errorf("avatar URL host is a forbidden IP")
	}
	if !avatarHostAllowed(host, provider) {
		return fmt.Errorf("avatar URL host %q is not allowlisted", host)
	}
	return nil
}

func providerOwnsHost(provider *OAuthProvider, host string) bool {
	if provider == nil {
		return false
	}
	for _, raw := range []string{provider.ProfileURL, provider.URL} {
		if strings.ToLower(strings.TrimSuffix(hostnameFromURL(raw), ".")) == host {
			return true
		}
	}
	if provider.OAuth2Config != nil {
		for _, raw := range []string{provider.OAuth2Config.Endpoint.AuthURL, provider.OAuth2Config.Endpoint.TokenURL} {
			if strings.ToLower(strings.TrimSuffix(hostnameFromURL(raw), ".")) == host {
				return true
			}
		}
	}
	return false
}

func avatarHostAllowed(host string, provider *OAuthProvider) bool {
	for _, suffix := range avatarAllowedHostSuffixes(provider) {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

func avatarAllowedHostSuffixes(provider *OAuthProvider) []string {
	seen := make(map[string]struct{}, len(defaultAvatarHostSuffixes)+4)
	var out []string
	add := func(h string) {
		h = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(h), "."))
		if h == "" {
			return
		}
		if _, ok := seen[h]; ok {
			return
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	for _, h := range defaultAvatarHostSuffixes {
		add(h)
	}
	if provider == nil {
		return out
	}
	add(hostnameFromURL(provider.ProfileURL))
	add(hostnameFromURL(provider.URL))
	if provider.OAuth2Config != nil {
		add(hostnameFromURL(provider.OAuth2Config.Endpoint.AuthURL))
		add(hostnameFromURL(provider.OAuth2Config.Endpoint.TokenURL))
	}
	return out
}

func hostnameFromURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return ""
	}
	return u.Hostname()
}

func forbiddenAvatarIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	ip4 := ip.To4()
	if ip4 != nil {
		ip = ip4
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}
