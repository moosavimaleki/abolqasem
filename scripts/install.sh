#!/usr/bin/env bash

set -e

echo "Installing ai-session-viewer..."

# Build
echo "Building binary..."
go build -o ai-session-viewer cmd/ai-session-viewer/main.go

# Install to ~/.local/bin
echo "Installing to ~/.local/bin..."
mkdir -p ~/.local/bin
mv ai-session-viewer ~/.local/bin/

# Check PATH
if [[ ":$PATH:" != *":$HOME/.local/bin:"* ]]; then
    echo "Warning: ~/.local/bin is not in your PATH."
    echo "Please add 'export PATH=\$HOME/.local/bin:\$PATH' to your shell profile."
fi

# Run installer
echo "To configure hooks for an agent (e.g., codex, claude, gemini), run:"
echo "  ai-session-viewer install --agent <agent_name> --scope user"

echo ""
echo "Installation complete!"
echo "Run 'ai-session-viewer server' to start the viewer."
