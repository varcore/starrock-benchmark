#!/bin/bash
set -euo pipefail

#
# Builds .deb packages and creates a GitHub release with them attached.
# Requires: gh (GitHub CLI), dpkg-deb (apt: dpkg), go
#
# Usage:
#   ./scripts/release.sh              # release current VERSION
#   ./scripts/release.sh 1.2.3        # release specific version
#   ./scripts/release.sh --bump patch  # bump version first, then release
#

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

# Parse arguments
BUMP=""
VERSION=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --bump) BUMP="$2"; shift 2 ;;
        *) VERSION="$1"; shift ;;
    esac
done

if [ -n "$BUMP" ]; then
    bash scripts/bump-version.sh "$BUMP"
fi

VERSION="${VERSION:-$(cat VERSION | tr -d '[:space:]')}"
TAG="v${VERSION}"
APP_NAME="starrock-benchmark"

echo "==> Releasing ${APP_NAME} ${TAG}"
echo ""

# Verify clean git state
if [ -n "$(git status --porcelain)" ]; then
    echo "WARNING: Working directory has uncommitted changes."
    read -p "Continue anyway? [y/N] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Aborted."
        exit 1
    fi
fi

# Verify gh CLI is available
if ! command -v gh &> /dev/null; then
    echo "Error: GitHub CLI (gh) is required. Install: https://cli.github.com/"
    exit 1
fi

# Verify dpkg-deb is available
if ! command -v dpkg-deb &> /dev/null; then
    echo "Error: dpkg-deb is required."
    echo "  macOS:  brew install dpkg"
    echo "  Linux:  sudo apt install dpkg"
    exit 1
fi

# Build both architectures
echo "==> Building linux/amd64..."
make build-linux VERSION="$VERSION"

echo "==> Building linux/arm64..."
make build-linux-arm64 VERSION="$VERSION"

# Package .deb files
echo "==> Packaging .deb (amd64)..."
bash scripts/build-deb.sh "$VERSION" amd64

echo "==> Packaging .deb (arm64)..."
bash scripts/build-deb.sh "$VERSION" arm64

DEB_AMD64="dist/${APP_NAME}_${VERSION}_amd64.deb"
DEB_ARM64="dist/${APP_NAME}_${VERSION}_arm64.deb"
BIN_AMD64="bin/${APP_NAME}-linux-amd64"
BIN_ARM64="bin/${APP_NAME}-linux-arm64"

# Verify artifacts exist
for f in "$DEB_AMD64" "$DEB_ARM64" "$BIN_AMD64" "$BIN_ARM64"; do
    if [ ! -f "$f" ]; then
        echo "Error: Expected artifact not found: $f"
        exit 1
    fi
done

# Create git tag if it doesn't exist
if git rev-parse "$TAG" &>/dev/null; then
    echo "==> Tag $TAG already exists, skipping tag creation."
else
    echo "==> Creating git tag $TAG..."
    git tag -a "$TAG" -m "Release ${TAG}"
    git push origin "$TAG"
fi

# Generate release notes
RELEASE_NOTES=$(cat <<EOF
## ${APP_NAME} ${TAG}

### Install on Ubuntu/Debian server

**One-liner install (recommended):**
\`\`\`bash
curl -fsSL https://raw.githubusercontent.com/$(gh repo view --json nameWithOwner -q .nameWithOwner)/main/scripts/install.sh | sudo bash
\`\`\`

**Manual install:**
\`\`\`bash
# amd64
wget https://github.com/$(gh repo view --json nameWithOwner -q .nameWithOwner)/releases/download/${TAG}/${APP_NAME}_${VERSION}_amd64.deb
sudo apt install ./${APP_NAME}_${VERSION}_amd64.deb

# arm64
wget https://github.com/$(gh repo view --json nameWithOwner -q .nameWithOwner)/releases/download/${TAG}/${APP_NAME}_${VERSION}_arm64.deb
sudo apt install ./${APP_NAME}_${VERSION}_arm64.deb
\`\`\`

### Run
\`\`\`bash
# Edit config
sudo nano /etc/starrock-benchmark/config.yaml

# Run benchmark
starrock-benchmark --config /etc/starrock-benchmark/config.yaml
\`\`\`

### Artifacts
| File | Platform |
|------|----------|
| \`${APP_NAME}_${VERSION}_amd64.deb\` | Linux x86_64 (.deb) |
| \`${APP_NAME}_${VERSION}_arm64.deb\` | Linux ARM64 (.deb) |
| \`${APP_NAME}-linux-amd64\` | Linux x86_64 (binary) |
| \`${APP_NAME}-linux-arm64\` | Linux ARM64 (binary) |
EOF
)

# Create GitHub release
echo "==> Creating GitHub release ${TAG}..."
gh release create "$TAG" \
    --title "${APP_NAME} ${TAG}" \
    --notes "$RELEASE_NOTES" \
    "$DEB_AMD64" \
    "$DEB_ARM64" \
    "$BIN_AMD64" \
    "$BIN_ARM64"

REPO_URL="https://github.com/$(gh repo view --json nameWithOwner -q .nameWithOwner)"

echo ""
echo "============================================"
echo "  Release ${TAG} published!"
echo "============================================"
echo ""
echo "Release page: ${REPO_URL}/releases/tag/${TAG}"
echo ""
echo "Install on any server:"
echo "  curl -fsSL ${REPO_URL}/raw/main/scripts/install.sh | sudo bash"
echo ""
