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

func TestFrom(t *testing.T) {
	sm := &SipMsg{}
	str := "\"Unknown\" <sip:5554441000@0.0.0.0;user=phone;noa=national>;tag=dd737a8-co7387-INS002"
	sm.parseFrom(str)
	if sm.Error != nil {
		t.Errorf("[TestFrom] Error parsing from hdr: \"Unknown\" <sip:5554441000@0.0.0.0;user=phone;noa=national>;tag=dd737a8-co7387-INS002. Received err: %v", sm.Error)
	}
	if sm.From.Name != "Unknown" {
		t.Errorf("[TestFrom] Error parsing from hdr: \"Unknown\" <sip:5554441000@0.0.0.0;user=phone;noa=national>;tag=dd737a8-co7387-INS002. Name field should be \"Unknown\".")
	}
	if sm.From.URI.User != "5554441000" {
		t.Errorf("[TestFrom] Error parsing from hdr: \"Unknown\" <sip:5554441000@0.0.0.0;user=phone;noa=national>;tag=dd737a8-co7387-INS002. URI.User field should be \"5554441000\".")
	}
	if sm.From.Tag != "dd737a8-co7387-INS002" {
		t.Errorf("[TestFrom] Error parsing from hdr: \"Unknown\" <sip:5554441000@0.0.0.0;user=phone;noa=national>;tag=dd737a8-co7387-INS002. sm.From.Tag should be \"dd737a8-co7387-INS002\".")
	}
	str = "<sip:5554441000@0.0.0.0;user=phone;noa=national>;tag=dd737a8-co7387-INS002"
	sm.parseFrom(str)
	if sm.Error != nil {
		t.Errorf("[TestFrom] Error parsing from hdr: \"<sip:5554441000@0.0.0.0;user=phone;noa=national>;tag=dd737a8-co7387-INS002\". Received err: %v", sm.Error)
	}
	if sm.From.Name != "" {
		t.Errorf("[TestFrom] Error parsing from hdr: \"<sip:5554441000@0.0.0.0;user=phone;noa=national>;tag=dd737a8-co7387-INS002\". Name should be \"\" but received: \"%s\"", sm.From.Name)
	}
	if sm.From.URI.User != "5554441000" {
		t.Errorf("[TestFrom] Error parsing from hdr:  \"<sip:5554441000@0.0.0.0;user=phone;noa=national>;tag=dd737a8-co7387-INS002\". URI.User should be \"5554441000\" but received: \"%s\"", sm.From.URI.User)
	}
	str = "sip:+12125551212@phone2net.com;tag=887s"
	sm.parseFrom(str)
	if sm.Error != nil {
		t.Errorf("[TestFrom] Error parsing from hdr: sip:+12125551212@phone2net.com;tag=887s. Received err: %v", sm.Error)
	}
	if sm.From.Tag != "887s" {
		t.Errorf("[TestFrom] Error parsing from hdr: sip:+12125551212@phone2net.com;tag=887s. Tag should be \"887s\" but received: \"%s\".", sm.From.Tag)
	}
	sm.parseFrom("tel:+4512345678;tag=752520ac91292bae839ce09f3fa382aa")
	if sm.From.URI.User != "+4512345678" {
		t.Errorf("[TestFrom] Error parsing from hdr: tel:+4512345678;tag=752520ac91292bae839ce09f3fa382aa. User should be \"+4512345678\" but received: \"%s\".", sm.From.URI.User)
	}
	sm.parseFrom("<tel:180012345678;user=phone>;tag=sbc09033drebier-CC-3")
	if sm.From.URI.User != "180012345678" {
		t.Errorf("[TestFrom] Error parsing from hdr: <tel:180012345678;user=phone>;tag=sbc09033drebier-CC-3. User should be \"180012345678\" but received: \"%s\".", sm.From.URI.User)
	}
}
