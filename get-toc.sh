#!/usr/bin/env bash
set -euo pipefail

REPO="louismorgner/tiny-oc"
INSTALL_DIR="/usr/local/bin"
FALLBACK_DIR="$HOME/.local/bin"
BIN_NAME="toc"

echo "Installing toc..."
echo ""

# Detect platform
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
  *)
    echo "Error: unsupported architecture: $ARCH"
    exit 1
    ;;
esac

case "$OS" in
  darwin|linux) ;;
  *)
    echo "Error: unsupported OS: $OS"
    echo "Supported: darwin (macOS), linux"
    exit 1
    ;;
esac

echo "Platform: ${OS}/${ARCH}"

# Resolve latest version
if ! command -v curl &>/dev/null; then
  echo "Error: curl is required but not installed."
  exit 1
fi

VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')

if [ -z "$VERSION" ]; then
  echo "Error: could not determine latest version."
  echo "Check https://github.com/${REPO}/releases for available releases."
  exit 1
fi

echo "Version: v${VERSION}"

# Download and extract
ARCHIVE="toc_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/v${VERSION}/${ARCHIVE}"

TOC_TMP=$(mktemp -d)
trap 'rm -rf "$TOC_TMP"' EXIT

echo "Downloading ${ARCHIVE}..."
if ! curl -fsSL "$URL" -o "${TOC_TMP}/${ARCHIVE}"; then
  echo "Error: failed to download ${URL}"
  echo "Check https://github.com/${REPO}/releases for available binaries."
  exit 1
fi

tar -xzf "${TOC_TMP}/${ARCHIVE}" -C "$TOC_TMP"

# Install binary
chmod +x "${TOC_TMP}/${BIN_NAME}"

if cp "${TOC_TMP}/${BIN_NAME}" "${INSTALL_DIR}/${BIN_NAME}" 2>/dev/null; then
  echo "Installed to ${INSTALL_DIR}/${BIN_NAME}"
else
  mkdir -p "$FALLBACK_DIR"
  cp "${TOC_TMP}/${BIN_NAME}" "${FALLBACK_DIR}/${BIN_NAME}"
  echo "Installed to ${FALLBACK_DIR}/${BIN_NAME}"
  if [[ ":$PATH:" != *":$FALLBACK_DIR:"* ]]; then
    echo ""
    echo "Add ${FALLBACK_DIR} to your PATH:"
    echo "  export PATH=\"${FALLBACK_DIR}:\$PATH\""
  fi
fi

echo ""
echo "Done. Run 'toc --help' to get started."
