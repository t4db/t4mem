#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
PLUGIN_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
REPO_ROOT=$(CDPATH= cd -- "$PLUGIN_DIR/../.." && pwd)
BIN_PATH=${T4MEMD_BIN:-"$PLUGIN_DIR/bin/t4memd"}
ROOT_DIR=${T4MEM_ROOT:-"$REPO_ROOT/.t4mem"}
INSTALL_SCRIPT="$PLUGIN_DIR/scripts/install_t4memd_release.sh"

if [ ! -x "$BIN_PATH" ]; then
  echo "t4memd binary not found at $BIN_PATH; installing release binary..." >&2
  "$INSTALL_SCRIPT"
fi

exec "$BIN_PATH" -root "$ROOT_DIR"
