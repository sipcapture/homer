// Copyright (C) 2025 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package coordinator

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// UIContentSecurityPolicy is sent on every coordinator HTTP response.
//
// Scripts: 'self' only — no unsafe-eval / unsafe-inline (GHSA-626p-c2xw-r7pg).
// Styles: 'unsafe-inline' is required for React style attributes.
// Frames: http/https so the dashboard iframe widget can embed Grafana etc.
// Workers/wasm: chess engine worker (Vite may use blob:) and Doom WASM.
const UIContentSecurityPolicy = "default-src 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'self'; form-action 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self' ws: wss:; worker-src 'self' blob:; child-src 'self' blob:; frame-src 'self' http: https:; media-src 'self' blob:; wasm-src 'self'"

func securityHeaders() echo.MiddlewareFunc {
	return middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:         "0",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "SAMEORIGIN",
		HSTSMaxAge:            0,
		ContentSecurityPolicy: UIContentSecurityPolicy,
		ReferrerPolicy:        "strict-origin-when-cross-origin",
	})
}
