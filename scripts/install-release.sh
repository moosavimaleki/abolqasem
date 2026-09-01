#!/usr/bin/env sh

set -eu

APP="abolqasem"
REPO="${ABOLQASEM_REPO:-${AI_AGENT_MANAGER_REPO:-moosavimaleki/abolqasem}}"
VERSION="${ABOLQASEM_VERSION:-${AI_AGENT_MANAGER_VERSION:-latest}}"
RELEASE_BASE_URL="${ABOLQASEM_RELEASE_BASE_URL:-${AI_AGENT_MANAGER_RELEASE_BASE_URL:-}}"
BIN_DIR="${BIN_DIR:-}"

usage() {
  cat <<'EOF'
Install abolqasem from GitHub release assets.

Usage:
  install-release.sh [options]

Options:
  --repo OWNER/REPO   GitHub repository. Default: moosavimaleki/abolqasem.
  --version TAG       Release tag. Default: latest.
  --bin-dir DIR       Install binary into DIR.
  -h, --help          Show this help.

Environment:
  ABOLQASEM_REPO
  ABOLQASEM_VERSION
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
  if [ -n "$RELEASE_BASE_URL" ]; then
    URL="$RELEASE_BASE_URL/latest/download/$ASSET"
  else
    URL="https://github.com/$REPO/releases/latest/download/$ASSET"
  fi
else
  if [ -n "$RELEASE_BASE_URL" ]; then
    URL="$RELEASE_BASE_URL/download/$VERSION/$ASSET"
  else
    URL="https://github.com/$REPO/releases/download/$VERSION/$ASSET"
  fi
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
SIDECAR="$(find "$EXTRACT_DIR" -type f -name "codex-manager-gateway$TARGET_SUFFIX" | head -n 1)"
[ -n "$SIDECAR" ] || die "Codex Manager sidecar not found in $ASSET"

INSTALL_DIR="$(default_bin_dir)"
INSTALL_PATH="$INSTALL_DIR/$APP$TARGET_SUFFIX"
if [ -x "$INSTALL_PATH" ]; then
  log "Stopping existing service"
  "$INSTALL_PATH" service stop >/dev/null 2>&1 || true
fi
install_file "$BINARY" "$INSTALL_PATH"
install_file "$SIDECAR" "$INSTALL_DIR/codex-manager-gateway$TARGET_SUFFIX"
"$INSTALL_PATH" --help >/dev/null

log "Installed $APP to $INSTALL_PATH"
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    log "PATH notice: add this to your shell profile if needed:"
    log "  export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac

log "Installing persistent service and all agent hooks"
"$INSTALL_PATH" install
