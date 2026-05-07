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

func TestPAssertedId(t *testing.T) {
	s := "\"VoIP Call\"<sip:8885551000@0.0.0.0>"
	p := &PAssertedId{Val: s}
	p.parse()
	if p.Error != nil {
		t.Errorf("[TestPAssertedId] Error parsing p-asserted-id hdr: \"VoIP Call\"<sip:8885551000@0.0.0.0>.  Received err: %v", p.Error)
	}
	if p.Name != "VoIP Call" {
		t.Errorf("[TestPAssertedId] Error parsing p-assertd-id hdr: \"VoIP Call\"<sip:8885551000@0.0.0.0>. Name should be \"VoIP Call\" but received: \"%s\"", p.Name)
	}
	if p.URI == nil {
		t.Error("[TestPAssertedId] Error parsing p-asserted-id hdr: \"VoIP Call\"<sip:8885551000@0.0.0.0>.  No URI in parsed hdr.")
	}
	if p.Params != nil {
		t.Error("[TestPAssertedId] Error parsing p-asserted-id hdr: \"VoIP Call\"<sip:8885551000@0.0.0.0>.  p.Params should be nil.")
	}
	s = "bad header"
	p = &PAssertedId{Val: s}
	p.parse()
	if p.Error == nil {
		t.Error("[TestPAssertedId] Should have received an err when parsing bad hdr: \"bad header\".")
	}
	s = "<sip:4.71.122.181:5060;user=phone>"
	p = &PAssertedId{Val: s}
	p.parse()
	if p.URI == nil {
		t.Error("[TestPAssertedId] No URI for parsing p-asserted-id hdr: <sip:4.71.122.181:5060;user=phone>")
	}
}
