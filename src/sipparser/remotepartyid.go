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

type parseRpidStateFn func(r *RemotePartyId) parseRpidStateFn

type RemotePartyId struct {
	Error   error
	Val     string
	Name    string
	URI     *URI
	Party   string
	Screen  string
	Privacy string
	Params  []*Param
}

func (r *RemotePartyId) addParam(s string) {
	p := getParam(s)
	switch {
	case p.Param == "screen":
		r.Screen = p.Val
	case p.Param == "party":
		r.Party = p.Val
	case p.Param == "privacy":
		r.Privacy = p.Val
	default:
		switch {
		case r.Params == nil:
			r.Params = []*Param{p}
		default:
			r.Params = append(r.Params, p)
		}
	}
}

func (r *RemotePartyId) parse() {
	for state := parseRpid; state != nil; {
		state = state(r)
	}
}

func parseRpid(r *RemotePartyId) parseRpidStateFn {
	if r.Error != nil {
		return nil
	}
	r.Name, _ = getName(r.Val)
	return parseRpidGetUri
}

func parseRpidGetUri(r *RemotePartyId) parseRpidStateFn {
	left := 0
	right := 0
	for i := range r.Val {
		if r.Val[i] == '<' && left == 0 {
			left = i
		}
		if r.Val[i] == '>' && right == 0 {
			right = i
		}
	}
	if left < right {
		r.URI = ParseURI(r.Val[left+1 : right])
		if r.URI.Error != nil {
			r.Error = fmt.Errorf("parseRpidGetUri err: received err getting uri: %v", r.URI.Error)
			return nil
		}
		return parseRpidGetParams
	}
	r.Error = errors.New("parseRpidGetUri err: could not locate bracks in uri")
	return nil
}

func parseRpidGetParams(r *RemotePartyId) parseRpidStateFn {
	var pos []int
	right := 0
	for i := range r.Val {
		if r.Val[i] == '>' && right == 0 {
			right = i
		}
	}
	if len(r.Val) > right+1 {
		pos = make([]int, 0)
		for i := range r.Val[right+1:] {
			if r.Val[right+1:][i] == ';' {
				pos = append(pos, i)
			}
		}
	}
	if pos == nil {
		return nil
	}
	for i := range pos {
		if len(pos)-1 == i {
			if len(r.Val[right+1:])-1 > pos[i]+1 {
				r.addParam(r.Val[right+1:][pos[i]+1:])
			}
		}
		if len(pos)-1 > i {
			r.addParam(r.Val[right+1:][pos[i]+1 : pos[i+1]])
		}
	}
	return nil
}
