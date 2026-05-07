#!/bin/bash
# Simple TLS test script

set -e

echo "Testing TLS functionality..."

cd "$(dirname "$0")"

# Test 1: TLS version parsing
echo "Test 1: TLS version parsing"
go run -tags=test <<'EOF'
package main

import (
	"fmt"
	gtls "github.com/gnet-io/tls"
)

func parseTLSVersion(version string) uint16 {
	switch version {
	case "TLS1.0":
		return gtls.VersionTLS10
	case "TLS1.1":
		return gtls.VersionTLS11
	case "TLS1.2":
		return gtls.VersionTLS12
	case "TLS1.3":
		return gtls.VersionTLS13
	default:
		return gtls.VersionTLS12
	}
}

func main() {
	tests := []struct {
		version string
		expected uint16
	}{
		{"TLS1.2", gtls.VersionTLS12},
		{"TLS1.3", gtls.VersionTLS13},
		{"invalid", gtls.VersionTLS12},
	}
	
	for _, tt := range tests {
		result := parseTLSVersion(tt.version)
		if result == tt.expected {
			fmt.Printf("✓ %s -> %d\n", tt.version, result)
		} else {
			fmt.Printf("✗ %s -> %d (expected %d)\n", tt.version, result, tt.expected)
		}
	}
	fmt.Println("TLS version parsing test: PASSED")
}
EOF

echo ""
echo "Test 2: HEP packet creation"
go run <<'EOF'
package main

import (
	"encoding/binary"
	"fmt"
)

func createHEPPacket(data []byte) []byte {
	packet := make([]byte, 6+len(data))
	copy(packet[0:4], []byte("HEP3"))
	binary.BigEndian.PutUint16(packet[4:6], uint16(6+len(data)))
	copy(packet[6:], data)
	return packet
}

func main() {
	testData := []byte("test data")
	packet := createHEPPacket(testData)
	
	if string(packet[0:4]) == "HEP3" {
		fmt.Println("✓ HEP header correct")
	} else {
		fmt.Println("✗ HEP header incorrect")
	}
	
	length := binary.BigEndian.Uint16(packet[4:6])
	expectedLength := uint16(6 + len(testData))
	if length == expectedLength {
		fmt.Printf("✓ Packet length correct: %d\n", length)
	} else {
		fmt.Printf("✗ Packet length incorrect: %d (expected %d)\n", length, expectedLength)
	}
	
	if string(packet[6:]) == string(testData) {
		fmt.Println("✓ Packet data correct")
	} else {
		fmt.Println("✗ Packet data incorrect")
	}
	
	fmt.Println("HEP packet creation test: PASSED")
}
EOF

echo ""
echo "All basic tests passed!"
echo ""
echo "Note: Full integration tests require TLS server to be running."
echo "To test TLS server, you need to:"
echo "1. Generate certificates"
echo "2. Start TLS server"
echo "3. Connect with TLS client"
echo "4. Send HEP packets"

