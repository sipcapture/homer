//go:build !linux

// Package sysctl reads network sysctl knobs used by ingest.
package sysctl

// EffectiveUDPSocketRecvBuffer is a no-op on non-Linux platforms: the
// /proc/sys/net/core/rmem_max knob does not exist there, so the requested
// buffer size is returned unchanged.
func EffectiveUDPSocketRecvBuffer(requested int) int {
	return requested
}
