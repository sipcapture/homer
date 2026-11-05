package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

type timeoutRuntimeState struct{}

func (timeoutRuntimeState) Get(ctx context.Context, _, _ string, _ bool) ([]byte, bool, error) {
	<-ctx.Done()
	return nil, false, ctx.Err()
}
func (timeoutRuntimeState) Put(context.Context, string, string, []byte, time.Time) error { return nil }

func TestSessionRevocationTimeoutFallback(t *testing.T) {
	for _, middleware := range []string{"legacy", "v4"} {
		for _, answer := range []string{"active", "revoked", "missing", "expired"} {
			t.Run(middleware+"/"+answer, func(t *testing.T) {
				h := &AuthHandler{jwtSecret: "timeout-regression", expireHours: 1, sessionStore: NewSessionStore(), runtimeState: timeoutRuntimeState{}}
				token, id, err := h.generateToken("alice", false, false)
				if err != nil {
					t.Fatal(err)
				}
				want := http.StatusServiceUnavailable
				if answer == "active" || answer == "revoked" {
					h.revocations.remember(id, answer == "revoked")
					want = http.StatusNoContent
					if answer == "revoked" {
						want = http.StatusUnauthorized
					}
				}
				if answer == "expired" {
					h.revocations.entries = map[string]revocationAnswer{id: {expires: time.Now().Add(-time.Second)}}
				}
				e := echo.New()
				mw := h.JWTMiddleware()
				if middleware == "v4" {
					mw = h.JWTMiddlewareV4()
				}
				e.GET("/protected", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) }, mw)
				req := httptest.NewRequest(http.MethodGet, "/protected", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, req)
				if rec.Code != want {
					t.Fatalf("timeout: got %d want %d: %s", rec.Code, want, rec.Body.String())
				}
			})
		}
	}
}
