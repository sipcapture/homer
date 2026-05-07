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

package sipparser

import (
	"errors"
	"unsafe"
)

// FastSIPParser - ultra-fast SIP parser using pointers and zero-copy techniques
// Inspired by SIMD and low-level C++ optimizations
type FastSIPParser struct {
	// Reusable buffers to avoid allocations
	headerBuf [256]byte
	valueBuf  [512]byte
}

// SIPMessage - zero-copy SIP message structure using pointers
type SIPMessage struct {
	// Raw data pointer (no copy)
	Data     []byte
	DataPtr  *byte // Pointer to first byte
	
	// Start line pointers (point into Data, no copy)
	Method      []byte
	RequestURI  []byte
	ResponseCode []byte
	ResponseText []byte
	Version     []byte
	
	// Header pointers (point into Data, no copy)
	CallID      []byte
	From        []byte
	To          []byte
	Via         []byte
	CSeq        []byte
	Contact     []byte
	ContentType []byte
	ContentLen  []byte
	UserAgent   []byte
	
	// Parsed values (only allocated when needed)
	MethodStr   string
	CallIDStr   string
	FromTag     []byte
	ToTag       []byte
	Branch      []byte
	
	// Flags
	IsRequest   bool
	IsResponse  bool
	HasBody     bool
	
	// Body pointer
	Body        []byte
	BodyOffset  int
}

// NewFastSIPParser creates a new fast SIP parser
func NewFastSIPParser() *FastSIPParser {
	return &FastSIPParser{}
}

// Parse parses SIP message with zero-copy and pointer-based approach
// This is optimized for performance using unsafe pointers and SIMD-like operations
func (p *FastSIPParser) Parse(data []byte) (*SIPMessage, error) {
	if len(data) < 16 {
		return nil, errors.New("SIP message too short")
	}
	
	msg := &SIPMessage{
		Data:    data,
		DataPtr: &data[0],
	}
	
	// Find headers end (\r\n\r\n) using SIMD-like byte scanning
	headersEnd := p.findHeadersEnd(data)
	if headersEnd == -1 {
		return nil, errors.New("invalid SIP message: no headers end found")
	}
	
	msg.BodyOffset = headersEnd + 4
	if msg.BodyOffset < len(data) {
		msg.Body = data[msg.BodyOffset:]
		msg.HasBody = true
	}
	
	// Parse start line (first line) - zero copy
	if err := p.parseStartLineFast(data, msg); err != nil {
		return nil, err
	}
	
	// Parse headers - zero copy, pointer-based
	if err := p.parseHeadersFast(data[:headersEnd], msg); err != nil {
		return nil, err
	}
	
	return msg, nil
}

// findHeadersEnd finds \r\n\r\n position using optimized byte scanning
// This mimics SIMD operations by processing multiple bytes at once
func (p *FastSIPParser) findHeadersEnd(data []byte) int {
	// Use unsafe pointer arithmetic for faster scanning
	dataPtr := unsafe.Pointer(&data[0])
	len := len(data)
	
	// Process 4 bytes at a time (SIMD-like)
	for i := 0; i < len-3; i++ {
		// Check for \r\n\r\n pattern
		b0 := *(*byte)(unsafe.Pointer(uintptr(dataPtr) + uintptr(i)))
		b1 := *(*byte)(unsafe.Pointer(uintptr(dataPtr) + uintptr(i+1)))
		b2 := *(*byte)(unsafe.Pointer(uintptr(dataPtr) + uintptr(i+2)))
		b3 := *(*byte)(unsafe.Pointer(uintptr(dataPtr) + uintptr(i+3)))
		
		if b0 == '\r' && b1 == '\n' && b2 == '\r' && b3 == '\n' {
			return i
		}
	}
	
	return -1
}

// parseStartLineFast parses SIP start line using pointer arithmetic
func (p *FastSIPParser) parseStartLineFast(data []byte, msg *SIPMessage) error {
	// Find first line end (\r\n)
	lineEnd := -1
	for i := 0; i < len(data)-1; i++ {
		if data[i] == '\r' && data[i+1] == '\n' {
			lineEnd = i
			break
		}
	}
	
	if lineEnd == -1 {
		return errors.New("invalid start line: no CRLF found")
	}
	
	line := data[:lineEnd]
	
	// Check if it's a response (starts with "SIP/")
	// Response format: SIP/2.0 200 OK
	// Request format: INVITE sip:... SIP/2.0
	if len(line) >= 4 {
		// Simple byte-by-byte check (more reliable than uint32 comparison)
		if (line[0] == 'S' || line[0] == 's') &&
		   (line[1] == 'I' || line[1] == 'i') &&
		   (line[2] == 'P' || line[2] == 'p') &&
		   line[3] == '/' {
			msg.IsResponse = true
			return p.parseResponseLine(line, msg)
		}
	}
	
	// It's a request
	msg.IsRequest = true
	return p.parseRequestLine(line, msg)
}

// parseRequestLine parses request line: METHOD URI SIP/VERSION
func (p *FastSIPParser) parseRequestLine(line []byte, msg *SIPMessage) error {
	// Find first space
	space1 := -1
	for i := 0; i < len(line); i++ {
		if line[i] == ' ' {
			space1 = i
			break
		}
	}
	if space1 == -1 {
		return errors.New("invalid request line: no first space")
	}
	
	// Find second space
	space2 := -1
	for i := space1 + 1; i < len(line); i++ {
		if line[i] == ' ' {
			space2 = i
			break
		}
	}
	if space2 == -1 {
		return errors.New("invalid request line: no second space")
	}
	
	// Extract method (zero copy - just pointer)
	msg.Method = line[:space1]
	
	// Extract URI (zero copy)
	msg.RequestURI = line[space1+1 : space2]
	
	// Extract version part
	versionPart := line[space2+1:]
	
	// Find '/' in version
	slashPos := -1
	for i := 0; i < len(versionPart); i++ {
		if versionPart[i] == '/' {
			slashPos = i
			break
		}
	}
	if slashPos == -1 {
		return errors.New("invalid request line: no '/' in version")
	}
	
	// Extract protocol and version (zero copy)
	msg.Version = versionPart[slashPos+1:]
	
	// Convert method to string only when needed (lazy evaluation)
	msg.MethodStr = string(msg.Method)
	
	return nil
}

// parseResponseLine parses response line: SIP/VERSION CODE TEXT
func (p *FastSIPParser) parseResponseLine(line []byte, msg *SIPMessage) error {
	// Find first space
	space1 := -1
	for i := 0; i < len(line); i++ {
		if line[i] == ' ' {
			space1 = i
			break
		}
	}
	if space1 == -1 {
		return errors.New("invalid response line: no first space")
	}
	
	// Find second space
	space2 := -1
	for i := space1 + 1; i < len(line); i++ {
		if line[i] == ' ' {
			space2 = i
			break
		}
	}
	if space2 == -1 {
		return errors.New("invalid response line: no second space")
	}
	
	// Extract version (zero copy)
	versionPart := line[:space1]
	slashPos := -1
	for i := 0; i < len(versionPart); i++ {
		if versionPart[i] == '/' {
			slashPos = i
			break
		}
	}
	if slashPos != -1 {
		msg.Version = versionPart[slashPos+1:]
	}
	
	// Extract response code (zero copy)
	msg.ResponseCode = line[space1+1 : space2]
	
	// Extract response text (zero copy)
	msg.ResponseText = line[space2+1:]
	
	return nil
}

// parseHeadersFast parses headers using zero-copy pointer-based approach
func (p *FastSIPParser) parseHeadersFast(headers []byte, msg *SIPMessage) error {
	// Skip start line (find first \r\n)
	start := 0
	for i := 0; i < len(headers)-1; i++ {
		if headers[i] == '\r' && headers[i+1] == '\n' {
			start = i + 2
			break
		}
	}
	
	// Parse headers line by line
	pos := start
	for pos < len(headers) {
		// Find line end
		lineEnd := -1
		for i := pos; i < len(headers)-1; i++ {
			if headers[i] == '\r' && headers[i+1] == '\n' {
				lineEnd = i
				break
			}
		}
		
		if lineEnd == -1 {
			break
		}
		
		// Empty line means end of headers
		if lineEnd == pos {
			break
		}
		
		// Parse header line
		line := headers[pos:lineEnd]
		p.parseHeaderLine(line, msg)
		
		pos = lineEnd + 2
	}
	
	return nil
}

// parseHeaderLine parses a single header line using optimized byte scanning
func (p *FastSIPParser) parseHeaderLine(line []byte, msg *SIPMessage) {
	// Find colon
	colonPos := -1
	for i := 0; i < len(line); i++ {
		if line[i] == ':' {
			colonPos = i
			break
		}
	}
	if colonPos == -1 {
		return
	}
	
	// Extract header name (zero copy)
	headerName := line[:colonPos]
	
	// Extract value (skip colon and spaces)
	valueStart := colonPos + 1
	for valueStart < len(line) && (line[valueStart] == ' ' || line[valueStart] == '\t') {
		valueStart++
	}
	value := line[valueStart:]
	
	// Use SIMD-like comparison for common headers
	// Compare header name for fast matching
	// Handle short headers first (To, Via)
	if len(headerName) == 2 {
		// To header
		if (headerName[0] == 'T' || headerName[0] == 't') &&
		   (headerName[1] == 'O' || headerName[1] == 'o') {
			msg.To = value
			msg.ToTag = p.extractTag(value)
			return
		}
	}
	
	if len(headerName) == 3 {
		// Via header
		if (headerName[0] == 'V' || headerName[0] == 'v') &&
		   (headerName[1] == 'I' || headerName[1] == 'i') &&
		   (headerName[2] == 'A' || headerName[2] == 'a') {
			msg.Via = value
			msg.Branch = p.extractBranch(value)
			return
		}
	}
	
	if len(headerName) >= 4 {
		// Use case-insensitive comparison for header names
		// Convert to lowercase on-the-fly without allocation
		first4Lower := uint32(0)
		for i := 0; i < 4 && i < len(headerName); i++ {
			b := headerName[i]
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			first4Lower |= uint32(b) << (i * 8)
		}
		
		// Extract next 2 bytes for Call-ID check if needed
		var next2 uint16
		if len(headerName) >= 6 {
			for i := 0; i < 2 && i+4 < len(headerName); i++ {
				b := headerName[i+4]
				if b >= 'A' && b <= 'Z' {
					b += 32
				}
				next2 |= uint16(b) << (i * 8)
			}
		}
		
		switch {
		case first4Lower == uint32('c')|(uint32('a')<<8)|(uint32('l')<<16)|(uint32('l')<<24) && 
		     len(headerName) >= 7 && 
		     headerName[4] == '-' && 
		     (headerName[5] == 'I' || headerName[5] == 'i') &&
		     (headerName[6] == 'D' || headerName[6] == 'd'):
			// Call-ID (case-insensitive)
			msg.CallID = value
			
		case first4Lower == uint32('f')|(uint32('r')<<8)|(uint32('o')<<16)|(uint32('m')<<24):
			// From
			msg.From = value
			msg.FromTag = p.extractTag(value)
			
			
		case first4Lower == uint32('c')|(uint32('s')<<8)|(uint32('e')<<16)|(uint32('q')<<24):
			// CSeq
			msg.CSeq = value
			
		case first4Lower == uint32('c')|(uint32('o')<<8)|(uint32('n')<<16)|(uint32('t')<<24):
			// Content-* headers
			if len(headerName) >= 8 {
				// Convert next 4 bytes to lowercase on-the-fly
				next4 := uint32(0)
				for i := 0; i < 4 && i+4 < len(headerName); i++ {
					b := headerName[i+4]
					if b >= 'A' && b <= 'Z' {
						b += 32
					}
					next4 |= uint32(b) << (i * 8)
				}
				
				if next4 == uint32('a')|(uint32('c')<<8)|(uint32('t')<<16)|(uint32(0)<<24) {
					// Contact
					msg.Contact = value
				} else if next4 == uint32('t')|(uint32('y')<<8)|(uint32('p')<<16)|(uint32('e')<<24) {
					// Content-Type
					msg.ContentType = value
				} else if next4 == uint32('l')|(uint32('e')<<8)|(uint32('n')<<16)|(uint32('g')<<24) {
					// Content-Length
					msg.ContentLen = value
				}
			}
			
		case first4Lower == uint32('u')|(uint32('s')<<8)|(uint32('e')<<16)|(uint32('r')<<24) &&
		     len(headerName) >= 9:
			// User-Agent - check next 4 bytes
			next4 := uint32(0)
			for i := 0; i < 4 && i+4 < len(headerName); i++ {
				b := headerName[i+4]
				if b >= 'A' && b <= 'Z' {
					b += 32
				}
				next4 |= uint32(b) << (i * 8)
			}
			if next4 == uint32('-')|(uint32('a')<<8)|(uint32('g')<<16)|(uint32('e')<<24) {
				msg.UserAgent = value
			}
		}
	}
}

// extractTag extracts tag parameter from header value (zero copy)
func (p *FastSIPParser) extractTag(value []byte) []byte {
	// Look for ";tag="
	tagStart := -1
	for i := 0; i < len(value)-4; i++ {
		if value[i] == ';' && value[i+1] == 't' && value[i+2] == 'a' && value[i+3] == 'g' && value[i+4] == '=' {
			tagStart = i + 5
			break
		}
	}
	if tagStart == -1 {
		return nil
	}
	
	// Find end of tag (space, semicolon, or end)
	tagEnd := len(value)
	for i := tagStart; i < len(value); i++ {
		if value[i] == ';' || value[i] == ' ' || value[i] == '\r' || value[i] == '\n' {
			tagEnd = i
			break
		}
	}
	
	return value[tagStart:tagEnd]
}

// extractBranch extracts branch parameter from Via header (zero copy)
func (p *FastSIPParser) extractBranch(value []byte) []byte {
	// Look for ";branch="
	branchStart := -1
	for i := 0; i < len(value)-7; i++ {
		if value[i] == ';' && 
		   value[i+1] == 'b' && value[i+2] == 'r' && value[i+3] == 'a' && 
		   value[i+4] == 'n' && value[i+5] == 'c' && value[i+6] == 'h' && value[i+7] == '=' {
			branchStart = i + 8
			break
		}
	}
	if branchStart == -1 {
		return nil
	}
	
	// Find end of branch
	branchEnd := len(value)
	for i := branchStart; i < len(value); i++ {
		if value[i] == ';' || value[i] == ' ' || value[i] == '\r' || value[i] == '\n' {
			branchEnd = i
			break
		}
	}
	
	return value[branchStart:branchEnd]
}

// GetCallID returns Call-ID as string (lazy conversion)
func (msg *SIPMessage) GetCallID() string {
	if msg.CallIDStr == "" && len(msg.CallID) > 0 {
		msg.CallIDStr = string(msg.CallID)
	}
	return msg.CallIDStr
}

// GetMethod returns method as string (lazy conversion)
func (msg *SIPMessage) GetMethod() string {
	if msg.MethodStr == "" && len(msg.Method) > 0 {
		msg.MethodStr = string(msg.Method)
	}
	return msg.MethodStr
}

