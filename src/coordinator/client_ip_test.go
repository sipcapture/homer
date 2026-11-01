// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package coordinator

import (
	"net/http"
	"testing"
)

func TestClientIPExtractor(t *testing.T) {
	extract := clientIPExtractor()
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		realIP     string
		want       string
	}{
		{
			name:       "public peer ignores spoofed xff",
			remoteAddr: "203.0.113.9:443",
			xff:        "198.51.100.20",
			want:       "203.0.113.9",
		},
		{
			name:       "public peer ignores spoofed x-real-ip",
			remoteAddr: "203.0.113.9:443",
			realIP:     "198.51.100.20",
			want:       "203.0.113.9",
		},
		{
			name:       "loopback proxy keeps xff client",
			remoteAddr: "127.0.0.1:1234",
			xff:        "198.51.100.20",
			want:       "198.51.100.20",
		},
		{
			name:       "private proxy keeps x-real-ip client",
			remoteAddr: "10.1.2.3:80",
			realIP:     "198.51.100.20",
			want:       "198.51.100.20",
		},
		{
			name:       "private proxy walks xff to the client",
			remoteAddr: "10.1.2.3:80",
			xff:        "198.51.100.20, 10.1.2.3",
			want:       "198.51.100.20",
		},
		{
			name:       "no forwarding headers uses the peer",
			remoteAddr: "203.0.113.9:443",
			want:       "203.0.113.9",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "http://homer.example/", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.realIP != "" {
				req.Header.Set("X-Real-IP", tt.realIP)
			}
			if got := extract(req); got != tt.want {
				t.Fatalf("client IP = %q, want %q", got, tt.want)
			}
		})
	}
}
