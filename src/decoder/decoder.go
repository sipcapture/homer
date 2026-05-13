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

package decoder

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	reflect "reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/VictoriaMetrics/fastcache"
	xxhash "github.com/cespare/xxhash/v2"
	"github.com/sipcapture/homer-core/src/homerconfig"
	"github.com/sipcapture/homer-core/src/sipparser"
	logger "github.com/sipcapture/homer-core/src/utils/logging"
)

// The first 4 bytes are the string "HEP3". The next 2 bytes are the length of the
// whole message (len("HEP3") + length of all the chunks we have. The next bytes
// are all the chunks created by makeChunks()
// Bytes: 0 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24 25 26 27 28 29 30 31......
//        +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//        | "HEP3"|len|chunks(0x0001|0x0002|0x0003|0x0004|0x0007|0x0008|0x0009|0x000a|0x000b|......)
//        +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

var (
	dedupCache            = fastcache.New(32 * 1024 * 1024)
	scriptCache           = fastcache.New(32 * 1024 * 1024)
	strBackslashQuote     = []byte(`\"`)
	strBackslashBackslash = []byte(`\\`)
	strBackslashN         = []byte(`\n`)
	strBackslashR         = []byte(`\r`)
	strBackslashT         = []byte(`\t`)
	strBackslashF         = []byte(`\u000c`)
	strBackslashB         = []byte(`\u0008`)
	strBackslashLT        = []byte(`\u003c`)
	strBackslashQ         = []byte(`\u0027`)
	strEmpty              = []byte(``)
)

// DecoderConfig holds decoder configuration
type DecoderConfig struct {
	// HEP protocol settings
	HepV2Enable    bool
	HepV3Enable    bool
	ProtobufEnable bool
	Deduplicate    bool

	// SIP settings
	// AlegIDs: SIP header names (case-insensitive); first matching header in wire order fills XCallID during zero-copy parse (see sipparser.ZeroCopyOpts).
	AlegIDs []string
	// CustomHeaders: optional SIP header names whose values are stored in CustomHeader / data_extra.custom_headers.
	CustomHeaders  []string
	ForceALegID    bool
	CensorMethods  []string
	DiscardMethods []string

	// Force payload storage for specific HEP types
	ForceHEPPayload []int
}

// Decoder is a HEP packet decoder with configuration
type Decoder struct {
	config *DecoderConfig
}

// NewDecoder creates a new decoder with the given configuration
func NewDecoder(cfg *DecoderConfig) *Decoder {
	if cfg == nil {
		cfg = &DecoderConfig{}
	}
	return &Decoder{config: cfg}
}

// DefaultDecoder is used for legacy compatibility (uses homerconfig)
var DefaultDecoder = &Decoder{config: &DecoderConfig{}}

// HEP chunks
const (
	Version   = 1  // Chunk 0x0001 IP protocol family (0x02=IPv4, 0x0a=IPv6)
	Protocol  = 2  // Chunk 0x0002 IP protocol ID (0x06=TCP, 0x11=UDP)
	IP4SrcIP  = 3  // Chunk 0x0003 IPv4 source address
	IP4DstIP  = 4  // Chunk 0x0004 IPv4 destination address
	IP6SrcIP  = 5  // Chunk 0x0005 IPv6 source address
	IP6DstIP  = 6  // Chunk 0x0006 IPv6 destination address
	SrcPort   = 7  // Chunk 0x0007 Protocol source port
	DstPort   = 8  // Chunk 0x0008 Protocol destination port
	Tsec      = 9  // Chunk 0x0009 Unix timestamp, seconds
	Tmsec     = 10 // Chunk 0x000a Unix timestamp, microseconds
	ProtoType = 11 // Chunk 0x000b Protocol type (DNS, LOG, RTCP, SIP, DIAMETER)
	NodeID    = 12 // Chunk 0x000c Capture client ID
	NodePW    = 14 // Chunk 0x000e Authentication key (plain text / TLS connection)
	Payload   = 15 // Chunk 0x000f Captured packet payload
	CID       = 17 // Chunk 0x0011 Correlation ID
	Vlan      = 18 // Chunk 0x0012 VLAN
	NodeName  = 19 // Chunk 0x0013 NodeName
)

// HEP represents HEP packet
type HEP struct {
	Version     uint32 `protobuf:"varint,1,req,name=Version" json:"Version"`
	Protocol    uint32 `protobuf:"varint,2,req,name=Protocol" json:"Protocol"`
	SrcIP       string `protobuf:"bytes,3,req,name=SrcIP" json:"SrcIP"`
	DstIP       string `protobuf:"bytes,4,req,name=DstIP" json:"DstIP"`
	SrcPort     uint32 `protobuf:"varint,5,req,name=SrcPort" json:"SrcPort"`
	DstPort     uint32 `protobuf:"varint,6,req,name=DstPort" json:"DstPort"`
	Tsec        uint32 `protobuf:"varint,7,req,name=Tsec" json:"Tsec"`
	Tmsec       uint32 `protobuf:"varint,8,req,name=Tmsec" json:"Tmsec"`
	ProtoType   uint32 `protobuf:"varint,9,req,name=ProtoType" json:"ProtoType"`
	NodeID      uint32 `protobuf:"varint,10,req,name=NodeID" json:"NodeID"`
	NodePW      string `protobuf:"bytes,11,req,name=NodePW" json:"NodePW"`
	Payload     string `protobuf:"bytes,12,req,name=Payload" json:"Payload"`
	CID         string `protobuf:"bytes,13,req,name=CID" json:"CID"`
	Vlan        uint32 `protobuf:"varint,14,req,name=Vlan" json:"Vlan"`
	ProtoString string
	Timestamp   time.Time
	SIP         *sipparser.SipMsg
	NodeName    string
	TargetName  string
	SID         string

	// CustomLokiLabels is set by Lua SetLokiLabel (allowlisted keys only, max 5 per message).
	CustomLokiLabels map[string]string `json:"-"`

	// decoder reference for config access
	decoder *Decoder

	// Timing (nanoseconds) — populated by parse() for profiling
	HEPParseNs int64
	SIPParseNs int64
}

// DecodeHEP returns a parsed HEP message (legacy, uses homerconfig)
func DecodeHEP(packet []byte) (*HEP, error) {
	return DefaultDecoder.Decode(packet)
}

// ShouldForcePayload checks if payload should be forced for this HEP type
func (h *HEP) ShouldForcePayload() bool {
	if h.decoder == nil || h.decoder.config == nil {
		return false
	}
	for _, pt := range h.decoder.config.ForceHEPPayload {
		if uint32(pt) == h.ProtoType {
			return true
		}
	}
	return false
}

// Decode parses a HEP packet using this decoder's configuration
func (d *Decoder) Decode(packet []byte) (*HEP, error) {
	hep := &HEP{decoder: d}
	err := hep.parse(packet)
	if err != nil {
		return nil, err
	}
	return hep, nil
}

func (h *HEP) parse(packet []byte) error {
	// Get HEP settings from decoder config or legacy homerconfig
	hepV2Enable := true    // default
	hepV3Enable := true    // default
	protobufEnable := true // default

	if h.decoder != nil && h.decoder.config != nil {
		hepV2Enable = h.decoder.config.HepV2Enable
		hepV3Enable = h.decoder.config.HepV3Enable
		protobufEnable = h.decoder.config.ProtobufEnable
	} else if homerconfig.MainConfig != nil {
		hepV2Enable = homerconfig.MainConfig.Setting.HEP_SETTINGS.HepV2Enable
		hepV3Enable = homerconfig.MainConfig.Setting.HEP_SETTINGS.HepV3Enable
		protobufEnable = homerconfig.MainConfig.Setting.HEP_SETTINGS.ProtobufEnable
	}

	hepStart := time.Now().UnixNano()

	var err error
	if bytes.HasPrefix(packet, []byte{0x48, 0x45, 0x50, 0x33}) {
		err = h.parseHEP(packet)
		if err != nil {
			logger.Error(fmt.Sprintf("%v", err))
			return err
		}
	} else if hepV2Enable && (bytes.HasPrefix(packet, []byte{0x1}) || bytes.HasPrefix(packet, []byte{0x2})) {
		err = h.parseHEP2(packet)
		if err != nil {
			logger.Error(fmt.Sprintf("bad HEPv1/v2 decoding: %v", err))
			return err
		}
	} else if protobufEnable {
		err = h.Unmarshal(packet)
		if err != nil {
			logger.Error(fmt.Sprintf("malformed packet with length %d which is neither hep nor protobuf encapsulated", len(packet)))
			return err
		}
	} else {
		return fmt.Errorf("packet format not recognized and protobuf is disabled (HEPv3: %v, HEPv2: %v, Protobuf: %v)",
			hepV3Enable, hepV2Enable, protobufEnable)
	}

	h.Timestamp = time.Unix(int64(h.Tsec), int64(h.Tmsec*1000))
	if h.Tsec == 0 && h.Tmsec == 0 {
		logger.Debug("got null timestamp", "nodeID", h.NodeID)
		h.Timestamp = time.Now()
	}

	h.normPayload()
	h.HEPParseNs = time.Now().UnixNano() - hepStart

	if h.ProtoType == 0 {
		return nil
	}

	if h.ProtoType == 1 && len(h.Payload) > 32 {
		sipStart := time.Now().UnixNano()
		err = h.parseSIP()
		if err != nil {
			logger.Error(fmt.Sprintf("%v\n%q\nnodeID: %d, protoType: %d, version: %d, protocol: %d, length: %d, flow: %s:%d->%s:%d\n\n",
				err, h.Payload, h.NodeID, h.ProtoType, h.Version, h.Protocol, len(h.Payload), h.SrcIP, h.SrcPort, h.DstIP, h.DstPort))
			return err
		}

		// Get censor/discard methods from decoder config or legacy homerconfig
		var censorMethods, discardMethods []string
		if h.decoder != nil && h.decoder.config != nil {
			censorMethods = h.decoder.config.CensorMethods
			discardMethods = h.decoder.config.DiscardMethods
		} else if homerconfig.MainConfig != nil {
			censorMethods = homerconfig.MainConfig.Setting.SIP_SETTINGS.CensorMethod
			discardMethods = homerconfig.MainConfig.Setting.SIP_SETTINGS.DiscardMethods
		}

		for _, m := range censorMethods {
			if m == h.SIP.CseqMethod {
				lb := len(h.SIP.Body)
				h.SIP.Body = strings.Repeat("x", lb)
				h.Payload = h.Payload[:len(h.Payload)-lb] + h.SIP.Body
			}
		}

		for _, m := range discardMethods {
			if m == h.SIP.CseqMethod {
				h.ProtoType = 0
				return nil
			}
		}
		h.SIPParseNs = time.Now().UnixNano() - sipStart
	}

	if h.NodeName == "" {
		h.NodeName = strconv.FormatUint(uint64(h.NodeID), 10)
	}

	return nil
}

func (h *HEP) normPayload() {
	// Check deduplicate setting from decoder config or legacy homerconfig
	deduplicate := false
	if h.decoder != nil && h.decoder.config != nil {
		deduplicate = h.decoder.config.Deduplicate
	} else if homerconfig.MainConfig != nil {
		deduplicate = homerconfig.MainConfig.Setting.HEP_SETTINGS.Deduplicate
	}

	if deduplicate {
		ts := uint64(h.Timestamp.UnixNano())
		ks := xxhash.Sum64String(h.Payload)
		var kh [8]byte
		binary.BigEndian.PutUint64(kh[:], ks)

		if buf := dedupCache.Get(nil, kh[:]); buf != nil {
			i := binary.BigEndian.Uint64(buf)
			d := ts - i
			if i > ts {
				d = i - ts
			}
			if d < 500e6 {
				h.ProtoType = 0
				return
			}
		}

		var tb [8]byte
		binary.BigEndian.PutUint64(tb[:], ts)
		dedupCache.Set(kh[:], tb[:])
	}

	h.Payload = toUTF8(h.Payload, "")
}

func (h *HEP) EscapeFields(w io.Writer, tag string) (int, error) {
	switch tag {
	case "callid":
		return WriteJSONString(w, h.SIP.CallID)
	case "cseq":
		return WriteJSONString(w, h.SIP.CseqVal)
	case "method":
		return WriteJSONString(w, h.SIP.FirstMethod)
	case "ruri_user":
		return WriteJSONString(w, h.SIP.URIUser)
	case "ruri_domain":
		return WriteJSONString(w, h.SIP.URIHost)
	case "from_user":
		return WriteJSONString(w, h.SIP.FromUser)
	case "from_domain":
		return WriteJSONString(w, h.SIP.FromHost)
	case "from_tag":
		return WriteJSONString(w, h.SIP.FromTag)
	case "to_user":
		return WriteJSONString(w, h.SIP.ToUser)
	case "to_domain":
		return WriteJSONString(w, h.SIP.ToHost)
	case "to_tag":
		return WriteJSONString(w, h.SIP.ToTag)
	case "via":
		return WriteJSONString(w, h.SIP.ViaOne)
	case "contact_user":
		return WriteJSONString(w, h.SIP.ContactUser)
	case "contact_domain":
		return WriteJSONString(w, h.SIP.ContactHost)
	case "user_agent":
		return WriteJSONString(w, h.SIP.UserAgent)
	case "pid_user":
		return WriteJSONString(w, h.SIP.PaiUser)
	case "auth_user":
		return WriteJSONString(w, h.SIP.AuthUser)
	case "server":
		return WriteJSONString(w, h.SIP.Server)
	case "content_type":
		return WriteJSONString(w, h.SIP.ContentType)
	case "reason":
		return WriteJSONString(w, h.SIP.ReasonVal)
	case "diversion":
		return WriteJSONString(w, h.SIP.DiversionVal)
	case "expires":
		return WriteJSONString(w, h.SIP.Expires)
	case "callid_aleg":
		return WriteJSONString(w, h.SIP.XCallID)
	default:
		return w.Write(strEmpty)
	}
}

func WriteJSONString(w io.Writer, s string) (int, error) {
	write := w.Write
	b := stb(s)
	j := 0
	n := len(b)
	if n > 0 {
		// Hint the compiler to remove bounds checks in the loop below.
		_ = b[n-1]
	}
	for i := 0; i < n; i++ {
		switch b[i] {
		case '"':
			write(b[j:i])
			write(strBackslashQuote)
			j = i + 1
		case '\\':
			write(b[j:i])
			write(strBackslashBackslash)
			j = i + 1
		case '\n':
			write(b[j:i])
			write(strBackslashN)
			j = i + 1
		case '\r':
			write(b[j:i])
			write(strBackslashR)
			j = i + 1
		case '\t':
			write(b[j:i])
			write(strBackslashT)
			j = i + 1
		case '\f':
			write(b[j:i])
			write(strBackslashF)
			j = i + 1
		case '\b':
			write(b[j:i])
			write(strBackslashB)
			j = i + 1
		default:
			if b[i] < 32 {
				write(b[j:i])
				fmt.Fprintf(w, "\\u%0.4x", b[i])
				j = i + 1
				continue
			}
		}
	}
	return write(b[j:])
}

func stb(s string) []byte {
	sh := (*reflect.StringHeader)(unsafe.Pointer(&s))
	var res []byte

	bh := (*reflect.SliceHeader)((unsafe.Pointer(&res)))
	bh.Data = sh.Data
	bh.Len = sh.Len
	bh.Cap = sh.Len
	return res
}

func toUTF8(s, replacement string) string {
	var b strings.Builder

	for i, c := range s {
		if c != utf8.RuneError && c != '\x00' {
			continue
		}

		_, wid := utf8.DecodeRuneInString(s[i:])
		if wid == 1 {
			b.Grow(len(s) + len(replacement))
			b.WriteString(s[:i])
			s = s[i:]
			break
		}
	}

	// Fast path for unchanged input
	if b.Cap() == 0 { // didn't call b.Grow above
		return s
	}

	invalid := false // previous byte was from an invalid UTF-8 sequence
	for i := 0; i < len(s); {
		c := s[i]
		if c == '\x00' {
			i++
			invalid = false
			continue
		} else if c < utf8.RuneSelf {
			i++
			invalid = false
			b.WriteByte(c)
			continue
		}
		_, wid := utf8.DecodeRuneInString(s[i:])
		if wid == 1 {
			i++
			if !invalid {
				invalid = true
				b.WriteString(replacement)
			}
			continue
		}
		invalid = false
		b.WriteString(s[i : i+wid])
		i += wid
	}

	return b.String()
}
