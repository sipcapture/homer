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

func TestDiv(t *testing.T) {
	sm := &SipMsg{}
	s := "\"Unknown\" <sip:+5558887777@0.0.0.0>;reason=unconditional;privacy=off;counter=1"
	sm.parseRemotePartyId(s)
	if sm.Error != nil {
		t.Errorf("[TestDiv] Error parsing div hdr: \"Unknown\" <sip:+5558887777@0.0.0.0>;reason=unconditional;privacy=off;counter=1.  Received err: %v", sm.Error)
	}
	if sm.RemotePartyId.Name != "Unknown" {
		t.Error("[TestDiv] Error parsing div hdr: \"Unknown\" <sip:+5558887777@0.0.0.0>;reason=unconditional;privacy=off;counter=1.  Name should be \"Unknown\".")
	}
	if sm.RemotePartyId.Privacy != "off" {
		t.Error("[TestDiv] Error parsing div hdr: \"Unknown\" <sip:+5558887777@0.0.0.0>;reason=unconditional;privacy=off;counter=1.  Privacy should be \"off\".")
	}
}
