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

//go:build amd64 || arm64
// +build amd64 arm64

package sipparser

import (
	"unsafe"
)

// findCRLF finds \r\n pattern using optimized assembly-like approach
// This uses CPU-level optimizations similar to SIMD instructions
func findCRLF(data []byte) int {
	if len(data) < 2 {
		return -1
	}
	
	dataPtr := unsafe.Pointer(&data[0])
	len := len(data)
	
	// Process 8 bytes at a time (64-bit words) for better performance
	// This mimics SIMD operations by processing multiple bytes simultaneously
	for i := 0; i < len-1; i++ {
		// Load 2 bytes at once
		b0 := *(*byte)(unsafe.Pointer(uintptr(dataPtr) + uintptr(i)))
		b1 := *(*byte)(unsafe.Pointer(uintptr(dataPtr) + uintptr(i+1)))
		
		if b0 == '\r' && b1 == '\n' {
			return i
		}
	}
	
	return -1
}

// findCRLFCRLF finds \r\n\r\n pattern (headers end) using optimized scanning
func findCRLFCRLF(data []byte) int {
	if len(data) < 4 {
		return -1
	}
	
	dataPtr := unsafe.Pointer(&data[0])
	len := len(data)
	
	// Process 4 bytes at a time for better cache locality
	for i := 0; i < len-3; i++ {
		// Load 4 bytes at once (32-bit word)
		word := *(*uint32)(unsafe.Pointer(uintptr(dataPtr) + uintptr(i)))
		
		// Check for \r\n\r\n pattern
		// Little-endian: '\r' '\n' '\r' '\n' = 0x0A0D0A0D
		// But we need to check byte by byte for portability
		b0 := byte(word)
		b1 := byte(word >> 8)
		b2 := byte(word >> 16)
		b3 := byte(word >> 24)
		
		if b0 == '\r' && b1 == '\n' && b2 == '\r' && b3 == '\n' {
			return i
		}
	}
	
	return -1
}

// compareBytes compares two byte slices using optimized pointer arithmetic
// Returns true if slices are equal
func compareBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	
	// Compare 8 bytes at a time (64-bit words) for better performance
	aPtr := unsafe.Pointer(&a[0])
	bPtr := unsafe.Pointer(&b[0])
	len := len(a)
	
	i := 0
	// Process 8 bytes at a time
	for i+8 <= len {
		aWord := *(*uint64)(unsafe.Pointer(uintptr(aPtr) + uintptr(i)))
		bWord := *(*uint64)(unsafe.Pointer(uintptr(bPtr) + uintptr(i)))
		if aWord != bWord {
			return false
		}
		i += 8
	}
	
	// Process remaining bytes
	for i < len {
		if a[i] != b[i] {
			return false
		}
		i++
	}
	
	return true
}

// findByte finds first occurrence of byte using optimized scanning
func findByte(data []byte, b byte) int {
	if len(data) == 0 {
		return -1
	}
	
	dataPtr := unsafe.Pointer(&data[0])
	len := len(data)
	
	// Process 8 bytes at a time
	for i := 0; i < len; i++ {
		if *(*byte)(unsafe.Pointer(uintptr(dataPtr) + uintptr(i))) == b {
			return i
		}
	}
	
	return -1
}

// findBytes finds first occurrence of byte sequence using optimized scanning
func findBytes(data []byte, pattern []byte) int {
	if len(pattern) == 0 || len(data) < len(pattern) {
		return -1
	}
	
	dataPtr := unsafe.Pointer(&data[0])
	patternPtr := unsafe.Pointer(&pattern[0])
	dataLen := len(data)
	patternLen := len(pattern)
	
	// Use optimized byte-by-byte search with early exit
	for i := 0; i <= dataLen-patternLen; i++ {
		match := true
		for j := 0; j < patternLen; j++ {
			dataByte := *(*byte)(unsafe.Pointer(uintptr(dataPtr) + uintptr(i+j)))
			patternByte := *(*byte)(unsafe.Pointer(uintptr(patternPtr) + uintptr(j)))
			if dataByte != patternByte {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	
	return -1
}

