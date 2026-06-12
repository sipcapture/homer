// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package siprecreceiver

import "testing"

func TestParseInviteBodyMultipart(t *testing.T) {
	body := []byte(`--boundary
Content-Type: application/sdp

v=0
o=- 0 0 IN IP4 10.0.0.1
s=-
c=IN IP4 10.0.0.1
t=0 0
m=audio 49170 RTP/AVP 0
a=sendonly

--boundary
Content-Type: application/rs-metadata+xml

<?xml version="1.0"?><recording xmlns="urn:ietf:params:xml:ns:recording:1"><session session_id="s1"><sipSessionID>call-1</sipSessionID></session></recording>
--boundary--
`)
	ct := `multipart/mixed; boundary=boundary`
	parsed, err := parseInviteBody(ct, body)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.SDP == nil || parsed.MetadataXML == nil {
		t.Fatal("expected sdp and metadata parts")
	}
}
