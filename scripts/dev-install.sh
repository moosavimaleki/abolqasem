#!/usr/bin/env sh

set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
APP="ai-agent-manager"
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
DIST_DIR="$ROOT_DIR/dist"
OUT="$DIST_DIR/$APP"
INSTALL_PATH="$BIN_DIR/$APP"
STARTUP="${AI_AGENT_MANAGER_DEV_STARTUP:-hook}"
HOOK_SCOPE="${AI_AGENT_MANAGER_DEV_HOOK_SCOPE:-user}"
INSTALL_HOOKS="${AI_AGENT_MANAGER_DEV_INSTALL_HOOKS:-1}"
OPEN_AFTER_INSTALL="${AI_AGENT_MANAGER_DEV_OPEN:-1}"

cd "$ROOT_DIR"

command -v go >/dev/null 2>&1 || {
  printf 'Error: Go 1.22+ is required and was not found in PATH\n' >&2
  exit 1
}

case "$STARTUP" in
  hook|service) ;;
  *)
    printf 'Error: unsupported AI_AGENT_MANAGER_DEV_STARTUP=%s (use hook or service)\n' "$STARTUP" >&2
    exit 1
    ;;
esac

if [ "${AI_AGENT_MANAGER_DEV_SKIP_WEB_BUILD:-0}" != "1" ]; then
  printf 'Building web assets...\n'
  AI_AGENT_MANAGER_SKIP_NPM_CI="${AI_AGENT_MANAGER_SKIP_NPM_CI:-1}" "$ROOT_DIR/scripts/prepare-web-assets.sh"
fi

printf 'Building %s from local source...\n' "$APP"
mkdir -p "$DIST_DIR"
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X ai-agent-manager/internal/buildinfo.Version=dev-local" -o "$OUT" ./cmd/ai-agent-manager

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

if [ "$INSTALL_HOOKS" = "1" ]; then
  printf 'Installing local hooks with startup=%s scope=%s...\n' "$STARTUP" "$HOOK_SCOPE"
  AI_AGENT_MANAGER_SUPPRESS_TRUST_NOTICE=1 "$INSTALL_PATH" install --all --scope "$HOOK_SCOPE" --startup "$STARTUP"
elif [ "$STARTUP" = "service" ]; then
  printf 'Installing local service without hooks...\n'
  "$INSTALL_PATH" install --startup service --no-hooks
fi

printf 'Restarting local server...\n'
"$INSTALL_PATH" restart

if [ "$OPEN_AFTER_INSTALL" = "1" ]; then
  printf 'Opening local UI...\n'
  "$INSTALL_PATH" open
fi

printf '\nInstalled local dev binary: %s\n' "$INSTALL_PATH"
printf 'Status:\n'
"$INSTALL_PATH" status
