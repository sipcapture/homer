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

/* func TestContentDisposition(t *testing.T) {
	sm := &SipMsg{}
	s := "session; handling=required"
	sm.parseContentDisposition(s)
	if sm.ContentDisposition.Val != "session; handling=required" {
		t.Errorf("[TestContentDisposition] Error parsing content-disposition hdr: session; handling=required.  sm.ContentDisposition.Val should be \"session; handling=required\" but received: \"%s\"", sm.ContentDisposition.Val)
	}
	if sm.ContentDisposition.DispType != "session" {
		t.Errorf("[TestContentDisposition] Error parsing content-disposition hdr: session; handling=required.  sm.ContentDisposition.DispType should be \"session\" but received: \"%s\"", sm.ContentDisposition.DispType)
	}
	if len(sm.ContentDisposition.Params) != 1 {
		t.Errorf("[TestContentDisposition] Error parsing content-disposition hdr: session; handling=required.  Length of sm.ContentDisposition.Params should be 1 but instead is: %d", len(sm.ContentDisposition.Params))
	}
	if sm.ContentDisposition.Params[0].Param != "handling" {
		t.Errorf("[TestContentDisposition] Error parsing content-disposition hdr: session; handling=required.  sm.ContentDisposition.Params[0].Param should be \"handling\" but received: \"%s\"", sm.ContentDisposition.Params[0].Param)
	}
	if sm.ContentDisposition.Params[0].Val != "required" {
		t.Errorf("[TestContentDisposition] Error parsing content-disposition hdr: session; handling=required.  sm.ContentDisposition.Params[0].Val should be \"required\" but received: \"%s\"", sm.ContentDisposition.Params[0].Val)
	}
}
*/
