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

// Imports from go standard library

// Param is just a struct that holds a parameter and a value
// As an example of this would be something like user=phone
type Param struct {
	Param string
	Val   string
}

// getParam is just a convenience function to pass a string
// and get a *Param
func getParam(s string) *Param {
	p := new(Param)
	for i := range s {
		if s[i] == '=' {
			p.Param = cleanWs(s[0:i])
			if i+1 < len(s) {
				p.Val = cleanWs(s[i+1:])
				return p
			}
			return p
		}
	}
	p.Param = cleanWs(s)
	return p
}
