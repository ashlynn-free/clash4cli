#!/bin/bash
set -e

# Build proxychains-ng for bundling with clash4cli
# Supports: darwin-amd64, darwin-arm64, linux-amd64, linux-arm64

PROXYCHAINS_VERSION="${PROXYCHAINS_NG_VERSION:-4.17}"
PROXYCHAINS_REPO="https://github.com/rofl0r/proxychains-ng"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
THIRD_PARTY_DIR="$PROJECT_DIR/third_party/proxychains"
BUILD_DIR="$PROJECT_DIR/.build/proxychains"

mkdir -p "$THIRD_PARTY_DIR"
mkdir -p "$BUILD_DIR"

# Detect current platform
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
esac

echo "=== Building proxychains-ng v${PROXYCHAINS_VERSION} for ${OS}-${ARCH} ==="

# Clone if not exists
if [ ! -d "$BUILD_DIR/proxychains-ng" ]; then
    echo "Cloning proxychains-ng..."
    git clone --depth 1 --branch "v${PROXYCHAINS_VERSION}" "$PROXYCHAINS_REPO" "$BUILD_DIR/proxychains-ng"
fi

cd "$BUILD_DIR/proxychains-ng"

# Clean previous build
make clean 2>/dev/null || true

# Configure and build
echo "Configuring..."
./configure --prefix="$BUILD_DIR/install-${OS}-${ARCH}" --sysconfdir="$BUILD_DIR/install-${OS}-${ARCH}/etc"

echo "Building..."
make -j$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)

echo "Installing to staging directory..."
make install

# Copy to third_party
OUTPUT_DIR="$THIRD_PARTY_DIR/${OS}-${ARCH}"
mkdir -p "$OUTPUT_DIR"

if [ "$OS" = "darwin" ]; then
    cp -f "$BUILD_DIR/install-${OS}-${ARCH}/bin/proxychains4" "$OUTPUT_DIR/"
    cp -f "$BUILD_DIR/install-${OS}-${ARCH}/lib/libproxychains4.dylib" "$OUTPUT_DIR/"
else
    cp -f "$BUILD_DIR/install-${OS}-${ARCH}/bin/proxychains4" "$OUTPUT_DIR/"
    cp -f "$BUILD_DIR/install-${OS}-${ARCH}/lib/libproxychains4.so" "$OUTPUT_DIR/"
fi

# Record the exact version we built (used by fetch script & packaging)
printf "%s\n" "$PROXYCHAINS_VERSION" > "$OUTPUT_DIR/proxychains-ng.version"

# Copy license files (for redistribution compliance)
shopt -s nullglob
for f in "$BUILD_DIR/proxychains-ng"/LICENSE* "$BUILD_DIR/proxychains-ng"/COPYING*; do
    base="$(basename "$f")"
    cp -f "$f" "$OUTPUT_DIR/${base}.proxychains-ng"
done
shopt -u nullglob

echo ""
echo "=== Build complete ==="
echo "Output: $OUTPUT_DIR"
ls -la "$OUTPUT_DIR"
