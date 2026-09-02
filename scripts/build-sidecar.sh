#!/usr/bin/env sh

set -eu

SIDECARE_DIR="sidecars/codex-manager-gateway"
DIST="dist/sidecars"
BUILD_ALL="0"
TARGET=""

cargo_zigbuild() {
  if command -v cargo-zigbuild >/dev/null 2>&1; then
    cargo-zigbuild build "$@"
    return
  fi
  if [ -n "${CARGO_HOME:-}" ] && [ -x "$CARGO_HOME/bin/cargo-zigbuild" ]; then
    "$CARGO_HOME/bin/cargo-zigbuild" build "$@"
    return
  fi
  if [ -x "$HOME/.cargo/bin/cargo-zigbuild" ]; then
    "$HOME/.cargo/bin/cargo-zigbuild" build "$@"
    return
  fi
  cargo zigbuild "$@"
}

cargo_zigbuild_available() {
  if command -v cargo-zigbuild >/dev/null 2>&1; then
    return 0
  fi
  if [ -n "${CARGO_HOME:-}" ] && [ -x "$CARGO_HOME/bin/cargo-zigbuild" ]; then
    return 0
  fi
  if [ -x "$HOME/.cargo/bin/cargo-zigbuild" ]; then
    return 0
  fi
  cargo zigbuild --version >/dev/null 2>&1
}

ensure_rust_target() {
  target="$1"
  if rustup target list --installed 2>/dev/null | grep -Fx "$target" >/dev/null 2>&1; then
    return 0
  fi
  command -v rustup >/dev/null 2>&1 || {
    echo "Rust target $target is not installed and rustup is unavailable" >&2
    exit 1
  }
  echo "Installing Rust target $target..." >&2
  rustup target add "$target"
}

usage() {
  cat <<'EOF'
Build the embedded Codex Manager Rust sidecar.

Usage:
  scripts/build-sidecar.sh [--target OS-ARCH | --all]

Cross builds require Rust, Zig, and cargo-zigbuild. A native target only
requires Rust.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --target)
      [ "$#" -ge 2 ] || { echo "--target requires OS-ARCH" >&2; exit 2; }
      TARGET="$2"
      shift 2
      ;;
    --all)
      BUILD_ALL="1"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      exit 2
      ;;
  esac
done

host_target() {
  case "$(uname -s)" in
    Linux*) os=linux ;;
    Darwin*) os=darwin ;;
    MINGW*|MSYS*|CYGWIN*) os=windows ;;
    *) echo "unsupported operating system" >&2; exit 1 ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) echo "unsupported architecture" >&2; exit 1 ;;
  esac
  printf '%s-%s' "$os" "$arch"
}

rust_target() {
  case "$1" in
    linux-amd64) printf '%s' x86_64-unknown-linux-musl ;;
    linux-arm64) printf '%s' aarch64-unknown-linux-musl ;;
    darwin-amd64) printf '%s' x86_64-apple-darwin ;;
    darwin-arm64) printf '%s' aarch64-apple-darwin ;;
    windows-amd64) printf '%s' x86_64-pc-windows-gnu ;;
    # Rust has no aarch64-pc-windows-gnu target. gnullvm is the Rust
    # Windows/ARM64 target compatible with Zig's GNU Windows linker.
    windows-arm64) printf '%s' aarch64-pc-windows-gnullvm ;;
    *) echo "unsupported target: $1" >&2; exit 1 ;;
  esac
}

build_target() {
  target="$1"
  triple="$(rust_target "$target")"
  output="$DIST/$target/codex-manager-gateway"
  case "$target" in windows-*) output="$output.exe" ;; esac
  mkdir -p "$(dirname "$output")"

  if [ "$target" = "$(host_target)" ]; then
    cargo build --release --manifest-path "$SIDECARE_DIR/Cargo.toml"
    artifact="$SIDECARE_DIR/target/release/codex-manager-gateway"
  else
    command -v zig >/dev/null 2>&1 || { echo "zig is required for cross-compiling $target" >&2; exit 1; }
    ensure_rust_target "$triple"
    cargo_zigbuild --release --manifest-path "$SIDECARE_DIR/Cargo.toml" --target "$triple"
    artifact="$SIDECARE_DIR/target/$triple/release/codex-manager-gateway"
  fi
  # Depending on the linker/cargo-zigbuild version, a Windows cross build may
  # emit either `codex-manager-gateway.exe` or the extensionless target name.
  # The release artifact is always named .exe, but select the file that Cargo
  # actually produced instead of assuming one convention.
  case "$target" in
    windows-*)
      if [ -f "$artifact.exe" ]; then
        artifact="$artifact.exe"
      fi
      ;;
  esac
  [ -f "$artifact" ] || { echo "sidecar artifact was not produced: $artifact" >&2; exit 1; }
  cp "$artifact" "$output"
  chmod 0755 "$output" 2>/dev/null || true
  printf 'Built %s\n' "$output"
}

command -v cargo >/dev/null 2>&1 || { echo "cargo is required to build the Codex Manager sidecar" >&2; exit 1; }

if [ "$BUILD_ALL" = "1" ]; then
  command -v zig >/dev/null 2>&1 || { echo "zig is required for --all" >&2; exit 1; }
  cargo_zigbuild_available || { echo "cargo-zigbuild is required for --all" >&2; exit 1; }
  for target in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64 windows-arm64; do
    build_target "$target"
  done
  exit 0
fi

if [ -z "$TARGET" ]; then
  TARGET="$(host_target)"
fi
build_target "$TARGET"
