#!/usr/bin/env sh

set -eu

APP="abolqasem"
PKG="./cmd/abolqasem"
DIST="dist"
BUILD="1"
BUILD_ALL="0"
BIN_DIR="${BIN_DIR:-}"

usage() {
  cat <<'EOF'
Build and install abolqasem from source.

Usage:
  scripts/build-from-source.sh [options]

Options:
  --bin-dir DIR       Install binary into DIR.
  --prefix DIR        Install binary into DIR/bin.
  --no-build          Install an existing dist binary for the current OS/arch.
  --build-all         Build release binaries for Linux, macOS, and Windows into dist/.
  -h, --help          Show this help.

Environment:
  BIN_DIR             Override install directory.

Examples:
  scripts/build-from-source.sh
  scripts/build-from-source.sh --build-all
  scripts/build-from-source.sh --bin-dir "$HOME/.local/bin"
EOF
}

die() {
  printf 'Error: %s\n' "$1" >&2
  exit 1
}

log() {
  printf '%s\n' "$1"
}

run_privileged() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
    return
  fi
  if command -v sudo >/dev/null 2>&1; then
    sudo "$@"
    return
  fi
  die "need elevated privileges to install Go, but sudo is not available"
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

install_go_if_missing() {
  if command -v go >/dev/null 2>&1; then
    return
  fi

  log "Go was not found in PATH. Attempting automatic installation."
  case "$TARGET_OS" in
    darwin)
      command -v brew >/dev/null 2>&1 || die "Homebrew is required to auto-install Go on macOS"
      brew install go
      ;;
    linux)
      if command -v apt-get >/dev/null 2>&1; then
        run_privileged apt-get update
        run_privileged apt-get install -y golang-go
      elif command -v dnf >/dev/null 2>&1; then
        run_privileged dnf install -y golang
      elif command -v yum >/dev/null 2>&1; then
        run_privileged yum install -y golang
      elif command -v pacman >/dev/null 2>&1; then
        run_privileged pacman -Sy --noconfirm go
      elif command -v zypper >/dev/null 2>&1; then
        run_privileged zypper --non-interactive install go
      elif command -v apk >/dev/null 2>&1; then
        run_privileged apk add --no-cache go
      else
        die "unsupported Linux package manager for automatic Go installation"
      fi
      ;;
    *)
      die "automatic Go installation is not supported on this platform"
      ;;
  esac

  command -v go >/dev/null 2>&1 || die "Go installation finished but 'go' is still not available in PATH"
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

prepare_web_assets() {
  command -v npm >/dev/null 2>&1 || die "npm is required to build the embedded web app"
  sh "$(dirname "$0")/prepare-web-assets.sh"
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

install_go_if_missing
prepare_web_assets

if [ "$BUILD_ALL" = "1" ]; then
  build_all_targets
else
  if [ "$BUILD" = "1" ]; then
    build_target "$TARGET_OS" "$TARGET_ARCH"
  fi
fi

[ -f "$TARGET_BIN" ] || die "expected binary not found: $TARGET_BIN"
if [ -x "$INSTALL_PATH" ]; then
  log "Stopping existing service"
  "$INSTALL_PATH" service stop >/dev/null 2>&1 || true
fi
install_file "$TARGET_BIN" "$INSTALL_PATH"

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
