#!/bin/bash
set -euo pipefail

VERSION="${1:?Usage: build-deb.sh <version> <arch>}"
ARCH="${2:-amd64}"

APP_NAME="starrock-benchmark"
BINARY_NAME="${APP_NAME}-linux-${ARCH}"
DESCRIPTION="High-performance benchmarking tool for StarRocks"
MAINTAINER="Benchmark Team"

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_DIR="${ROOT_DIR}/dist/deb-build-${ARCH}"
DEB_FILE="${ROOT_DIR}/dist/${APP_NAME}_${VERSION}_${ARCH}.deb"

if [ ! -f "${ROOT_DIR}/bin/${BINARY_NAME}" ]; then
    echo "Error: Binary not found at bin/${BINARY_NAME}"
    echo "Run 'make build-linux' first."
    exit 1
fi

rm -rf "${BUILD_DIR}"
mkdir -p "${BUILD_DIR}/DEBIAN"
mkdir -p "${BUILD_DIR}/usr/local/bin"
mkdir -p "${BUILD_DIR}/etc/${APP_NAME}"

cp "${ROOT_DIR}/bin/${BINARY_NAME}" "${BUILD_DIR}/usr/local/bin/${APP_NAME}"
chmod 755 "${BUILD_DIR}/usr/local/bin/${APP_NAME}"

cp "${ROOT_DIR}/config.yaml" "${BUILD_DIR}/etc/${APP_NAME}/config.yaml.example"

cat > "${BUILD_DIR}/DEBIAN/control" <<EOF
Package: ${APP_NAME}
Version: ${VERSION}
Section: utils
Priority: optional
Architecture: ${ARCH}
Maintainer: ${MAINTAINER}
Description: ${DESCRIPTION}
 A Go CLI tool that benchmarks StarRocks ingestion and update performance
 with configurable parallelism, table engines, and load methods.
 Supports SQL INSERT and HTTP Stream Load with real-time terminal metrics.
EOF

cat > "${BUILD_DIR}/DEBIAN/postinst" <<'POSTINST'
#!/bin/bash
set -e
CONFIG_DIR="/etc/starrock-benchmark"
if [ ! -f "${CONFIG_DIR}/config.yaml" ]; then
    cp "${CONFIG_DIR}/config.yaml.example" "${CONFIG_DIR}/config.yaml"
    echo "Created default config at ${CONFIG_DIR}/config.yaml"
    echo "Edit it with: sudo nano ${CONFIG_DIR}/config.yaml"
fi
echo "starrock-benchmark installed. Run: starrock-benchmark --config /etc/starrock-benchmark/config.yaml"
POSTINST
chmod 755 "${BUILD_DIR}/DEBIAN/postinst"

cat > "${BUILD_DIR}/DEBIAN/prerm" <<'PRERM'
#!/bin/bash
set -e
echo "Removing starrock-benchmark..."
PRERM
chmod 755 "${BUILD_DIR}/DEBIAN/prerm"

mkdir -p "${ROOT_DIR}/dist"
dpkg-deb --build --root-owner-group "${BUILD_DIR}" "${DEB_FILE}" 2>/dev/null || \
    dpkg-deb --build "${BUILD_DIR}" "${DEB_FILE}"

rm -rf "${BUILD_DIR}"

echo "Package created: ${DEB_FILE}"
echo ""
echo "Install with:"
echo "  sudo dpkg -i ${DEB_FILE}"
echo "  # or"
echo "  sudo apt install ./${APP_NAME}_${VERSION}_${ARCH}.deb"
