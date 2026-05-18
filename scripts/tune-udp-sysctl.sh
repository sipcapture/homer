#!/usr/bin/env bash
# Apply Homer-recommended UDP sysctl values (requires root).
set -euo pipefail

CONF="${1:-$(dirname "$0")/../examples/sysctl/99-homer-udp-buffers.conf}"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run as root: sudo $0" >&2
  exit 1
fi

if [[ -f "$CONF" ]]; then
  sysctl -p "$CONF"
else
  sysctl -w net.core.rmem_max=33554432
  sysctl -w net.core.rmem_default=8388608
  sysctl -w net.core.wmem_max=33554432
  sysctl -w net.core.wmem_default=4194304
  sysctl -w net.core.netdev_max_backlog=250000
fi

echo "OK: $(sysctl -n net.core.rmem_max) rmem_max, $(sysctl -n net.core.wmem_max) wmem_max"
