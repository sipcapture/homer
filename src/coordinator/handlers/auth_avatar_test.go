package handlers

import (
	"testing"
	"time"
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
