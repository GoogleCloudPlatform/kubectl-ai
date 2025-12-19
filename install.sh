#!/usr/bin/env bash
set -euo pipefail

# ============================================================================
# kubectl-ai Installation Script
# ============================================================================
# This script automatically downloads and installs the latest version of
# kubectl-ai for your system.
#
# Supported Environment Variables:
#   INSECURE        - Skip SSL certificate validation (NOT RECOMMENDED)
#   GITHUB_TOKEN    - GitHub API token to avoid rate limiting
#   INSTALL_DIR     - Custom installation directory (default: /usr/local/bin)
#
# Usage:
#   ./install.sh [OPTIONS]
#
# Options:
#   --dry-run, -n   Show what would be done without making changes
#   --help, -h      Display this help message
# ============================================================================

# Initialize flags
DRY_RUN=false

# Show help message
show_help() {
  cat << 'EOF'
kubectl-ai Installation Script

Usage: ./install.sh [OPTIONS]

Options:
  --dry-run, -n   Show what would be done without making changes
  --help, -h      Display this help message

Environment Variables:
  GITHUB_TOKEN    GitHub API token to avoid rate limiting
  INSTALL_DIR     Custom installation directory (default: /usr/local/bin)
  INSECURE        Skip SSL certificate validation (NOT RECOMMENDED)

Examples:
  # Standard installation
  ./install.sh

  # Dry-run to see what would happen
  ./install.sh --dry-run

  # Install to custom directory
  INSTALL_DIR="$HOME/.local/bin" ./install.sh

For more information, visit:
https://github.com/GoogleCloudPlatform/kubectl-ai
EOF
  exit 0
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run|-n)
      DRY_RUN=true
      shift
      ;;
    --help|-h)
      show_help
      ;;
    *)
      echo "Error: Unknown option '$1'"
      echo "Run './install.sh --help' for usage information."
      exit 1
      ;;
  esac
done

# ============================================================================
# Dependency Checks
# ============================================================================
# Verify that all required system commands are available before proceeding
for cmd in curl tar; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Error: $cmd is not installed. Please install $cmd to proceed."
    exit 1
  fi
done

# ============================================================================
# Configuration
# ============================================================================
# Repository and binary name constants
readonly REPO="GoogleCloudPlatform/kubectl-ai"
readonly BINARY="kubectl-ai"

# Set the insecure SSL argument if INSECURE environment variable is set
# WARNING: This bypasses SSL certificate validation and is a security risk
INSECURE_ARG=""
if [ -n "${INSECURE:-}" ]; then
  INSECURE_ARG="--insecure"
fi
readonly INSECURE_ARG

# ============================================================================
# Platform Detection
# ============================================================================
# Detect operating system and architecture to download the correct binary

# Detect OS (Linux or macOS)
sysOS="$(uname | tr '[:upper:]' '[:lower:]')"
case "$sysOS" in
  linux)   OS="Linux" ;;
  darwin)  OS="Darwin" ;;
  *)
    echo "Error: Unsupported operating system: $sysOS"
    echo "If you are on Windows or another unsupported OS, please follow the manual installation instructions at:"
    echo "https://github.com/GoogleCloudPlatform/kubectl-ai#manual-installation-linux-macos-and-windows"
    exit 1
    ;;
esac
readonly OS

# Special handling for NixOS - requires different installation method
nixos_check="$(grep "ID=nixos" /etc/os-release 2>/dev/null || echo "no-match")"
case "$nixos_check" in
  *nixos*)
    echo "NixOS detected, please follow the manual installation instructions at:"
    echo "https://github.com/GoogleCloudPlatform/kubectl-ai#install-on-nixos"
    exit 1
    ;;
esac

# Detect architecture (x86_64 or arm64)
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="x86_64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "Error: Unsupported architecture: $ARCH"
    echo "If you are on an unsupported architecture, please follow the manual installation instructions at:"
    echo "https://github.com/GoogleCloudPlatform/kubectl-ai#manual-installation-linux-macos-and-windows"
    exit 1
    ;;
esac
readonly ARCH

if [ "$DRY_RUN" = true ]; then
  echo "[DRY-RUN] Detected platform: OS=$OS, ARCH=$ARCH"
fi

# ============================================================================
# Version Detection
# ============================================================================
# Fetch the latest release version from GitHub API
# Use GITHUB_TOKEN if available to avoid rate limiting

# In dry-run mode, show what would be done without making network calls
if [ "$DRY_RUN" = true ]; then
  echo "[DRY-RUN] Would fetch latest version from: https://api.github.com/repos/${REPO}/releases/latest"
  readonly LATEST_TAG="<latest>"
  readonly TARBALL="kubectl-ai_${OS}_${ARCH}.tar.gz"
  readonly URL="https://github.com/${REPO}/releases/download/${LATEST_TAG}/${TARBALL}"

  echo "[DRY-RUN] Would download from: ${URL}"
  echo "[DRY-RUN] Would perform the following actions:"
  echo "  1. Download ${TARBALL} to temporary directory"
  echo "  2. Extract the tarball"
  echo "  3. Install ${BINARY} to ${INSTALL_DIR:-/usr/local/bin}"
  echo ""
  echo "[DRY-RUN] No changes were made. Run without --dry-run to install."
  exit 0
fi

# Set up GitHub API authentication header if token is provided
if [ -n "${GITHUB_TOKEN:-}" ]; then
  auth_hdr="Authorization: token ${GITHUB_TOKEN}"
else
  auth_hdr=""
fi

# Security warning for insecure SSL mode
if [ -n "${INSECURE:-}" ]; then
  echo "⚠️  SECURITY WARNING: INSECURE is set, SSL certificate validation will be skipped!"
  echo "   This makes you vulnerable to man-in-the-middle attacks and other security risks."
  echo "   Only proceed if you understand the security implications and trust your network."
  echo ""
  echo "   Continue with unsafe download? (yes/no)"
  read -r response
  case "$response" in
    [yY][eE][sS]|[yY])
      echo "Proceeding with insecure connection..."
      ;;
    *)
      echo "Installation aborted for security reasons."
      exit 1
      ;;
  esac
fi

# Fetch the latest release tag from GitHub API
LATEST_TAG=$(curl ${INSECURE_ARG} -s -H "${auth_hdr}" \
  "https://api.github.com/repos/${REPO}/releases/latest" \
  | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p')

if [ -z "$LATEST_TAG" ]; then
  echo "Error: Failed to fetch latest release tag."
  echo "This could be due to:"
  echo "  - Network connectivity issues"
  echo "  - GitHub API rate limiting (set GITHUB_TOKEN to avoid this)"
  echo "  - Repository access issues"
  exit 1
fi
readonly LATEST_TAG

echo "Latest version: ${LATEST_TAG}"

# ============================================================================
# Download URL Preparation
# ============================================================================
# Compose the download URL for the correct platform and version
readonly TARBALL="kubectl-ai_${OS}_${ARCH}.tar.gz"
readonly URL="https://github.com/${REPO}/releases/download/${LATEST_TAG}/${TARBALL}"

# ============================================================================
# Temporary Directory Setup
# ============================================================================
# Create a temporary directory for downloading and extracting the binary
# The cleanup trap ensures the temp directory is always removed, even on errors
# Declare and assign separately to avoid masking return values (SC2155)
TEMP_DIR="$(mktemp -d)"
readonly TEMP_DIR

# Cleanup function to remove temporary directory
cleanup() {
  if [ -n "${TEMP_DIR:-}" ] && [ -d "$TEMP_DIR" ]; then
    rm -rf "$TEMP_DIR"
  fi
}

# Register cleanup function to run on script exit, interrupt, or termination
trap cleanup EXIT INT TERM

# ============================================================================
# Installation Directory Configuration
# ============================================================================
# Determine installation directory before entering subshell
# Use INSTALL_DIR environment variable if set, otherwise default to /usr/local/bin
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
readonly INSTALL_DIR

# ============================================================================
# Download and Installation
# ============================================================================
# Download the binary tarball, extract it, and install to the target directory

# Run download and installation in a subshell
(
  cd "$TEMP_DIR" || exit 1

  echo "Downloading ${URL} ..."

  # Set curl flags for robust downloading
  readonly CURL_FLAGS="-fSL --retry 3"

  if [ -n "${INSECURE:-}" ]; then
    echo "⚠️  SSL certificate validation will be skipped for this download."
  fi

  # Download the tarball
  # Note: CURL_FLAGS is intentionally unquoted to allow word splitting (SC2086)
  # shellcheck disable=SC2086
  curl ${INSECURE_ARG} ${CURL_FLAGS} "${URL}" -o "${TARBALL}"

  # Extract the tarball
  echo "Extracting ${TARBALL} ..."
  tar --no-same-owner -xzf "${TARBALL}"

  # Verify the binary was extracted successfully
  if [ ! -f "$BINARY" ]; then
    echo "Error: Expected binary '${BINARY}' not found after extraction."
    echo "The tarball may be corrupted or have an unexpected structure."
    exit 1
  fi

  # Verify installation directory exists or can be created
  if [ ! -d "$INSTALL_DIR" ]; then
    echo "Warning: Installation directory ${INSTALL_DIR} does not exist."
    echo "It will be created during installation."
  fi

  echo "Installing ${BINARY} to ${INSTALL_DIR} (may require sudo)..."

  # Install the binary with appropriate permissions
  # The 'install' command will handle creating the directory if needed
  sudo install -m 0755 "$BINARY" "${INSTALL_DIR}/"
)

# ============================================================================
# Installation Complete
# ============================================================================
echo "✅ ${BINARY} installed successfully!"
echo ""
echo "Next steps:"
echo "  1. Run '${BINARY} --help' to see available commands"
echo "  2. Ensure ${INSTALL_DIR} is in your PATH"
echo ""
echo "Installed version: ${LATEST_TAG}"
