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

// ContentDisposition is a struct that holds a parsed
// content-disposition hdr:
// -- Val is the raw value
// -- DispType is the display type
// -- Params is slice of parameters
type ContentDisposition struct {
	Val      string
	DispType string
	Params   []*Param
}

func (c *ContentDisposition) addParam(s string) {
	if s == "" {
		return
	}
	if c.Params == nil {
		c.Params = []*Param{getParam(s)}
		return
	}
	c.Params = append(c.Params, getParam(s))
}

func (c *ContentDisposition) parse() {
	charPos := strings.IndexRune(c.Val, ';')
	if charPos == -1 {
		c.DispType = c.Val
		return
	}
	c.DispType = c.Val[0:charPos]
	if len(c.Val)-1 > charPos {
		params := strings.Split(c.Val[charPos+1:], ";")
		for i := range params {
			c.addParam(params[i])
		}
	}
	return
}
