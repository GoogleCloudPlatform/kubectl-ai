#!/usr/bin/env bash
set -euo pipefail

# Check for required commands
for cmd in curl tar; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Error: $cmd is not installed. Please install $cmd to proceed."
    exit 1
  fi
done

# Set the insecure SSL argument
INSECURE_ARG=""
if [ -n "${INSECURE:-}" ]; then
  INSECURE_ARG="--insecure"
fi

REPO="GoogleCloudPlatform/kubectl-ai"
BINARY="kubectl-ai"

# Detect OS
sysOS="$(uname | tr '[:upper:]' '[:lower:]')"
case "$sysOS" in
  linux)   OS="Linux" ;;
  darwin)  OS="Darwin" ;;
  *)
    echo "If you are on Windows or another unsupported OS, please follow the manual installation instructions at:"
    echo "https://github.com/GoogleCloudPlatform/kubectl-ai#manual-installation-linux-macos-and-windows"
    exit 1
    ;;
esac

# Detect NixOS
nixos_check="$(grep "ID=nixos" /etc/os-release 2>/dev/null || echo "no-match")"
case "$nixos_check" in
  *nixos*)
    echo "NixOS detected, please follow the manual installation instructions at:"
    echo "https://github.com/GoogleCloudPlatform/kubectl-ai#install-on-nixos"
    exit 1
    ;;
esac

# Detect ARCH
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="x86_64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "If you are on an unsupported architecture, please follow the manual installation instructions at:"
    echo "https://github.com/GoogleCloudPlatform/kubectl-ai#manual-installation-linux-macos-and-windows"
    exit 1
    ;;
esac

# Get latest version tag from GitHub API, Use GITHUB_TOKEN if available to avoid potential rate limit
if [ -n "${GITHUB_TOKEN:-}" ]; then
  auth_hdr="Authorization: token $GITHUB_TOKEN"
else
  auth_hdr=""

fi
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
LATEST_TAG=$(curl $INSECURE_ARG -s -H "$auth_hdr" \
  "https://api.github.com/repos/$REPO/releases/latest" \
  | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p')
if [ -z "$LATEST_TAG" ]; then
  echo "Failed to fetch latest release tag."
  exit 1
fi

# Compose download URL
TARBALL="kubectl-ai_${OS}_${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/$LATEST_TAG/$TARBALL"

# Create temp dir and set cleanup trap
TEMP_DIR="$(mktemp -d)"
cleanup() {
  if [ -n "${TEMP_DIR:-}" ] && [ -d "$TEMP_DIR" ]; then
    rm -rf "$TEMP_DIR"
  fi
}
trap cleanup EXIT INT TERM

# Download and extract in temp dir; install from there
(
  cd "$TEMP_DIR"
  echo "Downloading $URL ..."
  CURL_FLAGS="-fSL --retry 3"
  if [ -n "${INSECURE:-}" ]; then
    echo "⚠️  SSL certificate validation will be skipped for this download."
  fi
  curl $INSECURE_ARG $CURL_FLAGS "$URL" -o "$TARBALL"
  tar --no-same-owner -xzf "$TARBALL"

  if [ ! -f "$BINARY" ]; then
    echo "Error: expected binary '$BINARY' not found after extraction."
    exit 1
  fi

  echo "Installing $BINARY to /usr/local/bin (may require sudo)..."
  sudo install -m 0755 "$BINARY" /usr/local/bin/
)

echo "✅ $BINARY installed successfully! Run '$BINARY --help' to get started."
