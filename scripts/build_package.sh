#!/bin/bash
# Build script mirroring .github/workflows/release.yml
set -e

PACKAGE="homer-core"
BINARY="homer"
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
BUILD_DIR="$PWD"
SRC_DIR="$BUILD_DIR/src"
NFPM_VERSION="2.41.1"


# ── Remove old packages (glob must stay *outside* quotes or '*' is literal) ───
shopt -s nullglob
for f in "${BUILD_DIR}/${PACKAGE}"_*.deb "${BUILD_DIR}/${PACKAGE}"_*.rpm; do
  rm -f "$f"
done
shopt -u nullglob

# ── Version (from version.go — source of truth) ────────────────────────────────
VERSION=$(grep 'VERSION_APPLICATION = ' "$SRC_DIR/version.go" 2>/dev/null \
  | cut -d'"' -f2 | head -1)
if [ -z "$VERSION" ]; then
  VERSION=$(git describe --tags --exact-match 2>/dev/null | sed 's/^v//' || true)
fi
VERSION="${VERSION:-0.0.0}"

GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(date +%Y-%m-%d)
BUILD_TIME=$(date +%H:%M:%S)

echo "==> Building $PACKAGE $VERSION (commit $GIT_COMMIT, arch $ARCH)"

# ── System dependencies ────────────────────────────────────────────────────────
echo "==> Checking system dependencies ..."
if ! dpkg -s libluajit-5.1-dev >/dev/null 2>&1; then
  echo "    Installing: libluajit-5.1-dev"
  sudo apt update -q && sudo apt install -y libluajit-5.1-dev
fi

# ── Frontend ───────────────────────────────────────────────────────────────────
if [ -d "${SRC_DIR}/ui" ]; then
  echo "==> Building frontend ..."
  if ! node -e '
    const [a, b, c = 0] = process.version.slice(1).split(".").map(Number);
    const ok =
      a >= 23 ||
      (a === 22 && (b > 12 || (b === 12 && c >= 12))) ||
      (a === 21) ||
      (a === 20 && (b > 19 || (b === 19 && c >= 0)));
    if (!ok) {
      console.error("Node.js " + process.version + " is too old for this UI (Vite + Tailwind v4).");
      console.error("Use Node.js 20.19+ or 22.12+ (e.g. nvm install 22 && nvm use 22), then:");
      console.error("  rm -rf node_modules && npm ci");
      process.exit(1);
    }
  '; then
    exit 1
  fi
  cd "${SRC_DIR}/ui"
  npm ci --silent
  npm run build --silent
  cd "${BUILD_DIR}"
fi

# ── Compile homer (static LuaJIT + amd64 static libstdc++ for DuckDB, same as CI) ─
echo "==> Compiling ${BINARY} ..."
LDFLAGS="-s -w \
  -X main.VERSION_APPLICATION=${VERSION} \
  -X main.BuildDate=${BUILD_DATE} \
  -X main.BuildTime=${BUILD_TIME} \
  -X main.GitCommit=${GIT_COMMIT}"
STATIC_LINK_DIR="${BUILD_DIR}/.static-cgo-link"
mkdir -p "${STATIC_LINK_DIR}"
if [ "$ARCH" = "amd64" ]; then
  LIBSTDCXX_A=$(gcc --print-file-name=libstdc++.a 2>/dev/null)
  if [ -n "${LIBSTDCXX_A}" ] && [ -f "${LIBSTDCXX_A}" ]; then
    ln -sf "${LIBSTDCXX_A}" "${STATIC_LINK_DIR}/libstdc++.a"
  fi
  LUAJIT_A=$(gcc --print-file-name=libluajit-5.1.a 2>/dev/null)
  LDFLAGS="${LDFLAGS} -extldflags '-static-libgcc -ldl'"
elif [ "$ARCH" = "arm64" ]; then
  LUAJIT_A=$(gcc --print-file-name=libluajit-5.1.a 2>/dev/null)
else
  echo "Unsupported arch: $ARCH" >&2
  exit 1
fi
if [ -z "${LUAJIT_A}" ] || [ ! -f "${LUAJIT_A}" ]; then
  echo "error: libluajit-5.1.a not found (install libluajit-5.1-dev)" >&2
  exit 1
fi
ln -sf "${LUAJIT_A}" "${STATIC_LINK_DIR}/libluajit-5.1.a"
export CGO_LDFLAGS="-L${STATIC_LINK_DIR} -Wl,-Bstatic -lluajit-5.1 -Wl,-Bdynamic"

cd "${SRC_DIR}"
CGO_ENABLED=1 go build -ldflags "${LDFLAGS}" -o "${BUILD_DIR}/${BINARY}"
cd "${BUILD_DIR}"

if [ "$ARCH" = "amd64" ] && ldd "${BINARY}" 2>/dev/null | grep -q libluajit; then
  echo "error: ${BINARY} still links libluajit dynamically" >&2
  ldd "${BINARY}" || true
  exit 1
fi
if [ "$ARCH" = "arm64" ] && command -v ldd >/dev/null && ldd "${BINARY}" 2>/dev/null | grep -q libluajit; then
  echo "error: ${BINARY} still links libluajit dynamically" >&2
  ldd "${BINARY}" || true
  exit 1
fi

echo "    Binary: $(ls -lh "${BUILD_DIR}/${BINARY}" | awk '{print $5, $9}')"

# ── Download nfpm (same version as CI) ────────────────────────────────────────
if [ ! -x "${BUILD_DIR}/nfpm" ]; then
  echo "==> Downloading nfpm v${NFPM_VERSION} ..."
  NFPM_ARCH="x86_64"
  [ "$ARCH" = "arm64" ] && NFPM_ARCH="arm64"
  wget -qO- "https://github.com/goreleaser/nfpm/releases/download/v${NFPM_VERSION}/nfpm_${NFPM_VERSION}_Linux_${NFPM_ARCH}.tar.gz" \
    | tar --directory "${BUILD_DIR}" -xz nfpm
  chmod +x "${BUILD_DIR}/nfpm"
fi

# ── Create deb/rpm packages ────────────────────────────────────────────────────
NFPM_CONFIG="${BUILD_DIR}/homer-core_workflow.yaml"

# Patch arch in nfpm config (CI does the same via sed)
sed -i "s|arch: \".*\"|arch: \"${ARCH}\"|g" "${NFPM_CONFIG}"

echo "==> Downloading DuckDB extensions for ${ARCH} ..."
if [ "$ARCH" = "amd64" ]; then EXT_PLATFORM=linux_amd64; else EXT_PLATFORM=linux_arm64; fi
DUCKDB_VERSION=v1.5.3 "${BUILD_DIR}/scripts/download_duckdb_extensions.sh" "$EXT_PLATFORM"

echo "==> Packaging ${PACKAGE}_${VERSION}_${ARCH}.deb ..."
DUCKDB_VERSION=v1.5.3 EXT_PLATFORM="${EXT_PLATFORM}" VERSION="${VERSION}" "${BUILD_DIR}/nfpm" pkg \
  --config "${NFPM_CONFIG}" \
  --target "${BUILD_DIR}/${PACKAGE}_${VERSION}_${ARCH}.deb"

echo "==> Packaging ${PACKAGE}_${VERSION}_${ARCH}.rpm ..."
DUCKDB_VERSION=v1.5.3 EXT_PLATFORM="${EXT_PLATFORM}" VERSION="${VERSION}" "${BUILD_DIR}/nfpm" pkg \
  --config "${NFPM_CONFIG}" \
  --target "${BUILD_DIR}/${PACKAGE}_${VERSION}_${ARCH}.rpm"

echo ""
echo "==> Done:"
ls -lh "${BUILD_DIR}/${PACKAGE}_${VERSION}_${ARCH}".{deb,rpm} 2>/dev/null || true
