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

# ── Compile homer ──────────────────────────────────────────────────────────────
echo "==> Compiling ${BINARY} ..."
LDFLAGS="-s -w \
  -X main.VERSION_APPLICATION=${VERSION} \
  -X main.BuildDate=${BUILD_DATE} \
  -X main.BuildTime=${BUILD_TIME} \
  -X main.GitCommit=${GIT_COMMIT}"

cd "${SRC_DIR}"
CGO_ENABLED=1 go build -ldflags "${LDFLAGS}" -o "${BUILD_DIR}/${BINARY}"
cd "${BUILD_DIR}"

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

echo "==> Packaging ${PACKAGE}_${VERSION}_${ARCH}.deb ..."
VERSION="${VERSION}" "${BUILD_DIR}/nfpm" pkg \
  --config "${NFPM_CONFIG}" \
  --target "${BUILD_DIR}/${PACKAGE}_${VERSION}_${ARCH}.deb"

echo "==> Packaging ${PACKAGE}_${VERSION}_${ARCH}.rpm ..."
VERSION="${VERSION}" "${BUILD_DIR}/nfpm" pkg \
  --config "${NFPM_CONFIG}" \
  --target "${BUILD_DIR}/${PACKAGE}_${VERSION}_${ARCH}.rpm"

echo ""
echo "==> Done:"
ls -lh "${BUILD_DIR}/${PACKAGE}_${VERSION}_${ARCH}".{deb,rpm} 2>/dev/null || true
