#!/usr/bin/env bash
# Pre-download DuckDB extensions for deb/rpm (offline-capable installs).
# Install layout matches systemd: Environment HOME=/usr/local/homer-core
# -> ~/.duckdb/extensions/<version>/<platform>/...
#
# DUCKDB_VERSION must stay in sync with embedded DuckDB from
# github.com/duckdb/duckdb-go-bindings in src/go.mod (e.g. v0.10505.x -> v1.5.5).
#
# Usage:
#   ./scripts/download_duckdb_extensions.sh [EXT_PLATFORM]
#   If EXT_PLATFORM is omitted, it is derived from uname (linux_amd64, …).

set -euo pipefail

DUCKDB_VERSION="${DUCKDB_VERSION:-v1.5.5}"

if [ -n "${1:-}" ]; then
  EXT_PLATFORM="$1"
else
  arch=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  EXT_PLATFORM="${os}_${arch}"
fi

ext_dir="bundled_extensions/${DUCKDB_VERSION}/${EXT_PLATFORM}"
mkdir -p "${ext_dir}"

echo "Downloading DuckDB ${DUCKDB_VERSION} extensions for ${EXT_PLATFORM}..."

# httpfs + aws are required for S3 secrets using PROVIDER credential_chain
# (IAM-role / instance-profile credentials); aws depends on httpfs.
for ext in ducklake httpfs aws; do
  url="https://extensions.duckdb.org/${DUCKDB_VERSION}/${EXT_PLATFORM}/${ext}.duckdb_extension.gz"
  dest="${ext_dir}/${ext}.duckdb_extension"
  echo "  ${url} -> ${dest}"
  curl -fsSL "${url}" | gunzip > "${dest}"
done

echo "  sqlite_scanner (as sqlite_scanner.duckdb_extension)..."
curl -fsSL "https://extensions.duckdb.org/${DUCKDB_VERSION}/${EXT_PLATFORM}/sqlite_scanner.duckdb_extension.gz" \
  | gunzip > "${ext_dir}/sqlite_scanner.duckdb_extension"

echo "Extensions downloaded to ${ext_dir}/"
