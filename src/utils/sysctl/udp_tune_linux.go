//go:build linux

package sysctl

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Recommended UDP sysctl values for high-rate HEP ingest (loopback or
// production). Align with server_settings.udp_server.socket_recv_buffer
// default (8 MiB) plus headroom for kernel SO_RCVBUF doubling.
const (
	RecommendedRmemMaxBytes          = 33_554_432 // 32 MiB
	RecommendedWmemMaxBytes          = 33_554_432
	RecommendedRmemDefaultBytes      = 8_388_608  // 8 MiB
	RecommendedWmemDefaultBytes      = 4_194_304  // 4 MiB
	RecommendedNetdevMaxBacklog      = 250_000
)

// ApplyRecommendedUDPBuffers raises net.core rmem/wmem and netdev backlog
// when the process can write /proc/sys (typically root). Idempotent: only
// increases limits that are currently lower than the target.
func ApplyRecommendedUDPBuffers() error {
	return ApplyUDPBuffers(UDPBufferSettings{
		RmemMax:          RecommendedRmemMaxBytes,
		RmemDefault:      RecommendedRmemDefaultBytes,
		WmemMax:          RecommendedWmemMaxBytes,
		WmemDefault:      RecommendedWmemDefaultBytes,
		NetdevMaxBacklog: RecommendedNetdevMaxBacklog,
	})
}

// UDPBufferSettings lists kernel knobs for UDP-heavy workloads.
type UDPBufferSettings struct {
	RmemMax, RmemDefault int
	WmemMax, WmemDefault int
	NetdevMaxBacklog     int
}

// ApplyUDPBuffers writes sysctl values via /proc/sys. Requires CAP_SYS_ADMIN
// or root. Skips knobs that are already at or above the requested value.
func ApplyUDPBuffers(s UDPBufferSettings) error {
	if s.RmemMax > 0 {
		if err := raiseProcInt("net/core/rmem_max", s.RmemMax); err != nil {
			return fmt.Errorf("rmem_max: %w", err)
		}
	}
	if s.RmemDefault > 0 {
		if err := raiseProcInt("net/core/rmem_default", s.RmemDefault); err != nil {
			return fmt.Errorf("rmem_default: %w", err)
		}
	}
	if s.WmemMax > 0 {
		if err := raiseProcInt("net/core/wmem_max", s.WmemMax); err != nil {
			return fmt.Errorf("wmem_max: %w", err)
		}
	}
	if s.WmemDefault > 0 {
		if err := raiseProcInt("net/core/wmem_default", s.WmemDefault); err != nil {
			return fmt.Errorf("wmem_default: %w", err)
		}
	}
	if s.NetdevMaxBacklog > 0 {
		if err := raiseProcInt("net/core/netdev_max_backlog", s.NetdevMaxBacklog); err != nil {
			return fmt.Errorf("netdev_max_backlog: %w", err)
		}
	}
	return nil
}

func raiseProcInt(relPath string, target int) error {
	cur, ok := readProcInt(relPath)
	if ok && cur >= target {
		return nil
	}
	return writeProcInt(relPath, target)
}

func readProcInt(relPath string) (int, bool) {
	data, err := os.ReadFile("/proc/sys/" + relPath)
	if err != nil {
		return 0, false
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}
	return v, true
}

func writeProcInt(relPath string, value int) error {
	return os.WriteFile("/proc/sys/"+relPath, []byte(strconv.Itoa(value)), 0o644)
}
