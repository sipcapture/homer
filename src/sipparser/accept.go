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

// AcceptParam is just a key:value pair of params for the accept
// header
type AcceptParam struct {
	Type string
	Val  string
}

// Accept is a struct that holds the following:
// -- the raw value
// -- a slice of parced AcceptParam
type Accept struct {
	Val    string
	Params []*AcceptParam
}

// addParam is called when you want to add a parameter to the accept
// struct
func (a *Accept) addParam(s string) {
	for i := range s {
		if s[i] == '/' {
			if len(s)-1 > i {
				if a.Params == nil {
					a.Params = []*AcceptParam{&AcceptParam{Type: cleanWs(s[0:i]), Val: cleanWs(s[i+1:])}}
					return
				}
				a.Params = append(a.Params, &AcceptParam{Type: cleanWs(s[0:i]), Val: cleanWs(s[i+1:])})
				return
			}
			return
		}
	}
}

// parse just gets a comma seperated list of the parameters from
// the .Val and calls addparam on each of the parameters
func (a *Accept) parse() {
	cs := getCommaSeperated(a.Val)
	if cs == nil {
		a.addParam(a.Val)
		return
	}
	for i := range cs {
		a.addParam(cs[i])
	}
}
