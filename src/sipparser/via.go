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

type viaStateFn func(v *Via) viaStateFn

type vias struct {
	via  string
	vias []*Via
	err  error
}

func (vs *vias) parse() {
	parts := strings.Split(vs.via, ",")
	for _, p := range parts {
		v := &Via{Via: p}
		v.parse()
		if v.Error != nil {
			vs.err = v.Error
			return
		}
		vs.vias = append(vs.vias, v)
	}
}

type Via struct {
	State      string
	Error      error
	Via        string
	Proto      string
	Version    string
	Transport  string
	SentBy     string
	Branch     string
	Received   string
	RPort      string
	Params     []*Param
	protoEnd   int
	paramStart int
}

func (v *Via) parse() {
	for state := parseViaState; state != nil; {
		state = state(v)
	}
}

func (v *Via) addParam(s string) {
	if v.Params == nil {
		v.Params = make([]*Param, 0)
	}
	p := getParam(s)
	switch {
	case p.Param == "branch":
		v.Branch = p.Val
	case p.Param == "rport":
		v.RPort = p.Val
	case p.Param == "received":
		v.Received = p.Val
	default:
		v.Params = append(v.Params, p)
	}
}

func (v *Via) AddReceived(s string) {
	v.Received = s
}

func parseViaState(v *Via) viaStateFn {
	if v.Error != nil {
		return nil
	}
	return parseViaGetProto
}

func parseViaGetProto(v *Via) viaStateFn {
	v.protoEnd = strings.Index(v.Via, " ")
	if v.protoEnd == -1 {
		v.Error = errors.New("parseViaGetProto err: could not get LWS char")
		return nil
	}
	protoParts := strings.SplitN(v.Via[0:v.protoEnd], "/", 3)
	if len(protoParts) != 3 {
		v.Error = errors.New("parseViaGetProto err: split err on proto char")
		return nil
	}
	v.Proto = protoParts[0]
	v.Version = protoParts[1]
	v.Transport = protoParts[2]
	return parseViaGetParams
}

func parseViaGetParams(v *Via) viaStateFn {
	pChar := strings.IndexRune(v.Via, ';')
	if pChar == -1 {
		return parseViaGetHostPort
	}
	if v.paramStart == 0 {
		v.paramStart = pChar
	}
	if len(v.Via)-1 > pChar+1 {
		parts := strings.Split(v.Via[pChar+1:], ";")
		if len(parts) == 1 {
			v.addParam(parts[0])
			return parseViaGetHostPort
		}
		for i := 0; i < len(parts); i++ {
			v.addParam(parts[i])
		}
	}
	return parseViaGetHostPort
}

func parseViaGetHostPort(v *Via) viaStateFn {
	if v.protoEnd == 0 {
		v.Error = errors.New("parseViaGetHostPort err: protoEnd is 0")
		return nil
	}
	if v.protoEnd < v.paramStart {
		v.SentBy = v.Via[v.protoEnd+1 : v.paramStart]
	}
	return nil
}
