#!/usr/bin/env bash
# Raise storage.ducklake.batch_size on a local homer-core stand (fewer flushes, higher pps).
# Requires root to edit config and restart systemd.
set -euo pipefail

BATCH_SIZE="${HOMER_STORAGE_DUCKLAKE_BATCH_SIZE:-25000}"
CONFIG="${HOMER_CONFIG:-/usr/local/homer-core/etc/homer.json}"
SERVICE="${HOMER_SERVICE:-homer-core}"
RESTART="${RESTART:-1}"

usage() {
  cat <<EOF
Usage: sudo $0 [batch_size]

  batch_size   default: ${BATCH_SIZE} (or set HOMER_STORAGE_DUCKLAKE_BATCH_SIZE)

Patches "\$CONFIG" and restarts \${SERVICE} when RESTART=1.

Alternative without editing JSON (survives package upgrades if drop-in kept):
  sudo mkdir -p /etc/systemd/system/\${SERVICE}.service.d
  printf '%s\n' '[Service]' "Environment=HOMER_STORAGE_DUCKLAKE_BATCH_SIZE=${BATCH_SIZE}" \\
    | sudo tee /etc/systemd/system/\${SERVICE}.service.d/ducklake-batch.conf
  sudo systemctl daemon-reload && sudo systemctl restart \${SERVICE}
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ -n "${1:-}" ]]; then
  BATCH_SIZE="$1"
fi

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Run as root: sudo $0 [batch_size]" >&2
  exit 1
fi

if [[ ! -f "$CONFIG" ]]; then
  echo "Config not found: $CONFIG" >&2
  exit 1
fi

python3 - "$CONFIG" "$BATCH_SIZE" <<'PY'
import json, sys
path, size = sys.argv[1], int(sys.argv[2])
with open(path, encoding="utf-8") as f:
    cfg = json.load(f)
dl = cfg.setdefault("storage", {}).setdefault("ducklake", {})
old = dl.get("batch_size")
dl["batch_size"] = size
with open(path, "w", encoding="utf-8") as f:
    json.dump(cfg, f, indent=2)
    f.write("\n")
print(f"OK: {path} batch_size {old} -> {size}")
PY

if [[ "$RESTART" == "1" ]]; then
  if systemctl is-active --quiet "$SERVICE" 2>/dev/null; then
    systemctl restart "$SERVICE"
    echo "OK: restarted $SERVICE"
  else
    echo "Note: $SERVICE not active; config updated only"
  fi
fi
