#!/usr/bin/env sh

set -eu

APP="ai-session-viewer"
PKG="./cmd/ai-session-viewer"
DIST="dist"
SCOPE="user"
HOOKS="0"
BUILD="1"
BUILD_ALL="0"
AGENTS="codex claude gemini"
BIN_DIR="${BIN_DIR:-}"

usage() {
  cat <<'EOF'
Install ai-session-viewer from source.

Usage:
  scripts/install.sh [options]

Options:
  --bin-dir DIR       Install binary into DIR.
  --prefix DIR        Install binary into DIR/bin.
  --no-build          Install an existing dist binary for the current OS/arch.
  --build-all         Build release binaries for Linux, macOS, and Windows into dist/.
  --hooks             Install hooks after installing the binary.
  --all-agents        Same as --hooks for codex, claude, and gemini.
  --agent NAME        Install hook only for one agent: codex, claude, or gemini.
  --scope SCOPE       Hook scope: user or project. Default: user.
  -h, --help          Show this help.

Environment:
  BIN_DIR             Override install directory.

Examples:
  scripts/install.sh
  scripts/install.sh --hooks
  scripts/install.sh --build-all
  scripts/install.sh --bin-dir "$HOME/.local/bin"
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
    --bin-dir)
      [ "$#" -ge 2 ] || die "--bin-dir requires a value"
      BIN_DIR="$2"
      shift 2
      ;;
    --prefix)
      [ "$#" -ge 2 ] || die "--prefix requires a value"
      BIN_DIR="$2/bin"
      shift 2
      ;;
    --no-build)
      BUILD="0"
      shift
      ;;
    --build-all)
      BUILD_ALL="1"
      shift
      ;;
    --hooks|--all-agents)
      HOOKS="1"
      AGENTS="codex claude gemini"
      shift
      ;;
    --agent)
      [ "$#" -ge 2 ] || die "--agent requires a value"
      HOOKS="1"
      AGENTS="$2"
      shift 2
      ;;
    --scope)
      [ "$#" -ge 2 ] || die "--scope requires a value"
      SCOPE="$2"
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

command -v go >/dev/null 2>&1 || die "Go 1.22+ is required and was not found in PATH"

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

build_target() {
  os="$1"
  arch="$2"
  suffix=""
  [ "$os" = "windows" ] && suffix=".exe"
  out="$DIST/$APP-$os-$arch$suffix"
  log "Building $out"
  mkdir -p "$DIST"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags="-s -w" -o "$out" "$PKG"
}

build_all_targets() {
  rm -rf "$DIST"
  mkdir -p "$DIST"
  build_target linux amd64
  build_target linux arm64
  build_target darwin amd64
  build_target darwin arm64
  build_target windows amd64
  build_target windows arm64
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
[ "$TARGET_OS" = "windows" ] && TARGET_SUFFIX=".exe"
TARGET_BIN="$DIST/$APP-$TARGET_OS-$TARGET_ARCH$TARGET_SUFFIX"
INSTALL_DIR="$(default_bin_dir)"
INSTALL_PATH="$INSTALL_DIR/$APP$TARGET_SUFFIX"

if [ "$BUILD_ALL" = "1" ]; then
  build_all_targets
else
  if [ "$BUILD" = "1" ]; then
    build_target "$TARGET_OS" "$TARGET_ARCH"
  fi
fi

[ -f "$TARGET_BIN" ] || die "expected binary not found: $TARGET_BIN"
install_file "$TARGET_BIN" "$INSTALL_PATH"

log "Installed $APP to $INSTALL_PATH"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    log "PATH notice: add this to your shell profile if needed:"
    log "  export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac

if [ "$HOOKS" = "1" ]; then
  for agent in $AGENTS; do
    log "Installing $agent hook with scope=$SCOPE"
    "$INSTALL_PATH" install --agent "$agent" --scope "$SCOPE"
  done
else
  log "Hook install skipped. To install hooks later:"
  log "  $APP install --all --scope user"
fi

log "Run the server with:"
log "  $APP server"
