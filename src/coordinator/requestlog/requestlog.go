// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package requestlog

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// sensitiveQueryParams are query parameters that can carry a credential.
// Their values are masked in the request log.
var sensitiveQueryParams = map[string]struct{}{
	"access_token":  {},
	"id_token":      {},
	"refresh_token": {},
	"token":         {},
	"auth_token":    {},
	"api_key":       {},
	"apikey":        {},
	"password":      {},
	"secret":        {},
	"code":          {},
	"state":         {},
}

const redactedValue = "REDACTED"

// redactURI returns uri with the values of sensitive query parameters masked.
// The path and the other parameters are kept as they are.
func redactURI(uri string) string {
	i := strings.IndexByte(uri, '?')
	if i < 0 {
		return uri
	}
	path, rawQuery := uri[:i], uri[i+1:]
	parts := strings.Split(rawQuery, "&")
	changed := false
	for j, p := range parts {
		key := p
		if k := strings.IndexByte(p, '='); k >= 0 {
			key = p[:k]
		}
		name, err := url.QueryUnescape(key)
		if err != nil {
			name = key
		}
		if _, ok := sensitiveQueryParams[strings.ToLower(name)]; ok {
			parts[j] = key + "=" + redactedValue
			changed = true
		}
	}
	if !changed {
		return uri
	}
	return path + "?" + strings.Join(parts, "&")
}

// Middleware logs one line per request in the same format the coordinator
// used before ("time method uri status latency"), with credentials in the
// query string masked.
func Middleware(out io.Writer) echo.MiddlewareFunc {
	if out == nil {
		out = os.Stdout
	}
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogURI:      true,
		LogMethod:   true,
		LogStatus:   true,
		LogLatency:  true,
		HandleError: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			_, err := fmt.Fprintf(out, "%s %s %s %d %s\n",
				v.StartTime.UTC().Format(time.RFC3339),
				v.Method,
				redactURI(v.URI),
				v.Status,
				v.Latency.String(),
			)
			return err
		},
	})
}
