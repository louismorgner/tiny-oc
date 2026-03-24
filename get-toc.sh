#!/usr/bin/env bash
set -euo pipefail

REPO="https://github.com/tiny-oc/toc.git"
INSTALL_DIR="/usr/local/bin"
FALLBACK_DIR="$HOME/.local/bin"
BIN_NAME="toc"

echo "Installing toc..."
echo ""

# Check dependencies
if ! command -v go &>/dev/null; then
  echo "Error: Go is required but not installed."
  echo "Install it from https://go.dev/dl/"
  exit 1
fi

if ! command -v git &>/dev/null; then
  echo "Error: git is required but not installed."
  exit 1
fi

# Clone to temp directory
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

echo "Cloning repository..."
git clone --depth 1 "$REPO" "$TMPDIR" 2>/dev/null

echo "Building..."
cd "$TMPDIR"
go build -o "bin/$BIN_NAME" .

# Install binary
if cp "bin/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME" 2>/dev/null; then
  echo "Installed to $INSTALL_DIR/$BIN_NAME"
else
  mkdir -p "$FALLBACK_DIR"
  cp "bin/$BIN_NAME" "$FALLBACK_DIR/$BIN_NAME"
  echo "Installed to $FALLBACK_DIR/$BIN_NAME"
  if [[ ":$PATH:" != *":$FALLBACK_DIR:"* ]]; then
    echo ""
    echo "Add $FALLBACK_DIR to your PATH:"
    echo "  export PATH=\"$FALLBACK_DIR:\$PATH\""
  fi
fi

echo ""
echo "Done. Run 'toc --help' to get started."
