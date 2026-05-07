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

/* func TestReason(t *testing.T) {
	sm := &SipMsg{}
	s := "Q.850;cause=16;text=\"NORMAL_CLEARING\""
	sm.parseReason(s)
	if sm.Reason.Proto != "Q.850" {
		t.Errorf("[TestReason] Error parsing reason hdr: Q.850;cause=16;text=\"NORMAL_CLEARING\".  Proto should be \"Q.850\" but received: " + sm.Reason.Proto)
	}
	if sm.Reason.Cause != "16" {
		t.Errorf("[TestReason] Error parsing reason hdr: Q.850;cause=16;text=\"NORMAL_CLEARING\".  Cause should be \"16\" but received: " + sm.Reason.Cause)
	}
	if sm.Reason.Text != "NORMAL_CLEARING" {
		t.Errorf("[TestReason] Error parsing reason hdr: Q.850;cause=16;text=\"NORMAL_CLEARING\". Text should be \"NORMAL_CLEARING\" but received: " + sm.Reason.Text)
	}
	s = "Q.850;cause=102"
	sm.parseReason(s)
	if sm.Reason.Proto != "Q.850" {
		t.Errorf("[TestReason] Error parsing reason hdr: Q.850;cause=102.  Proto should be \"Q.850\" but received: " + sm.Reason.Proto)
	}
	if sm.Reason.Cause != "102" {
		t.Errorf("[TestReason] Error parsing reason hdr: Q.850;cause=102.  Cause should be \"102\" but received: " + sm.Reason.Cause)
	}
}
*/
