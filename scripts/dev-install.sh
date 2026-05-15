#!/usr/bin/env sh

set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
APP="ai-agent-manager"
BIN_DIR="$HOME/.local/bin"
DIST_DIR="$ROOT_DIR/dist"
OUT="$DIST_DIR/$APP"
INSTALL_PATH="$BIN_DIR/$APP"

cd "$ROOT_DIR"

command -v go >/dev/null 2>&1 || {
  printf 'Error: Go 1.22+ is required and was not found in PATH\n' >&2
  exit 1
}

printf 'Building %s...\n' "$APP"
mkdir -p "$DIST_DIR"
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$OUT" ./cmd/ai-agent-manager

printf 'Installing to %s...\n' "$INSTALL_PATH"
mkdir -p "$BIN_DIR"
if command -v install >/dev/null 2>&1; then
  install -m 0755 "$OUT" "$INSTALL_PATH"
else
  cp "$OUT" "$INSTALL_PATH"
  chmod 0755 "$INSTALL_PATH"
fi

printf 'Verifying installed binary...\n'
"$INSTALL_PATH" --help >/dev/null

printf 'Installed: %s\n' "$INSTALL_PATH"
printf 'Run: %s install\n' "$APP"

ai-agent-manager restart
