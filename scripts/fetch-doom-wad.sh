#!/usr/bin/env bash
# Download the shareware doom1.wad for the dashboard Doom widget.
#
# The WAD is served at runtime by the coordinator's /gamedata/ route
# (config key `gamedata_dir`) and is deliberately kept out of the UI
# build so it never gets embedded into the homer-core binary.
#
# Usage: ./scripts/fetch-doom-wad.sh [target_dir]   (default: ./gamedata)
set -euo pipefail

TARGET_DIR="${1:-./gamedata}"
WAD="${TARGET_DIR}/doom1.wad"

# Shareware DOOM1.WAD v1.9 (freely distributable per the original
# shareware license). Same copy the cloudflare/doom-wasm demo uses.
URL="https://silentspacemarine.com/doom1.wad"
SHA256="1d7d43be501e67d927e415e0b8f3e29c3bf33075e859721816f652a526cac771"

if [ -f "${WAD}" ]; then
    if echo "${SHA256}  ${WAD}" | sha256sum -c --quiet - 2>/dev/null; then
        echo "OK: ${WAD} already present and checksum matches."
        exit 0
    fi
    echo "WARN: ${WAD} exists but checksum differs (custom WAD?) — leaving it untouched."
    exit 0
fi

mkdir -p "${TARGET_DIR}"
echo "Downloading shareware doom1.wad -> ${WAD}"
curl -fSL --max-time 300 "${URL}" -o "${WAD}.tmp"

echo "${SHA256}  ${WAD}.tmp" | sha256sum -c --quiet - || {
    echo "ERROR: checksum mismatch, removing download." >&2
    rm -f "${WAD}.tmp"
    exit 1
}
mv "${WAD}.tmp" "${WAD}"
echo "OK: $(ls -la "${WAD}" | awk '{print $5}') bytes. Set coordinator gamedata_dir to $(cd "${TARGET_DIR}" && pwd)"
