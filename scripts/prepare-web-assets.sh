#!/usr/bin/env sh

set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
WEB_REACT_DIR="$ROOT_DIR/web-react"
EMBED_WEB_DIR="${AI_AGENT_MANAGER_EMBED_WEB_DIR:-$ROOT_DIR/web}"

cd "$WEB_REACT_DIR"
if [ "${AI_AGENT_MANAGER_SKIP_NPM_CI:-0}" != "1" ]; then
  npm ci
fi
npm run build:client

rm -rf "$EMBED_WEB_DIR"
mkdir -p "$EMBED_WEB_DIR"
cp -R "$WEB_REACT_DIR/dist/client/." "$EMBED_WEB_DIR/"
