// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/coordinator/services"
	"golang.org/x/oauth2"
)

// V4OAuth2Redirect handles GET /api/v4/auth/oauth2/{provider}/redirect
func (h *AuthHandler) V4OAuth2Redirect(c echo.Context) error {
	name := c.Param("provider")
	provider := h.findProvider(name)
	if provider == nil || !provider.Enable {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Provider not found")
	}
	if provider.OAuth2Config == nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "OAuth2 authorization code flow is not configured for this provider")
	}

	state, err := randomURLSafeState()
	if err != nil {
		return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to generate OAuth2 state")
	}
	var pkceVerifier string
	opts := []oauth2.AuthCodeOption{}
	if provider.UsePKCE {
		v, ch, err := newPKCEVerifierChallenge()
		if err != nil {
			return writeError(c, http.StatusInternalServerError, "Server Error", "Failed to generate PKCE verifier")
		}
		pkceVerifier = v
		opts = append(opts,
			oauth2.SetAuthURLParam("code_challenge", ch),
			oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		)
	}
	h.oauthState.Put(state, provider.Name, pkceVerifier, 15*time.Minute)

	authURL := provider.OAuth2Config.AuthCodeURL(state, opts...)
	return c.Redirect(http.StatusFound, authURL)
}

// V4OAuth2Callback handles GET /api/v4/auth/oauth2/{provider}/callback
func (h *AuthHandler) V4OAuth2Callback(c echo.Context) error {
	name := c.Param("provider")
	provider := h.findProvider(name)
	if provider == nil || !provider.Enable || provider.OAuth2Config == nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Provider not found")
	}

	if errParam := c.QueryParam("error"); errParam != "" {
		desc := c.QueryParam("error_description")
		msg := errParam
		if desc != "" {
			msg = msg + ": " + desc
		}
		return h.oauthRedirectWithError(c, provider, msg)
	}

	code := c.QueryParam("code")
	state := c.QueryParam("state")
	if code == "" || state == "" {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Authorization code and state are required")
	}

	entry, ok := h.oauthState.Take(state)
	if !ok || entry.ProviderName != provider.Name {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid or expired OAuth2 state")
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), 45*time.Second)
	defer cancel()

	exchangeOpts := []oauth2.AuthCodeOption{}
	if entry.PKCEVerifier != "" {
		exchangeOpts = append(exchangeOpts, oauth2.SetAuthURLParam("code_verifier", entry.PKCEVerifier))
	}

	tok, err := provider.OAuth2Config.Exchange(ctx, code, exchangeOpts...)
	if err != nil {
		return h.oauthRedirectWithError(c, provider, "token exchange failed: "+err.Error())
	}

	client := provider.OAuth2Config.Client(ctx, tok)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, provider.ProfileURL, nil)
	if err != nil {
		return h.oauthRedirectWithError(c, provider, "failed to build profile request")
	}
	resp, err := client.Do(req)
	if err != nil {
		return h.oauthRedirectWithError(c, provider, "profile request failed: "+err.Error())
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return h.oauthRedirectWithError(c, provider, "failed to read profile response")
	}
	if resp.StatusCode != http.StatusOK {
		return h.oauthRedirectWithError(c, provider, fmt.Sprintf("profile HTTP %d", resp.StatusCode))
	}

	var profile map[string]interface{}
	if err := json.Unmarshal(body, &profile); err != nil {
		return h.oauthRedirectWithError(c, provider, "invalid profile JSON")
	}

	email := oauthStringClaim(profile, "email")
	preferred := oauthStringClaim(profile, "preferred_username")
	sub := oauthStringClaim(profile, "sub")
	display := oauthStringClaim(profile, "name")
	if display == "" {
		display = preferred
	}
	if display == "" {
		display = email
	}

	username := oauthPickUsername(preferred, email, sub)
	if username == "" {
		return h.oauthRedirectWithError(c, provider, "OAuth profile did not contain a usable username (preferred_username, email, or sub)")
	}

	groups := oauthCollectGroups(profile, provider.GroupClaim)
	staffAdmin := oauthMatchAdminGroup(groups, provider.AdminGroups)
	if len(provider.AdminGroups) > 0 {
		claimKey := strings.TrimSpace(provider.GroupClaim)
		if claimKey == "" {
			claimKey = "groups"
		}
		_, claimPresent := profile[claimKey]
		slog.Debug("oauth2 admin group check",
			"provider", provider.Name,
			"group_claim", claimKey,
			"claim_present", claimPresent,
			"parsed_groups", groups,
			"admin_groups", provider.AdminGroups,
			"admin_match", staffAdmin,
		)
	}

	u, err := h.resolveOrProvisionOAuthUser(ctx, provider, username, email, display, staffAdmin)
	if err != nil {
		return h.oauthRedirectWithError(c, provider, err.Error())
	}

	isAdmin := u.IsAdmin || staffAdmin

	one := newSessionID()
	if h.oneTimeStore != nil {
		h.oneTimeStore.Put(one, u.Username, isAdmin, 10*time.Minute)
	}

	redirectURL := provider.CallbackURL
	if redirectURL == "" {
		redirectURL = c.QueryParam("redirect_uri")
	}
	if redirectURL == "" {
		redirectURL = "/"
	}
	target, err := url.Parse(redirectURL)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid redirect URL")
	}
	q := target.Query()
	q.Set("token", one)
	target.RawQuery = q.Encode()
	return c.Redirect(http.StatusFound, target.String())
}

func (h *AuthHandler) oauthRedirectWithError(c echo.Context, provider *OAuthProvider, detail string) error {
	redirectURL := provider.CallbackURL
	if redirectURL == "" {
		redirectURL = c.QueryParam("redirect_uri")
	}
	if redirectURL == "" {
		redirectURL = "/"
	}
	u, err := url.Parse(redirectURL)
	if err != nil {
		return writeError(c, http.StatusBadRequest, "Bad Request", "Invalid callback_url")
	}
	q := u.Query()
	q.Set("oauth_error", detail)
	u.RawQuery = q.Encode()
	return c.Redirect(http.StatusFound, u.String())
}

func (h *AuthHandler) resolveOrProvisionOAuthUser(ctx context.Context, p *OAuthProvider, username, email, display string, staffAdmin bool) (*services.User, error) {
	if h.userService == nil {
		return nil, fmt.Errorf("user database not configured")
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("empty username")
	}

	if u, err := h.userService.GetUserByUsername(ctx, username); err == nil && u != nil {
		if !u.IsActive {
			return nil, fmt.Errorf("user is disabled")
		}
		if staffAdmin && !u.IsAdmin {
			u2 := *u
			u2.IsAdmin = true
			return &u2, nil
		}
		return u, nil
	}

	if email != "" {
		if u, err := h.userService.GetUserByEmail(ctx, email); err == nil && u != nil {
			if !u.IsActive {
				return nil, fmt.Errorf("user is disabled")
			}
			if staffAdmin && !u.IsAdmin {
				u2 := *u
				u2.IsAdmin = true
				return &u2, nil
			}
			return u, nil
		}
	}

	if p.SkipAutoProvision {
		return nil, fmt.Errorf("no matching user and skip_auto_provision is true")
	}

	randPass, err := randomHex(32)
	if err != nil {
		return nil, fmt.Errorf("internal error")
	}
	isAdmin := staffAdmin
	if _, cerr := h.userService.CreateUser(ctx, username, randPass, email, display, isAdmin, true); cerr != nil {
		return nil, fmt.Errorf("failed to create user: %w", cerr)
	}
	return h.userService.GetUserByUsername(ctx, username)
}

func oauthStringClaim(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func oauthPickUsername(preferred, email, sub string) string {
	if strings.TrimSpace(preferred) != "" {
		return strings.TrimSpace(preferred)
	}
	email = strings.TrimSpace(strings.ToLower(email))
	if email != "" {
		return strings.ReplaceAll(email, "@", "_")
	}
	sub = strings.TrimSpace(sub)
	if sub != "" {
		return "oidc-" + strings.ReplaceAll(strings.ReplaceAll(sub, "|", "-"), " ", "")
	}
	return ""
}

func oauthCollectGroups(profile map[string]interface{}, claim string) []string {
	claim = strings.TrimSpace(claim)
	if claim == "" {
		claim = "groups"
	}
	v, ok := profile[claim]
	if !ok || v == nil {
		return nil
	}
	switch t := v.(type) {
	case string:
		if strings.TrimSpace(t) == "" {
			return nil
		}
		return []string{t}
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, x := range t {
			switch e := x.(type) {
			case string:
				if strings.TrimSpace(e) != "" {
					out = append(out, strings.TrimSpace(e))
				}
			case map[string]interface{}:
				// Some IdPs (including Authentik-style payloads) return group objects
				// with a display name under "name" or "group_name".
				if n := oauthStringClaim(e, "name"); n != "" {
					out = append(out, n)
				} else if n := oauthStringClaim(e, "group_name"); n != "" {
					out = append(out, n)
				}
			}
		}
		return out
	default:
		return nil
	}
}

func oauthMatchAdminGroup(groups, adminGroups []string) bool {
	if len(adminGroups) == 0 || len(groups) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(groups))
	for _, g := range groups {
		set[strings.TrimSpace(g)] = struct{}{}
	}
	for _, a := range adminGroups {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if _, ok := set[a]; ok {
			return true
		}
	}
	return false
}

func randomURLSafeState() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func newPKCEVerifierChallenge() (verifier string, challenge string, err error) {
	v := make([]byte, 32)
	if _, err = rand.Read(v); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(v)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func randomHex(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
