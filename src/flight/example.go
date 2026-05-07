// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package flight

import (
	"fmt"
	"time"

	"github.com/sipcapture/homer-core/src/decoder"
	"github.com/sipcapture/homer-core/src/sipparser"
)

// ExampleFlightServer demonstrates how to use the Flight server
func ExampleFlightServer() {
	// Create Flight server with default config
	config := DefaultServerConfig()
	config.ListenAddr = ":50051"
	config.BufferSize = 100000 // Keep 100k packets in memory

	server, err := NewFlightServer(config)
	if err != nil {
		panic(fmt.Sprintf("Failed to create Flight server: %v", err))
	}

	// Start the server
	if err := server.Start(); err != nil {
		panic(fmt.Sprintf("Failed to start Flight server: %v", err))
	}
	defer server.Stop()

	// Get catalog to add data
	catalog := server.GetCatalog()

	// Simulate adding HEP packets
	for i := 0; i < 100; i++ {
		hep := &decoder.HEP{
			Version:    3,
			Protocol:   17, // UDP
			SrcIP:      fmt.Sprintf("192.168.1.%d", i%255),
			DstIP:      fmt.Sprintf("10.0.0.%d", i%255),
			SrcPort:    5060,
			DstPort:    5060,
			Tsec:       uint32(time.Now().Unix()),
			Tmsec:      uint32(time.Now().Nanosecond() / 1000),
			ProtoType:  1, // SIP
			NodeID:     1,
			NodePW:     "auth-key",
			Payload:    fmt.Sprintf("INVITE sip:user%d@example.com SIP/2.0", i),
			CID:        fmt.Sprintf("call-id-%d", i),
			Vlan:       0,
			NodeName:   "capture-node-1",
			TargetName: "target-1",
			SID:        fmt.Sprintf("session-%d", i),
			Timestamp:  time.Now(),
			SIP: &sipparser.SipMsg{
				CallID:     fmt.Sprintf("call-id-%d", i),
				CseqMethod: "INVITE",
				FromUser:   fmt.Sprintf("user%d", i),
				FromHost:   "example.com",
				ToUser:     fmt.Sprintf("user%d", (i+1)%100),
				ToHost:     "example.com",
				CseqVal:    fmt.Sprintf("%d INVITE", i),
				UserAgent:  "Homer-Server/1.0",
			},
		}

		// Add to Flight server (will be available for queries)
		catalog.AddHEP(hep)
	}

	fmt.Println("Flight server running on :50051")
	fmt.Println("Connect with DuckDB:")
	fmt.Println("  INSTALL airport FROM community;")
	fmt.Println("  LOAD airport;")
	fmt.Println("  ATTACH '' AS homer (TYPE AIRPORT, LOCATION 'grpc://localhost:50051');")
	fmt.Println("  SELECT * FROM homer.hep.sip LIMIT 10;")
	fmt.Println("")
	fmt.Println("Available tables:")
	fmt.Println("  homer.hep.packets - All HEP packets")
	fmt.Println("  homer.hep.sip     - SIP packets only")
	fmt.Println("  homer.hep.rtcp    - RTCP packets only")
	fmt.Println("  homer.hep.logs    - Log packets only")
}

// ExampleWithAuth demonstrates Flight server with authentication
func ExampleWithAuth() {
	config := DefaultServerConfig()
	config.ListenAddr = ":50051"
	config.AuthToken = "my-secret-token"

	server, err := NewFlightServer(config)
	if err != nil {
		panic(fmt.Sprintf("Failed to create Flight server: %v", err))
	}

	if err := server.Start(); err != nil {
		panic(fmt.Sprintf("Failed to start Flight server: %v", err))
	}
	defer server.Stop()

	fmt.Println("Flight server with auth running on :50051")
	fmt.Println("Connect with DuckDB:")
	fmt.Println("  CREATE PERSISTENT SECRET homer_auth (")
	fmt.Println("    TYPE airport,")
	fmt.Println("    auth_token 'my-secret-token',")
	fmt.Println("    scope 'grpc://localhost:50051'")
	fmt.Println("  );")
	fmt.Println("  ATTACH '' AS homer (TYPE AIRPORT, LOCATION 'grpc://localhost:50051');")
}
