// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package sipparser

// CalleeUser returns the party stored as callee / to_user.
// Request-URI user is preferred when present; otherwise the To header user
// is used (responses and host-only RURI requests).
func (s *SipMsg) CalleeUser() string {
	if s == nil {
		return ""
	}
	if s.URIUser != "" {
		return s.URIUser
	}
	return s.ToUser
}
