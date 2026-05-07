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

func TestCseq(t *testing.T) {
	sm := &SipMsg{}
	sm.parseCseq("100 INVITE")
	if sm.Error != nil {
		t.Errorf("[TestCseq] Error parsing cseq: \"100 INVITE\". Received err: %v", sm.Error)
	}
	if sm.Cseq.Digit != "100" {
		t.Errorf("[TestCseq] Error parsing cseq: \"100 INVITE\".  Digit should be 100.")
	}
	if sm.Cseq.Method != "INVITE" {
		t.Errorf("[TestCseq] Error parsing cseq: \"100 INVITE\".  Method should be \"INVITE\".")
	}
	sm.parseCseq("1112423100   REGISTER   ")
	if sm.Error != nil {
		t.Errorf("[TestCseq] Error parsing cseq: \"1112423100   REGISTER   \". Received err: %v", sm.Error)
	}
	if sm.Cseq.Digit != "1112423100" {
		t.Errorf("[TestCseq] Error parsing cseq: \"1112423100   REGISTER   \".  Digit should be 1112423100.")
	}
	if sm.Cseq.Method != "REGISTER" {
		t.Errorf("[TestCseq] Error parsing cseq: \"1112423100   REGISTER   \".  Method should be \"REGISTER\".")
	}
}
