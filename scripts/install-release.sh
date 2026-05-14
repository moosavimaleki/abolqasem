#!/usr/bin/env sh

set -eu

APP="ai-session-viewer"
REPO="${AI_SESSION_VIEWER_REPO:-h-mousavi/codex-rtl-plugin}"
VERSION="${AI_SESSION_VIEWER_VERSION:-latest}"
BIN_DIR="${BIN_DIR:-}"
INSTALL_HOOKS="${AI_SESSION_VIEWER_INSTALL_HOOKS:-0}"
HOOK_SCOPE="${AI_SESSION_VIEWER_HOOK_SCOPE:-user}"
HOOK_AGENTS="${AI_SESSION_VIEWER_AGENTS:-all}"

usage() {
  cat <<'EOF'
Install ai-session-viewer from GitHub release assets.

Usage:
  install-release.sh [options]

Options:
  --repo OWNER/REPO   GitHub repository. Default: h-mousavi/codex-rtl-plugin.
  --version TAG       Release tag. Default: latest.
  --bin-dir DIR       Install binary into DIR.
  --hooks             Install hooks after installing the binary.
  --agent NAME        Install hook for one agent: codex, claude, or gemini.
  --scope SCOPE       Hook scope: user or project. Default: user.
  -h, --help          Show this help.

Environment:
  AI_SESSION_VIEWER_REPO
  AI_SESSION_VIEWER_VERSION
  AI_SESSION_VIEWER_INSTALL_HOOKS=1
  AI_SESSION_VIEWER_AGENTS=all|codex|claude|gemini
  AI_SESSION_VIEWER_HOOK_SCOPE=user|project
  BIN_DIR
EOF
}

die() {
  printf 'Error: %s\n' "$1" >&2
  exit 1
}

log() {
  printf '%s\n' "$1"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --repo)
      [ "$#" -ge 2 ] || die "--repo requires a value"
      REPO="$2"
      shift 2
      ;;
    --version)
      [ "$#" -ge 2 ] || die "--version requires a value"
      VERSION="$2"
      shift 2
      ;;
    --bin-dir)
      [ "$#" -ge 2 ] || die "--bin-dir requires a value"
      BIN_DIR="$2"
      shift 2
      ;;
    --hooks)
      INSTALL_HOOKS="1"
      shift
      ;;
    --agent)
      [ "$#" -ge 2 ] || die "--agent requires a value"
      INSTALL_HOOKS="1"
      HOOK_AGENTS="$2"
      shift 2
      ;;
    --scope)
      [ "$#" -ge 2 ] || die "--scope requires a value"
      HOOK_SCOPE="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown option: $1"
      ;;
  esac
done

detect_os() {
  case "$(uname -s 2>/dev/null || printf unknown)" in
    Linux*) printf 'linux' ;;
    Darwin*) printf 'darwin' ;;
    CYGWIN*|MINGW*|MSYS*) printf 'windows' ;;
    *) die "unsupported operating system: $(uname -s 2>/dev/null || printf unknown)" ;;
  esac
}

detect_arch() {
  case "$(uname -m 2>/dev/null || printf unknown)" in
    x86_64|amd64) printf 'amd64' ;;
    arm64|aarch64) printf 'arm64' ;;
    *) die "unsupported architecture: $(uname -m 2>/dev/null || printf unknown)" ;;
  esac
}

default_bin_dir() {
  if [ -n "$BIN_DIR" ]; then
    printf '%s' "$BIN_DIR"
    return
  fi

  case "$TARGET_OS" in
    windows)
      if [ -n "${LOCALAPPDATA:-}" ]; then
        printf '%s/%s/bin' "$LOCALAPPDATA" "$APP"
      else
        printf '%s/AppData/Local/%s/bin' "$HOME" "$APP"
      fi
      ;;
    *)
      printf '%s/.local/bin' "$HOME"
      ;;
  esac
}

download() {
  url="$1"
  out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$out"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO "$out" "$url"
    return
  fi
  die "curl or wget is required"
}

install_file() {
  src="$1"
  dest="$2"
  mkdir -p "$(dirname "$dest")"
  if command -v install >/dev/null 2>&1; then
    install -m 0755 "$src" "$dest"
  else
    cp "$src" "$dest"
    chmod 0755 "$dest" 2>/dev/null || true
  fi
}

TARGET_OS="$(detect_os)"
TARGET_ARCH="$(detect_arch)"
TARGET_SUFFIX=""
ARCHIVE_EXT="tar.gz"
[ "$TARGET_OS" = "windows" ] && TARGET_SUFFIX=".exe" && ARCHIVE_EXT="zip"

ASSET="$APP-$TARGET_OS-$TARGET_ARCH.$ARCHIVE_EXT"
if [ "$VERSION" = "latest" ]; then
  URL="https://github.com/$REPO/releases/latest/download/$ASSET"
else
  URL="https://github.com/$REPO/releases/download/$VERSION/$ASSET"
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

ARCHIVE="$TMP_DIR/$ASSET"
EXTRACT_DIR="$TMP_DIR/extract"
mkdir -p "$EXTRACT_DIR"

log "Downloading $URL"
download "$URL" "$ARCHIVE"

case "$ARCHIVE_EXT" in
  zip)
    command -v unzip >/dev/null 2>&1 || die "unzip is required for Windows archives"
    unzip -q "$ARCHIVE" -d "$EXTRACT_DIR"
    ;;
  tar.gz)
    tar -xzf "$ARCHIVE" -C "$EXTRACT_DIR"
    ;;
esac

BINARY="$(find "$EXTRACT_DIR" -type f -name "$APP$TARGET_SUFFIX" | head -n 1)"
[ -n "$BINARY" ] || die "binary not found in $ASSET"

INSTALL_DIR="$(default_bin_dir)"
INSTALL_PATH="$INSTALL_DIR/$APP$TARGET_SUFFIX"
install_file "$BINARY" "$INSTALL_PATH"
"$INSTALL_PATH" --help >/dev/null

log "Installed $APP to $INSTALL_PATH"
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    log "PATH notice: add this to your shell profile if needed:"
    log "  export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac

if [ "$INSTALL_HOOKS" = "1" ]; then
  if [ "$HOOK_AGENTS" = "all" ]; then
    "$INSTALL_PATH" install --all --scope "$HOOK_SCOPE"
  else
    for agent in $HOOK_AGENTS; do
      "$INSTALL_PATH" install --agent "$agent" --scope "$HOOK_SCOPE"
    done
  fi
fi

log "Run the server with:"
log "  $APP server"
