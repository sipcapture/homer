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
	"errors"
	"fmt"
	"strings"
)

const (
	sipParseStateStartLine = "SipParseStateStartLine"
	sipParseStateBody      = "SipMsgStateBody"
	sipParseStateHeaders   = "SipMsgStateHeaders"
	CR                     = "\r"
	LF                     = "\n"
	CALLING_PARTY_DEFAULT  = "default"
	CALLING_PARTY_RPID     = "rpid"
	CALLING_PARTY_PAID     = "paid"
)

type CallingPartyInfo struct {
	Name      string
	Number    string
	Anonymous bool
}

type Header struct {
	Header string
	Val    string
}

func (h *Header) String() string {
	return fmt.Sprintf("%s: %s", h.Header, h.Val)
}

type sipParserStateFn func(s *SipMsg) sipParserStateFn

type SipMsg struct {
	State            string
	Error            error
	Msg              string
	CallingParty     *CallingPartyInfo
	Body             string
	Authorization    *Authorization
	AuthVal          string
	AuthUser         string
	ContentLength    string
	ContentType      string
	From             *From
	FromUser         string
	FromHost         string
	FromTag          string
	MaxForwards      string
	Organization     string
	To               *From
	ToUser           string
	ToHost           string
	ToTag            string
	Expires          string
	Contact          *From
	ContactVal       string
	ContactUser      string
	ContactHost      string
	ContactPort      int
	CallID           string
	XCallID          string
	XHeader          []string
	CHeader          []string
	CustomHeader     map[string]string
	Cseq             *Cseq
	CseqMethod       string
	CseqVal          string
	ReasonVal        string
	RTPStatVal       string
	ViaOne           string
	ViaOneBranch     string
	Privacy          string
	RemotePartyIdVal string
	DiversionVal     string
	RemotePartyId    *RemotePartyId
	PAssertedIdVal   string
	PaiUser          string
	PaiHost          string
	PAssertedId      *PAssertedId
	UserAgent        string
	Server           string
	URIHost          string
	URIRaw           string
	URIUser          string
	FirstMethod      string
	FirstResp        string
	FirstRespText    string
	Profile          string
	eof              int
	hdr              string
	hdrv             string
	//Reason           *Reason
	//Via                []*Via
	//StartLine          *StartLine
	//Headers            []*Header
	//Accept             *Accept
	//AlertInfo          string
	//Allow              []string
	//AllowEvents        []string
	//ContentDisposition *ContentDisposition
	//ContentLengthInt   int
	//MaxForwardsInt     int
	//ProxyAuthenticate  *Authorization
	//ProxyRequire       []string
	//Rack               *Rack
	//Rseq               string
	//RseqInt            int
	//RecordRoute        []*URI
	//RTPStat            *RTPStat
	//Route              []*URI
	//Require            []string
	//Unsupported        []string
	//Subject            string
	//Supported          []string
	//Warning            *Warning
	//WWWAuthenticate    *Authorization
}

func (s *SipMsg) run() {
	for state := parseSip; state != nil; {
		state = state(s)
	}
}

func (s *SipMsg) addError(err string) sipParserStateFn {
	s.Error = errors.New(err)
	return nil
}

func (s *SipMsg) addErrorNoReturn(err string) {
	s.Error = errors.New(err)
}

func (s *SipMsg) addHdr(str string) {
	if str == "" || str == " " {
		return
	}
	sp := strings.IndexRune(str, ':')
	if sp == -1 {
		return
	}

	//s.hdr = strings.ToLower(strings.TrimSpace(str[0:sp]))
	s.hdr = cleanWs(str[0:sp])

	if len(str)-1 >= sp+1 {
		s.hdrv = cleanWs(str[sp+1:])
	} else {
		// No header value
		s.hdrv = ""
	}

	if len(s.hdr) == 1 {
		switch {
		case s.hdr == "I" || s.hdr == "i":
			s.CallID = s.hdrv
		case s.hdr == "F" || s.hdr == "f":
			s.parseFrom(s.hdrv)
		case s.hdr == "T" || s.hdr == "t":
			s.parseTo(s.hdrv)
		case s.hdr == "M" || s.hdr == "m":
			s.ContactVal = s.hdrv
			s.parseContact(str)
		case s.hdr == "V" || s.hdr == "v":
			s.parseVia(s.hdrv)
		case s.hdr == "C" || s.hdr == "c":
			s.ContentType = s.hdrv
		case s.hdr == "L" || s.hdr == "l":
			s.ContentLength = s.hdrv
		case s.hdr == "u":
		}
	} else if len(s.hdr) == 2 {
		// To header
		s.parseTo(s.hdrv)
	} else {
		switch {
		case s.hdr == "Via" || s.hdr == "VIA" || s.hdr == "via":
			s.parseVia(s.hdrv)
		case s.hdr == "From" || s.hdr == "FROM" || s.hdr == "from":
			s.parseFrom(s.hdrv)
		case s.hdr == "Call-ID" || s.hdr == "CALL-ID" || s.hdr == "Call-Id" || s.hdr == "Call-id" || s.hdr == "call-id":
			s.CallID = s.hdrv
		case s.hdr == "CSeq" || s.hdr == "CSEQ" || s.hdr == "Cseq" || s.hdr == "cseq":
			s.CseqVal = s.hdrv
			s.parseCseq(s.hdrv)
		case s.hdr == "Contact" || s.hdr == "CONTACT" || s.hdr == "contact":
			s.ContactVal = s.hdrv
			s.parseContact(str)
		case s.hdr == "User-Agent" || s.hdr == "USER-AGENT" || s.hdr == "user-agent":
			s.UserAgent = s.hdrv
		case s.hdr == "Server" || s.hdr == "server":
			s.Server = s.hdrv
		case s.hdr == "Content-Type" || s.hdr == "CONTENT-TYPE" || s.hdr == "content-type":
			s.ContentType = s.hdrv
		case s.hdr == "Content-Length" || s.hdr == "CONTENT-LENGTH" || s.hdr == "content-length":
			s.ContentLength = s.hdrv
		case s.hdr == "Accept" || s.hdr == "Accept-Encoding" || s.hdr == "Accept-Language":
			//s.parseAccept(s.hdrv)
		case s.hdr == "Allow":
			//s.parseAllow(s.hdrv)
		case s.hdr == "Allow‑Events":
			//s.parseAllowEvents(s.hdrv)
		case s.hdr == "Authorization" || s.hdr == "authorization" || s.hdr == "Proxy-Authorization" || s.hdr == "proxy-authorization":
			s.parseAuthorization(s.hdrv)
		case s.hdr == "Content-Disposition":
			//s.parseContentDisposition(s.hdrv)
		case s.hdr == "Route":
			//s.parseRoute(s.hdrv)
		case s.hdr == "Record-Route":
			//s.parseRecordRoute(s.hdrv)
		case s.hdr == "Max-Forwards" || s.hdr == "MAX-FORWARDS" || s.hdr == "max-forwards":
			s.MaxForwards = s.hdrv
		case s.hdr == "Organization" || s.hdr == "organization":
			s.Organization = s.hdrv
		case s.hdr == "P-Asserted-Identity" || s.hdr == "p-asserted-identity":
			s.PAssertedIdVal = s.hdrv
			s.parsePAssertedId(s.hdrv)
		case s.hdr == "Proxy-Authenticate" || s.hdr == "proxy-authenticate":
			//s.parseProxyAuthenticate(s.hdrv)
		case s.hdr == "RAck":
			//s.parseRack(s.hdrv)
		case s.hdr == "Reason" || s.hdr == "reason":
			s.ReasonVal = s.hdrv
			//s.parseReason(s.hdrv)
		case s.hdr == "Remote-Party-Id" || s.hdr == "remote-party-id":
			s.RemotePartyIdVal = s.hdrv
		case s.hdr == "Diversion" || s.hdr == "diversion":
			s.DiversionVal = s.hdrv
		case s.hdr == "Supported":
			//s.parseSupported(s.hdrv)
		case s.hdr == "Unsupported":
			//s.parseUnsupported(s.hdrv)
		case s.hdr == "Warning":
			//s.parseWarning(s.hdrv)
		case s.hdr == "WWW-Authenticate":
			//s.parseWWWAuthenticate(s.hdrv)
		case s.hdr == "Privacy" || s.hdr == "privacy":
			s.Privacy = s.hdrv
		case s.hdr == "X-RTP-Stat":
			s.parseRTPStat(s.hdrv)
		case s.hdr == "Expires":
			s.Expires = s.hdrv
		default:
			// if len(s.XHeader) > 0 {
			// 	for i := range s.XHeader {
			// 		if s.hdr == s.XHeader[i] {
			// 			filter, ok := config.CompileStore.RegexMap[s.XHeader[i]]
			// 			if ok {
			// 				if filter.MatchString(s.hdrv) {
			// 					s.XCallID = filter.FindStringSubmatch(s.hdrv)[1]
			// 				}
			// 			} else {
			// 				s.XCallID = s.hdrv
			// 			}
			// 		}
			// 	}
			// }
			// if len(s.CHeader) > 0 {
			// 	if s.CustomHeader == nil {
			// 		s.CustomHeader = make(map[string]string)
			// 	}

			// 	for i := range s.CHeader {
			// 		//if we wanna compare ignoring case
			// 		if config.Setting.IgnoreCaseCH {
			// 			if strings.EqualFold(s.hdr, s.CHeader[i]) {
			// 				s.CustomHeader[s.CHeader[i]] = s.hdrv
			// 			}
			// 		} else {
			// 			if s.hdr == s.CHeader[i] {
			// 				s.CustomHeader[s.hdr] = s.hdrv
			// 			}
			// 		}
			// 	}
			// }
		}
	}
}

func GetSIPHeaderVal(header string, data string) (val string) {
	l := len(header)
	if startPos := strings.Index(data, header); startPos > -1 {
		restData := data[startPos:]
		if endPos := strings.Index(restData, "\r\n"); endPos > l {
			val = restData[l:endPos]
			i := 0
			for i < len(val) && (val[i] == ' ' || val[i] == '\t') {
				i++
			}
			return val[i:]
		}
	}
	return ""
}

func (s *SipMsg) GetCallingParty(str string) error {
	switch {
	case str == CALLING_PARTY_RPID:
		return s.getCallingPartyRpid()
	case str == CALLING_PARTY_PAID:
		return s.getCallingPartyPaid()
	}
	return s.getCallingPartyDefault()
}

func (s *SipMsg) getCallingPartyDefault() error {
	if s.From == nil {
		return errors.New("getCallingPartyDefault err: no from header found")
	}
	if s.From.URI == nil {
		return errors.New("getCallingPartyDefault err: no uri found in from header")
	}
	s.CallingParty = &CallingPartyInfo{Name: s.From.Name, Number: s.From.URI.User}
	return nil
}

func (s *SipMsg) getCallingPartyPaid() error {
	if s.PAssertedId == nil {
		if s.PAssertedIdVal == "" {
			return s.getCallingPartyDefault()
		}
		s.parsePAssertedId(s.PAssertedIdVal)
		if s.Error != nil {
			return s.Error
		}
		if s.PAssertedId.URI == nil {
			return errors.New("getCallingPartyPaid err: p-asserted-id uri is nil")
		}
		s.CallingParty = &CallingPartyInfo{Name: s.PAssertedId.Name, Number: s.PAssertedId.URI.User}
		return nil
	}
	if s.PAssertedId.URI == nil {
		return errors.New("getCallingPartyPaid err: p-asserted-id uri is nil")
	}
	s.CallingParty = &CallingPartyInfo{Name: s.PAssertedId.Name, Number: s.PAssertedId.URI.User}
	return nil
}

func (s *SipMsg) getCallingPartyRpid() error {
	if s.RemotePartyId == nil {
		if s.RemotePartyIdVal == "" {
			return s.getCallingPartyDefault()
		}
		s.parseRemotePartyId(s.RemotePartyIdVal)
		if s.Error != nil {
			return s.Error
		}
		if s.RemotePartyId.URI == nil {
			return errors.New("getCallingPartyRpid err: remote party id uri is nil")
		}
		s.CallingParty = &CallingPartyInfo{Name: s.RemotePartyId.Name, Number: s.RemotePartyId.URI.User}
		return nil
	}
	if s.RemotePartyId.URI == nil {
		return errors.New("getCallingPartyRpid err: remote party id uri is nil")
	}
	s.CallingParty = &CallingPartyInfo{Name: s.RemotePartyId.Name, Number: s.RemotePartyId.URI.User}
	return nil
}

/* func (s *SipMsg) parseAccept(str string) {
	s.Accept = &Accept{Val: str}
	s.Accept.parse()
} */

/* func (s *SipMsg) parseAllow(str string) {
	s.Allow = getCommaSeperated(str)
	if s.Allow == nil {
		s.Allow = []string{str}
	}
} */

/* func (s *SipMsg) parseAllowEvents(str string) {
	s.AllowEvents = getCommaSeperated(str)
	if s.AllowEvents == nil {
		s.Allow = []string{str}
	}
} */

func (s *SipMsg) parseAuthorization(str string) {
	s.Authorization = &Authorization{Val: str}
	if s.Error = s.Authorization.parse(); s.Error == nil {
		s.AuthUser = s.Authorization.Username
		s.AuthVal = s.Authorization.Val
	}
}

func (s *SipMsg) parseContact(str string) {
	s.Contact = getFrom(str)
	if s.Contact.Error == nil {
		s.ContactUser = s.Contact.URI.User
		s.ContactHost = s.Contact.URI.Host
		s.ContactPort = s.Contact.URI.PortInt
	} else {
		s.Error = s.Contact.Error
	}
}

func (s *SipMsg) ParseContact(str string) {
	s.parseContact(str)
}

/* func (s *SipMsg) parseContentDisposition(str string) {
	s.ContentDisposition = &ContentDisposition{Val: str}
	s.ContentDisposition.parse()
} */

func (s *SipMsg) parseCseq(str string) {
	s.Cseq = &Cseq{Val: str}
	if s.Error = s.Cseq.parse(); s.Error == nil {
		s.CseqMethod = s.Cseq.Method
	}
}

func (s *SipMsg) parseFrom(str string) {
	s.From = getFrom(str)
	if s.From.Error == nil {
		s.FromUser = s.From.URI.User
		s.FromHost = s.From.URI.Host
		s.FromTag = s.From.Tag
	} else {
		s.Error = s.From.Error
	}
}

func (s *SipMsg) parsePAssertedId(str string) {
	s.PAssertedId = &PAssertedId{Val: str}
	s.PAssertedId.parse()
	if s.PAssertedId.Error == nil {
		if s.PaiUser == "" {
			s.PaiUser = s.PAssertedId.URI.User
		}
		if s.PaiHost == "" {
			s.PaiHost = s.PAssertedId.URI.Host
		}
	} else {
		s.PaiUser = s.PAssertedIdVal
	}
}

func (s *SipMsg) ParsePAssertedId(str string) {
	s.parsePAssertedId(str)
}

/* func (s *SipMsg) parseProxyAuthenticate(str string) {
	s.ProxyAuthenticate = &Authorization{Val: str}
	s.Error = s.ProxyAuthenticate.parse()
} */

/* func (s *SipMsg) parseRack(str string) {
	s.Rack = &Rack{Val: str}
	s.Error = s.Rack.parse()
} */

/* func (s *SipMsg) parseReason(str string) {
	s.Reason = &Reason{Val: str}
	s.Reason.parse()
} */

func (s *SipMsg) parseRTPStat(str string) {
	s.RTPStatVal = str
}

/* func (s *SipMsg) parseRecordRoute(str string) {
	cs := []string{str}
	for rt := range cs {
		left := 0
		right := 0
		for i := range cs[rt] {
			if cs[rt][i] == '<' && left == 0 {
				left = i
			}
			if cs[rt][i] == '>' && right == 0 {
				right = i
			}
		}
		if left < right {
			u := ParseURI(cs[rt][left+1 : right])
			if u.Error != nil {
				s.Error = fmt.Errorf("parseRecordRoute err: received err parsing uri: %v", u.Error)
				return
			}
			if s.RecordRoute == nil {
				s.RecordRoute = []*URI{u}
			}
			s.RecordRoute = append(s.RecordRoute, u)
		}
	}
	return
} */

func (s *SipMsg) parseRemotePartyId(str string) {
	s.RemotePartyId = &RemotePartyId{Val: str}
	s.RemotePartyId.parse()
	if s.RemotePartyId.Error != nil {
		s.Error = s.RemotePartyId.Error
	}
}

func (s *SipMsg) ParseRemotePartyId(str string) {
	s.parseRemotePartyId(str)
}

/* func (s *SipMsg) parseRequire(str string) {
	s.Require = getCommaSeperated(str)
	if s.Require == nil {
		s.Require = []string{str}
	}
} */

/* func (s *SipMsg) parseRoute(str string) {
	cs := getCommaSeperated(str)
	for rt := range cs {
		left := 0
		right := 0
		for i := range cs[rt] {
			if cs[rt][i] == '<' && left == 0 {
				left = i
			}
			if cs[rt][i] == '>' && right == 0 {
				right = i
			}
		}
		if left < right {
			u := ParseURI(cs[rt][left+1 : right])
			if u.Error != nil {
				s.Error = fmt.Errorf("parseRoute err: received err parsing uri: %v", u.Error)
				return
			}
			if s.Route == nil {
				s.Route = []*URI{u}
			}
			s.Route = append(s.Route, u)
		}
	}
} */

func (s *SipMsg) parseStartLine(str string) {
	s.State = sipParseStateStartLine
	sLine := ParseStartLine(str)
	s.FirstMethod = sLine.Method
	s.FirstResp = sLine.Resp
	s.FirstRespText = sLine.RespText
	if sLine.URI != nil {
		s.URIHost = sLine.URI.Host
		s.URIRaw = sLine.URI.Raw
		s.URIUser = sLine.URI.User
	}
	if sLine.Error != nil {
		s.Error = fmt.Errorf("parseStartLine err: received err while parsing start line: %v", sLine.Error)
	}
}

/* func (s *SipMsg) parseSupported(str string) {
	s.Supported = getCommaSeperated(str)
	if s.Supported == nil {
		s.Supported = []string{str}
	}
} */

func (s *SipMsg) parseTo(str string) {
	s.To = getFrom(str)
	if s.To.Error == nil {
		s.ToUser = s.To.URI.User
		s.ToHost = s.To.URI.Host
		s.ToTag = s.To.Tag
	} else {
		s.Error = s.To.Error
	}
}

/* func (s *SipMsg) parseUnsupported(str string) {
	s.Unsupported = getCommaSeperated(str)
	if s.Unsupported == nil {
		s.Unsupported = []string{str}
	}
} */

func (s *SipMsg) parseVia(str string) {
	/* 	vs := &vias{via: str}
	   	vs.parse()
	   	if vs.err != nil {
	   		s.Error = vs.err
	   		return
	   	}
	   	for _, v := range vs.vias {
	   		s.Via = append(s.Via, v)
	   	}
	*/
	s.ViaOne = str
	if a := strings.Index(str, "branch="); a > -1 && a < len(str) {
		b := str[a:]
		l := len(b)
		if c := strings.Index(b, ";"); c > -1 && c < l && l > 7 {
			s.ViaOneBranch = b[7:c]
		} else if l > 7 {
			s.ViaOneBranch = b[7:]
		}
	}
}

/* func (s *SipMsg) parseWarning(str string) {
	s.Warning = &Warning{Val: str}
	s.Error = s.Warning.parse()
} */

/* func (s *SipMsg) parseWWWAuthenticate(str string) {
	s.WWWAuthenticate = &Authorization{Val: str}
	s.Error = s.WWWAuthenticate.parse()
} */

func getHeaders(s *SipMsg) sipParserStateFn {
	s.State = sipParseStateHeaders
	var hdr string
	msgLen := len(s.Msg)

	for curPos, crlfPos := 0, 0; curPos < s.eof+2 && s.eof+2 <= msgLen; curPos += 2 {
		crlfPos = strings.Index(s.Msg[curPos:s.eof+2], "\n")
		if crlfPos > 0 && curPos+crlfPos < msgLen {
			if strings.HasSuffix(s.Msg[curPos:curPos+crlfPos], "\r") {
				crlfPos--
				hdr = s.Msg[curPos : curPos+crlfPos]
			} else {
				hdr = s.Msg[curPos : curPos+crlfPos]
				crlfPos--
			}
		}

		hdr = cleanWs(hdr)
		if curPos == 0 {
			s.parseStartLine(hdr)
		} else {
			s.addHdr(hdr)
		}
		if s.Error != nil {
			return nil
		}
		curPos += crlfPos
	}
	return nil
}

func getBody(s *SipMsg) sipParserStateFn {
	s.State = sipParseStateBody
	if len(s.Msg)-1 > s.eof+4 {
		s.Body = s.Msg[s.eof+4:]
	}
	return getHeaders
}

func ParseMsg(str string, xcid, cheader []string) (s *SipMsg) {
	headersEnd := strings.Index(str, "\r\n\r\n")
	if headersEnd == -1 {
		headersEnd = strings.LastIndex(str, "\r\n")
	}
	s = &SipMsg{Msg: str, XHeader: xcid, CHeader: cheader, eof: headersEnd}
	if s.eof == -1 {
		s.Error = errors.New("ParseMsg: err parsing msg no SIP eof found")
		return s
	}
	s.run()
	return s
}

func parseSip(s *SipMsg) sipParserStateFn {
	if s.Error != nil {
		return nil
	}
	return getBody
}
