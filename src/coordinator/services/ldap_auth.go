// Copyright (C) 2026 Homer Server Contributors
//
// LDAP password authentication for the coordinator API (aligned with homer-app
// ldap_config + utils/ldap behaviour: search + user bind, optional group checks).

package services

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"

	"github.com/go-ldap/ldap/v3"
	"github.com/sipcapture/homer-core/src/config"
)

// LDAPAuthService validates credentials against a directory server.
type LDAPAuthService struct {
	cfg config.LDAPConfig
}

// NewLDAPAuthService returns nil when LDAP is disabled or misconfigured.
func NewLDAPAuthService(cfg *config.LDAPConfig) *LDAPAuthService {
	if cfg == nil || !cfg.Enable || strings.TrimSpace(cfg.Host) == "" {
		return nil
	}
	c := *cfg
	if c.UserFilter == "" {
		c.UserFilter = "(uid=%s)"
	}
	if c.GroupFilter == "" {
		c.GroupFilter = "(memberUid=%s)"
	}
	if len(c.GroupAttribute) == 0 {
		c.GroupAttribute = []string{"memberOf"}
	}
	if len(c.Attributes) == 0 {
		c.Attributes = []string{"givenName", "sn", "mail", "uid"}
	}
	if c.Port <= 0 {
		c.Port = 389
	}
	return &LDAPAuthService{cfg: c}
}

// Enabled reports whether LDAP login should be offered and accepted.
func (s *LDAPAuthService) Enabled() bool {
	return s != nil && s.cfg.Enable && strings.TrimSpace(s.cfg.Host) != ""
}

// Authenticate mirrors homer-app LDAPClient.Authenticate + group membership rules.
func (s *LDAPAuthService) Authenticate(ctx context.Context, username, password string) (*User, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("ldap authentication is not configured")
	}
	if username == "" || password == "" {
		return nil, fmt.Errorf("username and password are required")
	}

	conn, err := s.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if !s.cfg.Anonymous {
		if s.cfg.BindDN != "" && s.cfg.BindPassword != "" {
			if err := conn.Bind(s.cfg.BindDN, s.cfg.BindPassword); err != nil {
				return nil, fmt.Errorf("ldap bind (service account): %w", err)
			}
		}

		attrs := append(append([]string{}, s.cfg.Attributes...), "dn")
		userFilter := fmt.Sprintf(s.cfg.UserFilter, ldap.EscapeFilter(username))
		sr, err := conn.Search(ldap.NewSearchRequest(
			s.cfg.Base,
			ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
			userFilter,
			attrs,
			nil,
		))
		if err != nil {
			return nil, fmt.Errorf("ldap user search: %w", err)
		}
		if len(sr.Entries) < 1 {
			return nil, fmt.Errorf("invalid credentials")
		}
		if len(sr.Entries) > 1 {
			return nil, fmt.Errorf("ldap user search: ambiguous result")
		}

		userDN := sr.Entries[0].DN
		if err := conn.Bind(userDN, password); err != nil {
			return nil, fmt.Errorf("invalid credentials")
		}

		if s.cfg.BindDN != "" && s.cfg.BindPassword != "" {
			if err := conn.Bind(s.cfg.BindDN, s.cfg.BindPassword); err != nil {
				return nil, fmt.Errorf("ldap rebind after user auth: %w", err)
			}
		}

		isAdmin := s.cfg.AdminMode
		groupKey := username
		if s.cfg.UseDNForGroupSearch && userDN != "" {
			groupKey = userDN
		}
		groups, gerr := s.groupsForUser(conn, groupKey)
		if gerr != nil {
			if !s.cfg.UserMode && !s.cfg.AdminMode {
				return nil, gerr
			}
		} else if len(groups) > 0 {
			if s.cfg.AdminGroup != "" && containsFold(groups, s.cfg.AdminGroup) {
				isAdmin = true
			} else if s.cfg.UserGroup != "" && containsFold(groups, s.cfg.UserGroup) {
				isAdmin = false
			} else {
				if !isAdmin && s.cfg.UserMode {
					// keep non-admin
				} else if isAdmin && s.cfg.AdminMode {
					// keep admin from bind phase
				} else if s.cfg.AdminGroup != "" || s.cfg.UserGroup != "" {
					return nil, fmt.Errorf("ldap group membership required for login")
				}
			}
		} else if s.cfg.AdminGroup != "" || s.cfg.UserGroup != "" {
			if !isAdmin && !s.cfg.UserMode && !s.cfg.AdminMode {
				return nil, fmt.Errorf("ldap group membership required for login")
			}
		}

		return &User{
			ID:       0,
			Username: username,
			IsAdmin:  isAdmin,
			IsActive: true,
		}, nil
	}

	if s.cfg.UserDN == "" {
		return nil, fmt.Errorf("ldap anonymous mode requires user_dn")
	}
	userDN := fmt.Sprintf(s.cfg.UserDN, username)
	if err := conn.Bind(userDN, password); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}
	return &User{
		ID:       0,
		Username: username,
		IsAdmin:  s.cfg.AdminMode,
		IsActive: true,
	}, nil
}

func (s *LDAPAuthService) dial(ctx context.Context) (*ldap.Conn, error) {
	_ = ctx
	hosts := strings.Fields(s.cfg.Host)
	var lastErr error
	for _, host := range hosts {
		addr := fmt.Sprintf("%s:%d", host, s.cfg.Port)
		var conn *ldap.Conn
		var err error
		if s.cfg.UseSSL {
			tlsCfg := &tls.Config{
				InsecureSkipVerify: s.cfg.InsecureSkipVerify,
				ServerName:         s.cfg.ServerName,
			}
			conn, err = ldap.DialTLS("tcp", addr, tlsCfg)
		} else {
			conn, err = ldap.Dial("tcp", addr)
		}
		if err != nil {
			lastErr = err
			continue
		}
		if !s.cfg.UseSSL && !s.cfg.SkipTLS {
			tlsCfg := &tls.Config{
				InsecureSkipVerify: s.cfg.InsecureSkipVerify,
				ServerName:         firstNonEmpty(s.cfg.ServerName, host),
			}
			if err := conn.StartTLS(tlsCfg); err != nil {
				conn.Close()
				lastErr = err
				continue
			}
		}
		return conn, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no ldap hosts configured")
	}
	return nil, fmt.Errorf("ldap dial: %w", lastErr)
}

func (s *LDAPAuthService) groupsForUser(conn *ldap.Conn, groupLookupKey string) ([]string, error) {
	filter := fmt.Sprintf(s.cfg.GroupFilter, ldap.EscapeFilter(groupLookupKey))
	sr, err := conn.Search(ldap.NewSearchRequest(
		s.cfg.Base,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		filter,
		s.cfg.GroupAttribute,
		nil,
	))
	if err != nil {
		return nil, fmt.Errorf("ldap group search: %w", err)
	}
	var out []string
	for _, e := range sr.Entries {
		for _, attr := range s.cfg.GroupAttribute {
			out = append(out, e.GetAttributeValues(attr)...)
		}
	}
	return out, nil
}

func containsFold(haystack []string, needle string) bool {
	if needle == "" {
		return false
	}
	for _, h := range haystack {
		if strings.EqualFold(strings.TrimSpace(h), strings.TrimSpace(needle)) {
			return true
		}
	}
	return false
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
