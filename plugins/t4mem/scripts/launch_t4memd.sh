#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
PLUGIN_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
BIN_PATH=${T4MEMD_BIN:-"$PLUGIN_DIR/bin/t4memd"}
PROJECT_DIR=${CLAUDE_PROJECT_DIR:-${PWD}}
ROOT_DIR=${T4MEM_ROOT:-"$PROJECT_DIR/.t4mem"}
INSTALL_SCRIPT="$PLUGIN_DIR/scripts/install_t4memd_release.sh"

if [ ! -x "$BIN_PATH" ]; then
  echo "t4memd binary not found at $BIN_PATH; installing release binary..." >&2
  "$INSTALL_SCRIPT"
fi

exec "$BIN_PATH" -root "$ROOT_DIR"
