#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
PLUGIN_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
BIN_DIR="$PLUGIN_DIR/bin"
TARGET="$BIN_DIR/t4memd"

OWNER=${T4MEM_RELEASE_OWNER:-t4db}
REPO=${T4MEM_RELEASE_REPO:-t4mem}
VERSION=${T4MEM_VERSION:-latest}

case "$(uname -s)" in
  Darwin) OS=darwin ;;
  Linux) OS=linux ;;
  *)
    echo "unsupported OS: $(uname -s)" >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *)
    echo "unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

ASSET_BASENAME="t4memd_${OS}_${ARCH}"
if [ "$VERSION" = "latest" ]; then
  URL="https://github.com/$OWNER/$REPO/releases/latest/download/${ASSET_BASENAME}.tar.gz"
  CHECKSUMS_URL="https://github.com/$OWNER/$REPO/releases/latest/download/t4memd_checksums.txt"
else
  URL="https://github.com/$OWNER/$REPO/releases/download/$VERSION/${ASSET_BASENAME}.tar.gz"
  CHECKSUMS_URL="https://github.com/$OWNER/$REPO/releases/download/$VERSION/t4memd_checksums.txt"
fi

TMP_DIR=$(mktemp -d)
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

mkdir -p "$BIN_DIR"

ARCHIVE="$TMP_DIR/${ASSET_BASENAME}.tar.gz"
CHECKSUMS_FILE="$TMP_DIR/t4memd_checksums.txt"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$URL" -o "$ARCHIVE"
  curl -fsSL "$CHECKSUMS_URL" -o "$CHECKSUMS_FILE"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "$ARCHIVE" "$URL"
  wget -qO "$CHECKSUMS_FILE" "$CHECKSUMS_URL"
else
  echo "need curl or wget to download release assets" >&2
  exit 1
fi

EXPECTED_SUM=$(awk -v asset="${ASSET_BASENAME}.tar.gz" '$2 ~ ("(^|/)" asset "$") { print $1; exit }' "$CHECKSUMS_FILE")
if [ -z "$EXPECTED_SUM" ]; then
  echo "could not find checksum for ${ASSET_BASENAME}.tar.gz in $(basename "$CHECKSUMS_FILE")" >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL_SUM=$(sha256sum "$ARCHIVE" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL_SUM=$(shasum -a 256 "$ARCHIVE" | awk '{print $1}')
else
  echo "need sha256sum or shasum to verify release asset" >&2
  exit 1
fi

if [ "$EXPECTED_SUM" != "$ACTUAL_SUM" ]; then
  echo "checksum mismatch for ${ASSET_BASENAME}.tar.gz" >&2
  exit 1
fi

tar -xzf "$ARCHIVE" -C "$TMP_DIR"

if [ ! -f "$TMP_DIR/t4memd" ]; then
  echo "release archive did not contain t4memd" >&2
  exit 1
fi

mv "$TMP_DIR/t4memd" "$TARGET"
chmod 755 "$TARGET"

echo "installed $TARGET from $URL"
