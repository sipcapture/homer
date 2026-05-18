package handlers

import "testing"

func TestOauthPickUsername(t *testing.T) {
	if g, w := oauthPickUsername("alice", "a@b.com", "sub1"), "alice"; g != w {
		t.Fatalf("preferred: got %q want %q", g, w)
	}
	if g, w := oauthPickUsername("", "User@Example.COM", ""), "user_example.com"; g != w {
		t.Fatalf("email: got %q want %q", g, w)
	}
	if g := oauthPickUsername("", "", "abc|def"); g != "oidc-abc-def" {
		t.Fatalf("sub: got %q", g)
	}
}

func TestOauthMatchAdminGroup(t *testing.T) {
	if !oauthMatchAdminGroup([]string{"a", "Admins"}, []string{"Admins"}) {
		t.Fatal("expected match")
	}
	if oauthMatchAdminGroup([]string{"a"}, []string{"b"}) {
		t.Fatal("unexpected match")
	}
}
