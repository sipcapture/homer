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
	"errors"
)

// Rack is a struct that holds the parsed rack hdr
// The fields are as follows:
// -- Val is the raw value
// -- RseqVal is the rseq value from the rack hdr
// -- CseqVal is the cseq value from the rack hdr
// -- CseqMethod is the method from the cseq hdr
type Rack struct {
	Val        string
	RseqVal    string
	CseqVal    string
	CseqMethod string
}

// parse parses the .Val of the Rack struct
func (r *Rack) parse() error {
	r.Val = cleanWs(r.Val)
	pos := make([]int, 0)
	for i := range r.Val {
		if r.Val[i] == ' ' {
			pos = append(pos, i)
		}
	}
	if len(pos) != 2 {
		return errors.New("Rack.parse err: could not locate two LWS")
	}
	r.RseqVal = r.Val[0:pos[0]]
	r.CseqVal = r.Val[pos[0]+1 : pos[1]]
	if len(r.Val)-1 > pos[1] {
		r.CseqMethod = r.Val[pos[1]+1:]
		return nil
	}
	return errors.New("Rack.parse err: value of rack ends in LWS")
}
