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
	"strings"
)

// Reason is a struct that holds a parsed reason hdr
// Fields are as follows:
// -- Val is the raw value
// -- Proto is the protocol (i.e. SIP)
// -- Cause is the cause code (i.e. 41)
// -- Text is the actual text response
type Reason struct {
	Val   string
	Proto string
	Cause string
	Text  string
}

// addParam is a method for the Reason type that looks at the
// parameter passed into it
func (r *Reason) addParam(s string) {
	np := getParam(s)
	if np.Param == "cause" {
		r.Cause = np.Val
	}
	if np.Param == "text" {
		r.Text = np.Val
	}
}

// parse is the method that actual parses the .Val of the Reason type
func (r *Reason) parse() {
	pos := make([]int, 0)
	for i := range r.Val {
		if r.Val[i] == ';' {
			pos = append(pos, i)
		}
	}
	if len(pos) == 0 {
		return
	}
	if len(pos) == 1 {
		r.Proto = cleanWs(r.Val[0:pos[0]])
		if len(r.Val)-1 > pos[0] {
			r.addParam(strings.Replace(r.Val[pos[0]+1:], "\"", "", -1))
		}
		return
	}
	if len(pos) > 1 {
		r.Proto = cleanWs(r.Val[0:pos[0]])
		for i := range pos {
			if len(pos)-1 == i {
				if len(r.Val)-1 > pos[i] {
					r.addParam(strings.Replace(r.Val[pos[i]+1:], "\"", "", -1))
				}
				return
			}
			r.addParam(strings.Replace(r.Val[pos[i]+1:pos[i+1]], "\"", "", -1))
		}
		return
	}
	return
}
