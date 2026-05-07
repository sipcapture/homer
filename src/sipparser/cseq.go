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
	"strings"
)

// Cseq is a struct that holds the values for a cseq header:
//
//	-- Val is the raw string value of the cseq hdr
//	-- Method is the SIP method
//	-- Digit is the numeric indicator for the method
type Cseq struct {
	Val    string
	Method string
	Digit  string
}

func (c *Cseq) parse() error {
	if len(c.Val) < 3 {
		return errors.New("Cseq.parse err: length of CSeq is < 3")
	}
	s := strings.IndexRune(c.Val, ' ')
	if s == -1 {
		return errors.New("Cseq.parse err: lws err with: " + c.Val)
	}
	if s == 0 {
		return errors.New("Cseq.parse err: lws at pos 0 in val: " + c.Val)
	}
	if len(c.Val)-1 < s+1 {
		return errors.New("Cseq.parse err: first lws is end of line in val: " + c.Val)
	}
	c.Digit = c.Val[0:s]
	if c.Val[s+1] != ' ' {
		c.Method = c.Val[s+1:]
	} else {
		c.Method = cleanWs(c.Val[s+1:])
	}
	return nil
}
