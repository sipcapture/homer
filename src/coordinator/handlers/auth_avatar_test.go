package handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestOauthAvatarURLsMicrosoft(t *testing.T) {
	prof := map[string]interface{}{
		"picture": "https://graph.microsoft.com/v1.0/me/photo/$value",
	}
	p := &OAuthProvider{
		Name:       "azure",
		ProfileURL: "https://graph.microsoft.com/oidc/userinfo",
	}
	urls := oauthAvatarURLs(prof, p)
	if len(urls) < 1 || urls[0] != "https://graph.microsoft.com/v1.0/me/photo/$value" {
		t.Fatalf("urls: %#v", urls)
	}
}

func TestOauthAvatarURLs_DropsSSRFTargets(t *testing.T) {
	p := &OAuthProvider{
		Name:       "azure",
		ProfileURL: "https://graph.microsoft.com/oidc/userinfo",
	}
	prof := map[string]interface{}{
		"picture": "http://169.254.169.254/latest/meta-data/",
		"avatar":  "https://127.0.0.1/avatar.png",
		"photo":   "https://evil.example/p.png",
	}
	urls := oauthAvatarURLs(prof, p)
	for _, u := range urls {
		if strings.Contains(u, "169.254") || strings.Contains(u, "127.0.0.1") || strings.Contains(u, "evil.example") {
			t.Fatalf("SSRF target leaked into fetch list: %#v", urls)
		}
	}
}

func TestValidateAvatarURL(t *testing.T) {
	azure := &OAuthProvider{
		Name:       "azure",
		ProfileURL: "https://graph.microsoft.com/oidc/userinfo",
	}
	google := &OAuthProvider{
		ProfileURL: "https://www.googleapis.com/oauth2/v3/userinfo",
	}
	custom := &OAuthProvider{
		ProfileURL: "https://idp.corp.example/oauth/userinfo",
		OAuth2Config: &oauth2.Config{
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://idp.corp.example/oauth/authorize",
				TokenURL: "https://idp.corp.example/oauth/token",
			},
		},
	}

	tests := []struct {
		name     string
		raw      string
		provider *OAuthProvider
		wantErr  bool
	}{
		{name: "microsoft graph", raw: "https://graph.microsoft.com/v1.0/me/photo/$value", provider: azure},
		{name: "google cdn", raw: "https://lh3.googleusercontent.com/a/xxx", provider: google},
		{name: "provider host", raw: "https://idp.corp.example/users/1/avatar", provider: custom},
		{name: "http metadata", raw: "http://169.254.169.254/latest/meta-data/", provider: azure, wantErr: true},
		{name: "https loopback", raw: "https://127.0.0.1/avatar.png", provider: azure, wantErr: true},
		{name: "https private", raw: "https://10.0.0.5/avatar.png", provider: azure, wantErr: true},
		{name: "off-allowlist host", raw: "https://evil.example/p.png", provider: azure, wantErr: true},
		{name: "userinfo trick", raw: "https://graph.microsoft.com@169.254.169.254/latest/", provider: azure, wantErr: true},
		{name: "file scheme", raw: "file:///etc/passwd", provider: azure, wantErr: true},
		{name: "empty", raw: "  ", provider: azure, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAvatarURL(tt.raw, tt.provider)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestFetchAuthenticatedImage_RejectsRedirectOffAllowlist(t *testing.T) {
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("redirect to off-allowlist host must not be followed")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(internal.Close)

	pub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL+"/secret", http.StatusFound)
	}))
	t.Cleanup(pub.Close)

	client := avatarHTTPClient(http.DefaultClient, &OAuthProvider{
		ProfileURL: "https://graph.microsoft.com/oidc/userinfo",
	})
	req, err := http.NewRequest(http.MethodGet, pub.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected redirect validation error")
	}
}

func TestFetchAuthenticatedImage_AllowsHTTPSOnProviderHost(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = io.WriteString(w, "PNG")
	}))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	provider := &OAuthProvider{ProfileURL: "https://" + u.Host + "/userinfo"}
	avatar := srv.URL + "/photo.png"

	if err := validateAvatarURL(avatar, provider); err != nil {
		t.Fatalf("validate: %v", err)
	}

	data, ctype, err := fetchAuthenticatedImage(context.Background(), srv.Client(), avatar, provider)
	if err != nil {
		t.Fatal(err)
	}
	if ctype != "image/png" || string(data) != "PNG" {
		t.Fatalf("got ctype=%s data=%q", ctype, data)
	}
}

func TestAvatarStorePutGet(t *testing.T) {
	s := NewAvatarStore(time.Minute)
	s.Put("alice", []byte{1, 2, 3}, "image/png")
	data, ctype, ok := s.Get("alice")
	if !ok || ctype != "image/png" || len(data) != 3 {
		t.Fatalf("get: ok=%v ctype=%s len=%d", ok, ctype, len(data))
	}
	if !s.Has("alice") {
		t.Fatal("expected Has true")
	}
}
