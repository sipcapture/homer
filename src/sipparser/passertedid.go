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
	"fmt"
)

// pAssertedIdStateFn is just a fn type
type pAssertedIdStateFn func(p *PAssertedId) pAssertedIdStateFn

// PAssertedId is a struct that holds:
// -- Error is just an os.Error
// -- Val is the raw value
// -- Name is the name value from the p-asserted-id hdr
// -- URI is the parsed uri from the p-asserted-id hdr
// -- Params is a slice of the *Params from the p-asserted-id hdr
type PAssertedId struct {
	Error   error
	Val     string
	Name    string
	URI     *URI
	Params  []*Param
	nameInt int
}

// addParam adds the *Param to the Params field
func (p *PAssertedId) addParam(s string) {
	np := getParam(s)
	if p.Params == nil {
		p.Params = []*Param{np}
		return
	}
	p.Params = append(p.Params, np)
}

// parse actually parsed the .Val field of the PAssertedId struct
func (p *PAssertedId) parse() {
	for state := parsePAssertedId; state != nil; {
		state = state(p)
	}
}

func parsePAssertedId(p *PAssertedId) pAssertedIdStateFn {
	if p.Error != nil {
		return nil
	}
	p.Name, p.nameInt = getName(p.Val)
	return parsePAssertedIdGetUri
}

func parsePAssertedIdGetUri(p *PAssertedId) pAssertedIdStateFn {
	left := 0
	right := 0
	for i := range p.Val {
		if p.Val[i] == '<' && left == 0 {
			left = i
		}
		if p.Val[i] == '>' && right == 0 {
			right = i
		}
	}
	if left < right {
		p.URI = ParseURI(p.Val[left+1 : right])
		if p.URI.Error != nil {
			p.Error = fmt.Errorf("parsePAssertedIdGetUri err: received err getting uri: %v", p.URI.Error)
			return nil
		}
		return parsePAssertedIdGetParams
	}
	p.Error = errors.New("parsePAssertedIdGetUri err: could not locate bracks in uri")
	return nil
}

func parsePAssertedIdGetParams(p *PAssertedId) pAssertedIdStateFn {
	var pos []int
	right := 0
	for i := range p.Val {
		if p.Val[i] == '>' && right == 0 {
			right = i
		}
	}
	if len(p.Val) > right+1 {
		pos = make([]int, 0)
		for i := range p.Val[right+1:] {
			if p.Val[right+1:][i] == ';' {
				pos = append(pos, i)
			}
		}
	}
	if pos == nil {
		return nil
	}
	for i := range pos {
		if len(pos)-1 == i {
			if len(p.Val[right+1:])-1 > pos[i]+1 {
				p.addParam(p.Val[right+1:][pos[i]+1:])
			}
		}
		if len(pos)-1 > i {
			p.addParam(p.Val[right+1:][pos[i]+1 : pos[i+1]])
		}
	}
	return nil
}
