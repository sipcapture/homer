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
	"strings"
)

func cleanWs(s string) string {
	if s == "" {
		return s
	}
PREFIXWS:
	if len(s) > 0 {
		if s[0] == ' ' || s[0] == '\t' {
			s = s[1:]
			goto PREFIXWS
		}
	}

SUFFIXWS:
	if len(s) > 0 {
		if s[len(s)-1] == ' ' || s[len(s)-1] == '\t' {
			s = s[0 : len(s)-1]
			goto SUFFIXWS
		}
	}
	return s
}

func cleanBrack(s string) string {
	if s == "" {
		return ""
	}
	sLen := len(s)
	var n string
	switch {
	case sLen > 0 && s[0] == '<':
		n = s[1:]
	default:
		n = s
	}
	for i := range n {
		if n[i] == '>' {
			if len(n)-1 > i+1 {
				if n[i+1] == ';' {
					n = n[0:i] + n[i+1:]
					return n
				}
			}
			if i == len(n)-1 {
				n = n[0:i]
				return n
			}
		}
	}
	return n
}

func getQuoteChars(s string) (one int, two int, chk bool) {
	ct := 0
	for i := range s {
		if s[i] == '"' {
			switch {
			case ct == 0:
				one = i
				ct = 1
			case ct == 1:
				two = i
				return one, two, true
			default:
				return one, two, false
			}
		}
	}
	return 0, 0, false
}

func getBracks(s string) (one int, two int, chk bool) {
	one, two, ok := findURIBrackets([]byte(s))
	if !ok {
		return 0, 0, false
	}
	return one, two, true
}

// findURIBrackets locates the SIP URI angle brackets in a From/To/Contact-style
// header value. Angle brackets inside a leading quoted display-name are ignored;
// otherwise the last '<...>' pair is used (display-names may contain '<' unquoted).
func findURIBrackets(val []byte) (one int, two int, ok bool) {
	searchFrom := 0
	if _, q2, found := quoteSpan(val); found {
		searchFrom = q2 + 1
	}
	sub := val[searchFrom:]
	lrel := -1
	for i := len(sub) - 1; i >= 0; i-- {
		if sub[i] == '<' {
			lrel = i
			break
		}
	}
	if lrel == -1 {
		return 0, 0, false
	}
	one = searchFrom + lrel
	rrel := indexByte(val[one+1:], '>')
	if rrel == -1 {
		return 0, 0, false
	}
	two = one + 1 + rrel
	return one, two, true
}

func quoteSpan(val []byte) (one int, two int, ok bool) {
	ct := 0
	for i := range val {
		if val[i] == '"' {
			switch ct {
			case 0:
				one = i
				ct = 1
			case 1:
				two = i
				return one, two, true
			default:
				return 0, 0, false
			}
		}
	}
	return 0, 0, false
}

func indexByte(data []byte, b byte) int {
	for i := range data {
		if data[i] == b {
			return i
		}
	}
	return -1
}

func getName(s string) (name string, end int) {
	if s == "" {
		return "", 0
	}
	posOne, posTwo, chk := getQuoteChars(s)
	if chk == true {
		if len(s)-1 > posTwo {
			return cleanWs(s[posOne+1 : posTwo]), posTwo
		}
		return "", 0
	}
	posOne = strings.IndexRune(s, '<')
	if posOne == -1 {
		return "", 0
	}
	if posOne == 0 {
		return "", 0
	}
	return cleanWs(s[0:posOne]), posOne
}

func getCommaSeperated(str string) []string {
	s := strings.Split(str, ",")
	if len(s) == 1 {
		return nil
	}
	for i := range s {
		s[i] = cleanWs(s[i])
	}
	return s
}
