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
import (
	"errors"
	"strings"
)

type Authorization struct {
	Val         string
	Credentials string
	Username    string
	//Params      []*Param
}

func (a *Authorization) parse() error {
	pos := strings.IndexRune(a.Val, ' ')
	if pos == -1 {
		return errors.New("Authorization.parse err: no LWS found")
	}
	a.Credentials = a.Val[0:pos]
	if len(a.Val)-1 <= pos {
		return errors.New("Authorization.parse err: no digest-resp found")
	}

	params := strings.Split(a.Val[pos+1:], ",")
	for _, param := range params {
		param = strings.TrimSpace(param)
		if strings.HasPrefix(param, "username=\"") {
			a.Username = strings.Trim(param[len("username=\""):], "\"")
			break
		}
	}

	return nil
}
