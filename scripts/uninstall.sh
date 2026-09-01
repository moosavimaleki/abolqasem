#!/usr/bin/env sh
set -eu

BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
STATE_DIR="${CODEX_MANAGER_HOME:-$HOME/.local/state/abolqasem/codex-manager}"
DELETE_MANAGER_DATA=0
for arg in "$@"; do
  case "$arg" in
    --delete-manager-data) DELETE_MANAGER_DATA=1 ;;
    --help|-h) echo "Usage: uninstall.sh [--delete-manager-data]"; exit 0 ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

rm -f "$BIN_DIR/abolqasem" "$BIN_DIR/codex-manager-gateway" "$BIN_DIR/codex-manager-gateway.exe"
if [ "$DELETE_MANAGER_DATA" -eq 1 ]; then
  printf 'This deletes Codex Manager accounts, history and bindings under %s. Type DELETE to continue: ' "$STATE_DIR" >&2
  read -r confirmation
  [ "$confirmation" = "DELETE" ] || { echo "Cancelled; manager data was kept." >&2; exit 1; }
  rm -rf -- "$STATE_DIR"
fi
echo "Abolqasem binaries removed; manager data kept unless --delete-manager-data was confirmed."
