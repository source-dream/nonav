#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="$ROOT_DIR/bin"
DIST_DIR="$ROOT_DIR/dist"

resolve_version() {
  if [[ -n "${VERSION:-}" ]]; then
    echo "$VERSION"
    return
  fi

  if ! command -v git >/dev/null 2>&1; then
    echo "snapshot-$(date +%Y%m%d-%H%M%S)"
    return
  fi

  local head_tag latest_tag short_sha dirty_suffix
  head_tag="$(git -C "$ROOT_DIR" tag --points-at HEAD | head -n 1 || true)"
  latest_tag="$(git -C "$ROOT_DIR" describe --abbrev=0 --tags 2>/dev/null || true)"
  short_sha="$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo unknown)"

  dirty_suffix=""
  if ! git -C "$ROOT_DIR" diff --quiet --ignore-submodules -- 2>/dev/null || ! git -C "$ROOT_DIR" diff --cached --quiet --ignore-submodules -- 2>/dev/null; then
    dirty_suffix="-dirty"
  fi

  if [[ -n "$head_tag" ]]; then
    echo "${head_tag}${dirty_suffix}"
    return
  fi

  if [[ -n "$latest_tag" ]]; then
    echo "${latest_tag}-${short_sha}${dirty_suffix}"
    return
  fi

  echo "snapshot-${short_sha}${dirty_suffix}"
}

VERSION="$(resolve_version)"

mkdir -p "$DIST_DIR"
rm -f "$DIST_DIR/nonav-${VERSION}-linux-amd64" "$DIST_DIR/nonav-gateway-${VERSION}-linux-amd64" "$DIST_DIR/SHA256SUMS-${VERSION}.txt"

echo "[release] version: $VERSION"
echo "[release] building release artifacts..."
make -C "$ROOT_DIR" build

cp "$BIN_DIR/nonav" "$DIST_DIR/nonav-${VERSION}-linux-amd64"
cp "$BIN_DIR/nonav-gateway" "$DIST_DIR/nonav-gateway-${VERSION}-linux-amd64"

(cd "$DIST_DIR" && sha256sum "nonav-${VERSION}-linux-amd64" "nonav-gateway-${VERSION}-linux-amd64" > "SHA256SUMS-${VERSION}.txt")

echo "[release] done"
ls -lh "$DIST_DIR/nonav-${VERSION}-linux-amd64" "$DIST_DIR/nonav-gateway-${VERSION}-linux-amd64" "$DIST_DIR/SHA256SUMS-${VERSION}.txt"
