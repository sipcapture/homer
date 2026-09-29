package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// NormalizeOrigin parses a serialized browser origin (scheme://host[:port]) and
// returns it lowercased with the scheme's default port removed, which is the
// form browsers send in the Origin header. Paths other than "/", query strings,
// fragments, userinfo, wildcards, and non-HTTP(S) schemes are rejected.
func NormalizeOrigin(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("empty origin")
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("invalid origin %q: %w", raw, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("origin %q must use http or https", raw)
	}
	if u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(s, "#") {
		return "", fmt.Errorf("origin %q must be scheme://host[:port] only", raw)
	}
	if u.Path != "" && u.Path != "/" {
		return "", fmt.Errorf("origin %q must not contain a path", raw)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("origin %q has no host", raw)
	}
	if strings.Contains(host, "*") {
		return "", fmt.Errorf("origin %q must not contain wildcards", raw)
	}
	port := u.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return scheme + "://" + host, nil
}

func normalizeCookieTrustedOrigins(cfg *Config) error {
	origins := cfg.Coordinator.JWT.CookieTrustedOrigins
	for i, o := range origins {
		n, err := NormalizeOrigin(o)
		if err != nil {
			return fmt.Errorf("coordinator.jwt.cookie_trusted_origins[%d]: %w", i, err)
		}
		origins[i] = n
	}
	return nil
}
