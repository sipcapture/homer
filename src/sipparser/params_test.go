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
	"testing"
)

func TestGetParam(t *testing.T) {
	s := "key=value"
	p := getParam(s)
	if p.Param != "key" {
		t.Errorf("[TestGetParam] Error with getParam parsing \"key=value\".")
	}
	if p.Val != "value" {
		t.Errorf("[TestGetParam] Error with getParam parsing \"key=value\". Bad value.")
	}
	s = "key"
	p = getParam(s)
	if p.Val != "" {
		t.Errorf("[TestGetParam] Error with getParam.  Received value from \"key\".")
	}
}
