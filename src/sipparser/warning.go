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
	"strconv"
	"strings"
)

type Warning struct {
	Val     string
	Code    string
	CodeInt int
	Agent   string
	Text    string
}

func (w *Warning) parse() error {
	parts := strings.SplitN(w.Val, " ", 3)
	if got, want := len(parts), 3; got != want {
		return fmt.Errorf("Warning.parse err: split on LWS returned %d fields, want %d", got, want)
	}
	c, err := strconv.Atoi(parts[0])
	if err != nil || c < 0 || c > 999 {
		return fmt.Errorf("Warning.parse err: got code %q, want 3-digit code", parts[0])
	}
	w.Code = parts[0]
	w.CodeInt = c
	w.Agent = parts[1]
	w.Text = strings.Replace(parts[2], "\"", "", -1)
	return nil
}
