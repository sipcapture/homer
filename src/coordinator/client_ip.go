// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package coordinator

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// clientIPExtractor is the coordinator's Echo IPExtractor.
//
// Echo v4.16.3 ignores X-Forwarded-For and X-Real-IP unless an extractor is
// set, and uses the connection peer instead. Homer sits behind a reverse
// proxy on loopback or a private address, so those headers still carry the
// client address when the direct peer is trusted (Echo's default: loopback,
// link-local, and private ranges). A client that connects directly cannot
// choose its address by sending the headers.
func clientIPExtractor() echo.IPExtractor {
	fromXFF := echo.ExtractIPFromXFFHeader()
	fromRealIP := echo.ExtractIPFromRealIPHeader()
	direct := echo.ExtractIPDirect()
	return func(req *http.Request) string {
		if req.Header.Get(echo.HeaderXForwardedFor) != "" {
			return fromXFF(req)
		}
		if req.Header.Get(echo.HeaderXRealIP) != "" {
			return fromRealIP(req)
		}
		return direct(req)
	}
}
