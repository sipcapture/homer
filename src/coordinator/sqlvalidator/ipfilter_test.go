package sqlvalidator

import "testing"

func TestIPEqualsClause(t *testing.T) {
	const v6 = "src_ip IN ('2001:db8::10', '2001:0db8:0000:0000:0000:0000:0000:0010')"
	for _, tc := range []struct{ in, want string }{
		{"10.0.0.1", "src_ip = '10.0.0.1'"},
		{"not-an-ip'x", "src_ip = 'not-an-ip''x'"},
		{"2001:db8::10", v6},
		{"2001:0db8:0000:0000:0000:0000:0000:0010", v6},
		{"2001:DB8::10", v6},
		{" 2001:db8::10 ", v6},
		{"::ffff:192.0.2.1", "src_ip IN ('192.0.2.1', '0000:0000:0000:0000:0000:ffff:c000:0201')"},
		{"fe80::1%eth0", "src_ip IN ('fe80::1', 'fe80:0000:0000:0000:0000:0000:0000:0001')"},
	} {
		if got := IPEqualsClause("src_ip", tc.in); got != tc.want {
			t.Errorf("IPEqualsClause(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIPNotEqualsClause(t *testing.T) {
	if got := IPNotEqualsClause("dst_ip", "10.0.0.1"); got != "dst_ip != '10.0.0.1'" {
		t.Errorf("IPv4: got %q", got)
	}
	want := "dst_ip NOT IN ('2001:db8::1', '2001:0db8:0000:0000:0000:0000:0000:0001')"
	if got := IPNotEqualsClause("dst_ip", "2001:db8::1"); got != want {
		t.Errorf("IPv6: got %q, want %q", got, want)
	}
}
