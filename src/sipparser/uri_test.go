// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//

package sipparser

// Imports from the go standard library
import (
	"testing"
)

func TestUri(t *testing.T) {
	s := "sip:15555551000;npdi=yes;rn=15555551999@0.0.0.0:5060;user=phone;key"
	u := ParseURI(s)
	if u.Scheme != SIP_SCHEME {
		t.Errorf("[TestUri] Error parsing URI \"sip:15555551000;npdi=yes;rn=15555551999@0.0.0.0:5060;user=phone;key\". Scheme should be sip not received val: %s", u.Scheme)
	}
	if u.User != "15555551000" {
		t.Errorf("[TestUri] Error parsing URI \"sip:15555551000@0.0.0.0:5060;user=phone\".  User value is not \"15555551000\".")
	}
	if u.Host != "0.0.0.0" {
		t.Errorf("[TestUri] Error parsing URI \"sip:15555551000@0.0.0.0:5060;user=phone\".  Host value should be \"0.0.0.0\" but received: %s", u.Host)
	}
	if u.Port != "5060" {
		t.Errorf("[TestUri] Error parsing URI \"sip:15555551000@0.0.0.0:5060;user=phone\". Port value should be '5060' ... but it is not.")
	}
	if u.PortInt != 5060 {
		t.Errorf("[TestUri] Error parsing URI \"sip:15555551000@0.0.0.0:5060;user=phone\". Port value should be 5060 ... but it is not.")
	}
	s = "tel:5554448000@myfoo.com"
	u = ParseURI(s)
	if u.User != "5554448000" {
		t.Errorf("[TestUri] Error parsing URI \"tel:5554448000@myfoo.com\".  Should have received \"5554448000\" as the user.  Received: %s", u.User)
	}
	if u.Host != "myfoo.com" {
		t.Errorf("[TestUri] Error parsing URI \"tel:5554448000@myfoo.com\".  Host should be \"myfoo.com\" but received: %s", u.Host)
	}
	s = "tel:+5554448000"
	u = ParseURI(s)
	if u.User != "+5554448000" {
		t.Errorf("[TestUri] Error parsing URI \"tel:+5554448000\".  Should have received \"+5554448000\" as the user.  Received: %s", u.User)
	}
	s = "sip:myfoo.com"
	u = ParseURI(s)
	if u.Raw != "myfoo.com" {
		t.Errorf("[TestUri] Error parsing URI \"sip:myfoo.com\".  Should have received \"myfoo.com\" as Raw.  Received: %s", u.Raw)
	}
	if u.User != "" {
		t.Errorf("[TestUri] Error parsing URI \"sip:myfoo.com\".  User should be \"\" but received: %s", u.User)
	}
	if u.Host != "myfoo.com" {
		t.Errorf("[TestUri] Error parsing URI \"sip:myfoo.com\".  Host should be \"myfoo.com\" but received: %s", u.Host)
	}
}
