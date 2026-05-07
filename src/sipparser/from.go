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
	"fmt"
	"strings"
)

type parseFromStateFn func(f *From) parseFromStateFn

// From holds a parsed header that has a format like:
// "NAME" <sip:user@hostinfo>;param=val
// and is used for the parsing of the from, to, and
// contact header in the parser program
// From holds the following public fields:
// -- Error is an error
// -- Val is the raw value
// -- Name is the name value
// -- Tag is the tag value
// -- URI is a parsed uri
// -- Params are for any generic params that are part of
//
//	the header
type From struct {
	Error error
	Val   string
	Name  string
	Tag   string
	URI   *URI
	//Params     []*Param
	endName    int
	rightBrack int
	leftBrack  int
	brackChk   bool
}

func (f *From) parse() {
	for state := parseFromState; state != nil; {
		state = state(f)
	}
}

/*
func (f *From) addParam(s string) {
	p := getParam(s)
	switch {
	case p.Param == "tag":
		f.Tag = p.Val
	default:
		if f.Params == nil {
			f.Params = make([]*Param, 0)
		}
		f.Params = append(f.Params, p)
	}
}
*/

func parseFromState(f *From) parseFromStateFn {
	if f.Error != nil {
		return nil
	}
	f.Name, f.endName = getName(f.Val)
	return parseFromGetURI
}

func parseFromGetURI(f *From) parseFromStateFn {
	f.leftBrack, f.rightBrack, f.brackChk = getBracks(f.Val)
	if f.brackChk == false {
		f.URI = ParseURI(f.Val)
		if f.URI.Error != nil {
			f.Error = fmt.Errorf("parseFromGetURI err: rcvd err parsing uri: %v", f.URI.Error)
			return nil
		}
		/*
			if f.URI.UriParams != nil {
				for i := range f.URI.UriParams {
					if f.URI.UriParams[i].Param == "tag" {
						f.Tag = f.URI.UriParams[i].Val
					}
				}
			}
		*/
		return nil
	}
	if f.brackChk == true {
		f.URI = ParseURI(f.Val[f.leftBrack+1 : f.rightBrack])
		if f.URI.Error != nil {
			f.Error = fmt.Errorf("parseFromGetURI err: rcvd err parsing uri: %v", f.URI.Error)
			return nil
		}
		return parseFromGetParams
	}
	return nil
}

func parseFromGetParams(f *From) parseFromStateFn {

	return nil
}

func getFrom(s string) *From {
	f := &From{Val: s}

	if strings.Contains(s, "tag=") {
		parts := strings.Split(s, "tag=")
		if len(parts) > 1 {
			f.Tag = strings.Split(parts[1], ";")[0]
		}
	}

	f.parse()
	return f
}
