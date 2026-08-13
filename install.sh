#!/usr/bin/env bash
# PyGo CLI installer — installs from GitHub Releases
# Usage:
#   curl -sL https://raw.githubusercontent.com/PyGo-Labs/pygo-framework/main/install.sh | bash
#   Or with version: PYGO_VERSION=v2.0.0 curl -sL ... | bash
set -euo pipefail

REPO="PyGo-Labs/pygo-framework"
GITHUB="https://github.com/${REPO}"
INSTALL_DIR="${PYGO_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${PYGO_VERSION:-latest}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info()  { echo -e "${BLUE}ℹ️  $*${NC}"; }
ok()    { echo -e "${GREEN}✅ $*${NC}"; }
warn()  { echo -e "${YELLOW}⚠️  $*${NC}"; }
fail()  { echo -e "${RED}❌ $*${NC}" >&2; exit 1; }

# Detect OS and architecture
detect_platform() {
  local os arch

  case "$(uname -s)" in
    Linux*)  os="linux" ;;
    Darwin*) os="darwin" ;;
    MINGW*|MSYS*|CYGWIN*) os="windows" ;;
    *) fail "Unsupported OS: $(uname -s)" ;;
  esac

  case "$(uname -m)" in
    x86_64|amd64)  arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *) fail "Unsupported architecture: $(uname -m)" ;;
  esac

  PLATFORM="${os}-${arch}"
  EXT=""
  if [ "$os" = "windows" ]; then EXT=".exe"; fi
}

download_from_github() {
  local version="$1"
  local filename="pygo-${version#v}-${PLATFORM}"
  local url="${GITHUB}/releases/download/${pygo_version}/${filename}${EXT}"

  info "Downloading ${filename}..."
  info "  URL: ${url}"

  local tmp
  tmp=$(mktemp)

  if ! curl -fsSL "$url" -o "$tmp"; then
    rm -f "$tmp"
    fail "Download failed. Check that ${pygo_version} exists: ${GITHUB}/releases"
  fi

  # Make binary name "pygo" (or "pygo.exe" on Windows)
  local bin_name="pygo${EXT}"
  mv "$tmp" "${INSTALL_DIR}/${bin_name}"
  chmod +x "${INSTALL_DIR}/${bin_name}"
  ok "Installed to ${INSTALL_DIR}/${bin_name}"
}

# Resolve "latest" to actual version tag
resolve_version() {
  if [ "$VERSION" = "latest" ]; then
    info "Resolving latest release..."
    local latest_url
    latest_url=$(curl -fsSL -o /dev/null -w '%{url_effective}' "${GITHUB}/releases/latest" 2>/dev/null || echo "")
    if [ -z "$latest_url" ]; then
      fail "Could not resolve latest release. No releases found."
    fi
    VERSION=$(basename "$latest_url")
    info "Latest version: ${VERSION}"
  fi
  pygo_version="$VERSION"
}

# Ensure install dir exists and is in PATH
ensure_in_path() {
  mkdir -p "$INSTALL_DIR"

  # Check if INSTALL_DIR is in PATH
  case ":${PATH}:" in
    *":${INSTALL_DIR}:"*) ;;
    *)
      warn "${INSTALL_DIR} is not in PATH"
      echo ""
      echo "Add this to your ~/.bashrc or ~/.zshrc:"
      echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
      echo ""
      echo "Or run:"
      echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
      ;;
  esac
}

# Verify installation
verify() {
  info "Verifying installation..."
  if command -v pygo &>/dev/null; then
    local v
    v=$(pygo version 2>&1 | head -1 || echo "unknown")
    ok "PyGo installed: $v"
    echo ""
    echo "Available commands:"
    echo "  pygo new <name>    Create a new PyGo project"
    echo "  pygo dev          Start hot-reload dev server"
    echo "  pygo docs         Generate HTML documentation"
    echo "  pygo upgrade      Update to latest version"
    echo "  pygo version      Show version info"
  else
    warn "pygo binary not found in PATH"
    echo "Run: export PATH=\"\$HOME/.local/bin:\$PATH\""
  fi
}

# Main
echo "📥 PyGo Installer"
echo "  Platform: $(uname -s) $(uname -m)"
echo ""

detect_platform
ok "Platform: ${PLATFORM}"

ensure_in_path
resolve_version
download_from_github "$pygo_version"
verify

echo ""
echo "🚀 Ready! Try: pygo version"
