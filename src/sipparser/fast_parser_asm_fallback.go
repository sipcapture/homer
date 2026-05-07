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

//go:build !amd64 && !arm64
// +build !amd64,!arm64

package sipparser

import (
	"unsafe"
)

// Fallback implementations for platforms without assembly optimizations

func findCRLF(data []byte) int {
	for i := 0; i < len(data)-1; i++ {
		if data[i] == '\r' && data[i+1] == '\n' {
			return i
		}
	}
	return -1
}

func findCRLFCRLF(data []byte) int {
	for i := 0; i < len(data)-3; i++ {
		if data[i] == '\r' && data[i+1] == '\n' && data[i+2] == '\r' && data[i+3] == '\n' {
			return i
		}
	}
	return -1
}

func compareBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	
	aPtr := unsafe.Pointer(&a[0])
	bPtr := unsafe.Pointer(&b[0])
	len := len(a)
	
	for i := 0; i < len; i++ {
		aByte := *(*byte)(unsafe.Pointer(uintptr(aPtr) + uintptr(i)))
		bByte := *(*byte)(unsafe.Pointer(uintptr(bPtr) + uintptr(i)))
		if aByte != bByte {
			return false
		}
	}
	
	return true
}

func findByte(data []byte, b byte) int {
	for i := 0; i < len(data); i++ {
		if data[i] == b {
			return i
		}
	}
	return -1
}

func findBytes(data []byte, pattern []byte) int {
	if len(pattern) == 0 || len(data) < len(pattern) {
		return -1
	}
	
	dataPtr := unsafe.Pointer(&data[0])
	patternPtr := unsafe.Pointer(&pattern[0])
	dataLen := len(data)
	patternLen := len(pattern)
	
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

