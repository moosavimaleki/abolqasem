#!/usr/bin/env bash

set -e

echo "Installing codex-rtl viewer..."

# Build
echo "Building binary..."
go build -o codex-rtl cmd/codex-rtl/main.go

# Install to ~/.local/bin
echo "Installing to ~/.local/bin..."
mkdir -p ~/.local/bin
mv codex-rtl ~/.local/bin/

# Check PATH
if [[ ":$PATH:" != *":$HOME/.local/bin:"* ]]; then
    echo "Warning: ~/.local/bin is not in your PATH."
    echo "Please add 'export PATH=\$HOME/.local/bin:\$PATH' to your shell profile."
fi

# Run codex-rtl install
echo "Configuring Codex hook..."
~/.local/bin/codex-rtl install

echo ""
echo "Installation complete!"
echo "Run 'codex-rtl server' to start the viewer."
