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

func TestStartLine(t *testing.T) {
	str := "SIP/2.0 487 Request Cancelled"
	s := &StartLine{Val: str}
	s.run()
	if s.Type != SIP_RESPONSE {
		t.Error("[TestStartLine] Error parsing startline: SIP/2.0 487 Request Cancelled.  s.Type should be \"RESPONSE\".")
	}
	if s.Resp != "487" {
		t.Error("[TestStartLine] Error parsing startline: SIP/2.0 487 Request Cancelled.  s.Resp should be \"487\".")
	}
	if s.RespText != "Request Cancelled" {
		t.Error("[TestStartLine] Error parsing startline: SIP/2.0 487 Request Cancelled.  s.RespText should be \"Request Cancelled\".")
	}
	str = "1412@34922@336312786@1.2.3.4:5061;transport=tcp;user=phone@home1.2.3.4                                            111111111"
	s = ParseStartLine(str)
	if s.Error == nil {
		t.Error("[TestStartLine] Error parsing startline.  s.Error should not be nil.")
	}
	str = "dlskmgkfmdg ldf,l,"
	s = ParseStartLine(str)
	if s.Error == nil {
		t.Error("[TestStartLine] Error parsing startline.  s.Error should not be nil for string: \"dlskmgkfmdg ldf,l,\".")
	}
	str = "INVITE sip:+15554440000@0.0.0.0;user=phone SIP/2.0"
	s = ParseStartLine(str)
	if s.Error != nil {
		t.Errorf("[TestStartLine] Got error when parsing startline: \"INVITE sip:+15554440000@0.0.0.0;user=phone SIP/2.0\".  Received err: %v", s.Error)
	}
	if s.Type != SIP_REQUEST {
		t.Error("[TestStartLine] Got error when parsing startline: \"INVITE sip:+15554440000@0.0.0.0;user=phone SIP/2.0\".  s.Type should be \"Request\".")
	}
	if s.Method != SIP_METHOD_INVITE {
		t.Error("[TestStartLine] Got error when parsing startline: \"INVITE sip:+15554440000@0.0.0.0;user=phone SIP/2.0\".  s.Method should be \"INVITE\".")
	}
	if s.Proto != "SIP" {
		t.Errorf("[TestStartLine] Got error when startline: \"INVITE sip:+15554440000@0.0.0.0;user=phone SIP/2.0\".  s.Proto should be \"SIP\".  Received: \"%s\"", s.Proto)
	}
	if s.Version != "2.0" {
		t.Errorf("[TestStartLine] Got error when parsing startline: \"INVITE sip:+15554440000@0.0.0.0;user=phone SIP/2.0\".  s.Version should be \"2.0\". Received: \"%s\"", s.Version)
	}
	// throwing this in to make sure we don't toss an index error
	str = "INVITE foo@bar.com SIP/"
	s = ParseStartLine(str)
	if s.Error == nil {
		t.Error("[TestStartLine] Should have a no version err when parsing request line: \"INVITE foo@bar.com SIP/\".")
	}
}
