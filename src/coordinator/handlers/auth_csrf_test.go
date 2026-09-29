package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/sipcapture/homer-core/src/config"
)

func TestValidateCSRFForCookieAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		trusted []string
		host    string
		headers map[string]string
		want    bool
	}{
		{
			name:    "same origin",
			host:    "homer.local:8080",
			headers: map[string]string{"Origin": "http://homer.local:8080"},
			want:    true,
		},
		{
			name:    "same origin via referer",
			host:    "homer.local:8080",
			headers: map[string]string{"Referer": "http://homer.local:8080/search?q=1"},
			want:    true,
		},
		{
			name:    "no origin or referer",
			host:    "homer.local:8080",
			headers: map[string]string{},
			want:    true,
		},
		{
			name:    "cross origin rejected",
			host:    "homer.local:8080",
			headers: map[string]string{"Origin": "https://evil.example"},
			want:    false,
		},
		{
			name:    "referer prefix trick rejected",
			host:    "homer.local:8080",
			headers: map[string]string{"Referer": "http://homer.local:8080.evil.example/x"},
			want:    false,
		},
		{
			name:    "opaque origin rejected",
			host:    "homer.local:8080",
			headers: map[string]string{"Origin": "null"},
			want:    false,
		},
		{
			name: "proxy rewrites host, origin not trusted",
			host: "127.0.0.1:8080",
			headers: map[string]string{
				"Origin":            "https://homer.example.com",
				"X-Forwarded-Proto": "https",
			},
			want: false,
		},
		{
			name:    "proxy rewrites host, origin trusted",
			trusted: []string{"https://homer.example.com"},
			host:    "127.0.0.1:8080",
			headers: map[string]string{
				"Origin":            "https://homer.example.com",
				"X-Forwarded-Proto": "https",
			},
			want: true,
		},
		{
			name:    "trusted origin matched via referer",
			trusted: []string{"https://homer.example.com"},
			host:    "127.0.0.1:8080",
			headers: map[string]string{"Referer": "https://homer.example.com/call/123"},
			want:    true,
		},
		{
			name:    "trusted origin with default port matches browser origin",
			trusted: []string{"https://Homer.Example.com:443/"},
			host:    "127.0.0.1:8080",
			headers: map[string]string{"Origin": "https://homer.example.com"},
			want:    true,
		},
		{
			name:    "scheme must match trusted origin",
			trusted: []string{"https://homer.example.com"},
			host:    "127.0.0.1:8080",
			headers: map[string]string{"Origin": "http://homer.example.com"},
			want:    false,
		},
		{
			name:    "forged X-Forwarded-Host is ignored",
			trusted: []string{"https://homer.example.com"},
			host:    "127.0.0.1:8080",
			headers: map[string]string{
				"Origin":           "https://evil.example",
				"X-Forwarded-Host": "evil.example",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := NewAuthHandlerWithUserService(
				"test-secret-minimum-32-characters-long",
				24,
				config.JWTConfig{CookieTrustedOrigins: tt.trusted},
				nil, nil, nil, config.APISettingsConfig{}, nil, "", false,
			)
			req := httptest.NewRequest(http.MethodPost, "/api/v4/search", nil)
			req.Host = tt.host
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			c := echo.New().NewContext(req, httptest.NewRecorder())
			if got := h.validateCSRFForCookieAuth(c); got != tt.want {
				t.Fatalf("validateCSRFForCookieAuth() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJWTMiddlewareV4_CookieCSRFBehindHostRewritingProxy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		trusted []string
		want    int
	}{
		{name: "public origin not configured", want: http.StatusForbidden},
		{name: "public origin configured", trusted: []string{"https://homer.example.com"}, want: http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newCookieAuthHandler(t)
			for _, o := range tc.trusted {
				h.trustedOrigins[o] = struct{}{}
			}
			token, _, err := h.generateToken("admin", true, false)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v4/search", nil)
			req.Host = "127.0.0.1:9080"
			req.Header.Set("Origin", "https://homer.example.com")
			req.Header.Set("X-Forwarded-Proto", "https")
			req.Header.Set("X-Forwarded-Host", "homer.example.com")
			req.AddCookie(&http.Cookie{Name: "homer_session", Value: token})
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)

			handler := h.JWTMiddlewareV4()(func(c echo.Context) error {
				return c.NoContent(http.StatusNoContent)
			})
			if err := handler(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != tc.want {
				t.Fatalf("status: got %d want %d", rec.Code, tc.want)
			}
		})
	}
}
