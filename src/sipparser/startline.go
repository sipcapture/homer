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
	"fmt"
	"strings"
)

/*
var (
	SIP_METHODS = []string{
		SIP_METHOD_INVITE,
		SIP_METHOD_ACK,
		SIP_METHOD_OPTIONS,
		SIP_METHOD_BYE,
		SIP_METHOD_CANCEL,
		SIP_METHOD_REGISTER,
		SIP_METHOD_INFO,
		SIP_METHOD_PRACK,
		SIP_METHOD_SUBSCRIBE,
		SIP_METHOD_NOTIFY,
		SIP_METHOD_UPDATE,
		SIP_METHOD_MESSAGE,
		SIP_METHOD_REFER,
		SIP_METHOD_PUBLISH,
	}
)
*/

type parseStartLineStateFn func(s *StartLine) parseStartLineStateFn

type StartLine struct {
	Error    error  //internal error condition
	Val      string //StartLine as a String
	Type     string //one of SIP_REQUEST or SIP_RESPONSE
	Method   string //INV, ACK, BYE etc
	URI      *URI   //Request URI; sip:alice@chicago.com
	Resp     string //Response code: e.g. 200, 400
	RespText string //Response String e.g. "Trying"
	Proto    string //e.g. "SIP" in the string "SIP/2.0"
	Version  string //e.g. "2.0" in the string "SIP/2.0"
}

func (s *StartLine) run() {
	for state := parseStartLine; state != nil; {
		state = state(s)
	}
}

func parseStartLine(s *StartLine) parseStartLineStateFn {
	if s.Error != nil {
		return nil
	}
	if len(s.Val) < 3 {
		s.Error = errors.New("parseStartLine err: length of s.Val is less than 3. Invalid start line")
		return nil
	}
	if s.Val[0:3] == "SIP" {
		s.Type = SIP_RESPONSE
		return parseStartLineResponse
	}
	s.Type = SIP_REQUEST
	return parseStartLineRequest
}

func parseStartLineResponse(s *StartLine) parseStartLineStateFn {
	parts := strings.SplitN(s.Val, " ", 3)
	if len(parts) != 3 {
		s.Error = errors.New("parseStartLineRespone err: err getting parts from LWS")
		return nil
	}
	charPos := strings.IndexRune(parts[0], '/')
	if charPos == -1 {
		s.Error = errors.New("parseStartLineRespone err: err getting proto char")
		return nil
	}
	s.Proto = parts[0][0:charPos]
	if len(parts[0])-1 < charPos+1 {
		s.Error = errors.New("parseStartLineResponse err: proto char appears to be at end of proto")
		return nil
	}
	s.Version = parts[0][charPos+1:]
	s.Resp = parts[1]
	s.RespText = parts[2]
	return nil
}

func parseStartLineRequest(s *StartLine) parseStartLineStateFn {
	parts := strings.SplitN(s.Val, " ", 3)
	if len(parts) != 3 {
		s.Error = errors.New("parseStartLineRequest err: request line did not split on LWS correctly")
		return nil
	} else if len(parts[1]) == 0 {
		s.Error = errors.New("parseStartLineRequest err: empty request uri part")
		return nil
	}
	s.Method = parts[0]
	s.URI = ParseURI(parts[1])
	if s.URI.Error != nil {
		s.Error = fmt.Errorf("parseStartLineRequest err: err in URI: %v", s.URI.Error)
		return nil
	}
	charPos := strings.IndexRune(parts[2], '/')
	if charPos == -1 {
		s.Error = errors.New("parseStartLineRequest err: could not get \"/\" pos in parts[2]")
		return nil
	}
	if len(parts[2])-1 < charPos+1 {
		s.Error = errors.New("parseStartLineRequest err: \"/\" char appears to be at end of line")
		return nil
	}
	s.Proto = parts[2][0:charPos]
	s.Version = parts[2][charPos+1:]
	return nil
}

func ParseStartLine(str string) *StartLine {
	s := &StartLine{Val: str}
	s.run()
	return s
}
