//go:build !linux

package sysctl

// ApplyRecommendedUDPBuffers is a no-op on non-Linux platforms.
func ApplyRecommendedUDPBuffers() error {
	return nil
}

// ApplyUDPBuffers is a no-op on non-Linux platforms.
func ApplyUDPBuffers(_ UDPBufferSettings) error {
	return nil
}

// UDPBufferSettings lists kernel knobs for UDP-heavy workloads (Linux only).
type UDPBufferSettings struct {
	RmemMax, RmemDefault int
	WmemMax, WmemDefault int
	NetdevMaxBacklog     int
}
