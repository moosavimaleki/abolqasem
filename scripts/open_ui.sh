#!/usr/bin/env sh
set -eu

binary="$(command -v abolqasem || true)"
if [ -z "$binary" ]; then
  echo "abolqasem is not installed on PATH" >&2
  exit 1
fi
exec "$binary" open --start-server
