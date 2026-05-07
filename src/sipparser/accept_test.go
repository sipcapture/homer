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

/* // TestAccept tests the accept header and parsing functions
func TestAccept(t *testing.T) {
	sm := &SipMsg{}
	s := "application/sdp"
	sm.parseAccept(s)
	if sm.Accept.Val != "application/sdp" {
		t.Errorf("[TestAccept] Error parsing accept hdr: application/sdp.  sm.Accept.Val should be application/sdp but received: " + sm.Accept.Val)
	}
	if len(sm.Accept.Params) != 1 {
		t.Errorf("[TestAccept] Error parsing accept hdr: application/sdp.  sm.Accept.Params should have length of 1.")
	}
	if sm.Accept.Params[0].Type != "application" {
		t.Errorf("[TestAccept] Error parsing accept hdr: application/sdp.  sm.Accept.Params[0].Type should be \"application\".  Received: %q", sm.Accept.Params[0].Type)
	}
	if sm.Accept.Params[0].Val != "sdp" {
		t.Errorf("[TestAccept] Error parsing accept hdr: application/sdp.  sm.Accept.Params[0].Val should be \"sdp\" but received: %q", sm.Accept.Params[0].Val)
	}
}
*/
