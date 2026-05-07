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

package tests

import (
	"testing"
	"github.com/sipcapture/homer-core/src/sipparser"
)

var testSIPRequest = []byte("INVITE sip:alice@example.com SIP/2.0\r\n" +
	"Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds\r\n" +
	"Max-Forwards: 70\r\n" +
	"To: Bob <sip:bob@biloxi.com>\r\n" +
	"From: Alice <sip:alice@atlanta.com>;tag=1928301774\r\n" +
	"Call-ID: a84b4c76e66710@pc33.atlanta.com\r\n" +
	"CSeq: 314159 INVITE\r\n" +
	"Contact: <sip:alice@pc33.atlanta.com>\r\n" +
	"Content-Type: application/sdp\r\n" +
	"Content-Length: 142\r\n" +
	"User-Agent: Homer-Server/1.0\r\n" +
	"\r\n" +
	"v=0\r\n" +
	"o=alice 53655765 2353687637 IN IP4 pc33.atlanta.com\r\n" +
	"s=Session\r\n" +
	"t=0 0\r\n" +
	"c=IN IP4 pc33.atlanta.com\r\n" +
	"m=audio 3456 RTP/AVP 0 1 3 99\r\n" +
	"a=rtpmap:0 PCMU/8000\r\n")

var testSIPResponse = []byte("SIP/2.0 200 OK\r\n" +
	"Via: SIP/2.0/UDP pc33.atlanta.com;branch=z9hG4bK776asdhds\r\n" +
	"To: Bob <sip:bob@biloxi.com>;tag=a6c85cf\r\n" +
	"From: Alice <sip:alice@atlanta.com>;tag=1928301774\r\n" +
	"Call-ID: a84b4c76e66710@pc33.atlanta.com\r\n" +
	"CSeq: 314159 INVITE\r\n" +
	"Contact: <sip:bob@pc33.biloxi.com>\r\n" +
	"Content-Type: application/sdp\r\n" +
	"Content-Length: 131\r\n" +
	"User-Agent: Homer-Server/1.0\r\n" +
	"\r\n" +
	"v=0\r\n" +
	"o=bob 2890844526 2890844526 IN IP4 pc33.biloxi.com\r\n" +
	"s=Session\r\n" +
	"t=0 0\r\n" +
	"c=IN IP4 pc33.biloxi.com\r\n" +
	"m=audio 49172 RTP/AVP 0\r\n" +
	"a=rtpmap:0 PCMU/8000\r\n")

func TestFastParser_ParseRequest(t *testing.T) {
	parser := sipparser.NewFastSIPParser()
	msg, err := parser.Parse(testSIPRequest)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	
	if !msg.IsRequest {
		t.Error("Expected request, got response")
	}
	if msg.IsResponse {
		t.Error("Expected request, not response")
	}
	
	if msg.GetMethod() != "INVITE" {
		t.Errorf("Expected method INVITE, got %s", msg.GetMethod())
	}
	
	if len(msg.CallID) == 0 {
		t.Error("Call-ID not parsed")
	}
	
	if msg.GetCallID() != "a84b4c76e66710@pc33.atlanta.com" {
		t.Errorf("Expected Call-ID 'a84b4c76e66710@pc33.atlanta.com', got '%s'", msg.GetCallID())
	}
	
	if len(msg.FromTag) == 0 {
		t.Error("From tag not parsed")
	}
	
	if string(msg.FromTag) != "1928301774" {
		t.Errorf("Expected From tag '1928301774', got '%s'", string(msg.FromTag))
	}
	
	if !msg.HasBody {
		t.Error("Body not detected")
	}
}

func TestFastParser_ParseResponse(t *testing.T) {
	parser := sipparser.NewFastSIPParser()
	msg, err := parser.Parse(testSIPResponse)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	
	if !msg.IsResponse {
		t.Error("Expected response, got request")
	}
	if msg.IsRequest {
		t.Error("Expected response, not request")
	}
	
	if string(msg.ResponseCode) != "200" {
		t.Errorf("Expected response code 200, got %s", string(msg.ResponseCode))
	}
	
	if len(msg.CallID) == 0 {
		t.Error("Call-ID not parsed")
	}
	
	if len(msg.ToTag) == 0 {
		t.Error("To tag not parsed")
	}
	
	if string(msg.ToTag) != "a6c85cf" {
		t.Errorf("Expected To tag 'a6c85cf', got '%s'", string(msg.ToTag))
	}
}

func BenchmarkFastParser_ParseRequest(b *testing.B) {
	parser := sipparser.NewFastSIPParser()
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, err := parser.Parse(testSIPRequest)
		if err != nil {
			b.Fatalf("Parse failed: %v", err)
		}
	}
}

func BenchmarkFastParser_ParseResponse(b *testing.B) {
	parser := sipparser.NewFastSIPParser()
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, err := parser.Parse(testSIPResponse)
		if err != nil {
			b.Fatalf("Parse failed: %v", err)
		}
	}
}

// BenchmarkOldParser_ParseRequest benchmarks the old parser (commented out to avoid dependencies)
// Uncomment and import parser package to enable:
// func BenchmarkOldParser_ParseRequest(b *testing.B) {
// 	for i := 0; i < b.N; i++ {
// 		msg := ParseMsg(string(testSIPRequest), nil, nil)
// 		if msg.Error != nil {
// 			b.Fatalf("Parse failed: %v", msg.Error)
// 		}
// 	}
// }

// BenchmarkOldParser_ParseResponse benchmarks the old parser (commented out to avoid dependencies)
// func BenchmarkOldParser_ParseResponse(b *testing.B) {
// 	for i := 0; i < b.N; i++ {
// 		msg := ParseMsg(string(testSIPResponse), nil, nil)
// 		if msg.Error != nil {
// 			b.Fatalf("Parse failed: %v", msg.Error)
// 		}
// 	}
// }

// BenchmarkComparison compares FastParser performance
func BenchmarkComparison(b *testing.B) {
	b.Run("FastParser", func(b *testing.B) {
		parser := sipparser.NewFastSIPParser()
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = parser.Parse(testSIPRequest)
		}
	})
}

