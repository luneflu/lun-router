#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

TARGET_OS="${1:-${GOOS:-$(go env GOOS)}}"
TARGET_ARCH="${2:-${GOARCH:-$(go env GOARCH)}}"
OUTPUT="${3:-lun-router}"

if [[ "$TARGET_OS" != "$(go env GOOS)" ]]; then
    OUTPUT="lun-router-${TARGET_OS}-${TARGET_ARCH}"
fi

echo "==> Building Svelte frontend (SPA, no SSR)..."
if command -v pnpm &> /dev/null; then
    pnpm build
else
    npm run build
fi

echo "==> Compiling Go server for ${TARGET_OS}/${TARGET_ARCH} with embedded frontend..."
CGO_ENABLED=0 GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" go build -ldflags="-s -w" -o "$OUTPUT" .

echo "==> Done. Binary: ./$OUTPUT"
