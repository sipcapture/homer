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

func TestRpid(t *testing.T) {
	sm := &SipMsg{}
	s := "\"Unknown\" <sip:5558887777@0.0.0.0>;party=calling;screen=yes;privacy=off"
	sm.parseRemotePartyId(s)
	if sm.Error != nil {
		t.Errorf("[TestRpid] Error parsing rpid hdr: \"Unknown\" <sip:5558887777@0.0.0.0>;party=calling;screen=yes;privacy=off.  Received err: %v", sm.Error)
	}
	if sm.RemotePartyId.Name != "Unknown" {
		t.Error("[TestRpid] Error parsing rpid hdr: \"Unknown\" <sip:5558887777@0.0.0.0>;party=calling;screen=yes;privacy=off.  Name should be \"Unknown\".")
	}
	if sm.RemotePartyId.URI == nil {
		t.Error("[TestRpid] Error parsing rpid hdr: \"Unknown\" <sip:5558887777@0.0.0.0>;party=calling;screen=yes;privacy=off.  sm.RemotePartyId.URI is nil.")
	}
	if sm.RemotePartyId.Privacy != "off" {
		t.Error("[TestRpid] Error parsing rpid hdr: \"Unknown\" <sip:5558887777@0.0.0.0>;party=calling;screen=yes;privacy=off.  Privacy should be \"off\".")
	}
}
