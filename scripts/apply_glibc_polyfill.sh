#!/usr/bin/env bash
# Patch an ELF binary for older glibc using corsix/polyfill-glibc built from a
# pinned git revision. Do not pull ghcr.io/lmangani/polyfill-glibc-action
# (unpinned :latest rewritten the release binary in CI — GHSA-7567-57wg-qcvj).
set -euo pipefail

BINARY="${1:?usage: apply_glibc_polyfill.sh BINARY [GLIBC_TARGET]}"
GLIBC_TARGET="${2:-${GLIBC_TARGET:-2.34}}"

# corsix/polyfill-glibc main @ 2024-10-30 (PR #10).
POLYFILL_GLIBC_COMMIT="${POLYFILL_GLIBC_COMMIT:-dd59051faaa10ee63c1b96f1b47bf9fcd3770ee2}"
NINJA_VERSION="${NINJA_VERSION:-1.13.2}"

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TOOLS_DIR="${ROOT}/.tools"
SRC_DIR="${ROOT}/.polyfill-glibc-src"
NINJA_BIN="${TOOLS_DIR}/ninja"
PF_BIN="${ROOT}/polyfill-glibc"

if [ ! -f "${BINARY}" ]; then
  echo "error: binary not found: ${BINARY}" >&2
  exit 1
fi

if [ ! -x "${PF_BIN}" ]; then
  mkdir -p "${TOOLS_DIR}"
  if [ ! -x "${NINJA_BIN}" ]; then
    echo "==> Downloading ninja ${NINJA_VERSION} ..." >&2
    curl -fsSL -o "${TOOLS_DIR}/ninja.zip" \
      "https://github.com/ninja-build/ninja/releases/download/v${NINJA_VERSION}/ninja-linux.zip"
    unzip -oq "${TOOLS_DIR}/ninja.zip" -d "${TOOLS_DIR}"
    chmod +x "${NINJA_BIN}"
  fi
  if [ ! -d "${SRC_DIR}/.git" ]; then
    echo "==> Cloning corsix/polyfill-glibc @ ${POLYFILL_GLIBC_COMMIT} ..." >&2
    git init "${SRC_DIR}"
    git -C "${SRC_DIR}" remote add origin https://github.com/corsix/polyfill-glibc.git
    git -C "${SRC_DIR}" fetch --depth 1 origin "${POLYFILL_GLIBC_COMMIT}"
    git -C "${SRC_DIR}" checkout --detach FETCH_HEAD
  else
    git -C "${SRC_DIR}" fetch --depth 1 origin "${POLYFILL_GLIBC_COMMIT}"
    git -C "${SRC_DIR}" checkout --detach FETCH_HEAD
  fi
  # go1.26+ on glibc 2.43 may link sqrtf@GLIBC_2.43; upstream renames lack it yet.
  renames="${SRC_DIR}/src/x86_64/renames.txt"
  if [ -f "${renames}" ] && ! grep -q 'sqrtf@GLIBC_2.43' "${renames}"; then
    echo 'sqrtf@GLIBC_2.43 sqrtf@GLIBC_2.2.5' >> "${renames}"
  fi
  echo "==> Building polyfill-glibc from source ..." >&2
  (cd "${SRC_DIR}" && "${NINJA_BIN}" polyfill-glibc)
  cp "${SRC_DIR}/polyfill-glibc" "${PF_BIN}"
  chmod +x "${PF_BIN}"
fi

echo "==> Applying glibc polyfill (target ${GLIBC_TARGET}) to ${BINARY} ..."
"${PF_BIN}" --target-glibc="${GLIBC_TARGET}" "${BINARY}"
if objdump -T "${BINARY}" | grep -qE '@GLIBC_2\.(3[5-9]|[4-9][0-9])'; then
  echo "error: ${BINARY} still references GLIBC > ${GLIBC_TARGET} after polyfill" >&2
  objdump -T "${BINARY}" | grep GLIBC | sort -u >&2 || true
  exit 1
fi
echo "    OK: glibc <= ${GLIBC_TARGET}"
