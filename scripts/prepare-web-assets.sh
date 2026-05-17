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

mkdir -p "$EMBED_WEB_DIR"
find "$EMBED_WEB_DIR" -mindepth 1 -maxdepth 1 ! -name '.gitignore' ! -name 'README.md' -exec rm -rf {} +
cp -R "$WEB_REACT_DIR/dist/client/." "$EMBED_WEB_DIR/"

# The same built client is mounted both at / and /legacy/. Keep emitted files
# relative on disk so the stripped /legacy/ file server can load its assets.
if [ -f "$EMBED_WEB_DIR/index.html" ]; then
  tmp_index="$EMBED_WEB_DIR/index.html.tmp"
  sed \
    -e 's#href="/#href="./#g' \
    -e 's#src="/#src="./#g' \
    "$EMBED_WEB_DIR/index.html" > "$tmp_index"
  mv "$tmp_index" "$EMBED_WEB_DIR/index.html"
fi

if [ -d "$EMBED_WEB_DIR/assets" ]; then
  find "$EMBED_WEB_DIR/assets" -type f -name '*.css' -exec sh -c '
    for file do
      tmp_file="$file.tmp"
      sed -e "s#url(/#url(../#g" "$file" > "$tmp_file"
      mv "$tmp_file" "$file"
    done
  ' sh {} +
fi

# Keep the legacy /legacy/ asset contract stable even though the React build
# now emits hashed CSS under assets/.
mkdir -p "$EMBED_WEB_DIR/styles"
: > "$EMBED_WEB_DIR/styles.css"
: > "$EMBED_WEB_DIR/styles/base.css"
: > "$EMBED_WEB_DIR/styles/icons.css"
