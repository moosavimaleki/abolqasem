#!/usr/bin/env sh

set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
APP="abolqasem"
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
DIST_DIR="$ROOT_DIR/dist"
OUT="$DIST_DIR/$APP"
INSTALL_PATH="$BIN_DIR/$APP"
OPEN_AFTER_INSTALL="${ABOLQASEM_DEV_OPEN:-${AI_AGENT_MANAGER_DEV_OPEN:-1}}"
DEV_PORT="${ABOLQASEM_DEV_PORT:-${AI_AGENT_MANAGER_DEV_PORT:-}}"
DEV_PROXY="${ABOLQASEM_DEV_PROXY:-${AI_AGENT_MANAGER_DEV_PROXY:-}}"
DEV_NO_PROXY="${ABOLQASEM_DEV_NO_PROXY:-${AI_AGENT_MANAGER_DEV_NO_PROXY:-${NO_PROXY:-${no_proxy:-}}}}"

if [ "$DEV_PROXY" != "" ] && [ "$DEV_PROXY" != "0" ] && [ "$DEV_PROXY" != "off" ]; then
  if [ "$DEV_NO_PROXY" = "" ]; then
    DEV_NO_PROXY="127.0.0.1,localhost,::1"
  else
    DEV_NO_PROXY="$DEV_NO_PROXY,127.0.0.1,localhost,::1"
  fi
  export HTTP_PROXY="$DEV_PROXY"
  export HTTPS_PROXY="$DEV_PROXY"
  export ALL_PROXY="$DEV_PROXY"
  export http_proxy="$DEV_PROXY"
  export https_proxy="$DEV_PROXY"
  export all_proxy="$DEV_PROXY"
  export NO_PROXY="$DEV_NO_PROXY"
  export no_proxy="$DEV_NO_PROXY"
  printf 'Using dev proxy for local server install/update steps: %s\n' "$DEV_PROXY"

  if command -v systemctl >/dev/null 2>&1; then
    systemctl --user import-environment HTTP_PROXY HTTPS_PROXY ALL_PROXY NO_PROXY http_proxy https_proxy all_proxy no_proxy >/dev/null 2>&1 || true
  fi
  if command -v launchctl >/dev/null 2>&1; then
    launchctl setenv HTTP_PROXY "$HTTP_PROXY" >/dev/null 2>&1 || true
    launchctl setenv HTTPS_PROXY "$HTTPS_PROXY" >/dev/null 2>&1 || true
    launchctl setenv ALL_PROXY "$ALL_PROXY" >/dev/null 2>&1 || true
    launchctl setenv NO_PROXY "$NO_PROXY" >/dev/null 2>&1 || true
    launchctl setenv http_proxy "$http_proxy" >/dev/null 2>&1 || true
    launchctl setenv https_proxy "$https_proxy" >/dev/null 2>&1 || true
    launchctl setenv all_proxy "$all_proxy" >/dev/null 2>&1 || true
    launchctl setenv no_proxy "$no_proxy" >/dev/null 2>&1 || true
  fi
fi

cd "$ROOT_DIR"

command -v go >/dev/null 2>&1 || {
  printf 'Error: Go 1.22+ is required and was not found in PATH\n' >&2
  exit 1
}

if [ "${ABOLQASEM_DEV_SKIP_WEB_BUILD:-${AI_AGENT_MANAGER_DEV_SKIP_WEB_BUILD:-0}}" != "1" ]; then
  printf 'Building web assets...\n'
  ABOLQASEM_SKIP_NPM_CI="${ABOLQASEM_SKIP_NPM_CI:-${AI_AGENT_MANAGER_SKIP_NPM_CI:-1}}" "$ROOT_DIR/scripts/prepare-web-assets.sh"
fi

printf 'Building %s from local source...\n' "$APP"
mkdir -p "$DIST_DIR"
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X abolqasem/internal/buildinfo.Version=dev-local" -o "$OUT" ./cmd/abolqasem

printf 'Installing to %s...\n' "$INSTALL_PATH"
mkdir -p "$BIN_DIR"
if [ -x "$INSTALL_PATH" ]; then
  "$INSTALL_PATH" service stop >/dev/null 2>&1 || true
fi
if command -v install >/dev/null 2>&1; then
  install -m 0755 "$OUT" "$INSTALL_PATH"
else
  cp "$OUT" "$INSTALL_PATH"
  chmod 0755 "$INSTALL_PATH"
fi

printf 'Verifying installed binary...\n'
"$INSTALL_PATH" --help >/dev/null

if [ "$DEV_PORT" != "" ]; then
  export ABOLQASEM_SERVICE_PORT="$DEV_PORT"
  export AI_AGENT_MANAGER_SERVICE_PORT="$DEV_PORT"
  printf 'Installing local service on port %s and hooks...\n' "$DEV_PORT"
else
  printf 'Installing local service and hooks...\n'
fi
"$INSTALL_PATH" install

if [ "$OPEN_AFTER_INSTALL" = "1" ]; then
  printf 'Opening local UI...\n'
  "$INSTALL_PATH" open
fi

printf '\nInstalled local dev binary: %s\n' "$INSTALL_PATH"
printf 'Status:\n'
"$INSTALL_PATH" status
