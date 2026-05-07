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

import (
	"testing"
)

func TestWarning(t *testing.T) {
	s := "301 isi.edu \"Incompatible network address type 'E.164'\""
	w := &Warning{Val: s}
	err := w.parse()
	if err != nil {
		t.Errorf("[TestWarning] Error parsing warning.  Got err: %v", err)
	}
	if w.Code != "301" {
		t.Errorf("[TestWarning] Error parsing warning.  Code is not \"301\".  Rcvd: \"%s\"", w.Code)
	}
	if w.CodeInt != 301 {
		t.Errorf("[TestWarning] Error parsing warning. CodeInt is not 301. Rcvd: %d", w.CodeInt)
	}
	if w.Agent != "isi.edu" {
		t.Errorf("[TestWarning] Error parsing warning.  Agent is not \"isi.edu\". Rcvd: \"%s\"", w.Agent)
	}
	if w.Text != "Incompatible network address type 'E.164'" {
		t.Errorf("[TestWarning] Error parsing warning.  Text should be \"Incompatible network address type 'E.164'\".  Rcvd: \"%s\"", w.Text)
	}
}
