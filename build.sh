#!/bin/bash

# Build script for Intenseye Network Diagnostics CLI
# Builds binaries for all major platforms

set -e

VERSION="1.0.0"
OUTPUT_DIR="dist"
BINARY_NAME="intenseye-netcheck"

echo "Building Intenseye NetCheck CLI v${VERSION}..."

# Clean previous builds
rm -rf ${OUTPUT_DIR}
mkdir -p ${OUTPUT_DIR}

# Build for each platform
platforms=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
)

for platform in "${platforms[@]}"; do
    GOOS=${platform%/*}
    GOARCH=${platform#*/}
    
    output_name="${BINARY_NAME}-${GOOS}-${GOARCH}"
    if [ "$GOOS" = "windows" ]; then
        output_name="${output_name}.exe"
    fi
    
    echo "Building for ${GOOS}/${GOARCH}..."
    
    GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="-s -w -X main.Version=${VERSION}" \
        -o "${OUTPUT_DIR}/${output_name}" .
done

# Create checksums
cd ${OUTPUT_DIR}
sha256sum * > checksums.txt
cd ..

echo ""
echo "Build complete! Binaries are in ${OUTPUT_DIR}/"
ls -la ${OUTPUT_DIR}/
