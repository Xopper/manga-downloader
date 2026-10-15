#!/usr/bin/env bash
# Stops the Suwayomi-Server run.sh started
set -euo pipefail
HOME_DIR="${SUWAYOMI_HOME:-$HOME/.cache/manga-downloader-suwayomi}"
if [ -f "$HOME_DIR/server.pid" ]; then
  kill "$(cat "$HOME_DIR/server.pid")" 2>/dev/null || true
  rm -f "$HOME_DIR/server.pid"
  echo stopped
else
  echo "not running (no $HOME_DIR/server.pid)"
fi
