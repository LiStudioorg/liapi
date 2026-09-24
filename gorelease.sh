#!/bin/bash
set -euo pipefail

# gorelease — cross-compile release binaries for common platforms.
# Pure Go (CGO_ENABLED=0), no NDK needed. For Android/iOS use buildrelease.sh.
#
# Usage:
#   ./gorelease.sh                 # all default targets
#   ./gorelease.sh linux/amd64     # single target
#   VERSION=1.2.3 ./gorelease.sh   # explicit version (default: git tag / dev)

APP_NAME="${APP_NAME:-liapi}"
VERSION="${VERSION:-${GITHUB_REF_NAME:-}}"
VERSION="${VERSION#v}"
if [ -z "$VERSION" ]; then
  VERSION="$(git describe --tags --always 2>/dev/null || echo dev)"
fi
DIST_DIR="${DIST_DIR:-dist}"
VERSION_SYMBOL="${VERSION_SYMBOL:-main.version}"

log()  { echo -e "\033[1;34m[INFO]\033[0m $*"; }
err()  { echo -e "\033[1;31m[ERR ]\033[0m $*" >&2; }

DEFAULT_TARGETS=(
  "linux/amd64"
  "linux/arm64"
  "linux/arm"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
  "windows/arm64"
)

ext_for() {
  case "$1" in
    windows) echo ".exe" ;;
    *) echo "" ;;
  esac
}

build_one() {
  local goos="$1" goarch="$2"
  local ext; ext="$(ext_for "$goos")"
  local out="${DIST_DIR}/${APP_NAME}_${VERSION}_${goos}_${goarch}${ext}"

  log "building ${goos}/${goarch} → ${out}"
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
    go build -trimpath -ldflags "-s -w -X ${VERSION_SYMBOL}=${VERSION}" -o "$out" .

  case "$goos" in
    windows)
      (cd "$DIST_DIR" && zip -q "$(basename "${out%.exe}").zip" "$(basename "$out")")
      rm -f "$out"
      ;;
    *)
      (cd "$DIST_DIR" && tar -czf "$(basename "${out}.tar.gz")" "$(basename "$out")")
      rm -f "$out"
      ;;
  esac
}

mkdir -p "$DIST_DIR"
log "version=${VERSION}"

if [ $# -gt 0 ]; then
  for t in "$@"; do
    build_one "${t%/*}" "${t#*/}"
  done
else
  for t in "${DEFAULT_TARGETS[@]}"; do
    build_one "${t%/*}" "${t#*/}" || err "failed: $t"
  done
fi

log "checksums:"
(cd "$DIST_DIR" && sha256sum -- * 2>/dev/null | tee SHA256SUMS || true)
log "done → ${DIST_DIR}/"
