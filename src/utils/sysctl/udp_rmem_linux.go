// Package sysctl reads Linux network sysctl knobs used by ingest.
package sysctl

import (
	"fmt"
	"os"
	"strings"
)

// EffectiveUDPSocketRecvBuffer caps requested SO_RCVBUF-style sizes by
// net.core.rmem_max when /proc is readable. If rmem_max is unknown or not
// below requested, returns requested unchanged.
func EffectiveUDPSocketRecvBuffer(requested int) int {
	if requested <= 0 {
		return requested
	}
	max, ok := readRmemMax()
	if !ok || max <= 0 || max >= requested {
		return requested
	}
	return max
}

func readRmemMax() (int, bool) {
	data, err := os.ReadFile("/proc/sys/net/core/rmem_max")
	if err != nil {
		return 0, false
	}
	var rmemMax int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &rmemMax); err != nil {
		return 0, false
	}
	return rmemMax, true
}
