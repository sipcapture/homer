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
	if !oauthMatchAdminGroup([]string{"VoIP Admin"}, []string{"VoIP Admin"}) {
		t.Fatal("expected match with space in group name")
	}
}

func TestOauthCollectGroups(t *testing.T) {
	prof := map[string]interface{}{
		"groups": []interface{}{
			"plain",
			map[string]interface{}{"name": "VoIP Admin", "pk": "x"},
		},
	}
	got := oauthCollectGroups(prof, "groups")
	if len(got) != 2 || got[0] != "plain" || got[1] != "VoIP Admin" {
		t.Fatalf("groups: got %#v", got)
	}
}
