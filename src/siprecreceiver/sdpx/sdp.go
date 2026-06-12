// Package sdpx implements minimal SDP parsing and answer generation for SIPREC SRS.
package sdpx

import (
	"fmt"
	"strconv"
	"strings"
)

const ContentType = "application/sdp"

type Codec struct {
	PayloadType uint8
	Name        string
	ClockRate   int
	Channels    int
	Fmtp        string
}

type Media struct {
	Type         string
	Port         int
	Proto        string
	PayloadTypes []uint8
	Codecs       []Codec
	Label        string
	Direction    string
	Connection   string
}

type SessionDescription struct {
	Origin     string
	SessionID  string
	Connection string
	Media      []Media
}

func (m *Media) CodecByPayload(pt uint8) Codec {
	for _, c := range m.Codecs {
		if c.PayloadType == pt {
			return c
		}
	}
	switch pt {
	case 0:
		return Codec{PayloadType: 0, Name: "PCMU", ClockRate: 8000, Channels: 1}
	case 8:
		return Codec{PayloadType: 8, Name: "PCMA", ClockRate: 8000, Channels: 1}
	}
	return Codec{PayloadType: pt, Name: fmt.Sprintf("PT%d", pt), ClockRate: 8000, Channels: 1}
}

func Parse(data []byte) (*SessionDescription, error) {
	sd := &SessionDescription{}
	var cur *Media
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if len(line) < 2 || line[1] != '=' {
			continue
		}
		typ, val := line[0], line[2:]
		switch typ {
		case 'o':
			sd.Origin = val
		case 'c':
			addr := parseConnection(val)
			if cur != nil {
				cur.Connection = addr
			} else {
				sd.Connection = addr
			}
		case 'm':
			m, err := parseMediaLine(val)
			if err != nil {
				return nil, err
			}
			sd.Media = append(sd.Media, m)
			cur = &sd.Media[len(sd.Media)-1]
		case 'a':
			if cur != nil {
				parseMediaAttribute(cur, val)
			}
		}
	}
	if len(sd.Media) == 0 {
		return nil, fmt.Errorf("sdp: no media sections found")
	}
	return sd, nil
}

func parseConnection(val string) string {
	fields := strings.Fields(val)
	if len(fields) == 3 {
		return fields[2]
	}
	return ""
}

func parseMediaLine(val string) (Media, error) {
	fields := strings.Fields(val)
	if len(fields) < 4 {
		return Media{}, fmt.Errorf("sdp: malformed m= line %q", val)
	}
	port, err := strconv.Atoi(fields[1])
	if err != nil {
		return Media{}, fmt.Errorf("sdp: bad port in m= line %q", val)
	}
	m := Media{Type: fields[0], Port: port, Proto: fields[2]}
	for _, f := range fields[3:] {
		pt, err := strconv.ParseUint(f, 10, 8)
		if err != nil {
			continue
		}
		m.PayloadTypes = append(m.PayloadTypes, uint8(pt))
	}
	return m, nil
}

func parseMediaAttribute(m *Media, val string) {
	name, rest, _ := strings.Cut(val, ":")
	switch name {
	case "rtpmap":
		ptStr, enc, ok := strings.Cut(rest, " ")
		if !ok {
			return
		}
		pt, err := strconv.ParseUint(ptStr, 10, 8)
		if err != nil {
			return
		}
		parts := strings.Split(enc, "/")
		c := Codec{PayloadType: uint8(pt), Name: parts[0], ClockRate: 8000, Channels: 1}
		if len(parts) >= 2 {
			if cr, err := strconv.Atoi(parts[1]); err == nil {
				c.ClockRate = cr
			}
		}
		m.Codecs = append(m.Codecs, c)
	case "label":
		m.Label = rest
	case "sendonly", "recvonly", "sendrecv", "inactive":
		m.Direction = name
	}
}

type AnswerMedia struct {
	Accepted bool
	Port     int
	Codec    Codec
	Label    string
}

func BuildAnswer(offer *SessionDescription, localIP string, answers []AnswerMedia, sessionID int64) string {
	var b strings.Builder
	fmt.Fprintf(&b, "v=0\r\n")
	fmt.Fprintf(&b, "o=homer-siprec %d %d IN IP4 %s\r\n", sessionID, sessionID, localIP)
	fmt.Fprintf(&b, "s=SIPREC Recording Session\r\n")
	fmt.Fprintf(&b, "c=IN IP4 %s\r\n", localIP)
	fmt.Fprintf(&b, "t=0 0\r\n")
	for i, om := range offer.Media {
		if i >= len(answers) {
			break
		}
		a := answers[i]
		if !a.Accepted {
			pt := "0"
			if len(om.PayloadTypes) > 0 {
				pt = strconv.Itoa(int(om.PayloadTypes[0]))
			}
			fmt.Fprintf(&b, "m=%s 0 %s %s\r\n", om.Type, om.Proto, pt)
			continue
		}
		c := a.Codec
		fmt.Fprintf(&b, "m=%s %d %s %d\r\n", om.Type, a.Port, om.Proto, c.PayloadType)
		fmt.Fprintf(&b, "a=rtpmap:%d %s/%d\r\n", c.PayloadType, c.Name, c.ClockRate)
		if a.Label != "" {
			fmt.Fprintf(&b, "a=label:%s\r\n", a.Label)
		}
		fmt.Fprintf(&b, "a=recvonly\r\n")
	}
	return b.String()
}

func SelectCodec(m *Media) Codec {
	for _, want := range []string{"PCMU", "PCMA"} {
		for _, pt := range m.PayloadTypes {
			c := m.CodecByPayload(pt)
			if strings.EqualFold(c.Name, want) {
				return c
			}
		}
	}
	for _, pt := range m.PayloadTypes {
		return m.CodecByPayload(pt)
	}
	return m.CodecByPayload(0)
}
