#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_NAME="toc"

echo "Building toc..."
cd "$SCRIPT_DIR"
make build

INSTALL_DIR="/usr/local/bin"
if ln -sf "$SCRIPT_DIR/bin/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME" 2>/dev/null; then
  echo "Linked to $INSTALL_DIR/$BIN_NAME"
else
  INSTALL_DIR="$HOME/.local/bin"
  mkdir -p "$INSTALL_DIR"
  ln -sf "$SCRIPT_DIR/bin/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
  echo "Linked to $INSTALL_DIR/$BIN_NAME"
  if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    echo ""
    echo "Add $INSTALL_DIR to your PATH:"
    echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
  fi
fi

echo ""
echo "Done. Run 'toc --help' to get started."
