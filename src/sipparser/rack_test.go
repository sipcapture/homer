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

/* func TestRack(t *testing.T) {
	sm := &SipMsg{}
	s := "776656 1 INVITE"
	sm.parseRack(s)
	if sm.Error != nil {
		t.Errorf("[TestRack] Error parsing rack hdr: 776656 1 INVITE.  Received err: %v", sm.Error)
	}
	if sm.Rack.RseqVal != "776656" {
		t.Errorf("[TestRack] Error parsing rack hdr: 776656 1 INVITE.  RseqVal should be 776656 but received: %v", sm.Rack.RseqVal)
	}
	if sm.Rack.CseqVal != "1" {
		t.Errorf("[TestRack] Error parsing rack hdr: 776656 1 INVITE.  CseqVal should be 1 but received: %v", sm.Rack.CseqVal)
	}
	if sm.Rack.CseqMethod != "INVITE" {
		t.Errorf("[TestRack] Error parsing rack hdr: 776656 1 INVITE.  CseqMethod should be \"INVITE\" but received: \"%s\"", sm.Rack.CseqMethod)
	}
}
*/
