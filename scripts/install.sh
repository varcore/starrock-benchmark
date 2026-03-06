#!/bin/bash
set -euo pipefail

#
# Remote installer for starrock-benchmark.
# Downloads the latest release from GitHub and installs via dpkg.
#
# Usage (from any server):
#   curl -fsSL https://raw.githubusercontent.com/OWNER/REPO/main/scripts/install.sh | sudo bash
#   curl -fsSL ... | sudo bash -s -- --version 0.2.0
#

APP_NAME="starrock-benchmark"
GITHUB_REPO="varcore/starrock-benchmark"
REQUESTED_VERSION=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --version) REQUESTED_VERSION="$2"; shift 2 ;;
        *) echo "Unknown arg: $1"; exit 1 ;;
    esac
done

ARCH=$(dpkg --print-architecture 2>/dev/null || echo "amd64")
case "$ARCH" in
    amd64|arm64) ;; # supported
    x86_64) ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo "==> Installing ${APP_NAME} for ${ARCH}..."

# Determine version
if [ -n "$REQUESTED_VERSION" ]; then
    VERSION="$REQUESTED_VERSION"
    TAG="v${VERSION}"
else
    echo "==> Fetching latest release..."
    TAG=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | grep '"tag_name"' | head -1 | cut -d'"' -f4)
    if [ -z "$TAG" ]; then
        echo "Error: No releases found at https://github.com/${GITHUB_REPO}/releases"
        echo "Create one first: make release (from your dev machine)"
        exit 1
    fi
    VERSION="${TAG#v}"
fi

DEB_FILE="${APP_NAME}_${VERSION}_${ARCH}.deb"
DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${TAG}/${DEB_FILE}"

echo "==> Version: ${VERSION} (${TAG})"
echo "==> Downloading: ${DOWNLOAD_URL}"

TMP_DIR=$(mktemp -d)
trap "rm -rf $TMP_DIR" EXIT

curl -fSL -o "${TMP_DIR}/${DEB_FILE}" "$DOWNLOAD_URL"

echo "==> Installing ${DEB_FILE}..."
dpkg -i "${TMP_DIR}/${DEB_FILE}" || apt-get install -f -y

echo ""
echo "============================================"
echo "  ${APP_NAME} ${VERSION} installed!"
echo "============================================"
echo ""
echo "Quick start:"
echo "  1. Edit config:   sudo nano /etc/${APP_NAME}/config.yaml"
echo "  2. Run benchmark: ${APP_NAME} --config /etc/${APP_NAME}/config.yaml"
echo "  3. Check version: ${APP_NAME} --version"
echo ""
echo "Upgrade later:"
echo "  curl -fsSL https://raw.githubusercontent.com/${GITHUB_REPO}/main/scripts/install.sh | sudo bash"
echo ""
echo "Uninstall:"
echo "  sudo apt remove ${APP_NAME}"
echo ""
