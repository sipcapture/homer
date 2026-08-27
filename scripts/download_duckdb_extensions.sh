#!/usr/bin/env bash
# Pre-download DuckDB extensions for deb/rpm (offline-capable installs).
# Install layout matches systemd: Environment HOME=/usr/local/homer-core
# -> ~/.duckdb/extensions/<version>/<platform>/...
#
# DUCKDB_VERSION must stay in sync with embedded DuckDB from
# github.com/duckdb/duckdb-go-bindings in src/go.mod (e.g. v0.10505.x -> v1.5.5).
#
# Gzip blobs are SHA-256 pinned in scripts/duckdb_extension_checksums.txt
# (GHSA-vqh9-j3rh-cj62). Update that file when bumping DUCKDB_VERSION.
#
# Usage:
#   ./scripts/download_duckdb_extensions.sh [EXT_PLATFORM]
#   If EXT_PLATFORM is omitted, it is derived from uname (linux_amd64, …).

set -euo pipefail

DUCKDB_VERSION="${DUCKDB_VERSION:-v1.5.5}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CHECKSUMS="${SCRIPT_DIR}/duckdb_extension_checksums.txt"

if [ -n "${1:-}" ]; then
  EXT_PLATFORM="$1"
else
  arch=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  EXT_PLATFORM="${os}_${arch}"
fi

if [ ! -f "${CHECKSUMS}" ]; then
  echo "error: missing checksum file ${CHECKSUMS}" >&2
  exit 1
fi

expected_sha256() {
  local key="$1"
  awk -v k="${key}" '$1 !~ /^#/ && NF>=2 && $2==k {print $1; found=1; exit} END{exit !found}' "${CHECKSUMS}"
}

ext_dir="bundled_extensions/${DUCKDB_VERSION}/${EXT_PLATFORM}"
mkdir -p "${ext_dir}"

echo "Downloading DuckDB ${DUCKDB_VERSION} extensions for ${EXT_PLATFORM}..."

# httpfs + aws are required for S3 secrets using PROVIDER credential_chain
# (IAM-role / instance-profile credentials); aws depends on httpfs. azure is
# the equivalent for Azure Blob Storage secrets (including Managed Identity
# via PROVIDER credential_chain).
for ext in ducklake httpfs aws azure sqlite_scanner; do
  rel="${DUCKDB_VERSION}/${EXT_PLATFORM}/${ext}.duckdb_extension.gz"
  url="https://extensions.duckdb.org/${rel}"
  dest="${ext_dir}/${ext}.duckdb_extension"
  expected="$(expected_sha256 "${rel}" || true)"
  if [ -z "${expected}" ]; then
    echo "error: no SHA-256 pin for ${rel} in ${CHECKSUMS}" >&2
    exit 1
  fi
  echo "  ${url} -> ${dest}"
  gz_tmp="$(mktemp)"
  trap 'rm -f "${gz_tmp}"' EXIT
  curl -fsSL -o "${gz_tmp}" "${url}"
  actual="$(sha256sum "${gz_tmp}" | awk '{print $1}')"
  if [ "${actual}" != "${expected}" ]; then
    echo "error: SHA-256 mismatch for ${rel}" >&2
    echo "  expected ${expected}" >&2
    echo "  got      ${actual}" >&2
    exit 1
  fi
  gunzip -c "${gz_tmp}" > "${dest}"
  rm -f "${gz_tmp}"
  trap - EXIT
done

echo "Extensions downloaded to ${ext_dir}/"
