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

const (
	SIP_SCHEME  = "sip"
	SIPS_SCHEME = "sips"
	TEL_SCHEME  = "tel"
)

// uriStateFn is just a type used by the parse method
type uriStateFn func(*URI) uriStateFn

// URI is a struct that holds an error (hopefully nil), the raw value,
// and the parsed uri.
// Fields are as follows:
// -- Error is the error (or nil)
// -- Scheme is the scheme (i.e. sip)
// -- Raw is the raw value of the uri
// -- UserInfo is the user:password;userparams=foo;
// -- User is the user (i.e. the phone number)
// -- HostInfo is the host:port combination
// -- Host is the host
// -- Port is the port (if any)
// -- UriParams are the uri's parameters
// -- Secure is if the scheme is "sips"
// -- atPos is just used by the parser to identify where the
//
//	"@" char is in the .Raw field (or 0 if not present)
type URI struct {
	Error    error  // error if any
	Scheme   string // scheme .. i.e. tel, sip, sips,etc.
	Raw      string // this is the actual raw uri unparsed
	UserInfo string // this is everything before the "@"
	User     string // this is the actual called party
	HostInfo string // this is everything after the @ or the entire uri
	Host     string // the host in the uri
	Port     string // the port
	PortInt  int
	//UriParams    []*Param
	Secure bool // Indicates SIP-URI or SIPS-URI (true for SIPS-URI)
	atPos  int
}

// NewURI is a convenience function that creates a *URI for you
func NewURI(s string) *URI {
	u := new(URI)
	u.Raw = s
	return u
}

// ParseURI is NewURI ... but also parses it
func ParseURI(s string) *URI {
	u := NewURI(s)
	u.Parse()
	return u
}

// Parse parses the .Raw field
func (u *URI) Parse() {
	for state := parseUri; state != nil; {
		state = state(u)
	}
}

// parseUri is the for loop that does the actual parsing
func parseUri(u *URI) uriStateFn {
	if u.Error == nil {
		return parseUriGetScheme
	}
	return nil
}

// parseUriGetAt determines if there is an "@" character in the
// .Raw field
func parseUriGetAt(u *URI) uriStateFn {
	for i := range u.Raw {
		if u.Raw[i] == '@' {
			u.atPos = i
			return parseUriUser
		}
	}
	if u.atPos == 0 && u.Scheme == TEL_SCHEME {
		return parseUriUser
	}
	return parseUriHost
}

func parseUriGetScheme(u *URI) uriStateFn {
	sLen := len(u.Raw)
	switch {
	case sLen > 4:
		if u.Raw[0:4] == "sip:" {
			u.Raw = u.Raw[4:]
			u.Scheme = SIP_SCHEME
			return parseUriGetAt
		}
		if u.Raw[0:4] == "tel:" {
			u.Raw = u.Raw[4:]
			u.Scheme = TEL_SCHEME
			return parseUriGetAt
		}
		if sLen > 5 && u.Raw[0:5] == "sips:" {
			u.Raw = u.Raw[5:]
			u.Scheme = SIPS_SCHEME
			return parseUriGetAt
		}
	default:
		return parseUriGetAt
	}
	return parseUriGetAt
}

func parseUriUser(u *URI) uriStateFn {
	if u.atPos == 0 && u.Scheme != TEL_SCHEME {
		return parseUriHost
	}
	firstSemi := strings.IndexRune(u.Raw[0:u.atPos], ';')
	colon := 0

	switch {
	case colon != 0:
		u.User = u.Raw[0:colon]
	default:
		switch {
		case firstSemi != -1:
			u.User = u.Raw[0:firstSemi]
		default:
			if u.Scheme == TEL_SCHEME && u.atPos == 0 {
				u.User = u.Raw[0:len(u.Raw)]
				if e := strings.IndexRune(u.User, ';'); e > -1 {
					u.User = u.Raw[0:e]
				}
			} else {
				u.User = u.Raw[0:u.atPos]
			}
		}
	}
	return parseUriHost
}

func parseUriHost(u *URI) uriStateFn {
	if len(u.Raw) <= u.atPos {
		u.Error = fmt.Errorf("malformed host part inside URI: %s", u.Raw)
		return nil
	}

	firstSemi := strings.IndexRune(u.Raw[u.atPos:], ';')
	/*
		if firstSemi != -1 {
			if u.UriParams == nil {
				u.UriParams = make([]*Param, 0)
			}
			if len(u.Raw[u.atPos:])-1 > firstSemi+1 {
				uriparams := strings.Split(u.Raw[u.atPos:][firstSemi+1:], ";")
				for i := range uriparams {
					u.UriParams = append(u.UriParams, getParam(uriparams[i]))
				}
			}
		}
	*/
	colon := 0
	if firstSemi != -1 {
		for i := range u.Raw[u.atPos : u.atPos+firstSemi] {
			if i != len(u.Raw[u.atPos+1:u.atPos+firstSemi]) {
				if u.Raw[u.atPos+1 : u.atPos+firstSemi][i] == ':' {
					//u.Port = cleanWs(u.Raw[u.atPos+1 : u.atPos+firstSemi][i+1:])
					u.Port = u.Raw[u.atPos+1 : u.atPos+firstSemi][i+1:]
					if len(u.Port) >= 2 {
						u.PortInt, _ = strconv.Atoi(u.Port)
					}
					colon = i
				}
				if colon != 0 {
					break
				}
			}
		}
		switch {
		case colon == 0:
			if u.atPos != 0 {
				u.Host = u.Raw[u.atPos+1 : u.atPos+firstSemi]
			} else {
				u.Host = u.Raw[u.atPos : u.atPos+firstSemi]
			}

		default:
			if u.atPos != 0 {
				u.Host = u.Raw[u.atPos+1 : u.atPos+colon+1]
			} else {
				u.Host = u.Raw[u.atPos : u.atPos+colon+1]
			}
		}
	}
	if firstSemi == -1 {
		for i := range u.Raw[u.atPos+1:] {
			if i != len(u.Raw[u.atPos+1:]) {
				if u.Raw[u.atPos+1:][i] == ':' {
					//u.Port = cleanWs(u.Raw[u.atPos+1:][i+1:])
					u.Port = u.Raw[u.atPos+1:][i+1:]
					if len(u.Port) >= 2 {
						u.PortInt, _ = strconv.Atoi(u.Port)
					}
					colon = i
				}
				if colon != 0 {
					break
				}
			}
		}
		switch {
		case colon == 0:
			if u.atPos != 0 {
				u.Host = u.Raw[u.atPos+1:]
			} else {
				u.Host = u.Raw[u.atPos:]
			}

		default:
			if u.atPos != 0 {
				u.Host = u.Raw[u.atPos+1 : u.atPos+colon+1]
			} else {
				u.Host = u.Raw[u.atPos : u.atPos+colon+1]
			}
		}
	}
	return nil
}
